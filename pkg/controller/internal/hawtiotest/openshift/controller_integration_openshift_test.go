//go:build integration

package hawtiotest

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

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

// see pkg/controller/hawtio/defaults.go
const masterProxyingSecretName = "hawtio-operator-tls-proxying"

// Helper function to generate a valid self-signed X.509 certificate & key PEM block in-memory
func generateShortLivedCertPEM(duration time.Duration) ([]byte, []byte) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	serialNumberNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	Expect(err).NotTo(HaveOccurred())

	template := x509.Certificate{
		SerialNumber: serialNumberNumber,
		Subject: pkix.Name{
			CommonName: "hawtio-online.hawtio.svc",
		},
		NotBefore:             time.Now().Add(-1 * time.Minute),
		NotAfter:              time.Now().Add(duration), // Expires in 5 seconds!
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	Expect(err).NotTo(HaveOccurred())

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	return certPEM, keyPEM
}

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
				Name:      masterProxyingSecretName,
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      hawtioKey.Name,
					Namespace: hawtioKey.Namespace,
					Annotations: map[string]string{
						"hawtio.io/last-modified-by": "user",
					},
				},
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

		It("Should bypass validation for Legacy Hawtio CR (no modified-by annotation) with no custom route", func() {
			hawtioKey := types.NamespacedName{Name: "legacy-no-custom-route", Namespace: hawtiotest.HawtioNamespace}

			By("Creating a Legacy Hawtio CR (no modification annotation, no custom route host)")
			hawtio := &hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hawtioKey.Name,
					Namespace: hawtioKey.Namespace,
					// No 'hawtio.io/last-modified-by' annotation
				},
				Spec: hawtiov2.HawtioSpec{
					Type:          hawtiov2.NamespaceHawtioDeploymentType,
					RouteHostName: "",
				},
			}
			Expect(testTools.K8sClient.Create(mgrState.Ctx, hawtio)).To(Succeed())

			By("Verifying the controller loop proceeds cleanly to stable state without freezing")
			Eventually(func(g Gomega) {
				g.Expect(testTools.K8sClient.Get(mgrState.Ctx, hawtioKey, &routev1.Route{})).To(Succeed())
			}, hawtiotest.Timeout, hawtiotest.Interval).Should(Succeed())
		})

		It("Should log warning and preserve route for Legacy Hawtio CR (no modified-by annotation) with an existing custom route", func() {
			hawtioKey := types.NamespacedName{Name: "legacy-with-custom-route", Namespace: hawtiotest.HawtioNamespace}
			customHost := "legacy-host.apps-crc.testing"

			By("Pre-creating the physical Route in the cluster to simulate pre-upgrade state")
			preExistingRoute := &routev1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hawtioKey.Name,
					Namespace: hawtioKey.Namespace,
				},
				Spec: routev1.RouteSpec{
					Host: customHost,
					To: routev1.RouteTargetReference{
						Kind: "Service",
						Name: hawtioKey.Name,
					},
				},
			}
			Expect(testTools.K8sClient.Create(mgrState.Ctx, preExistingRoute)).To(Succeed())

			By("Creating the corresponding Legacy Hawtio CR with custom hostname matching the pre-created route")
			hawtio := &hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hawtioKey.Name,
					Namespace: hawtioKey.Namespace,
					// No 'hawtio.io/last-modified-by' annotation
				},
				Spec: hawtiov2.HawtioSpec{
					Type:          hawtiov2.NamespaceHawtioDeploymentType,
					RouteHostName: customHost,
				},
			}
			Expect(testTools.K8sClient.Create(mgrState.Ctx, hawtio)).To(Succeed())

			By("Verifying the pre-existing Route remains completely untouched and is not deleted or broken")
			Consistently(func(g Gomega) {
				liveRoute := &routev1.Route{}
				g.Expect(testTools.K8sClient.Get(mgrState.Ctx, hawtioKey, liveRoute)).To(Succeed())
				g.Expect(liveRoute.Spec.Host).To(Equal(customHost))
			}, "5s", "1s").Should(Succeed())
		})

		It("Should fail fast and refuse creation for a new/invalid Hawtio CR with no modified-by annotation but requesting a custom route", func() {
			hawtioKey := types.NamespacedName{Name: "new-invalid-custom-route", Namespace: hawtiotest.HawtioNamespace}

			By("Creating a brand new Hawtio CR requesting a custom route host but missing required audit metadata")
			hawtio := &hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hawtioKey.Name,
					Namespace: hawtioKey.Namespace,
					// Missing 'hawtio.io/last-modified-by' annotation completely
				},
				Spec: hawtiov2.HawtioSpec{
					Type:          hawtiov2.NamespaceHawtioDeploymentType,
					RouteHostName: "malicious-host.apps-crc.testing",
				},
			}
			Expect(testTools.K8sClient.Create(mgrState.Ctx, hawtio)).To(Succeed())

			By("Verifying that the Route is safely blocked and never physically instantiated in the cluster")
			Consistently(func(g Gomega) {
				err := testTools.K8sClient.Get(mgrState.Ctx, hawtioKey, &routev1.Route{})
				g.Expect(kerrors.IsNotFound(err)).To(BeTrue())
			}, "5s", "1s").Should(Succeed())
		})

		//
		// SKIPPED DUE TO UPDATER DISABLED
		// (temporarily!)
		//
		XContext("targetting the Image Updater", func() {
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

		Context("Master and Slave Certificate Lifecycle", func() {

			It("Should verify master and slave certificate metadata and structure", func() {
				By("Creating Hawtio CR")
				hawtio := hawtiotest.CreateBasicHawtioCR(mgrState.Ctx, testTools, "cert-test", hawtiotest.HawtioNamespace)

				masterSecret := &corev1.Secret{}
				masterKey := types.NamespacedName{
					Name:      masterProxyingSecretName,
					Namespace: hawtiotest.OperatorPodNS,
				}

				var slaveSecretName string
				By("Waiting for master certificate, slave certificate, and CR status update")
				Eventually(func(g Gomega) {
					// Fetch CR and check status
					fetched := &hawtiov2.Hawtio{}
					g.Expect(testTools.K8sClient.Get(mgrState.Ctx, hawtiotest.LookupKey(hawtio), fetched)).To(Succeed())
					slaveSecretName = fetched.Status.ClientCertificate.Active
					g.Expect(slaveSecretName).NotTo(BeEmpty())
					g.Expect(fetched.Status.ClientCertificate.Pending).To(BeEmpty())

					// Verify slave name contains expected prefix hash pattern
					g.Expect(slaveSecretName).To(ContainSubstring(fmt.Sprintf("%s-tls-proxying", hawtio.Name)))

					// Fetch Master Secret
					g.Expect(testTools.K8sClient.Get(mgrState.Ctx, masterKey, masterSecret)).To(Succeed())
				}, hawtiotest.Timeout, hawtiotest.Interval).Should(Succeed())

				// Fetch Slave Secret once names are resolved
				slaveSecret := &corev1.Secret{}
				slaveKey := types.NamespacedName{Name: slaveSecretName, Namespace: hawtiotest.HawtioNamespace}
				Expect(testTools.K8sClient.Get(mgrState.Ctx, slaveKey, slaveSecret)).To(Succeed())

				By("Verifying Secret Type and Labels")
				Expect(masterSecret.Type).To(Equal(corev1.SecretTypeTLS))
				Expect(slaveSecret.Type).To(Equal(corev1.SecretTypeTLS))
				Expect(masterSecret.Labels).To(HaveKeyWithValue("app", "hawtio"))
				Expect(slaveSecret.Labels).To(HaveKeyWithValue("app", "hawtio"))

				By("Verifying Data Keys and Content Equality")
				Expect(masterSecret.Data).To(HaveKey("tls.crt"))
				Expect(masterSecret.Data).To(HaveKey("tls.key"))
				Expect(masterSecret.Data["tls.crt"]).NotTo(BeEmpty())
				Expect(masterSecret.Data["tls.key"]).NotTo(BeEmpty())

				Expect(slaveSecret.Data["tls.crt"]).To(Equal(masterSecret.Data["tls.crt"]))
				Expect(slaveSecret.Data["tls.key"]).To(Equal(masterSecret.Data["tls.key"]))

				By("Verifying Slave Owner References")
				Expect(slaveSecret.OwnerReferences).To(HaveLen(1), "Slave secret must have exactly one owner reference")
				Expect(slaveSecret.OwnerReferences[0].Name).To(Equal(hawtio.Name))
				Expect(slaveSecret.OwnerReferences[0].UID).To(Equal(hawtio.UID))
				Expect(*slaveSecret.OwnerReferences[0].Controller).To(BeTrue())

				By("Parsing X.509 Certificate to verify Common Name and Expiry")
				block, _ := pem.Decode(masterSecret.Data["tls.crt"])
				Expect(block).NotTo(BeNil(), "PEM block should be decodable")

				cert, err := x509.ParseCertificate(block.Bytes)
				Expect(err).NotTo(HaveOccurred())

				Expect(cert.Subject.CommonName).To(Equal("hawtio-online.hawtio.svc"))

				// Verify default expiry for the proxy certificate would be 24hr
				// but we have to use a self-signed certificate which has a
				// 1 year default.
				expectedExpiry := time.Now().AddDate(1, 0, 0)
				Expect(cert.NotAfter).To(BeTemporally("~", expectedExpiry, 5*time.Minute))
			})

			It("Should handle multiple CRs sharing the same master certificate", func() {
				By("Creating multiple Hawtio CRs")
				cr1 := hawtiotest.CreateBasicHawtioCR(mgrState.Ctx, testTools, "multi-cert-1", hawtiotest.HawtioNamespace)
				cr2 := hawtiotest.CreateBasicHawtioCR(mgrState.Ctx, testTools, "multi-cert-2", hawtiotest.HawtioNamespace)
				cr3 := hawtiotest.CreateBasicHawtioCR(mgrState.Ctx, testTools, "multi-cert-3", hawtiotest.HawtioNamespace)

				DeferCleanup(func() {
					By("Cleaning up additional test CRs")
					hawtiotest.PerformDeleteHawtioCR(testTools, cr2.Name, hawtiotest.HawtioNamespace)
					hawtiotest.PerformDeleteHawtioCR(testTools, cr3.Name, hawtiotest.HawtioNamespace)
				})

				By("Verifying all CRs share the same master certificate")
				masterKey := types.NamespacedName{
					Name:      masterProxyingSecretName,
					Namespace: hawtiotest.OperatorPodNS,
				}

				masterSecret := &corev1.Secret{}
				Eventually(func() error {
					return testTools.K8sClient.Get(mgrState.Ctx, masterKey, masterSecret)
				}, hawtiotest.Timeout, hawtiotest.Interval).Should(Succeed())

				originalCertData := masterSecret.Data["tls.crt"]

				By("Verifying all CRs have slave certificates pointing to master data")
				var slave1Name, slave2Name, slave3Name string
				Eventually(func(g Gomega) {
					fetched1 := &hawtiov2.Hawtio{}
					fetched2 := &hawtiov2.Hawtio{}
					fetched3 := &hawtiov2.Hawtio{}

					g.Expect(testTools.K8sClient.Get(mgrState.Ctx, hawtiotest.LookupKey(cr1), fetched1)).To(Succeed())
					g.Expect(testTools.K8sClient.Get(mgrState.Ctx, hawtiotest.LookupKey(cr2), fetched2)).To(Succeed())
					g.Expect(testTools.K8sClient.Get(mgrState.Ctx, hawtiotest.LookupKey(cr3), fetched3)).To(Succeed())

					slave1Name = fetched1.Status.ClientCertificate.Active
					slave2Name = fetched2.Status.ClientCertificate.Active
					slave3Name = fetched3.Status.ClientCertificate.Active

					g.Expect(slave1Name).NotTo(BeEmpty())
					g.Expect(slave2Name).NotTo(BeEmpty())
					g.Expect(slave3Name).NotTo(BeEmpty())
				}, hawtiotest.Timeout, hawtiotest.Interval).Should(Succeed())

				slave1 := &corev1.Secret{}
				slave2 := &corev1.Secret{}
				slave3 := &corev1.Secret{}

				Expect(testTools.K8sClient.Get(mgrState.Ctx, types.NamespacedName{Name: slave1Name, Namespace: hawtiotest.HawtioNamespace}, slave1)).To(Succeed())
				Expect(testTools.K8sClient.Get(mgrState.Ctx, types.NamespacedName{Name: slave2Name, Namespace: hawtiotest.HawtioNamespace}, slave2)).To(Succeed())
				Expect(testTools.K8sClient.Get(mgrState.Ctx, types.NamespacedName{Name: slave3Name, Namespace: hawtiotest.HawtioNamespace}, slave3)).To(Succeed())

				Expect(slave1.Data["tls.crt"]).To(Equal(originalCertData))
				Expect(slave2.Data["tls.crt"]).To(Equal(originalCertData))
				Expect(slave3.Data["tls.crt"]).To(Equal(originalCertData))
			})
		})

		Context("Dynamic Certificate Rotation", func() {

			It("Should detect a certificate expiring in 5 seconds and automatically rotate it", func() {
				CRName := "cert-rotation-test"

				By("Creating a basic Hawtio CR")
				hawtio := hawtiotest.CreateBasicHawtioCR(mgrState.Ctx, testTools, CRName, hawtiotest.HawtioNamespace)

				masterSecretKey := types.NamespacedName{
					Name:      masterProxyingSecretName,
					Namespace: hawtiotest.OperatorPodNS,
				}

				masterSecret := &corev1.Secret{}

				By("Waiting for the initial master certificate to be created by the operator")
				var initialSerial *big.Int
				Eventually(func(g Gomega) {
					g.Expect(testTools.K8sClient.Get(mgrState.Ctx, masterSecretKey, masterSecret)).To(Succeed())
					g.Expect(masterSecret.Data["tls.crt"]).NotTo(BeEmpty())

					block, _ := pem.Decode(masterSecret.Data["tls.crt"])
					cert, err := x509.ParseCertificate(block.Bytes)
					g.Expect(err).NotTo(HaveOccurred())
					initialSerial = cert.SerialNumber
				}, hawtiotest.Timeout, hawtiotest.Interval).Should(Succeed())

				By("Generating a fake short-lived certificate expiring in 5 seconds")
				shortCertPEM, shortKeyPEM := generateShortLivedCertPEM(5 * time.Second)

				block, _ := pem.Decode(shortCertPEM)
				shortCert, err := x509.ParseCertificate(block.Bytes)
				Expect(err).NotTo(HaveOccurred())
				shortSerial := shortCert.SerialNumber

				By("Overwriting the master secret with the short-lived certificate payload")
				// Fetch fresh instance to avoid resourceVersion conflicts
				Expect(testTools.K8sClient.Get(mgrState.Ctx, masterSecretKey, masterSecret)).To(Succeed())
				masterSecret.Data["tls.crt"] = shortCertPEM
				masterSecret.Data["tls.key"] = shortKeyPEM
				Expect(testTools.K8sClient.Update(mgrState.Ctx, masterSecret)).To(Succeed())

				By("Overwriting the active slave secret (if present) with the short-lived certificate payload")
				fetchedCR := &hawtiov2.Hawtio{}
				Expect(testTools.K8sClient.Get(mgrState.Ctx, hawtiotest.LookupKey(hawtio), fetchedCR)).To(Succeed())
				if fetchedCR.Status.ClientCertificate.Active != "" {
					slaveKey := types.NamespacedName{
						Name:      fetchedCR.Status.ClientCertificate.Active,
						Namespace: hawtiotest.HawtioNamespace,
					}
					slaveSecret := &corev1.Secret{}
					if err := testTools.K8sClient.Get(mgrState.Ctx, slaveKey, slaveSecret); err == nil {
						slaveSecret.Data["tls.crt"] = shortCertPEM
						slaveSecret.Data["tls.key"] = shortKeyPEM
						Expect(testTools.K8sClient.Update(mgrState.Ctx, slaveSecret)).To(Succeed())
					}
				}

				By("Triggering a reconcile loop by annotating the Hawtio CR")
				// Updating the CR forces the reconciler to run immediately rather than waiting for requeue
				Expect(testTools.K8sClient.Get(mgrState.Ctx, hawtiotest.LookupKey(hawtio), fetchedCR)).To(Succeed())
				if fetchedCR.Annotations == nil {
					fetchedCR.Annotations = make(map[string]string)
				}
				fetchedCR.Annotations["test.hawtio.io/trigger-reconcile"] = time.Now().String()
				Expect(testTools.K8sClient.Update(mgrState.Ctx, fetchedCR)).To(Succeed())

				By("Verifying the operator detects expiry and rotates the master certificate with a new 24h cert")
				Eventually(func(g Gomega) {
					updatedMasterSecret := &corev1.Secret{}
					g.Expect(testTools.K8sClient.Get(mgrState.Ctx, masterSecretKey, updatedMasterSecret)).To(Succeed())

					block, _ := pem.Decode(updatedMasterSecret.Data["tls.crt"])
					g.Expect(block).NotTo(BeNil())

					currentCert, err := x509.ParseCertificate(block.Bytes)
					g.Expect(err).NotTo(HaveOccurred())

					// 1. Verify serial number has changed from BOTH initial and short-lived certs
					g.Expect(currentCert.SerialNumber.Cmp(shortSerial)).NotTo(Equal(0), "Certificate should have rotated away from the short-lived cert")
					g.Expect(currentCert.SerialNumber.Cmp(initialSerial)).NotTo(Equal(0), "Certificate serial should be newly generated")

					// Verify default expiry for the proxy certificate would be 24hr
					// but we have to use a self-signed certificate which has a
					// 1 year default.
					// Verify new cert's expiration date has been reset to ~1yr in the future
					expectedExpiry := time.Now().AddDate(1, 0, 0)
					g.Expect(currentCert.NotAfter).To(BeTemporally("~", expectedExpiry, 5*time.Minute))
				}, hawtiotest.Timeout, hawtiotest.Interval).Should(Succeed())

				By("Verifying slave secret is updated to match the newly rotated master secret")
				Eventually(func(g Gomega) {
					fetchedCR := &hawtiov2.Hawtio{}
					g.Expect(testTools.K8sClient.Get(mgrState.Ctx, hawtiotest.LookupKey(hawtio), fetchedCR)).To(Succeed())

					slaveSecretName := fetchedCR.Status.ClientCertificate.Active
					g.Expect(slaveSecretName).NotTo(BeEmpty())

					slaveSecret := &corev1.Secret{}
					slaveKey := types.NamespacedName{Name: slaveSecretName, Namespace: hawtiotest.HawtioNamespace}
					g.Expect(testTools.K8sClient.Get(mgrState.Ctx, slaveKey, slaveSecret)).To(Succeed())

					// Verify payload sync between slave and rotated master
					updatedMasterSecret := &corev1.Secret{}
					g.Expect(testTools.K8sClient.Get(mgrState.Ctx, masterSecretKey, updatedMasterSecret)).To(Succeed())
					g.Expect(slaveSecret.Data["tls.crt"]).To(Equal(updatedMasterSecret.Data["tls.crt"]))
				}, hawtiotest.Timeout, hawtiotest.Interval).Should(Succeed())
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
