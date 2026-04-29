//go:build integration

package hawtiotest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/hawtio/hawtio-operator/pkg/clients"
	hawtiomgr "github.com/hawtio/hawtio-operator/pkg/manager"
	"github.com/hawtio/hawtio-operator/pkg/util"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/go-logr/logr"
	"sigs.k8s.io/yaml"

	"github.com/hawtio/hawtio-operator/pkg/resources"
)

const (
	HawtioName      = "hawtio-online"
	HawtioNamespace = "default"

	Timeout  = time.Second * 10
	Interval = time.Millisecond * 250
)

// These vars are used by tests to interact with the simulated cluster
type TestTools struct {
	Platform        string
	Cfg             *rest.Config
	Scheme          *runtime.Scheme
	ClientTools     *clients.ClientTools
	K8sClient       client.Client
	Cancel          context.CancelFunc
	Ctx             context.Context
	Logger          logr.Logger
	WatchNamespaces string
}

type ManagerState struct {
	Manager manager.Manager
	Wg      *sync.WaitGroup
	Ctx     context.Context
	Cancel  context.CancelFunc
}

// MockRegistryTransport intercepts HTTP calls and returns fake image digests.
type MockRegistryTransport struct {
	mu sync.Mutex
	// Maps a registry URL path to the sha256 digest we want to return
	DigestMap  map[string][]string
	ShouldFail bool
}

func (m *MockRegistryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.ShouldFail {
		return nil, http.ErrServerClosed // Simulate a hard network/air-gap failure
	}

	// Intercept the mandatory Docker API version ping
	if req.URL.Path == "/v2/" {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString("{}")),
			Header:     make(http.Header),
		}, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	digests, ok := m.DigestMap[req.URL.Path]
	if !ok || len(digests) == 0 {
		errMsg := "mock missing path: " + req.URL.Path
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString(errMsg)),
			Header:     make(http.Header),
		}, nil
	}

	// Get the current digest for this request
	digest := digests[0]

	// If there's another digest in the sequence, pop it so the next
	// request gets the new one
	if len(digests) > 1 {
		m.DigestMap[req.URL.Path] = digests[1:]
	}

	// go-containerregistry strictly requires this header to extract the digest
	header := make(http.Header)
	header.Set("Docker-Content-Digest", digest)
	header.Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("{}")),
		Header:     header,
	}, nil
}

var buildVariables = util.BuildVariables{
	ImageRepository:                      "quay.io/hawtio/online",
	ImageVersion:                         "2.3.0",
	GatewayImageVersion:                  "2.3.0",
	GatewayImageRepository:               "quay.io/hawtio/online-gateway",
	LegacyServingCertificateMountVersion: "",
	ProductName:                          "",
	ServerRootDirectory:                  "",
	ClientCertCommonName:                 "",
	AdditionalLabels:                     "",
}

// Define GLOB patterns for map keys to ignore anywhere
var mapKeyGlobsToIgnore = []string{
	"openshift.io/*",
	"kubectl.kubernetes.io/*",
	"deployment.kubernetes.io/*",
	"*rht.*",
	"*company*",
	"*/certversion",
	"*/configversion",
	"UID",
}

var metaTypeFieldsToIgnore = []string{
	"Kind",
	"APIVersion",
}

var metaObjFieldsToIgnore = []string{
	"ResourceVersion",
	"Generation",
	"SelfLink",
	"CreationTimestamp",
	"UID",
	"ManagedFields",
	"OwnerReferences",
}

var svcSpecFieldsToIgnore = []string{
	"ClusterIP",  // Different ip address each time
	"ClusterIPs", // Different ip address each time
}

var podSpecFieldsToIgnore = []string{
	"DeprecatedServiceAccount", // Created at runtime for kube backward-compatibility but not required
}

var saFieldsToIgnore = []string{
	"Secrets",          // Created at runtime with random hashes
	"ImagePullSecrets", // Created at runtime with random hashes
}

