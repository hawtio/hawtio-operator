//go:build integration

package hawtiotest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	"github.com/hawtio/hawtio-operator/pkg/controller"
	"github.com/hawtio/hawtio-operator/pkg/util"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"

	configclient "github.com/openshift/client-go/config/clientset/versioned"
	oauthclient "github.com/openshift/client-go/oauth/clientset/versioned"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "k8s.io/client-go/kubernetes"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/go-logr/logr"
	"sigs.k8s.io/yaml"
)

const (
	HawtioName      = "hawtio-online"
	HawtioNamespace = "default"

	Timeout  = time.Second * 10
	Interval = time.Millisecond * 250
)

// These vars are used by tests to interact with the simulated cluster
type TestTools struct {
	ApiClient    kclient.Interface
	ApiSpec      *capabilities.ApiServerSpec
	Cancel       context.CancelFunc
	Cfg          *rest.Config
	ConfigClient configclient.Interface
	CoreClient   corev1client.CoreV1Interface
	Ctx          context.Context
	K8sClient    client.Client
	Logger       logr.Logger
	OauthClient  oauthclient.Interface
}

type ManagerState struct {
	Manager manager.Manager
	Wg      *sync.WaitGroup
	Ctx     context.Context
	Cancel  context.CancelFunc
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

// k8sSpecComparator defines reusable options for comparing Kubernetes specs
var k8sSpecComparator = cmp.Options{
	// Ignore specific field names anywhere they appear
	cmpopts.IgnoreFields(metav1.TypeMeta{}, metaTypeFieldsToIgnore...),
	cmpopts.IgnoreFields(metav1.ObjectMeta{}, metaObjFieldsToIgnore...),
	cmpopts.IgnoreFields(corev1.ServiceSpec{}, svcSpecFieldsToIgnore...),

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

func StartManager(cfg *rest.Config) *ManagerState {
	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}

	By("Starting Manager")
	mgr, err := manager.New(cfg, manager.Options{
		Scheme: scheme.Scheme,
		Metrics: metricserver.Options{
			BindAddress: "0", // Disable metrics for tests
		},
	})
	Expect(err).NotTo(HaveOccurred())

	err = controller.AddToManager(mgr, buildVariables)
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

func PerformEmptyTypeHawtioCR(testTools *TestTools, ctx context.Context) {
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

func PerformCommonResourceTest(testTools *TestTools, ctx context.Context, platform string) {
	By("Creating a new Hawtio CR")
	hawtio := &hawtiov2.Hawtio{
		ObjectMeta: metav1.ObjectMeta{
			Name:      HawtioName,
			Namespace: HawtioNamespace,
		},
		Spec: hawtiov2.HawtioSpec{
			Type:    hawtiov2.NamespaceHawtioDeploymentType,
			Version: "latest",
		},
	}
	Expect(testTools.K8sClient.Create(ctx, hawtio)).Should(Succeed())

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

	By("Checking the created ConfigMap against expected test data")
	expConfigMap := &corev1.ConfigMap{}
	loadExpectedFile("configmap", platform, expConfigMap)
	compareResource("ConfigMap", configMap.Data, expConfigMap.Data)

	By("Checking if the Deployment was created")
	deployment := &appsv1.Deployment{}
	deploymentLookupKey := lookupKey(hawtio)
	Eventually(func() bool {
		err := testTools.K8sClient.Get(ctx, deploymentLookupKey, deployment)
		return err == nil
	}, Timeout, Interval).Should(BeTrue(), "Deployment should be created")

	By("Checking the created Deployment against expected test data")
	expDeployment := &appsv1.Deployment{}
	loadExpectedFile("deployment", platform, expDeployment)
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
	loadExpectedFile("service", platform, expSvc)
	Expect(service.Labels).To(HaveKeyWithValue("app", "hawtio"), "Should have expected app label")
	compareResource("Service", service, expSvc)
}
