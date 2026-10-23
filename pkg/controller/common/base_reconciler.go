package common

import (
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/hawtio/hawtio-operator/pkg/capabilities"
	"github.com/hawtio/hawtio-operator/pkg/util"
)

// ReconcileBase is a base reconciler for the hawtio implementations
type ReconcileBase struct {
	util.BuildVariables
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the API server
	Client client.Client
	Scheme *runtime.Scheme
	// Reads directly from the live cluster so maybe ahead of the client
	// cache. Only used sparingly for secrets.
	ApiReader client.Reader
	// Derivative go-client version of apiReader
	CoreClient       corev1client.CoreV1Interface
	ApiSpec          *capabilities.ApiServerSpec
	Logger           logr.Logger
	OperatorPod      types.NamespacedName
	CertExpiryPeriod time.Duration
}