// k8sSpecComparator defines reusable options for comparing Kubernetes specs
var k8sSpecComparator = cmp.Options{
	// Ignore specific field names anywhere they appear
	cmpopts.IgnoreFields(metav1.TypeMeta{}, metaTypeFieldsToIgnore...),
	cmpopts.IgnoreFields(metav1.ObjectMeta{}, metaObjFieldsToIgnore...),
	cmpopts.IgnoreFields(corev1.ServiceSpec{}, svcSpecFieldsToIgnore...),
	cmpopts.IgnoreFields(corev1.PodSpec{}, podSpecFieldsToIgnore...),
	cmpopts.IgnoreFields(corev1.ServiceAccount{}, saFieldsToIgnore...),

	// Ignore map entries based on key patterns anywhere
	cmp.FilterPath(func(p cmp.Path) bool {
		mapIndex, ok := p.Last().(cmp.MapIndex)
		if !ok {
			return false // Not a map key
		}
		key := mapIndex.Key().String()
		for _, glob := range mapKeyGlobsToIgnore {
			// if key == "hawtio.hawt.io/certversion" {
			// 	m, _ := filepath.Match(glob, key)
			// 	fmt.Printf("DEBUG: Matching key=[%s] against glob=[%s]: result=[%t]\n", key, glob, m)
			// }
			if match, _ := filepath.Match(glob, key); match {
				return true // Ignore if pattern matches
			}
		}
		return false
	}, cmp.Ignore()),
}

// This regex finds the phrase and captures the word "Kubernetes" or "OpenShift"
var platformRegex = regexp.MustCompile(`applications deployed on [A-Za-z]+`)

func loadExpectedFile(filename string, platform string, obj runtime.Object) {
	platformPath := filepath.Join("testdata", fmt.Sprintf("%s.yml", filename))
	bytes, err := os.ReadFile(platformPath)
	if err == nil {
		// Found plaform-specific file
		By(fmt.Sprintf("Loading platform-specific golden file: %s", platformPath))
		err = yaml.Unmarshal(bytes, obj)
		Expect(err).NotTo(HaveOccurred(), "Failed to unmarshal golden file: "+platformPath)
		return
	}

	Expect(os.IsNotExist(err)).To(BeTrue(), "Failed to read platform golden file "+platformPath+": "+err.Error())

	genericPath := filepath.Join("..", "testdata", fmt.Sprintf("%s.yml", filename))
	bytes, err = os.ReadFile(genericPath)

	if err == nil {
		// Found generic file
		By(fmt.Sprintf("Loading generic golden file: %s", genericPath))
		err = yaml.Unmarshal(bytes, obj)
		Expect(err).NotTo(HaveOccurred(), "Failed to unmarshal golden file: "+genericPath)
		return
	}

	// If here, *both* files are missing. Fail the test.
	Expect(os.IsNotExist(err)).To(BeTrue(), "Failed to read generic golden file "+genericPath+": "+err.Error())
	Fail(fmt.Sprintf("Could not find golden file: %s (platform) or %s (generic)", platformPath, genericPath))
}

func compareResource(name string, actual interface{}, expected interface{}) {
	areSpecsEqual := equality.Semantic.DeepEqual(actual, expected)
	if !areSpecsEqual {
		diff := cmp.Diff(expected, actual, k8sSpecComparator)

		if diff != "" { // Check if there are still differences after ignoring
			Expect(diff).To(BeNil(), fmt.Sprintf("%s should match golden file (after ignores), see diff", name))
		} else {
			Expect(diff).To(BeEmpty(), "Differences found only in ignored fields, which is expected.")
		}
	} else {
		// If the code reaches here, the specs were semantically equal.
		Expect(areSpecsEqual).To(BeTrue())
	}
}

func lookupKey(hawtio *hawtiov2.Hawtio) types.NamespacedName {
	return types.NamespacedName{
		Name:      hawtio.Name,
		Namespace: hawtio.Namespace,
	}
}

// createBasicHawtioCR is a lightweight helper for the cache tests
func createBasicHawtioCR(ctx context.Context, testTools *TestTools, name, namespace string) *hawtiov2.Hawtio {
	hawtio := &hawtiov2.Hawtio{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: hawtiov2.HawtioSpec{
			Type:    hawtiov2.NamespaceHawtioDeploymentType,
			Version: "latest",
		},
	}
	Expect(testTools.K8sClient.Create(ctx, hawtio)).Should(Succeed())
	return hawtio
}

func createNamespaces(ctx context.Context, testTools *TestTools, namespaces ...string) {
	for _, ns := range namespaces {
		namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}
		err := testTools.K8sClient.Create(ctx, namespace)
		if err != nil {
			Expect(kerrors.IsAlreadyExists(err)).To(BeTrue(), "Expected AlreadyExists error if namespace is present")
		}
	}
}

