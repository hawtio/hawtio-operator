package hawtio

import (
	"context"
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

	"github.com/go-logr/logr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	configclient "github.com/openshift/client-go/config/clientset/versioned"
	oauthclient "github.com/openshift/client-go/oauth/clientset/versioned"
	kclient "k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

// These vars are used by tests to interact with the simulated cluster
type TestTools struct {
	apiClient    kclient.Interface
	apiSpec      *capabilities.ApiServerSpec
	cancel       context.CancelFunc
	cfg          *rest.Config
	configClient configclient.Interface
	coreClient   corev1client.CoreV1Interface
	ctx          context.Context
	k8sClient    client.Client
	logger       logr.Logger
	oauthClient  oauthclient.Interface
	testEnv      *envtest.Environment
}

var testTools *TestTools

// TestAPIs registers the testing suite with 'go test'.
func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

// BeforeSuite runs once before any tests in the suite.
var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	// Use a context that can be cancelled
	ctx, cancel := context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv := &envtest.Environment{
		// CRDDirectoryPaths tells envtest where to find your CRD YAMLs
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "deploy", "crd")},
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

	// Create the Kubernetes client for interacting with the test environment.
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	apiClient, err := kclient.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())

	oauthClient, err := oauthclient.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())

	configClient, err := configclient.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())

	apiSpec, err := capabilities.APICapabilities(ctx, apiClient, configClient)
	Expect(err).NotTo(HaveOccurred())

	logger := logf.Log.WithName("Integration_Test")

	// Optional: You can set a timeout for test setup
	SetDefaultEventuallyTimeout(30 * time.Second)

	testTools = &TestTools{
		apiClient:    apiClient,
		apiSpec:      apiSpec,
		cancel:       cancel,
		cfg:          cfg,
		configClient: configClient,
		coreClient:   apiClient.CoreV1(),
		ctx:          ctx,
		k8sClient:    k8sClient,
		logger:       logger,
		oauthClient:  oauthClient,
		testEnv:      testEnv,
	}
})

// AfterSuite runs once after all tests in the suite have finished.
var _ = AfterSuite(func() {
	// Cancel the context
	testTools.cancel()

	By("tearing down the test environment")
	err := testTools.testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
