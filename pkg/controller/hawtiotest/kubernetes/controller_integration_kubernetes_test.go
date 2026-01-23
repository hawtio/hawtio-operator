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

	Context("on Kubernetes", func() {

		BeforeEach(func() {
			mgrState = hawtiotest.StartManager(testTools)

			// Used in preference to AfterEach since
			// DeferCleanup executes right at the end of the test
			// and has a LIFO stack so any other cleanups added
			// in tests will be executed before this one.
			DeferCleanup(func() {
				By("Deleting the Hawtio CR")
				hawtiotest.PerformDeleteHawtioCR(testTools, hawtiotest.HawtioName, hawtiotest.HawtioNamespace)

				By("Stopping the Kubernetes manager")
				mgrState.Cancel()  // Signal manager to stop
				mgrState.Wg.Wait() // Wait for it to shut down
			})
		})

		It("Should handle empty type Hawtio CR", func() {
			hawtiotest.PerformEmptyTypeHawtioCR(testTools, mgrState.Ctx)
		})

		It("Should create expected common resources", func() {
			hawtiotest.PerformCommonResourceTest(testTools, mgrState.Ctx, "Kubernetes")
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
})
