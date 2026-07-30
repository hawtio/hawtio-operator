//go:build integration

package hawtiotest

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/hawtio/hawtio-operator/pkg/controller/internal/hawtiotest"
)

var _ = Describe("Testing the Hawtio Controller", Ordered, func() {
	var mgrState *hawtiotest.ManagerState

	Context("on Kubernetes in a single namespace", func() {

		BeforeEach(func() {
			// Confine all tests to the single namespace
			testTools.WatchNamespaces = hawtiotest.HawtioNamespace
			mgrState = hawtiotest.SetupManagerWithCleanup(testTools)
		})

		It("Should handle empty type Hawtio CR", func() {
			hawtiotest.PerformEmptyTypeHawtioCR(mgrState.Ctx, testTools)
		})

		It("Should ignore CRs in other namespaces", func() {
			hawtiotest.PerformIgnoreNamespaceTest(mgrState.Ctx, testTools)
		})

		It("Should create expected common resources", func() {
			hawtiotest.PerformCommonResourceTest(mgrState.Ctx, testTools)
		})

		It("Should create ingress", func() {
			By("Creating a new Hawtio CR")
			hawtio := &hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hawtiotest.HawtioName,
					Namespace: hawtiotest.HawtioNamespace,
				},
				Spec: hawtiov2.HawtioSpec{
					Type:    hawtiov2.NamespaceHawtioDeploymentType,
					Version: "latest",
				},
			}
			Expect(testTools.K8sClient.Create(mgrState.Ctx, hawtio)).To(Succeed())

			By("Waiting for Ingress to be created")
			Eventually(func(g Gomega) {
				ingress := &networkingv1.Ingress{}
				g.Expect(testTools.K8sClient.Get(mgrState.Ctx, hawtiotest.LookupKey(hawtio), ingress)).To(Succeed())
			}, hawtiotest.Timeout, hawtiotest.Interval).Should(Succeed())
		})

		Context("targetting the Image Updater", func() {
			It("Dynamically updating Deployment images when the background poller detects new digests", func() {
				hawtiotest.PerformCommonUpdaterTest(testTools, mgrState, "Kubernetes")
			})

			It("Updater poller tries to update images but encounters a network failure", func() {
				hawtiotest.PerformCommonUpdaterNetworkFailureTest(testTools, mgrState, "OpenShift")
			})

			It("Updater poller tries to update images but encounters only a single updated image", func() {
				hawtiotest.PerformCommonUpdaterPartialFailureTest(testTools, mgrState, "OpenShift")
			})
		})
	})

	Context("on Kubernetes testing all namespaces watching", func() {

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
