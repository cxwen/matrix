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

// MatrixSpec defines the desired state of Matrix
type MatrixSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of Matrix. Edit Matrix_types.go to remove/update
	Master *MasterSpec      `json:"master,omitempty"`
	Etcd   *EtcdClusterSpec `json:"etcd,omitempty"`
	Dns    *DnsSpec         `json:"dns,omitempty"`
	NetworkPlugin *NetworkPluginSpec `json:"networkPlugin,omitempty"`
}

type MatrixPhase string

const (
	MatrixInitializingPhase MatrixPhase = "Initializing"
	MatrixReadyPhase        MatrixPhase = "Ready"
	MatrixTeminatingPhase   MatrixPhase = "Teminating"
)

// MatrixStatus defines the observed state of Matrix
type MatrixStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Phase MatrixPhase `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true

// +kubebuilder:printcolumn:name="K8SVERSION",type="string",JSONPath=".spec.master.version",description="version"
// +kubebuilder:printcolumn:name="K8SREPLICAS",type="string",JSONPath=".spec.master.replicas",description="replicas"
// +kubebuilder:printcolumn:name="ETCDVERSION",type="string",JSONPath=".spec.etcd.version",description="version"
// +kubebuilder:printcolumn:name="ETCDREPLICAS",type="string",JSONPath=".spec.etcd.replicas",description="replicas"
// +kubebuilder:printcolumn:name="DNSVERSION",type="string",JSONPath=".spec.dns.version",description="version"
// +kubebuilder:printcolumn:name="NETWORKPLUGIN",type="string",JSONPath=".spec.networkPlugin.type.",description="network plugin"
// +kubebuilder:printcolumn:name="NETWORKPLUGINVERION",type="string",JSONPath=".spec.networkPlugin.calico.version.",description="network plugin"
// +kubebuilder:printcolumn:name="PHASE",type="string",JSONPath=".status.phase",description="phase"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// Matrix is the Schema for the matrices API
type Matrix struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MatrixSpec   `json:"spec,omitempty"`
	Status MatrixStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MatrixList contains a list of Matrix
type MatrixList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Matrix `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Matrix{}, &MatrixList{})
}
