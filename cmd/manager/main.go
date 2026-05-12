package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"github.com/operator-framework/operator-lib/leader"

	hawtiomgr "github.com/hawtio/hawtio-operator/pkg/manager"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

// DefaultPollingInterval is the default polling interval
// of the updater if no override has been specified with
// the UPDATE_POLLING_INTERVAL environment variable
const DefaultPollingInterval = 12 * time.Hour

// logLevelEnvVar is the constant for env variable OPERATOR_LOG_LEVEL
// which specifies the level of the operator logging.
// An empty value means the operator runs with a level of "Info".
var logLevelEnvVar = "OPERATOR_LOG_LEVEL"

// watchNamespacesEnvVar is the constant for env variable WATCH_NAMESPACES
// which specifies the Namespace to watch.
// An empty value means the operator is running with cluster scope.
var watchNamespacesEnvVar = "WATCH_NAMESPACES"

// watchNamespacesLegacyEnvVar is the constant for the legacy env var WATCH_NAMESPACE
// which despite only being singular can take a list of namespaces. This is also
// magically injected by OLM so lets observe this as well just in case.
// An empty value means the operator is running with cluster scope.
var watchNamespacesLegacyEnvVar = "WATCH_NAMESPACE"

// podNamespaceEnvVar is the constant for env variable POD_NAMESPACE
// which specifies the Namespace the operator pod is running in.
// This is required for Leader Election.
var podNamespaceEnvVar = "POD_NAMESPACE"

// updatePollingInterval is the constant for env variable UPDATE_POLLING_INTERVAL
// which specifies the duration between checks for the updater to determine
// if new operand images are available for the operator to upgrade to.
// Values should be in the form of a duration, ie. 6h, 12h, and the default
// will be 12h.
var updatePollingIntervalEnvVar = "UPDATE_POLLING_INTERVAL"

// Go build-time variables
var (
	ImageRepository                      string
	ImageVersion                         string
	GatewayImageVersion                  string
	GatewayImageRepository               string
	OperatorVersion                      string
	LegacyServingCertificateMountVersion string
	ProductName                          string
	ServerRootDirectory                  string
	CertificateCommonName                string
	AdditionalLabels                     string
)

var log = logf.Log.WithName("cmd")

func printVersion() {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))

	log.V(util.DebugLogLevel).Info("Debug logging has been enabled")
}

func printBuildVars(bv util.BuildVariables, watchNamespaces string) {
	log.Info(fmt.Sprintf("Hawtio Operator Version: %s", bv.OperatorVersion))
	log.Info(fmt.Sprintf("Hawtio Online Image Repository: %s", bv.ImageRepository))
	log.Info(fmt.Sprintf("Hawtio Online Image Version: %s", bv.GetOnlineVersion()))
	log.Info(fmt.Sprintf("Hawtio Online Gateway Image Repository: %s", bv.GatewayImageRepository))
	log.Info(fmt.Sprintf("Hawtio Online Gateway Image Version: %s", bv.GetGatewayVersion()))

	if watchNamespaces == "" {
		log.Info("Operator: Watching ALL namespaces (Cluster Scoped)")
	} else {
		log.Info(fmt.Sprintf("Operator: Watching '%s' namespaces only", watchNamespaces))
	}
}

func main() {
	// Implement the logger backend supporting given log-level
	zapConfig := zap.NewProductionConfig()
	zapConfig.Level = zap.NewAtomicLevelAt(getLogLevel())

	zapLog, err := zapConfig.Build()
	if err != nil {
		panic(err)
	}
	defer zapLog.Sync()

	// Converts zap log backend to zapr log implementation so that
	// it can be applied to controller-runtime/log
	logger := zapr.NewLogger(zapLog)
	logf.SetLogger(logger)

	printVersion()

	// Get a config to talk to the API server
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Check POD_NAMESPACE (Required for Leader Election)
	podNamespace, found := os.LookupEnv(podNamespaceEnvVar)
	if !found {
		log.Error(nil, fmt.Sprintf("%s must be set for leader election", podNamespaceEnvVar))
		os.Exit(1)
	}

	// Get Watch Namespaces (Empty = AllNamespaces)
	watchNamespaces, err := getWatchNamespaces()
	if err != nil {
		log.Error(err, "failed to get watch namespaces")
		os.Exit(1)
	}

	// Get update polling interval (Empty = 12h)
	updatePollingInterval := getUpdateInterval()

	flag.Parse()

	err = operatorRun(watchNamespaces, podNamespace, updatePollingInterval, cfg)
	if err != nil {
		os.Exit(1)
	}
}

