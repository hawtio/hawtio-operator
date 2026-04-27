//go:build integration

package hawtiotest

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	hawtiov2 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v2"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"

	"github.com/hawtio/hawtio-operator/pkg/controller/hawtiotest"
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
			hawtioKey := types.NamespacedName{Name: hawtiotest.HawtioName, Namespace: hawtiotest.HawtioNamespace}
			hawtio := &hawtiov2.Hawtio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hawtioKey.Name,
					Namespace: hawtioKey.Namespace,
				},
				Spec: hawtiov2.HawtioSpec{
					Type:    hawtiov2.NamespaceHawtioDeploymentType,
					Version: "latest",
				},
			}
			Expect(testTools.K8sClient.Create(mgrState.Ctx, hawtio)).To(Succeed())

			By("Waiting for Ingress to be created")
			ingressKey := types.NamespacedName{Name: hawtiotest.HawtioName, Namespace: hawtiotest.HawtioNamespace}
			Eventually(func(g Gomega) {
				ingress := &networkingv1.Ingress{}
				g.Expect(testTools.K8sClient.Get(mgrState.Ctx, ingressKey, ingress)).To(Succeed())
			}, hawtiotest.Timeout, hawtiotest.Interval).Should(Succeed())
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
