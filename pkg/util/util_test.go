package util

import (
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	oauthv1 "github.com/openshift/api/oauth/v1"
	routev1 "github.com/openshift/api/route/v1"
)

func TestUtil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Util Suite")
}

var _ = Describe("String Utility Functions", func() {

	Context("When evaluating the Contains function", func() {
		It("should correctly identify if a slice contains a specific string", func() {
			Expect(Contains([]string{"a", "b", "c"}, "b")).To(BeTrue())
			Expect(Contains([]string{"a", "b", "c"}, "x")).To(BeFalse())
		})

		It("should handle empty or nil slices safely", func() {
			Expect(Contains([]string{}, "x")).To(BeFalse())
			Expect(Contains(nil, "x")).To(BeFalse())
		})
	})

	Context("When evaluating pattern matching configurations", func() {
		It("should return true for exact matches and wildcards", func() {
			Expect(Match("hawt.io/label1", "hawt.io/label1")).To(BeTrue())
			Expect(Match("hawt.io/*", "hawt.io/label1")).To(BeTrue())
			Expect(Match("", "")).To(BeTrue())
		})

		It("should correctly parse and evaluate complex regular expressions", func() {
			complexPattern := `^\s\n.?+(abc)[def]{ghi}|\$`
			Expect(Match(complexPattern, complexPattern)).To(BeTrue())
		})

		It("should return false for mismatched configurations or partial strings", func() {
			Expect(Match("a", "b")).To(BeFalse())
			Expect(Match("hawt.io/label1", "hawt_io/label1")).To(BeFalse())
			Expect(Match("", "hawt.io/label1")).To(BeFalse())
		})
	})
})

var _ = Describe("Resource Redaction Utility", func() {

	Context("When dumping resources to JSON logs", func() {

		It("should scrub sensitive data out of a ConfigMap", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "hawtio-config", Namespace: "hawtio-dev"},
				Data: map[string]string{
					"hawtio-tabs.json": "{'tabs': []}",
					"admin-password":   "super-secret-cleartext-pass",
					"api-token":        "sha256-abcdef123456",
				},
			}

			output := ToJSONString(cm)
			Expect(output).To(ContainSubstring(`"name":"hawtio-config"`))
			Expect(output).To(ContainSubstring(`"hawtio-tabs.json":"{'tabs': []}"`))

			// Sensitive values must be completely blinded
			Expect(output).NotTo(ContainSubstring("super-secret-cleartext-pass"))
			Expect(output).NotTo(ContainSubstring("sha256-abcdef123456"))
			Expect(output).To(ContainSubstring(`"admin-password":"[REDACTED]"`))
			Expect(output).To(ContainSubstring(`"api-token":"[REDACTED]"`))
		})

		It("should scrub the private key out of a Route", func() {
			route := &routev1.Route{
				ObjectMeta: metav1.ObjectMeta{Name: "hawtio-online", Namespace: "hawtio-dev"},
				Spec: routev1.RouteSpec{
					Host: "hawtio.phantomjinx.org.uk",
					TLS: &routev1.TLSConfig{
						Termination:   routev1.TLSTerminationReencrypt,
						Certificate:   "-----BEGIN CERTIFICATE-----\nPUBLIC_DATA\n-----END CERTIFICATE-----",
						Key:           "-----BEGIN PRIVATE KEY-----\nSECRET_KEY_DATA\n-----END PRIVATE KEY-----",
						CACertificate: "-----BEGIN CERTIFICATE-----\nCA_DATA\n-----END CERTIFICATE-----",
					},
				},
			}

			output := ToJSONString(route)
			Expect(output).To(ContainSubstring("PUBLIC_DATA"))
			Expect(output).To(ContainSubstring("CA_DATA"))

			// Critical private key check
			Expect(output).NotTo(ContainSubstring("SECRET_KEY_DATA"))
			Expect(output).To(ContainSubstring(`"key":"[REDACTED]"`))
		})

		It("should scrub authorization or token keys from an Ingress", func() {
			ingress := &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hawtio-ingress",
					Namespace: "hawtio-dev",
					Annotations: map[string]string{
						"nginx.ingress.kubernetes.io/auth-secret": "system-token-xyz",
					},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{{
						Host: "hawtio.kube.local",
					}},
				},
			}

			output := ToJSONString(ingress)
			Expect(output).To(ContainSubstring("hawtio.kube.local"))
			Expect(output).NotTo(ContainSubstring("system-token-xyz"))
		})

		It("should process a Deployment safely even if environment variables contain tokens", func() {
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "hawtio-operator"},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "hawtio-engine",
								Image: "quay.io/hawtio/operator",
								Env: []corev1.EnvVar{
									{Name: "WATCH_NAMESPACES", Value: ""},
									{Name: "ENCRYPTION_SECRET", Value: "super-private-crypto-string"},
								},
							}},
						},
					},
				},
			}

			output := ToJSONString(deploy)
			Expect(output).To(ContainSubstring("quay.io/hawtio/operator"))
			Expect(output).To(ContainSubstring(`"name":"WATCH_NAMESPACES"`))

			// The environment variable value map string must mask the payload string
			Expect(output).NotTo(ContainSubstring("super-private-crypto-string"))
		})

		It("should scrub the clientSecret from an OAuthClient payload", func() {
			oauth := &oauthv1.OAuthClient{
				ObjectMeta:   metav1.ObjectMeta{Name: "hawtio-oauth"},
				Secret:       "oauth-client-secret-token-123",
				RedirectURIs: []string{"https://hawtio.phantomjinx.org.uk"},
			}

			output := ToJSONString(oauth)
			Expect(output).To(ContainSubstring("https://hawtio.phantomjinx.org.uk"))
			Expect(output).NotTo(ContainSubstring("oauth-client-secret-token-123"))
		})

		It("should leave a standard ServiceAccount structure unaffected", func() {
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "hawtio-operator", Namespace: "hawtio-dev"},
				Secrets: []corev1.ObjectReference{
					{Name: "hawtio-operator-token-abc"},
				},
			}

			output := ToJSONString(sa)
			Expect(output).To(ContainSubstring(`"name":"hawtio-operator"`))
			// The descriptor reference link itself is fine to log, only data blobs are hidden
			Expect(output).To(ContainSubstring("hawtio-operator-token-abc"))
		})

		It("should leave standard networking Services completely unmodified", func() {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "hawtio-online"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Port:       443,
						TargetPort: intstr.FromInt(8443),
					}},
				},
			}

			output := ToJSONString(svc)
			Expect(output).To(ContainSubstring(`"port":443`))
			Expect(output).To(ContainSubstring(`"targetPort":8443`))
		})

		It("should protect sensitive field mappings inside Volume definitions", func() {
			podWithVolumes := &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "webhook-certs",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "hawtio-operator-webhook-cert",
							},
						},
					},
					{
						Name: "legacy-token-storage",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: "oauth-token-map"},
							},
						},
					},
				},
			}

			output := ToJSONString(podWithVolumes)
			Expect(output).To(ContainSubstring("hawtio-operator-webhook-cert"))

			// The key identifier within namespaced values containing 'token' fields must filter safely
			Expect(strings.Contains(output, "oauth-token-map")).To(BeTrue())
		})
	})
})