// operatorRun setup and run the operator
func operatorRun(watchNamespaces string, podNamespace string, updatePollingInterval time.Duration, cfg *rest.Config) error {
	// Become the leader before proceeding
	// Note: leader.Become uses POD_NAMESPACE env var implicitly
	err := leader.Become(context.TODO(), "hawtio-lock")
	if err == leader.ErrNoNamespace {
		log.Info("Local run detected, leader election is disabled")
	} else if err != nil {
		log.Error(err, "")
		return err
	}

	// Setup all Controllers
	bv := util.BuildVariables{
		ImageRepository:                      ImageRepository,
		ImageVersion:                         ImageVersion,
		GatewayImageVersion:                  GatewayImageVersion,
		GatewayImageRepository:               GatewayImageRepository,
		OperatorVersion:                      OperatorVersion,
		LegacyServingCertificateMountVersion: LegacyServingCertificateMountVersion,
		ProductName:                          ProductName,
		ServerRootDirectory:                  ServerRootDirectory,
		ClientCertCommonName:                 CertificateCommonName,
		AdditionalLabels:                     AdditionalLabels,
	}

	printBuildVars(bv, watchNamespaces)

	mgr, err := hawtiomgr.New(
		hawtiomgr.WithRestConfig(cfg),
		hawtiomgr.WithWatchNamespaces(watchNamespaces),
		hawtiomgr.WithPodNamespace(podNamespace),
		hawtiomgr.WithBuildVariables(bv),
		hawtiomgr.WithUpdatePollingInterval(updatePollingInterval),
	)

	if err != nil {
		log.Error(err, "failed to create manager")
		return err
	}

	log.Info("Starting the Cmd.")

	// Start the Cmd
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "manager exited non-zero")
		return err
	}
	return nil
}

// getWatchNamespaces returns the Namespace the operator should be watching for changes
// Returns empty string if WATCH_NAMESPACES is not set (Cluster Scope)
func getWatchNamespaces() (string, error) {
	ns, found := os.LookupEnv(watchNamespacesEnvVar)
	if !found {
		log.Info(fmt.Sprintf("%s is not set. Defaulting to all namespaces.", watchNamespacesEnvVar))
		return "", nil
	}

	// Fall back to the legacy Operator SDK singular variant
	nsLegacy, foundLegacy := os.LookupEnv(watchNamespacesLegacyEnvVar)
	if foundLegacy && nsLegacy != "" {
		log.Info("WATCH_NAMESPACES not found, falling back to legacy WATCH_NAMESPACE")
		return nsLegacy, nil
	}

	log.Info("No watch namespace environment variables set. Defaulting to all namespaces.")
	return ns, nil
}

func getLogLevel() zapcore.Level {
	lvlEnv, found := os.LookupEnv(logLevelEnvVar)
	if found {
		switch lvl := strings.ToLower(lvlEnv); lvl {
		case "debug":
			return zapcore.DebugLevel
		case "info":
		default:
			return zapcore.InfoLevel
		}
	}

	fmt.Println("Defaulting to log level of info")
	return zap.InfoLevel
}

func getUpdateInterval() time.Duration {
	updatePollingInterval := DefaultPollingInterval
	updatePollingIntervalStr, found := os.LookupEnv(updatePollingIntervalEnvVar)
	if found {
		if updatePollingIntervalStr == "0" {
			updatePollingInterval = 0
		} else {
			d, err := time.ParseDuration(updatePollingIntervalStr)
			if err != nil {
				log.Error(err, "Invalid UPDATE_POLLING_INTERVAL format, defaulting to 12h")
				// It naturally falls back to the 12h default we set at the top
			} else {
				updatePollingInterval = d
			}
		}
	}

	return updatePollingInterval
}
