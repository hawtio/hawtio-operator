package util

import (
	"time"

	"k8s.io/api/core/v1"

	routev1 "github.com/openshift/api/route/v1"
)

func GetRouteURL(route *routev1.Route) string {
	var scheme string
	if route.Spec.TLS != nil && len(route.Spec.TLS.Termination) > 0 {
		scheme = "https"
	} else {
		scheme = "http"
	}

	host := GetRouteHost(route)

	url := scheme + "://" + host
	if len(route.Spec.Path) > 0 {
		url += route.Spec.Path
	}

	return url
}

func GetRouteHost(route *routev1.Route) string {
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
