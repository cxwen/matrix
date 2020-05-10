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

// DnsSpec defines the desired state of Dns
type DnsSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of Dns. Edit Dns_types.go to remove/update
	Type      string `json:"type"`
	Version   string `json:"version"`
	Replicas  int    `json:"replicas,omitempty"`
	ImageRegistry string `json:"imageRegistry,omitempty"`
	ImageRepo string `json:"imageRepo,omitempty"`
}

// DnsStatus defines the observed state of Dns
type DnsStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Phase string `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true

// Dns is the Schema for the dns API
type Dns struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DnsSpec   `json:"spec,omitempty"`
	Status DnsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DnsList contains a list of Dns
type DnsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Dns `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Dns{}, &DnsList{})
}
