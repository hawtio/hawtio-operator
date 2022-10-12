package resources

import (
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	routev1 "github.com/openshift/api/route/v1"

	hawtiov1alpha1 "github.com/hawtio/hawtio-operator/pkg/apis/hawtio/v1alpha1"
)

func NewRoute(hawtio *hawtiov1alpha1.Hawtio, routeTLSSecret, caCertRouteSecret *v1.Secret) *routev1.Route {
	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"app": "hawtio"},
			Name:   hawtio.Name,
		},
		Spec: routev1.RouteSpec{
			Host: hawtio.Spec.RouteHostName,
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: hawtio.Name,
			},
		},
	}

	tlsConfig := &routev1.TLSConfig{
		Termination:                   routev1.TLSTerminationReencrypt,
		InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
	}

	if routeTLSSecret != nil {
		tlsConfig.Key = string(routeTLSSecret.Data["tls.key"])
		tlsConfig.Certificate = string(routeTLSSecret.Data["tls.crt"])
		if caCertRouteSecret != nil {
			key := "tls.crt"
			if k := hawtio.Spec.Route.CaCert.Key; k != "" {
				key = k
			}
			tlsConfig.CACertificate = string(caCertRouteSecret.Data[key])
		}
	}

	route.Spec.TLS = tlsConfig

	return route
}

func GetRouteURL(route *routev1.Route) string {
	var scheme string
	if route.Spec.TLS != nil && len(route.Spec.TLS.Termination) > 0 {
		scheme = "https"
	} else {
		scheme = "http"
	}

	host := getRouteHost(route)

	url := scheme + "://" + host
	if len(route.Spec.Path) > 0 {
		url += route.Spec.Path
	}

	return url
}

func getRouteHost(route *routev1.Route) string {
	if len(route.Status.Ingress) == 0 {
		return route.Spec.Host
	}
	var oldestAdmittedIngress *routev1.RouteIngress
	var oldestAdmittedIngressTransitionTime *time.Time
	for _, ingress := range route.Status.Ingress {
		for _, condition := range ingress.Conditions {
			if condition.Status == v1.ConditionTrue && condition.Type == routev1.RouteAdmitted {
				if oldestAdmittedIngress == nil || oldestAdmittedIngressTransitionTime.After(condition.LastTransitionTime.Time) {
					oldestAdmittedIngress = &ingress
					oldestAdmittedIngressTransitionTime = &condition.LastTransitionTime.Time
				}
			}
		}
	}
	if oldestAdmittedIngress != nil {
		return oldestAdmittedIngress.Host
	}

	return route.Spec.Host
}
