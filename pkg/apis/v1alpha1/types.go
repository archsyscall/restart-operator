package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=rs,categories=restart-operator
// +kubebuilder:printcolumn:name="Target-Kind",type=string,JSONPath=`.spec.targetRef.kind`
// +kubebuilder:printcolumn:name="Target-Name",type=string,JSONPath=`.spec.targetRef.name`
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.schedule`
// +kubebuilder:printcolumn:name="Last-Restart",type=string,JSONPath=`.status.lastSuccessfulTime`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

type RestartSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RestartScheduleSpec   `json:"spec,omitempty"`
	Status RestartScheduleStatus `json:"status,omitempty"`
}

type RestartScheduleSpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^(\d+|\*)(/\d+)?(\s+(\d+|\*)(/\d+)?){4}$`
	Schedule string `json:"schedule"`

	// +kubebuilder:validation:Required
	TargetRef TargetRef `json:"targetRef"`
}

type TargetRef struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Deployment;StatefulSet;DaemonSet
	Kind string `json:"kind"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// +optional
	Namespace string `json:"namespace,omitempty"`
}

type RestartScheduleStatus struct {
	// +optional
	LastSuccessfulTime *metav1.Time `json:"lastSuccessfulTime,omitempty"`

	// +optional
	NextScheduledTime *metav1.Time `json:"nextScheduledTime,omitempty"`

	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true

type RestartScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RestartSchedule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RestartSchedule{}, &RestartScheduleList{})
}