// StartManager boots the controller-runtime manager using the configuration defined in TestTools.
func StartManager(testTools *TestTools, extraOpts ...hawtiomgr.MgrOption) *ManagerState {
	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}

	By(fmt.Sprintf("Starting Manager with WATCH_NAMESPACES='%s'", testTools.WatchNamespaces))

	metricsOptions := metricserver.Options{
		BindAddress: "0", // Disable metrics for tests
	}

	opts := []hawtiomgr.MgrOption{
		hawtiomgr.WithRestConfig(testTools.Cfg),
		hawtiomgr.WithWatchNamespaces(testTools.WatchNamespaces),
		hawtiomgr.WithPodNamespace(HawtioNamespace),
		hawtiomgr.WithBuildVariables(buildVariables),
		hawtiomgr.WithScheme(testTools.Scheme),
		hawtiomgr.WithClientTools(testTools.ClientTools),
		hawtiomgr.WithMetrics(metricsOptions),
	}
	opts = append(opts, extraOpts...)
	mgr, err := hawtiomgr.New(opts...)

	Expect(err).NotTo(HaveOccurred())

	wg.Add(1)
	go func() {
		defer GinkgoRecover()
		defer wg.Done()
		Expect(mgr.Start(ctx)).To(Succeed())
	}()

	By("Waiting for Manager.caches to sync")
	// This blocks the calling function from finishing until the
	// informers (Hawtio, ConfigMap, etc.) are synced.
	Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())
	By("Manager caches synced")

	return &ManagerState{
		Manager: mgr,
		Ctx:     ctx,
		Cancel:  cancel,
		Wg:      wg,
	}
}

// SetupManagerWithCleanup boots the manager and automatically registers
// the teardown logic to run at the end of the current test or context.
func SetupManagerWithCleanup(testTools *TestTools) *ManagerState {
	mgrState := StartManager(testTools)

	// DeferCleanup registers this block to run after the current test scope finishes.
	DeferCleanup(func() {
		By("Deleting the Hawtio CR")
		PerformDeleteHawtioCR(testTools, HawtioName, HawtioNamespace)

		By(fmt.Sprintf("Stopping the %s manager", testTools.Platform))
		mgrState.Cancel()  // Signal manager to stop
		mgrState.Wg.Wait() // Wait for it to shut down
	})

	return mgrState
}

// PerformDeleteHawtioCR deletes the hawtio CR under test
func PerformDeleteHawtioCR(testTools *TestTools, name string, namespace string) {
	// Use a detached context for cleanup to prevent "context canceled" errors
	// during the delete/wait process.
	ctx := context.Background()

	// Use your static test key
	hawtioKey := types.NamespacedName{Name: name, Namespace: namespace}

	// Fetch the object to delete
	hawtioToDelete := &hawtiov2.Hawtio{}
	err := testTools.K8sClient.Get(ctx, hawtioKey, hawtioToDelete)

	// Only delete if it was found
	if err == nil {
		Expect(testTools.K8sClient.Delete(ctx, hawtioToDelete)).To(Succeed())
	} else {
		// If it's already gone, that's fine
		Expect(kerrors.IsNotFound(err)).To(BeTrue())
	}

	// Wait until the object is *actually gone* from the API server.
	// This gives the reconciler time to run, remove its finalizer,
	// and let Kubernetes garbage collect the object.
	By("Waiting for Hawtio CR to be fully garbage collected")
	Eventually(func(g Gomega) {
		err := testTools.K8sClient.Get(ctx, hawtioKey, &hawtiov2.Hawtio{})
		g.Expect(kerrors.IsNotFound(err)).To(BeTrue())
	}, "10s", "100ms").Should(Succeed())
}

