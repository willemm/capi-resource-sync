/*
Copyright 2021.

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

package v1alpha1

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceCloneSpec defines the desired state of ResourceClone
type ResourceCloneSpec struct {
	// Object to clone to above mentioned clusters
	Source v1.TypedLocalObjectReference `json:"source"`
	Target ResourceCloneTarget          `json:"target"`
}

type ResourceCloneTarget struct {
	// Cluster to clone to (cluster-api secret referencing cluster)
	Cluster v1.SecretReference `json:"cluster"`
	// +optional  Defaults to source namespace
	Namespace string `json:"namespace,omitempty"`
	// +optional  Defaults to source name
	Name string `json:"name,omitempty"`
}

// ResourceCloneStatus defines the observed state of ResourceClone
type ResourceCloneStatus struct {
	// When was this last cloned
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`
	// Status of cloning operation
	Status string `json:"status,omitempty"`
	// Last error that occurred
	Message string `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",type="date",name="AGE",description="Resource Age"
// +kubebuilder:printcolumn:JSONPath=".status.status",type="string",name="STATUS",description="Clone Status"
// +kubebuilder:printcolumn:JSONPath=".spec.target.cluster.name",type="string",name="CLUSTER",description="Target Cluster",priority=1
// +kubebuilder:printcolumn:JSONPath=".status.lastUpdated",type="date",name="UPDATED",description="Update Timestamp",priority=1


// ResourceClone is the Schema for the resourceclones API
type ResourceClone struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceCloneSpec   `json:"spec,omitempty"`
	Status ResourceCloneStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ResourceCloneList contains a list of ResourceClone
type ResourceCloneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceClone `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResourceClone{}, &ResourceCloneList{})
}
