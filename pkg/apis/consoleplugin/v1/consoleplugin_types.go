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
// +kubebuilder:resource:scope=Cluster,path=consoleplugins,singular=consoleplugin,shortName=hcp;hcpio,categories=hawtio
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,description="Creation phase"
// +kubebuilder:printcolumn:name="PluginImage",type=string,JSONPath=`.status.pluginImage`,description="Plugin image"
// +kubebuilder:printcolumn:name="GatewayImage",type=string,JSONPath=`.status.gatewayImage`,description="Gateway image"
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`,description="Installation phase"

// ConsolePlugin is the Schema for the consoleplugins API.
type ConsolePlugin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConsolePluginSpec   `json:"spec,omitempty"`
	Status ConsolePluginStatus `json:"status,omitempty"`
}

// ConsolePluginSpec defines the desired state of ConsolePlugin.
type ConsolePluginSpec struct {
	// The configuration for which metadata on Hawtio custom resources to propagate to
	// generated resources such as deployments, pods, services, and routes.
	MetadataPropagation ConsolePluginMetadataPropagation `json:"metadataPropagation,omitempty"`
	// The Nginx runtime configuration
	Nginx ConsolePluginNginx `json:"nginx,omitempty"`
	// The RBAC configuration
	RBAC ConsolePluginRBAC `json:"rbac,omitempty"`
	// The ConsolePlugin console compute resources
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// The ConsolePlugin logging configuration
	Logging ConsolePluginLogging `json:"logging,omitempty"`
	// The ConsolePlugin health checking configuration
	HealthChecks ConsolePluginHealthCheckPeriods `json:"healthChecks,omitempty"`
}

// ConsolePluginMetadataPropagation defines the configuration for which
// metadata on ConsolePlugin custom resources to propagate to
// generated resources such as deployments, pods, services, and routes.
type ConsolePluginMetadataPropagation struct {
	// Annotations to propagate
	Annotations []string `json:"annotations,omitempty"`
	// Labels to propagate
	Labels []string `json:"labels,omitempty"`
}

// ConsolePluginNginx defines the Nginx runtime configuration
type ConsolePluginNginx struct {
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

// ConsolePluginRBAC defines the RBAC configuration
type ConsolePluginRBAC struct {
	// The name of the ConfigMap that contains the ACL definition.
	ConfigMap string `json:"configMap,omitempty"`
	// Disable performance improvement brought by RBACRegistry and revert to the classic behavior. Defaults to `false`.
	DisableRBACRegistry *bool `json:"disableRBACRegistry,omitempty"`
}

// ConsolePluginLogging defines the logging options for the servers
type ConsolePluginLogging struct {
	// Configure online log level {emerg|alert|crit|error|warn|notice|info}
	PluginLogLevel string `json:"pluginLogLevel,omitempty"`
	// Configure gateway log level {info|debug}
	GatewayLogLevel string `json:"gatewayLogLevel,omitempty"`
}

// ConsolePluginHealthCheckPeriods defines readiness and liveness periods
type ConsolePluginHealthCheckPeriods struct {
	// Configure the period, in seconds, between plugin container readiness probe checks
	PluginReadinessPeriod *int32 `json:"pluginReadinessPeriod,omitempty"`
	// Configure the period, in seconds, between plugin container liveness probe checks
	PluginLivenessPeriod *int32 `json:"pluginLivenessPeriod,omitempty"`
	// Configure the period, in seconds, between gateway container readiness probe checks
	GatewayReadinessPeriod *int32 `json:"gatewayReadinessPeriod,omitempty"`
	// Configure the period, in seconds, between gateway container liveness probe checks
	GatewayLivenessPeriod *int32 `json:"gatewayLivenessPeriod,omitempty"`
}

// ConsolePluginStatus defines the observed state of ConsolePlugin.
type ConsolePluginStatus struct {
	// The ConsolePlugin plugin container image
	PluginImage string `json:"pluginImage,omitempty"`
	// The ConsolePlugin gateway container image
	GatewayImage string `json:"gatewayImage,omitempty"`
	// The deployment phase
	Phase ConsolePluginPhase `json:"phase,omitempty"`
}

// ConsolePluginPhase is the deployment phases
// +kubebuilder:validation:Enum=Initialized;Deployed;Failed
type ConsolePluginPhase string

const (
	// PluginPhaseInitialized --
	PluginPhaseInitialized ConsolePluginPhase = "Initialized"
	// PluginPhaseDeployed --
	PluginPhaseDeployed ConsolePluginPhase = "Deployed"
	// PluginPhaseFailed --
	PluginPhaseFailed ConsolePluginPhase = "Failed"
)

// +kubebuilder:object:root=true

// ConsolePluginList contains a list of ConsolePlugin.
type ConsolePluginList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConsolePlugin `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConsolePlugin{}, &ConsolePluginList{})
}
