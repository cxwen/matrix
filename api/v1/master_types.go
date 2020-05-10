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

// MasterSpec defines the desired state of Master
type MasterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of Master. Edit Master_types.go to remove/update
	Version       string    `json:"version,omitempty"`
	Replicas      int       `json:"replicas,omitempty"`
	ImageRegistry string    `json:"imageRegistry,omitempty"`
	ImageRepo     ImageRepo `json:"imageRegistry,omitempty"`
	EtcdCluster   string    `json:"etcdCluster,omitempty"`
	Expose        Expose    `json:"expose,omitempty"`
}

type ImageRepo struct {
	Apiserver         string `json:"apiserver,omitempty"`
	ControllerManager string `json:"controllerManager,omitempty"`
	Scheduler         string `json:"scheduler,omitempty"`
	Proxy             string `json:"scheduler,omitempty"`
}

type Expose struct {
	Type string   `json:"type,omitempty"`
	Node []string `json:"type,omitempty"`
}

type MasterPhase string

const (
	MasterInitializingPhase MasterPhase = "Initializing"
	MasterRunningPhase      MasterPhase = "Running"
	MasterTeminatingPhase   MasterPhase = "Teminating"
)

// MasterStatus defines the observed state of Master
type MasterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Phase MasterPhase `json:"phase,omitempty"`
	ExposeUrl []string `json:"exposeUrl,omitempty"`
	AdminKubeconfig string `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true

// Master is the Schema for the masters API
type Master struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MasterSpec   `json:"spec,omitempty"`
	Status MasterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MasterList contains a list of Master
type MasterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Master `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Master{}, &MasterList{})
}