// PerformEmptyTypeHawtioCR test the effect of an empty Hawtio CR
func PerformEmptyTypeHawtioCR(ctx context.Context, testTools *TestTools) {
	By("Creating an empty type Hawtio CR")
	emptyTypeName := "empty-type-hawtio"
	emptyTypeHawtio := &hawtiov2.Hawtio{
		ObjectMeta: metav1.ObjectMeta{
			Name:      emptyTypeName,
			Namespace: HawtioNamespace, // Use a consistent test namespace
		},
		Spec: hawtiov2.HawtioSpec{
			Version: "latest",
			// Explicitly missed out type value
		},
	}
	lookupKey := lookupKey(emptyTypeHawtio)

	// Attempt to create the invalid CR.
	Expect(testTools.K8sClient.Create(ctx, emptyTypeHawtio)).Should(Succeed())
	// Ensure the CR is cleaned up after the test
	DeferCleanup(func() {
		By("Cleaning up empty type test CR")
		PerformDeleteHawtioCR(testTools, emptyTypeName, HawtioNamespace)
	})

	Eventually(func(g Gomega) {
		fetched := &hawtiov2.Hawtio{}
		g.Expect(testTools.K8sClient.Get(ctx, lookupKey, fetched)).To(Succeed())

		// Check that the default was applied
		g.Expect(fetched.Spec.Type).To(Equal(hawtiov2.ClusterHawtioDeploymentType))

		// Check that the reconciler continued to the 'Initialized' phase
		g.Expect(fetched.Status.Phase).To(Equal(hawtiov2.HawtioPhaseInitialized))
	}, Timeout, Interval).Should(Succeed())
}

// PerformCommonResourceTest tests the reconciler checking the generated resources
func PerformCommonResourceTest(ctx context.Context, testTools *TestTools) {
	By("Creating a new Hawtio CR")
	hawtio := createBasicHawtioCR(ctx, testTools, HawtioName, HawtioNamespace)

	By("Checking the Hawtio Phase has reached Initialized")
	Eventually(func() hawtiov2.HawtioPhase {
		fetched := &hawtiov2.Hawtio{}
		lookupKey := lookupKey(hawtio)
		err := testTools.K8sClient.Get(testTools.Ctx, lookupKey, fetched)
		if err != nil {
			return "" // Keep trying if Get fails
		}
		return fetched.Status.Phase
	}, Timeout, Interval).Should(Equal(hawtiov2.HawtioPhaseInitialized))

	By("Checking if the ConfigMap was created")
	configMap := &corev1.ConfigMap{}
	configMapLookupKey := lookupKey(hawtio)
	Eventually(func() bool {
		err := testTools.K8sClient.Get(ctx, configMapLookupKey, configMap)
		return err == nil
	}, Timeout, Interval).Should(BeTrue(), "ConfigMap should be created")

	Expect(configMap.Labels).To(HaveKeyWithValue("app", "hawtio"), "Should have expected app label")

	By("Checking the created ConfigMap against expected test data")
	expConfigMap := &corev1.ConfigMap{}
	loadExpectedFile("configmap", strings.ToLower(testTools.Platform), expConfigMap)
	compareResource("ConfigMap", configMap.Data, expConfigMap.Data)

	By("Checking if the service account was created")
	sa := &corev1.ServiceAccount{}
	saLookupKey := lookupKey(hawtio)
	Eventually(func() bool {
		err := testTools.K8sClient.Get(ctx, saLookupKey, sa)
		return err == nil
	}, Timeout, Interval).Should(BeTrue(), "ServiceAccount should be created")

	By("Checking if the service account role was created")
	role := &rbacv1.Role{}
	roleLookupKey := lookupKey(hawtio)
	Eventually(func() bool {
		err := testTools.K8sClient.Get(ctx, roleLookupKey, role)
		return err == nil
	}, Timeout, Interval).Should(BeTrue(), "ServiceAccount Role should be created")

	By("Checking if the service account role binding was created")
	roleBinding := &rbacv1.RoleBinding{}
	roleBindingLookupKey := lookupKey(hawtio)
	Eventually(func() bool {
		err := testTools.K8sClient.Get(ctx, roleBindingLookupKey, roleBinding)
		return err == nil
	}, Timeout, Interval).Should(BeTrue(), "ServiceAccount RoleBinding should be created")

	By("Checking if the Deployment was created")
	deployment := &appsv1.Deployment{}
	deploymentLookupKey := lookupKey(hawtio)
	Eventually(func() bool {
		err := testTools.K8sClient.Get(ctx, deploymentLookupKey, deployment)
		return err == nil
	}, Timeout, Interval).Should(BeTrue(), "Deployment should be created")

	By("Checking the created Deployment against expected test data")
	expDeployment := &appsv1.Deployment{}
	loadExpectedFile("deployment", strings.ToLower(testTools.Platform), expDeployment)
	Expect(deployment.Labels).To(HaveKeyWithValue("app", "hawtio"), "Should have expected app label")
	compareResource("Deployment.Spec", deployment.Spec, expDeployment.Spec)

	By("Checking if the Service was created")
	service := &corev1.Service{}
	svcLookupKey := lookupKey(hawtio)
	Eventually(func() bool {
		err := testTools.K8sClient.Get(ctx, svcLookupKey, service)
		return err == nil
	}, Timeout, Interval).Should(BeTrue(), "Service should be created")

	By("Checking the created Service against expected test data")
	expSvc := &corev1.Service{}
	loadExpectedFile("service", strings.ToLower(testTools.Platform), expSvc)
	Expect(service.Labels).To(HaveKeyWithValue("app", "hawtio"), "Should have expected app label")
	compareResource("Service", service, expSvc)
}

