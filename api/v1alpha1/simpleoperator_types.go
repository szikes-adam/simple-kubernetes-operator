/*
Copyright 2024.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	InternalError        = "InternalError"
	Creating             = "Creating"
	FailedToCreate       = "FailedToCreate"
	UpdatingChange       = "UpdatingChange"
	FailedToUpdateChange = "FailedToUpdateChange"
	Reconciling          = "Reconciling"
	Reconciled           = "Reconciled"
	Deleted              = "Deleted"
	FailedToDelete       = "FailedToDelete"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SimpleOperatorSpec defines the desired state of SimpleOperator.
type SimpleOperatorSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Specify the host for accessing Ingress e.g: szikes.hu
	// +kubebuilder:validation:Required
	Host string `json:"host,omitempty"`

	// Speficy the image with a tag optionally e.g: nginx:latest
	// +kubebuilder:validation:Required
	Image string `json:"image,omitempty"`

	// Specify the number of replicas.
	// +kubebuilder:validation:Maximum:=10
	// +kubebuilder:validation:Minimum:=1
	// +kubebuilder:default:=1
	Replicas int32 `json:"replicas,omitempty"`

	// https://book.kubebuilder.io/reference/markers/crd-validation.html
}

// SimpleOperatorStatus defines the observed state of SimpleOperator
type SimpleOperatorStatus struct {
	// Indicates the current state of deployment.
	DeploymentState string `json:"deploymentState"`

	// Shows error in case of deploymentState InternalError or FailedTo*
	DeploymentErrorMsg string `json:"deploymentErrorMsg,omitempty"`

	// Indicates the current state of service.
	ServiceState string `json:"serviceState"`

	// Shows error in case of serviceState InternalError or FailedTo*
	ServiceErrorMsg string `json:"serviceErrorMsg,omitempty"`

	// Indicates the current state of ingress.
	IngressState string `json:"ingressState"`

	// Shows error in case of ingressState InternalError or FailedTo*
	IngressErrorMsg string `json:"ingressErrorMsg,omitempty"`

	// Shows current number of available replicas.
	// Meaning of avabilableReplicas: https://stackoverflow.com/questions/66317251/couldnt-understand-availablereplicas-readyreplicas-unavailablereplicas-in-dep
	AvabilableReplicas int32 `json:"availableReplicas"`

	// Indicates the last time, when the `simpleoperator` has changed on resource
	LastUpdated metav1.Time `json:"lastUpdated"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=so

// SimpleOperator is the Schema for the simpleoperators API
type SimpleOperator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SimpleOperatorSpec   `json:"spec,omitempty"`
	Status SimpleOperatorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SimpleOperatorList contains a list of SimpleOperator
type SimpleOperatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SimpleOperator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SimpleOperator{}, &SimpleOperatorList{})
}
