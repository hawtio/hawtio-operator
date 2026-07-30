package hawtio

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Helper function to build a Kubernetes Secret containing an in-memory X.509 certificate
func createTestSecret(notBefore, notAfter time.Time) *corev1.Secret {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	Expect(err).NotTo(HaveOccurred())

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "hawtio-test-cert",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	Expect(err).NotTo(HaveOccurred())

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tls-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			corev1.TLSCertKey:       certPEM,
			corev1.TLSPrivateKeyKey: keyPEM,
		},
	}
}

var _ = Describe("Certificate Expiry and Requeue Calculation", func() {
	var logger logr.Logger

	BeforeEach(func() {
		logger = logr.Discard()
	})

	Context("calculateRequeueBuffer", func() {
		It("should cap the buffer at 1 hour for long-lived certificates (e.g. 24 hours)", func() {
			totalDuration := 24 * time.Hour
			buffer := calculateRequeueBuffer(totalDuration)
			Expect(buffer).To(Equal(1 * time.Hour))
		})

		It("should cap the buffer at 1 hour for multi-day certificates (e.g. 30 days)", func() {
			totalDuration := 30 * 24 * time.Hour
			buffer := calculateRequeueBuffer(totalDuration)
			Expect(buffer).To(Equal(1 * time.Hour))
		})

		It("should calculate a proportional 10% buffer for medium durations (e.g. 10 minutes)", func() {
			totalDuration := 10 * time.Minute
			buffer := calculateRequeueBuffer(totalDuration)
			Expect(buffer).To(Equal(1 * time.Minute))
		})

		It("should floor the buffer at 10 seconds for ultra-short test durations", func() {
			totalDuration := 30 * time.Second // 10% would be 3s, should floor to 10s
			buffer := calculateRequeueBuffer(totalDuration)
			Expect(buffer).To(Equal(10 * time.Second))
		})
	})

	Context("checkCertificateExpiry", func() {

		Describe("Malformed / Missing Secret Payloads", func() {
			It("should return 0 (rotate/recreate) if tls.crt key is missing", func() {
				secret := &corev1.Secret{Data: map[string][]byte{}}
				nextCheck := checkCertificateExpiry(24*time.Hour, secret, logger)
				Expect(nextCheck).To(Equal(time.Duration(0)))
			})

			It("should return 0 (rotate/recreate) if tls.crt contains corrupt non-PEM data", func() {
				secret := &corev1.Secret{
					Data: map[string][]byte{
						corev1.TLSCertKey: []byte("invalid pem data"),
					},
				}
				nextCheck := checkCertificateExpiry(24*time.Hour, secret, logger)
				Expect(nextCheck).To(Equal(time.Duration(0)))
			})
		})

		Describe("Freshly Minted Certificates", func() {
			It("should schedule sleep for 23 hours on a fresh 24-hour certificate (1h buffer)", func() {
				now := time.Now()
				secret := createTestSecret(now.Add(-1*time.Minute), now.Add(24*time.Hour))

				nextCheck := checkCertificateExpiry(24*time.Hour, secret, logger)

				// Expected sleep: 24h lifetime - 1h buffer = ~23h
				Expect(nextCheck).To(BeNumerically("~", 23*time.Hour, 10*time.Second))
			})

			It("should schedule sleep for 9 minutes on a fresh 10-minute certificate (1m buffer)", func() {
				now := time.Now()
				secret := createTestSecret(now.Add(-1*time.Minute), now.Add(10*time.Minute))

				nextCheck := checkCertificateExpiry(10*time.Minute, secret, logger)

				// Expected sleep: 10m lifetime - 1m buffer = ~9m
				Expect(nextCheck).To(BeNumerically("~", 9*time.Minute, 5*time.Second))
			})
		})

		Describe("Entering the Rotation Buffer Window", func() {
			It("should return 0 (rotate immediately) when remaining time is exactly inside the 1-hour buffer (e.g. 45m left)", func() {
				now := time.Now()
				// Certificate expires in 45 minutes
				secret := createTestSecret(now.Add(-23*time.Hour), now.Add(45*time.Minute))

				nextCheck := checkCertificateExpiry(24*time.Hour, secret, logger)

				Expect(nextCheck).To(Equal(time.Duration(0)), "Should return 0 to trigger immediate in-place rotation")
			})

			It("should return 0 (rotate immediately) when remaining time is inside a short 10-minute test buffer (e.g. 30s left)", func() {
				now := time.Now()
				// Certificate expires in 30 seconds (inside 1m buffer for a 10m cert)
				secret := createTestSecret(now.Add(-9*time.Minute), now.Add(30*time.Second))

				nextCheck := checkCertificateExpiry(10*time.Minute, secret, logger)

				Expect(nextCheck).To(Equal(time.Duration(0)), "Should return 0 to trigger immediate in-place rotation")
			})
		})

		Describe("Expired Certificates", func() {
			It("should return 0 (rotate immediately) for an already expired certificate", func() {
				now := time.Now()
				// Certificate expired 10 minutes ago
				secret := createTestSecret(now.Add(-25*time.Hour), now.Add(-10*time.Minute))

				nextCheck := checkCertificateExpiry(24*time.Hour, secret, logger)

				Expect(nextCheck).To(Equal(time.Duration(0)), "Expired certificates must return 0 to force rotation")
			})
		})
	})
})