// PerformIgnoreNamespaceTest tests that the a namespace and CR are ignored
func PerformIgnoreNamespaceTest(ctx context.Context, testTools *TestTools) {
	By("Creating a namespace that will be ignored")
	ignoredNS := "hawtio-ignored-ns"
	createNamespaces(ctx, testTools, ignoredNS)
	ignoredCR := createBasicHawtioCR(ctx, testTools, "hawtio-scoped-ignored", ignoredNS)
	DeferCleanup(func() {
		By("Deleting the Hawtio CR in ignored namespace")
		PerformDeleteHawtioCR(testTools, ignoredCR.Name, ignoredNS)
	})

	By("Verifying the ignored namespace CR is completely ignored")
	deployment := &appsv1.Deployment{}
	Consistently(func() error {
		return testTools.K8sClient.Get(ctx, types.NamespacedName{Name: ignoredCR.Name, Namespace: ignoredNS}, deployment)
	}, time.Second*3, Interval).ShouldNot(Succeed(), "Scoped manager MUST ignore CRs outside its target namespace")
}

// PerformWatchAllNamespacesTest executes the delineation tests for OLMv1 compatibility.
// It proves the operator can dynamically switch between AllNamespaces and SingleNamespace mode.
func PerformWatchAllNamespacesTest(ctx context.Context, testTools *TestTools) {
	otherNS1 := "hawtio-other1-ns"
	otherNS2 := "hawtio-other2-ns"

	By("Setting up test namespaces")
	createNamespaces(ctx, testTools, otherNS1, otherNS2)

	hawtioCR := createBasicHawtioCR(ctx, testTools, HawtioName, HawtioNamespace)
	otherCR1 := createBasicHawtioCR(ctx, testTools, "hawtio-other-1", otherNS1)
	DeferCleanup(func() {
		PerformDeleteHawtioCR(testTools, otherCR1.Name, otherNS1)
	})
	otherCR2 := createBasicHawtioCR(ctx, testTools, "hawtio-other-2", otherNS2)
	DeferCleanup(func() {
		PerformDeleteHawtioCR(testTools, otherCR2.Name, otherNS2)
	})

	By("Verifying CRs are reconciled globally by checking if a deployment is created")
	crs := []*hawtiov2.Hawtio{hawtioCR, otherCR1, otherCR2}
	for _, cr := range crs {
		deployment := &appsv1.Deployment{}
		Eventually(func() error {
			return testTools.K8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, deployment)
		}, Timeout, Interval).Should(Succeed(), fmt.Sprintf("Global manager should reconcile %s in namespace %s", cr.Name, cr.Namespace))
	}
}

