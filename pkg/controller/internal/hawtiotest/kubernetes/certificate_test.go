//go:build integration

package hawtiotest

import (
	"crypto/x509"
	"encoding/pem"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	"github.com/hawtio/hawtio-operator/pkg/controller/internal/hawtiotest"
)

var _ = Describe("Kubernetes Certificate Rotation Tests", Ordered, func() {
	var mgrState *hawtiotest.ManagerState

	BeforeEach(func() {
		// Confine all tests to the single namespace
		testTools.WatchNamespaces = hawtiotest.HawtioNamespace
		mgrState = hawtiotest.SetupManagerWithCleanup(testTools)
	})

	Context("Serving Certificate Management", func() {

		It("Should verify serving certificate metadata, structural properties, and cryptographic constraints", func() {
			By("Creating Hawtio CR with default settings")
			hawtio := hawtiotest.CreateBasicHawtioCR(mgrState.Ctx, testTools, "k8s-cert-meta", hawtiotest.HawtioNamespace)

			servingSecret := &corev1.Secret{}
			servingKey := types.NamespacedName{
				Name:      hawtio.Name + "-tls-serving",
				Namespace: hawtiotest.HawtioNamespace,
			}

			By("Waiting for serving certificate secret to be created")
			Eventually(func(g Gomega) {
				g.Expect(testTools.K8sClient.Get(mgrState.Ctx, servingKey, servingSecret)).To(Succeed())
			}, hawtiotest.Timeout, hawtiotest.Interval).Should(Succeed())

			By("Verifying Secret Type and Labels")
			Expect(servingSecret.Type).To(Equal(corev1.SecretTypeTLS))
			Expect(servingSecret.Labels).To(HaveKeyWithValue("app", "hawtio"))

			By("Verifying Secret Data Keys and Non-empty Content")
			Expect(servingSecret.Data).To(HaveKey("tls.crt"))
			Expect(servingSecret.Data).To(HaveKey("tls.key"))
			Expect(servingSecret.Data["tls.crt"]).NotTo(BeEmpty(), "Certificate data should not be empty")
			Expect(servingSecret.Data["tls.key"]).NotTo(BeEmpty(), "Key data should not be empty")

			By("Verifying Owner Reference")
			Expect(servingSecret.OwnerReferences).To(HaveLen(1), "Serving secret must have exactly one owner reference")
			Expect(servingSecret.OwnerReferences[0].Name).To(Equal(hawtio.Name))
			Expect(servingSecret.OwnerReferences[0].UID).To(Equal(hawtio.UID))
			Expect(*servingSecret.OwnerReferences[0].Controller).To(BeTrue())

			By("Parsing X.509 Certificate and verifying cryptographic properties")
			block, _ := pem.Decode(servingSecret.Data["tls.crt"])
			Expect(block).NotTo(BeNil(), "PEM block should be decodable")

			cert, err := x509.ParseCertificate(block.Bytes)
			Expect(err).NotTo(HaveOccurred())

			// Default expiry is 1 year from now
			expectedExpiry := time.Now().AddDate(1, 0, 0)
			Expect(cert.NotAfter).To(BeTemporally("~", expectedExpiry, 5*time.Minute))

			// Self-signed check
			Expect(cert.Issuer.String()).To(Equal(cert.Subject.String()), "Certificate should be self-signed")

			// Extended key usage check
			Expect(cert.ExtKeyUsage).To(ContainElement(x509.ExtKeyUsageClientAuth), "ExtKeyUsage must include ClientAuth")
		})

		It("Should create serving certificate with custom expiry date", func() {
			By("Creating Hawtio CR with custom 30 day expiry date from now")
			// Target date 30 days in the future
			customExpiryTime := metav1.NewTime(time.Now().Add(30 * 24 * time.Hour))

			hawtio := &hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "k8s-cert-custom",
					Namespace: hawtiotest.HawtioNamespace,
					Annotations: map[string]string{
						"hawtio.io/last-modified-by": "user",
					},
				},
				Spec: hawtiov2.HawtioSpec{
					Type:    hawtiov2.NamespaceHawtioDeploymentType,
					Version: "latest",
					Auth: hawtiov2.HawtioAuth{
						ClientCertExpirationDate: &customExpiryTime,
					},
				},
			}
			Expect(testTools.K8sClient.Create(mgrState.Ctx, hawtio)).Should(Succeed())

			By("Verifying serving certificate is created with custom expiry")
			servingSecret := &corev1.Secret{}
			servingKey := types.NamespacedName{
				Name:      hawtio.Name + "-tls-serving",
				Namespace: hawtiotest.HawtioNamespace,
			}

			Eventually(func(g Gomega) {
				g.Expect(testTools.K8sClient.Get(mgrState.Ctx, servingKey, servingSecret)).To(Succeed())

				certData := servingSecret.Data["tls.crt"]
				block, _ := pem.Decode(certData)
				cert, err := x509.ParseCertificate(block.Bytes)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(cert.NotAfter).To(BeTemporally("~", customExpiryTime.Time, 5*time.Minute))
			}, hawtiotest.Timeout, hawtiotest.Interval).Should(Succeed())
		})

		It("Should create independent certificates for multiple CRs", func() {
			By("Creating first Hawtio CR with default expiry date")
			hawtio1 := &hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "k8s-independent-1",
					Namespace: hawtiotest.HawtioNamespace,
					Annotations: map[string]string{
						"hawtio.io/last-modified-by": "user",
					},
				},
				Spec: hawtiov2.HawtioSpec{
					Type:    hawtiov2.NamespaceHawtioDeploymentType,
					Version: "latest",
				},
			}
			Expect(testTools.K8sClient.Create(mgrState.Ctx, hawtio1)).Should(Succeed())

			By("Creating second Hawtio CR with 30 day expiry date")
			customExpiryTime := time.Now().Add(30 * 24 * time.Hour)
			customExpiryDateTime := metav1.NewTime(customExpiryTime)
			hawtio2 := &hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "k8s-independent-2",
					Namespace: hawtiotest.HawtioNamespace,
					Annotations: map[string]string{
						"hawtio.io/last-modified-by": "user",
					},
				},
				Spec: hawtiov2.HawtioSpec{
					Type:    hawtiov2.NamespaceHawtioDeploymentType,
					Version: "latest",
					Auth: hawtiov2.HawtioAuth{
						ClientCertExpirationDate: &customExpiryDateTime,
					},
				},
			}
			Expect(testTools.K8sClient.Create(mgrState.Ctx, hawtio2)).Should(Succeed())

			DeferCleanup(func() {
				By("Cleaning up additional test CRs")
				hawtiotest.PerformDeleteHawtioCR(testTools, hawtio1.Name, hawtiotest.HawtioNamespace)
				hawtiotest.PerformDeleteHawtioCR(testTools, hawtio2.Name, hawtiotest.HawtioNamespace)
			})

			By("Verifying both certificates exist independently with different serials and expiry times")
			servingSecret1 := &corev1.Secret{}
			servingSecret2 := &corev1.Secret{}

			servingKey1 := types.NamespacedName{Name: hawtio1.Name + "-tls-serving", Namespace: hawtiotest.HawtioNamespace}
			servingKey2 := types.NamespacedName{Name: hawtio2.Name + "-tls-serving", Namespace: hawtiotest.HawtioNamespace}

			Eventually(func(g Gomega) {
				g.Expect(testTools.K8sClient.Get(mgrState.Ctx, servingKey1, servingSecret1)).To(Succeed())
				g.Expect(testTools.K8sClient.Get(mgrState.Ctx, servingKey2, servingSecret2)).To(Succeed())

				block1, _ := pem.Decode(servingSecret1.Data["tls.crt"])
				cert1, err := x509.ParseCertificate(block1.Bytes)
				g.Expect(err).NotTo(HaveOccurred())

				block2, _ := pem.Decode(servingSecret2.Data["tls.crt"])
				cert2, err := x509.ParseCertificate(block2.Bytes)
				g.Expect(err).NotTo(HaveOccurred())

				expectedExpiry1 := time.Now().AddDate(1, 0, 0)
				expectedExpiry2 := customExpiryTime

				g.Expect(cert1.NotAfter).To(BeTemporally("~", expectedExpiry1, 5*time.Minute))
				g.Expect(cert2.NotAfter).To(BeTemporally("~", expectedExpiry2, 5*time.Minute))

				g.Expect(cert1.SerialNumber).NotTo(Equal(cert2.SerialNumber))
			}, hawtiotest.Timeout, hawtiotest.Interval).Should(Succeed())
		})
	})
})
