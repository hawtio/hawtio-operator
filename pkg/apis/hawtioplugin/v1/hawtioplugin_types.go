/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,path=hawtioplugins,singular=hawtioplugin,shortName=hphawt;hcphawt,categories=hawtio
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,description="Creation phase"
// +kubebuilder:printcolumn:name="PluginImage",type=string,JSONPath=`.status.pluginImage`,description="Plugin image"
// +kubebuilder:printcolumn:name="GatewayImage",type=string,JSONPath=`.status.gatewayImage`,description="Gateway image"
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`,description="Installation phase"

// HawtioPlugin is the Schema for the hawtioplugins API.
type HawtioPlugin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HawtioPluginSpec   `json:"spec,omitempty"`
	Status HawtioPluginStatus `json:"status,omitempty"`
}

// HawtioPluginSpec defines the desired state of HawtioPlugin.
type HawtioPluginSpec struct {
	// The configuration for which metadata on Hawtio custom resources to propagate to
	// generated resources such as deployments, pods, services, and routes.
	MetadataPropagation HawtioPluginMetadataPropagation `json:"metadataPropagation,omitempty"`
	// The Nginx runtime configuration
	Nginx HawtioPluginNginx `json:"nginx,omitempty"`
	// The RBAC configuration
	RBAC HawtioPluginRBAC `json:"rbac,omitempty"`
	// The HawtioPlugin console compute resources
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// The HawtioPlugin logging configuration
	Logging HawtioPluginLogging `json:"logging,omitempty"`
	// The HawtioPlugin health checking configuration
	HealthChecks HawtioPluginHealthCheckPeriods `json:"healthChecks,omitempty"`
}

// HawtioPluginMetadataPropagation defines the configuration for which
// metadata on HawtioPlugin custom resources to propagate to
// generated resources such as deployments, pods, services, and routes.
type HawtioPluginMetadataPropagation struct {
	// Annotations to propagate
	Annotations []string `json:"annotations,omitempty"`
	// Labels to propagate
	Labels []string `json:"labels,omitempty"`
}

// HawtioPluginNginx defines the Nginx runtime configuration
type HawtioPluginNginx struct {
	// The buffer size for reading client request body. Defaults to `256k`.
	ClientBodyBufferSize string `json:"clientBodyBufferSize,omitempty"`
	// The number and size of the buffers used for reading a response from
	// the proxied server, for a single connection. Defaults to `16 128k`.
	ProxyBuffers string `json:"proxyBuffers,omitempty"`
	// The size of the buffer used for storing the response body of a subrequest.
	// Defaults to `10m`.
	SubrequestOutputBufferSize string `json:"subrequestOutputBufferSize,omitempty"`
	// Defines the maximum number of concurrent requests to the master cluster,
	// allowed to exceed the rate limit immediately. Must be sized to handle the
	// "thundering herd" of API calls when Hawtio initializes in Cluster Mode
	// (e.g., loading 1000+ pods across all namespaces).
	// Defaults to `5000`.
	MasterBurstSize string `json:"masterBurstSize,omitempty"`
}

// HawtioPluginRBAC defines the RBAC configuration
type HawtioPluginRBAC struct {
	// The name of the ConfigMap that contains the ACL definition.
	ConfigMap string `json:"configMap,omitempty"`
	// Disable performance improvement brought by RBACRegistry and revert to the classic behavior. Defaults to `false`.
	DisableRBACRegistry *bool `json:"disableRBACRegistry,omitempty"`
}

// HawtioPluginLogging defines the logging options for the servers
type HawtioPluginLogging struct {
	// Configure online log level {emerg|alert|crit|error|warn|notice|info}
	PluginLogLevel string `json:"pluginLogLevel,omitempty"`
	// Configure gateway log level {info|debug}
	GatewayLogLevel string `json:"gatewayLogLevel,omitempty"`
}

// HawtioPluginHealthCheckPeriods defines readiness and liveness periods
type HawtioPluginHealthCheckPeriods struct {
	// Configure the period, in seconds, between plugin container readiness probe checks
	PluginReadinessPeriod *int32 `json:"pluginReadinessPeriod,omitempty"`
	// Configure the period, in seconds, between plugin container liveness probe checks
	PluginLivenessPeriod *int32 `json:"pluginLivenessPeriod,omitempty"`
	// Configure the period, in seconds, between gateway container readiness probe checks
	GatewayReadinessPeriod *int32 `json:"gatewayReadinessPeriod,omitempty"`
	// Configure the period, in seconds, between gateway container liveness probe checks
	GatewayLivenessPeriod *int32 `json:"gatewayLivenessPeriod,omitempty"`
}

// HawtioPluginStatus defines the observed state of HawtioPlugin.
type HawtioPluginStatus struct {
	// The HawtioPlugin plugin container image
	PluginImage string `json:"pluginImage,omitempty"`
	// The HawtioPlugin gateway container image
	GatewayImage string `json:"gatewayImage,omitempty"`
	// The deployment phase
	Phase HawtioPluginPhase `json:"phase,omitempty"`
}

// HawtioPluginPhase is the deployment phases
// +kubebuilder:validation:Enum=Initialized;Deployed;Failed;Disabled
type HawtioPluginPhase string

const (
	// PluginPhaseInitialized --
	PluginPhaseInitialized HawtioPluginPhase = "Initialized"
	// PluginPhaseDeployed --
	PluginPhaseDeployed HawtioPluginPhase = "Deployed"
	// PluginPhaseDisabled --
	PluginPhaseDisabled HawtioPluginPhase = "Disabled"
	// PluginPhaseFailed --
	PluginPhaseFailed HawtioPluginPhase = "Failed"
)

// +kubebuilder:object:root=true

// HawtioPluginList contains a list of HawtioPlugin.
type HawtioPluginList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HawtioPlugin `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HawtioPlugin{}, &HawtioPluginList{})
}