//
// PerformCommonUpdaterTest tests the updater conducting
// queries on a mocked registry. The second time it makes
// a query, the digest is updated and the new update made
// to the deployment resource
func PerformCommonUpdaterTest(testTools *TestTools, mgrState *ManagerState, platform string) {
	By("Stopping the Manager to Configure Mocked Registry")
	// Stop the default manager started by BeforeEach
	mgrState.Cancel()
	mgrState.Wg.Wait()

	By("Configuring the mock registry")
	onlineHash := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	updatedOnlineHash := "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	gatewayHash := "sha256:3333333333333333333333333333333333333333333333333333333333333333"
	updatedGatewayHash := "sha256:4444444444444444444444444444444444444444444444444444444444444444"

	// Build the sequenced mock
	mockTransport := &MockRegistryTransport{
		DigestMap: map[string][]string{
			fmt.Sprintf("/v2/hawtio/online/manifests/%s", buildVariables.ImageVersion):                {onlineHash, updatedOnlineHash},
			fmt.Sprintf("/v2/hawtio/online-gateway/manifests/%s", buildVariables.GatewayImageVersion): {gatewayHash, updatedGatewayHash},
		},
	}

	By("Restarting the Manager with the Sequenced Mock Transport")
	// Restart the manager with the mocked internet by create a new temporary state
	newMgrState := StartManager(testTools,
		hawtiomgr.WithRegistryTransport(mockTransport),
		// Must be > 2 seconds so the Reconciler has time to build the baseline
		hawtiomgr.WithUpdatePollingInterval(3*time.Second))

	// Dereference BOTH pointers to overwrite the original manager
	*mgrState = *newMgrState

	By("Creating the Hawtio CR")
	hawtioName := "hawtio-poller-test"
	hawtio := &hawtiov2.Hawtio{
		ObjectMeta: metav1.ObjectMeta{Name: hawtioName, Namespace: HawtioNamespace},
		Spec: hawtiov2.HawtioSpec{
			Type: hawtiov2.NamespaceHawtioDeploymentType,
		},
	}
	Expect(testTools.K8sClient.Create(mgrState.Ctx, hawtio)).To(Succeed())

	DeferCleanup(func() {
		PerformDeleteHawtioCR(testTools, hawtioName, HawtioNamespace)
	})

	deploymentKey := types.NamespacedName{Name: hawtioName, Namespace: HawtioNamespace}
	createdDeployment := &appsv1.Deployment{}

	By("Waiting for the Reconciler to build the Deployment using the baseline digests")
	Eventually(func(g Gomega) {
		err := testTools.K8sClient.Get(mgrState.Ctx, deploymentKey, createdDeployment)
		g.Expect(err).NotTo(HaveOccurred())

		annotations := createdDeployment.Spec.Template.Annotations
		g.Expect(annotations).NotTo(BeNil())
		// The Reconciler should have successfully applied the first digest
		g.Expect(annotations[resources.OnlineDigestAnnotation]).To(Equal(onlineHash))
		g.Expect(annotations[resources.GatewayDigestAnnotation]).To(Equal(gatewayHash))
	}, Timeout, Interval).Should(Succeed())

	By("Waiting for the background poller to tick, fire an event, and trigger a dynamic update")
	Eventually(func(g Gomega) {
		err := testTools.K8sClient.Get(mgrState.Ctx, deploymentKey, createdDeployment)
		g.Expect(err).NotTo(HaveOccurred())

		annotations := createdDeployment.Spec.Template.Annotations
		g.Expect(annotations).NotTo(BeNil())
		// The Reconciler should have seamlessly swapped to the SECOND digest!
		g.Expect(annotations[resources.OnlineDigestAnnotation]).To(Equal(updatedOnlineHash))
		g.Expect(annotations[resources.GatewayDigestAnnotation]).To(Equal(updatedGatewayHash))
	}, Timeout, Interval).Should(Succeed())
}

