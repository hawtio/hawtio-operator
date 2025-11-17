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

	hawtioapis "github.com/hawtio/hawtio-operator/pkg/apis"
	consolev1 "github.com/openshift/api/console/v1"
	oauthv1 "github.com/openshift/api/oauth/v1"
	routev1 "github.com/openshift/api/route/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	configv1 "github.com/openshift/api/config/v1"
	configclient "github.com/openshift/client-go/config/clientset/versioned"
	oauthclient "github.com/openshift/client-go/oauth/clientset/versioned"
	kclient "k8s.io/client-go/kubernetes"

	"github.com/hawtio/hawtio-operator/pkg/controller/hawtiotest"
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

	err = hawtioapis.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = oauthv1.Install(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = routev1.Install(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = consolev1.Install(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = apiextensionsv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = configv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Create the Kubernetes client for interacting with the test environment.
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	apiClient, err := kclient.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())

	coreClient := apiClient.CoreV1()

	oauthClient, err := oauthclient.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())

	configClient, err := configclient.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())

	apiSpec, err := capabilities.APICapabilities(ctx, apiClient, configClient)
	Expect(err).NotTo(HaveOccurred())

	logger := logf.Log.WithName("Integration_Test")

	// Optional: You can set a timeout for test setup
	SetDefaultEventuallyTimeout(30 * time.Second)

	testTools = &hawtiotest.TestTools{
		ApiClient:    apiClient,
		ApiSpec:      apiSpec,
		Cancel:       cancel,
		Cfg:          cfg,
		ConfigClient: configClient,
		CoreClient:   coreClient,
		Ctx:          ctx,
		K8sClient:    k8sClient,
		Logger:       logger,
		OauthClient:  oauthClient,
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
