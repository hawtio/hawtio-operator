//go:build integration

package hawtiotest

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	configclient "github.com/openshift/client-go/config/clientset/versioned"
	kclient "k8s.io/client-go/kubernetes"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"

	"github.com/hawtio/hawtio-operator/pkg/controller/internal/hawtiotest"
)

var _ = Describe("Testing the Hawtio Controller", Ordered, func() {
	var mgrState *hawtiotest.ManagerState

	fakeCV := &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: "version",
		},
	}

	ocPublicNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "openshift-config-managed"},
	}

	fakeConsoleConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "console-public",
			Namespace: ocPublicNS.Name,
		},
		Data: map[string]string{
			"consoleURL": "https://console-openshift-console.apps-crc.testing",
		},
	}

	BeforeAll(func() {
		By("Creating fake ClusterVersion to enable OpenShift mode")
		// Create the cluster version to mark the cluster as Openshift
		Expect(testTools.K8sClient.Create(context.Background(), fakeCV)).To(Succeed())

		createdCV := &configv1.ClusterVersion{}
		Expect(testTools.K8sClient.Get(context.Background(), types.NamespacedName{Name: "version"}, createdCV)).To(Succeed())

		// 3. Set the Status field on the *fetched* object
		createdCV.Status.History = []configv1.UpdateHistory{
			{
				State:       configv1.CompletedUpdate,
				Version:     "4.19.0",
				StartedTime: metav1.Now(),
			},
		}

		// Use the .Status().Update() client to apply the status
		By("Updating fake ClusterVersion's status")
		Expect(testTools.K8sClient.Status().Update(context.Background(), createdCV)).To(Succeed())

		By("Creating fake OpenShift system namespace")
		Expect(testTools.K8sClient.Create(context.Background(), ocPublicNS)).To(Succeed())

		By("Creating fake 'console-public' ConfigMap in system namespace")
		Expect(testTools.K8sClient.Create(context.Background(), fakeConsoleConfig)).To(Succeed())
	})

	AfterAll(func() {
		ctx := context.Background()

		By("Deleting the ClusterVersion object")
		// Remove the object from the API server's memory
		Expect(testTools.K8sClient.Delete(ctx, fakeCV)).To(Succeed())

		By("Cleaning up fake console config")
		Expect(testTools.K8sClient.Delete(ctx, fakeConsoleConfig)).To(Succeed())

		By("Cleaning up fake OpenShift system namespace")
		Expect(testTools.K8sClient.Delete(ctx, ocPublicNS)).To(Succeed())

		By("Waiting for fake ClusterVersion to be deleted")
		Eventually(func(g Gomega) {
			err := testTools.K8sClient.Get(ctx, client.ObjectKey{Name: fakeCV.Name}, &configv1.ClusterVersion{})
			g.Expect(kerrors.IsNotFound(err)).To(BeTrue())
		}, hawtiotest.Timeout, hawtiotest.Interval).Should(Succeed())

		By("Waiting for fake ClusterConfig to be deleted")
		Eventually(func(g Gomega) {
			err := testTools.K8sClient.Get(ctx, client.ObjectKeyFromObject(fakeConsoleConfig), &corev1.ConfigMap{})
			g.Expect(kerrors.IsNotFound(err)).To(BeTrue())
		}, hawtiotest.Timeout, hawtiotest.Interval).Should(Succeed())
	})

	Context("on OpenShift in a single namespace", func() {

		BeforeEach(func() {
			// Confine all tests to the single namespace
			testTools.WatchNamespaces = hawtiotest.HawtioNamespace
			mgrState = hawtiotest.SetupManagerWithCleanup(testTools)
		})

		It("Should correctly detect an OpenShift cluster", func() {
			By("Manually creating API clients")
			// Create the clients just like your controller's AddToManager does
			configClient, err := configclient.NewForConfig(testTools.Cfg)
			Expect(err).NotTo(HaveOccurred())

			apiClient, err := kclient.NewForConfig(testTools.Cfg)
			Expect(err).NotTo(HaveOccurred())

			By("Running APICapabilities check")
			// This runs the check *after* the BeforeEach created the fake ClusterVersion
			apiSpec, err := capabilities.APICapabilities(mgrState.Ctx, apiClient, configClient)
			Expect(err).NotTo(HaveOccurred())

			By("Asserting OpenShift mode is enabled")
			// This is the direct assertion you wanted
			Expect(apiSpec.IsOpenShift4).To(BeTrue())
			Expect(apiSpec.Routes).To(BeTrue())
		})

		It("Should handle empty type Hawtio CR", func() {
			hawtiotest.PerformEmptyTypeHawtioCR(mgrState.Ctx, testTools)
		})

		It("Should create expected common resources", func() {
			hawtiotest.PerformCommonResourceTest(mgrState.Ctx, testTools)

			// =====================================================================
			// TESTING THE TLS PROXY CERTIFICATES (MASTER & SLAVE)
			// =====================================================================
			By("Checking if the Hawtio Status has been updated with the Active Certificate name")
			hawtio := &hawtiov2.Hawtio{}
			Eventually(func() string {
				err := testTools.K8sClient.Get(mgrState.Ctx, types.NamespacedName{Name: hawtiotest.HawtioName, Namespace: hawtiotest.HawtioNamespace}, hawtio)
				if err != nil {
					return ""
				}
				return hawtio.Status.ClientCertificate.Active
			}, hawtiotest.Timeout, hawtiotest.Interval).Should(Not(BeEmpty()), "Hawtio status should eventually track the active slave secret name")

			By("Checking if the Master Certificate Secret exists in the Operator namespace")
			masterSecret := &corev1.Secret{}
			// Adjust 'testTools.OperatorNamespace' or wherever your operator holds its master key
			masterSecretKey := types.NamespacedName{
				Name:      fmt.Sprintf("%s-tls-proxying", hawtio.Name),
				Namespace: hawtiotest.OperatorPodNS,
			}
			Eventually(func() bool {
				err := testTools.K8sClient.Get(mgrState.Ctx, masterSecretKey, masterSecret)
				return err == nil
			}, hawtiotest.Timeout, hawtiotest.Interval).Should(BeTrue(), "Master TLS Secret must exist in operator namespace")

			Expect(masterSecret.Data).To(HaveKey("tls.crt"), "Master secret should contain a tls.crt payload")

			// Capture the hashed runtime name from the status block
			slaveSecretName := hawtio.Status.ClientCertificate.Active

			By("Checking if the Slave Certificate Secret was created in the Operand namespace")
			slaveSecret := &corev1.Secret{}
			slaveSecretKey := types.NamespacedName{
				Name:      slaveSecretName,
				Namespace: hawtio.Namespace,
			}
			Eventually(func() bool {
				err := testTools.K8sClient.Get(mgrState.Ctx, slaveSecretKey, slaveSecret)
				return err == nil
			}, hawtiotest.Timeout, hawtiotest.Interval).Should(BeTrue(), "Slave TLS Secret should be created in operand namespace")

			By("Verifying the Slave Secret matches the Master data")
			Expect(slaveSecret.Data["tls.crt"]).To(Equal(masterSecret.Data["tls.crt"]), "Slave secret data must match master data payload")
			Expect(slaveSecret.Type).To(Equal(masterSecret.Type), "Slave secret should retain master secret type configuration")

			By("Verifying the Slave Secret OwnerReference points directly to the Hawtio CR")
			Expect(slaveSecret.OwnerReferences).To(HaveLen(1), "Slave secret must have exactly one owner reference")
			Expect(slaveSecret.OwnerReferences[0].Name).To(Equal(hawtio.Name), "Owner name should point to the matching Hawtio CR name")
			Expect(slaveSecret.OwnerReferences[0].UID).To(Equal(hawtio.UID), "Owner UID must map perfectly to avoid garbage collection errors")
		})

		It("Should ignore CRs in other namespaces", func() {
			hawtiotest.PerformIgnoreNamespaceTest(mgrState.Ctx, testTools)
		})

		It("Should create a Route with a 'generated' annotation and not flap", func() {
			// Use a unique name for this test
			hawtioKey := types.NamespacedName{Name: "hawtio-route-test", Namespace: hawtiotest.HawtioNamespace}

			By("Creating a Hawtio CR with an empty RouteHostName")
			hawtio := &hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{Name: hawtioKey.Name, Namespace: hawtioKey.Namespace},
				Spec: hawtiov2.HawtioSpec{
					Type:          hawtiov2.NamespaceHawtioDeploymentType,
					RouteHostName: "", // This is the trigger for the bug
				},
			}
			Expect(testTools.K8sClient.Create(mgrState.Ctx, hawtio)).To(Succeed())

			createdRoute := &routev1.Route{}
			By("Waiting for route to be created with correct annotation")
			Eventually(func(g Gomega) {
				// Check that the route exists
				g.Expect(testTools.K8sClient.Get(mgrState.Ctx, hawtioKey, createdRoute)).To(Succeed())

				// Check that the 'host.generated' annotation is present
				g.Expect(createdRoute.Annotations).To(
					HaveKeyWithValue("openshift.io/host.generated", "true"),
				)
			}, hawtiotest.Timeout, hawtiotest.Interval).Should(Succeed())

			By("Ensuring route does not flap (steady state)")
			// Now wait for 5 seconds and consistently check that
			// the route *still exists*.
			Consistently(func(g Gomega) {
				g.Expect(testTools.K8sClient.Get(mgrState.Ctx, hawtioKey, &routev1.Route{})).To(Succeed())
			}, "5s", "1s").Should(Succeed())
		})

		Context("targetting the Image Updater", func() {
			It("Dynamically updating Deployment images when the background poller detects new digests", func() {
				hawtiotest.PerformCommonUpdaterTest(testTools, mgrState, "OpenShift")
			})

			It("Updater poller tries to update images but encounters a network failure", func() {
				hawtiotest.PerformCommonUpdaterNetworkFailureTest(testTools, mgrState, "OpenShift")
			})

			It("Updater poller tries to update images but encounters only a single updated image", func() {
				hawtiotest.PerformCommonUpdaterPartialFailureTest(testTools, mgrState, "OpenShift")
			})
		})
	})

	Context("on OpenShift testing all namespaces watching", func() {

		BeforeEach(func() {
			// By setting to the empty string, all namespaces will be watched
			testTools.WatchNamespaces = ""
			mgrState = hawtiotest.SetupManagerWithCleanup(testTools)
		})

		It("Should watch CRs in all namespaces and reconcile them", func() {
			hawtiotest.PerformWatchAllNamespacesTest(mgrState.Ctx, testTools)
		})

	})
})