//
// PerformCommonUpdaterNetworkFailureTest tests the updater failing to
// query the mocked registry simulating a network failure or air-gapped
// installation. The updating should retreat from any update and allow
// the deployment to continue with the original image:tags.
func PerformCommonUpdaterNetworkFailureTest(testTools *TestTools, mgrState *ManagerState, platform string) {
	By("Stopping the Manager to Configure Failing Mock Registry")
	mgrState.Cancel()
	mgrState.Wg.Wait()

	By("Configuring the mock registry to simulate a complete network outage")
	mockTransport := &MockRegistryTransport{
		ShouldFail: true, // Triggers simulated http.ErrServerClosed
	}

	By("Restarting the Manager with the Failing Transport")
	newMgrState := StartManager(testTools,
		hawtiomgr.WithRegistryTransport(mockTransport),
		// Must be > 2 seconds so the Reconciler has time to build the baseline
		hawtiomgr.WithUpdatePollingInterval(3*time.Second))

	*mgrState = *newMgrState

	By("Creating the Hawtio CR")
	hawtioName := "hawtio-network-fail-test"
	hawtio := &hawtiov2.Hawtio{
		ObjectMeta: metav1.ObjectMeta{Name: hawtioName, Namespace: HawtioNamespace},
		Spec:       hawtiov2.HawtioSpec{Type: hawtiov2.NamespaceHawtioDeploymentType},
	}
	Expect(testTools.K8sClient.Create(mgrState.Ctx, hawtio)).To(Succeed())

	DeferCleanup(func() {
		PerformDeleteHawtioCR(testTools, hawtioName, HawtioNamespace)
	})

	deploymentKey := types.NamespacedName{Name: hawtioName, Namespace: HawtioNamespace}
	createdDeployment := &appsv1.Deployment{}

	By("Waiting for the Reconciler to safely fallback to default tags")
	Eventually(func(g Gomega) {
		err := testTools.K8sClient.Get(mgrState.Ctx, deploymentKey, createdDeployment)
		g.Expect(err).NotTo(HaveOccurred())

		// Ensure the operator didn't inject empty annotations
		annotations := createdDeployment.Spec.Template.Annotations
		if annotations != nil {
			g.Expect(annotations).NotTo(HaveKey(resources.OnlineDigestAnnotation))
		}

		// Ensure the containers are using the standard tags, NOT digests (@sha256:...)
		spec := createdDeployment.Spec.Template.Spec
		g.Expect(spec.Containers).To(HaveLen(2))
		for _, c := range spec.Containers {
			g.Expect(c.Image).NotTo(ContainSubstring("@sha256:"))
			g.Expect(c.Image).To(ContainSubstring(":" + buildVariables.ImageVersion))
		}
	}, Timeout, Interval).Should(Succeed())
}

//
// PerformCommonUpdaterPartialFailureTest tests the use-case that one
// image has been updated in the mocked registy but not the other. Both
// images are required for a successful update so the updater backs off
// and continues with the original images.
func PerformCommonUpdaterPartialFailureTest(testTools *TestTools, mgrState *ManagerState, platform string) {
	By("Stopping the Manager to Configure Partial Mock Registry")
	mgrState.Cancel()
	mgrState.Wg.Wait()

	By("Configuring the mock registry to simulate a missing gateway image")
	validOnlineHash := "sha256:1111111111111111111111111111111111111111111111111111111111111111"

	mockTransport := &MockRegistryTransport{
		DigestMap: map[string][]string{
			// The Online image succeeds...
			fmt.Sprintf("/v2/hawtio/online/manifests/%s", buildVariables.ImageVersion): {validOnlineHash},
			// ...but the Gateway image is missing from the map! (Will return 404)
		},
	}

	By("Restarting the Manager with the Partial Transport")
	newMgrState := StartManager(testTools,
		hawtiomgr.WithRegistryTransport(mockTransport),
		// Must be > 2 seconds so the Reconciler has time to build the baseline
		hawtiomgr.WithUpdatePollingInterval(3*time.Second))

	*mgrState = *newMgrState

	By("Creating the Hawtio CR")
	hawtioName := "hawtio-partial-fail-test"
	hawtio := &hawtiov2.Hawtio{
		ObjectMeta: metav1.ObjectMeta{Name: hawtioName, Namespace: HawtioNamespace},
		Spec:       hawtiov2.HawtioSpec{Type: hawtiov2.NamespaceHawtioDeploymentType},
	}
	Expect(testTools.K8sClient.Create(mgrState.Ctx, hawtio)).To(Succeed())

	DeferCleanup(func() {
		PerformDeleteHawtioCR(testTools, hawtioName, HawtioNamespace)
	})

	deploymentKey := types.NamespacedName{Name: hawtioName, Namespace: HawtioNamespace}
	createdDeployment := &appsv1.Deployment{}

	By("Waiting for the Reconciler to reject the split-brain state and fallback to defaults")
	Eventually(func(g Gomega) {
		err := testTools.K8sClient.Get(mgrState.Ctx, deploymentKey, createdDeployment)
		g.Expect(err).NotTo(HaveOccurred())

		spec := createdDeployment.Spec.Template.Spec
		g.Expect(spec.Containers).To(HaveLen(2))
		for _, c := range spec.Containers {
			// Neither container should get a digest because the batch fetch failed
			g.Expect(c.Image).NotTo(ContainSubstring("@sha256:"))
		}
	}, Timeout, Interval).Should(Succeed())
}
