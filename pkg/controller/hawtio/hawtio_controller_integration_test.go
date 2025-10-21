package hawtio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"
)

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

func initReconcileTools() *ReconcileHawtio {
	return &ReconcileHawtio{
		client:       testTools.k8sClient,
		scheme:       testTools.k8sClient.Scheme(),
		coreClient:   testTools.coreClient,
		oauthClient:  testTools.oauthClient,
		configClient: testTools.configClient,
		apiClient:    testTools.apiClient,
		apiSpec:      testTools.apiSpec,
		logger:       testTools.logger,
	}
}

func initLookupKey(hawtio *hawtiov2.Hawtio) types.NamespacedName {
	return types.NamespacedName{
		Name:      hawtio.Name,
		Namespace: hawtio.Namespace,
	}
}

func initRequest(hawtio *hawtiov2.Hawtio) reconcile.Request {
	return reconcile.Request{initLookupKey(hawtio)}
}

func loadExpectedFile(filename string, obj runtime.Object) {
	// Assuming golden files are in a 'testdata' subdirectory
	goldenPath := filepath.Join("testdata", filename)

	bytes, err := os.ReadFile(goldenPath)
	Expect(err).NotTo(HaveOccurred(), "Failed to read golden file: %s", goldenPath)

	err = yaml.Unmarshal(bytes, obj)
	Expect(err).NotTo(HaveOccurred(), "Failed to unmarshal golden file: %s", goldenPath)
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

var _ = Describe("Hawtio Controller", func() {

	const (
		HawtioName      = "hawtio-online"
		HawtioNamespace = "default"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When handling an invalid CR", func() {
		It("Should handle a missing type by adding the default and requeuing", func() {
			By("Creating an invalid Hawtio CR (e.g., missing 'type')")
			invalidHawtio := &hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-hawtio",
					Namespace: HawtioNamespace, // Use a consistent test namespace
				},
				Spec: hawtiov2.HawtioSpec{
					// 'Type' field is deliberately omitted
					Version: "latest",
				},
			}

			// Attempt to create the invalid CR.
			err := testTools.k8sClient.Create(testTools.ctx, invalidHawtio)
			Expect(err).NotTo(HaveOccurred())

			// If Create succeeded (validation might be lenient or done in Reconcile),
			// proceed to run the Reconcile loop.
			DeferCleanup(testTools.k8sClient.Delete, testTools.ctx, invalidHawtio) // Ensure cleanup
			reconciler := initReconcileTools()

			reconcileRequest := initRequest(invalidHawtio)

			By("Running the Reconcile function")
			result, err := reconciler.Reconcile(testTools.ctx, reconcileRequest)

			By("Asserting no error occurred")
			Expect(err).NotTo(HaveOccurred())

			By("Asserting requeue was requested")
			Expect(result.Requeue).To(BeTrue())

			By("Fetching the updated Hawtio CR")
			updatedHawtio := &hawtiov2.Hawtio{}

			Eventually(func() hawtiov2.HawtioDeploymentType {
				err := testTools.k8sClient.Get(testTools.ctx, reconcileRequest.NamespacedName, updatedHawtio)
				if err != nil {
					return "" // Keep retrying if Get fails
				}
				return updatedHawtio.Spec.Type
			}, timeout, interval).Should(Equal(hawtiov2.ClusterHawtioDeploymentType), "Spec.Type should be default Cluster mode")
		})
	})

	Context("When creating a basic Hawtio CR", func() {
		It("Should create the necessary Deployment and ConfigMap", func() {
			By("Creating a new Hawtio CR")
			ctx := context.Background()
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
			Expect(testTools.k8sClient.Create(ctx, hawtio)).Should(Succeed())
			DeferCleanup(testTools.k8sClient.Delete, testTools.ctx, hawtio) // Ensure cleanup

			reconciler := initReconcileTools()

			// Define the reconcile request
			reconcileRequest := initRequest(hawtio)

			By("Running the first Reconcile loop to move to initialized phase")
			result, err := reconciler.Reconcile(testTools.ctx, reconcileRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue(), "First reconcile should requeue after initializing status")

			By("Waiting for the status to become Initializing")
			Eventually(func() hawtiov2.HawtioPhase {
				fetched := &hawtiov2.Hawtio{}
				lookupKey := initLookupKey(hawtio)
				err := testTools.k8sClient.Get(testTools.ctx, lookupKey, fetched)
				if err != nil {
					return "" // Keep trying if Get fails
				}
				return fetched.Status.Phase
			}, timeout, interval).Should(Equal(hawtiov2.HawtioPhaseInitialized))

			By("Running the second Reconcile loop")
			_, err = reconciler.Reconcile(ctx, reconcileRequest)
			Expect(err).NotTo(HaveOccurred(), "Second reconcile loop should succeed")

			By("Checking if the ConfigMap was created")
			configMap := &corev1.ConfigMap{}
			configMapLookupKey := initLookupKey(hawtio)
			Eventually(func() bool {
				err := testTools.k8sClient.Get(ctx, configMapLookupKey, configMap)
				return err == nil
			}, timeout, interval).Should(BeTrue(), "ConfigMap should be created")

			By("Checking the created ConfigMap against expected test data")
			expConfigMap := &corev1.ConfigMap{}
			loadExpectedFile("configmap.yml", expConfigMap)
			compareResource("ConfigMap", configMap.Data, expConfigMap.Data)

			By("Checking if the Deployment was created")
			deployment := &appsv1.Deployment{}
			deploymentLookupKey := initLookupKey(hawtio)
			Eventually(func() bool {
				err := testTools.k8sClient.Get(ctx, deploymentLookupKey, deployment)
				return err == nil
			}, timeout, interval).Should(BeTrue(), "Deployment should be created")

			By("Checking the created Deployment against expected test data")
			expDeployment := &appsv1.Deployment{}
			loadExpectedFile("deployment.yml", expDeployment)
			Expect(deployment.Labels).To(HaveKeyWithValue("app", "hawtio"), "Should have expected app label")
			compareResource("Deployment.Spec", deployment.Spec, expDeployment.Spec)

			By("Checking if the Service was created")
			service := &corev1.Service{}
			svcLookupKey := initLookupKey(hawtio)
			Eventually(func() bool {
				err := testTools.k8sClient.Get(ctx, svcLookupKey, service)
				return err == nil
			}, timeout, interval).Should(BeTrue(), "Service should be created")

			By("Checking the created Service against expected test data")
			expSvc := &corev1.Service{}
			loadExpectedFile("service.yml", expSvc)
			Expect(service.Labels).To(HaveKeyWithValue("app", "hawtio"), "Should have expected app label")
			compareResource("Service", service, expSvc)
		})
	})
})
