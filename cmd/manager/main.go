package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"github.com/operator-framework/operator-lib/leader"

	"github.com/hawtio/hawtio-operator/pkg/controller/hawtio"
	hawtiomgr "github.com/hawtio/hawtio-operator/pkg/manager"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

// logLevelEnvVar is the constant for env variable OPERATOR_LOG_LEVEL
// which specifies the level of the operator logging.
// An empty value means the operator runs with a level of "Info".
var logLevelEnvVar = "OPERATOR_LOG_LEVEL"

// watchNamespacesEnvVar is the constant for env variable WATCH_NAMESPACES
// which specifies the Namespace to watch.
// An empty value means the operator is running with cluster scope.
var watchNamespacesEnvVar = "WATCH_NAMESPACES"

// podNamespaceEnvVar is the constant for env variable POD_NAMESPACE
// which specifies the Namespace the operator pod is running in.
// This is required for Leader Election.
var podNamespaceEnvVar = "POD_NAMESPACE"

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

func printBuildVars(bv util.BuildVariables) {
	log.Info(fmt.Sprintf("Hawtio Operator Version: %s", bv.OperatorVersion))
	log.Info(fmt.Sprintf("Hawtio Online Image Repository: %s", bv.ImageRepository))
	log.Info(fmt.Sprintf("Hawtio Online Image Version: %s", bv.ImageVersion))
	log.Info(fmt.Sprintf("Hawtio Online Gateway Image Repository: %s", bv.GatewayImageRepository))
	log.Info(fmt.Sprintf("Hawtio Online Gateway Image Version: %s", bv.GatewayImageVersion))
}

func main() {
	secretName := ""
	namespace := ""
	var expirationPeriod int64

	certCheckCmd := flag.NewFlagSet("cert-expiry-check", flag.ExitOnError)
	certCheckCmd.StringVar(&namespace, "cert-namespace", "", "The certificate secret namespace")
	certCheckCmd.StringVar(&secretName, "cert-secret-name", "hawtio-online-tls-proxying", "The certificate secret's name")
	certCheckCmd.Int64Var(&expirationPeriod, "cert-expiration-period", 24, "The minimum amount of hours left for"+
		" the certificate till expiration. Certificate secret will be deleted if it's valid for less time that defined period ")

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
	// Check if arguments are passed
	if len(os.Args) > 1 {
		// Instead of running the operator, only certificate expiry is checked.
		// This is called within a cronjob
		if os.Args[1] == "cert-expiry-check" {
			err = certCheckCmd.Parse(os.Args[2:])
			if err != nil {
				log.Error(err, "")
				os.Exit(1)
			}
			if namespace == "" {
				log.Error(nil, "Namespace not specified!")
				os.Exit(1)
			}

			err = checkCertExpiry(namespace, secretName, float64(expirationPeriod), cfg)
			if err != nil {
				os.Exit(1)
			}
			os.Exit(0)
		} else {
			log.Error(nil, "Unknown argument ", os.Args[1])
			os.Exit(1)
		}
	}

	// Check POD_NAMESPACE (Required for Leader Election)
	podNamespace, found := os.LookupEnv(podNamespaceEnvVar)
	if !found {
		log.Error(nil, fmt.Sprintf("%s must be set for leader election", podNamespaceEnvVar))
		os.Exit(1)
	}

	// Get Watch Namespace (Empty = AllNamespaces)
	watchNamespace, err := getWatchNamespace()
	if err != nil {
		log.Error(err, "failed to get watch namespace")
		os.Exit(1)
	}
	flag.Parse()

	err = operatorRun(watchNamespace, podNamespace, cfg)
	if err != nil {
		os.Exit(1)
	}
}

// operatorRun setup and run the operator
func operatorRun(watchNamespace string, podNamespace string, cfg *rest.Config) error {
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

	printBuildVars(bv)

	mgr, err := hawtiomgr.New(
		hawtiomgr.WithRestConfig(cfg),
		hawtiomgr.WithWatchNamespace(watchNamespace),
		hawtiomgr.WithPodNamespace(podNamespace),
		hawtiomgr.WithBuildVariables(bv),
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

//checkCertExpiry checks whether generated certificate is expired. If yes certificate is deleted and
// the new one is generated by the operator
func checkCertExpiry(namespace string, secretName string, period float64, cfg *rest.Config) error {
	log.Info(fmt.Sprintf("Client certificate secret can be removed %.1f hours before expiration", period))
	var c client.Client
	c, err := client.New(cfg, client.Options{})
	if err != nil {
		log.Error(err, "Unable to create the client")
		return err
	}

	name := types.NamespacedName{
		Namespace: namespace,
		Name:      secretName,
	}

	caSecret := &corev1.Secret{}
	err = c.Get(context.TODO(), name, caSecret)

	if err != nil {
		log.Error(err, "Unable to get the clients certificate secret")
	} else {
		if ok, err := hawtio.ValidateCertificate(*caSecret, period); ok {
			log.Info("Certificate is not expired")
		} else {
			if err != nil {
				log.Info("Unable to parse certificate int the secret. Deleting the secret ", err)
			} else {
				log.Info(fmt.Sprintf("Certificate is expired, or will be expired in less than %.f hours.", period))
			}
			err = c.Delete(context.TODO(), caSecret)

			if err != nil {
				log.Error(err, "Unable to delete the secret")
				return err
			}
			log.Info("Expired certificate secret deleted.")
		}
	}
	return nil
}

// getWatchNamespace returns the Namespace the operator should be watching for changes
// Returns empty string if WATCH_NAMESPACES is not set (Cluster Scope)
func getWatchNamespace() (string, error) {
	ns, found := os.LookupEnv(watchNamespacesEnvVar)
	if !found {
		log.Info(fmt.Sprintf("%s is not set. Defaulting to all namespaces.", watchNamespacesEnvVar))
		return "", nil
	}
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
