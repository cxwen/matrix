/*
Copyright 2020 cxwen.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NetworkPluginSpec defines the desired state of NetworkPlugin
type NetworkPluginSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of NetworkPlugin. Edit NetworkPlugin_types.go to remove/update
	Type   string       `json:"type,omitempty"`
	Calico *CalicoConfig `json:"calico,omitempty"`
}

type CalicoConfig struct {
	Version                 string `json:"version,omitempty"`
	ImageRegistry           string `json:"imageRegistry,omitempty"`
	InstallCniImageRepo     string `json:"installCniImageRepo,omitempty"`
	NodeImageRepo           string `json:"nodeImageRepo,omitempty"`
	FlexvolDriverImageRepo  string `json:"nodeImageRepo,omitempty"`
	KubeControllerImageRepo string `json:"kubeControllerImageRepo,omitempty"`
	IpAutodetectionMethod   string `json:"ipAutodetectionMethod,omitempty"`
	Ipv4poolIpip            string `json:"ipv4poolIpip,omitempty"`
	Ipv4poolCidr            string `json:"ipv4poolCidr,omitempty"`
}

type NetworkPluginPhase string

const (
	NetworkPluginInitializingPhase NetworkPluginPhase = "Initializing"
	NetworkPluginRunningPhase      NetworkPluginPhase = "Running"
	NetworkPluginTeminatingPhase   NetworkPluginPhase = "Teminating"
)

// NetworkPluginStatus defines the observed state of NetworkPlugin
type NetworkPluginStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Phase NetworkPluginPhase `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true

// NetworkPlugin is the Schema for the networkplugins API
type NetworkPlugin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkPluginSpec   `json:"spec,omitempty"`
	Status NetworkPluginStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// +kubebuilder:printcolumn:name="TYPE",type="string",JSONPath=".spec.type",description="type"
// +kubebuilder:printcolumn:name="VERSION",type="string",JSONPath=".spec.calico.version",description="dns version"
// +kubebuilder:printcolumn:name="IPAUTODETECTIONMETHOD",type="string",JSONPath=".spec.calico.ipAutodetectionMethod",description="ipAutodetectionMethod"
// +kubebuilder:printcolumn:name="IPV4POOLIPIP",type="string",JSONPath=".spec.calico.Ipv4poolIpip",description="Ipv4poolIpip"
// +kubebuilder:printcolumn:name="IPV4POOLCIDR",type="string",JSONPath=".spec.calico.Ipv4poolCidr",description="Ipv4poolCidr"
// +kubebuilder:printcolumn:name="PHASE",type="string",JSONPath=".status.phase",description="phase"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// NetworkPluginList contains a list of NetworkPlugin
type NetworkPluginList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkPlugin `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkPlugin{}, &NetworkPluginList{})
}
