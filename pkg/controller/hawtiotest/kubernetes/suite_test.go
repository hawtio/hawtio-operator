//go:build integration

package hawtiotest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/hawtio/hawtio-operator/pkg/clients"

	"github.com/hawtio/hawtio-operator/pkg/controller/hawtiotest"
	hawtiomgr "github.com/hawtio/hawtio-operator/pkg/manager"
)

var testTools *hawtiotest.TestTools
var testEnv *envtest.Environment

// TestIntegrationController registers the testing suite with 'go test'.
func TestIntegrationController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

// BeforeSuite runs once before any tests in the suite.
var _ = BeforeSuite(func() {
	// Ensure the controller is placed under test mode
	os.Setenv("HAWTIO_UNDER_TEST", "true")
	os.Setenv("POD_NAME", "hawtio-operator-test-pod")
	os.Setenv("POD_NAMESPACE", "hawtio-dev-test")

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	// Use a context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		// CRDDirectoryPaths tells envtest where to find your CRD YAMLs
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "..", "deploy", "crd"),
		},
		ErrorIfCRDPathMissing: true,
		DownloadBinaryAssets:  true,
	}

	var err error
	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	scheme, err := hawtiomgr.ConfigureScheme()
	Expect(err).NotTo(HaveOccurred())

	// Create the Kubernetes client for interacting with the test environment.
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	clientTools, err := clients.NewTools(cfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(clientTools).NotTo(BeNil())

	logger := logf.Log.WithName("Integration_Test")

	// Optional: You can set a timeout for test setup
	SetDefaultEventuallyTimeout(30 * time.Second)

	testTools = &hawtiotest.TestTools{
		Scheme:      scheme,
		Cancel:      cancel,
		Cfg:         cfg,
		Ctx:         ctx,
		K8sClient:   k8sClient,
		Logger:      logger,
		ClientTools: clientTools,
	}
})

// AfterSuite runs once after all tests in the suite have finished.
var _ = AfterSuite(func() {
	By("stopping the manager")
	testTools.Cancel() // Signal the manager's goroutine to stop

	By("tearing down the test environment")
	// Now it's safe to stop the API server
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
