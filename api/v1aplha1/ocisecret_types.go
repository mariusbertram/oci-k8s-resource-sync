/*
Copyright 2025.

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

package v1aplha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// OCISecretSpec defines the desired state of OCISecret
type OCISecretSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Required
	OrasArtefact string `json:"orasArtefact,omitempty"`

	// +kubebuilder:validation:Required
	ArtefactRegistry string `json:"ArtefactRegistry,omitempty"`

	// +kubebuilder:validation:Optional
	Sync Sync `json:"Sync,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default:={}
	ArtefactPullSecret corev1.SecretReference `json:"ArtefactPullSecret,omitempty"`

	// +kubebuilder:validation:Required
	TargetSecret corev1.SecretReference `json:"targetSecret,omitempty"`
}

type Sync struct {

	// +kubebuilder:validation:Optional
	Files []string `json:"Files,omitempty"`
}

// OCISecretStatus defines the observed state of OCISecret
type OCISecretStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// OCISecret is the Schema for the ocisecrets API
type OCISecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OCISecretSpec   `json:"spec,omitempty"`
	Status OCISecretStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OCISecretList contains a list of OCISecret
type OCISecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OCISecret `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OCISecret{}, &OCISecretList{})
}
