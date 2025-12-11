/*
Copyright 2025 The KubeVirt Authors.

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

// +kubebuilder:validation:Enum=DryRun;Apply
// PlanAction defines the workflow step for the controller.
type PlanAction string

const (
	// PlanActionDryRun calculates the plan, populates the diff in .status, and pauses.
	PlanActionDryRun PlanAction = "DryRun"

	// PlanActionApply executes the plan if prerequisites are met.
	PlanActionApply PlanAction = "Apply"
)

// +kubebuilder:validation:Enum=Abort;Continue
// FailurePolicy defines how to handle failures in a multi-step plan.
type FailurePolicy string

const (
	// FailurePolicyAbort stops executing subsequent items if one fails.
	FailurePolicyAbort FailurePolicy = "Abort"

	// FailurePolicyContinue attempts to apply remaining items even if one fails.
	FailurePolicyContinue FailurePolicy = "Continue"
)

// ConfigurationPlanSpec defines the desired state of ConfigurationPlan
type ConfigurationPlanSpec struct {
	// Profile is the named capability to activate.
	// This field is immutable once set.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="Profile is immutable"
	Profile string `json:"profile"`

	// Action dictates the controller's behavior.
	// Defaults to "DryRun".
	// +kubebuilder:default="DryRun"
	Action PlanAction `json:"action,omitempty"`

	// FailurePolicy controls execution flow when an item fails.
	// Defaults to "Abort".
	// +kubebuilder:default="Abort"
	FailurePolicy FailurePolicy `json:"failurePolicy,omitempty"`

	// ConfigOverrides allows tuning specific parameters of the chosen profile.
	// +optional
	ConfigOverrides map[string]string `json:"configOverrides,omitempty"`
}

// +kubebuilder:validation:Enum=Pending;Drafting;PrerequisiteFailed;ReviewRequired;InProgress;Completed;CompletedWithErrors;Failed;Drifted
// PlanPhase summarizes the high-level state of the plan.
type PlanPhase string

const (
	PlanPhasePending             PlanPhase = "Pending"
	PlanPhaseDrafting            PlanPhase = "Drafting"
	PlanPhasePrerequisiteFailed  PlanPhase = "PrerequisiteFailed"
	PlanPhaseReviewRequired      PlanPhase = "ReviewRequired"
	PlanPhaseInProgress          PlanPhase = "InProgress"
	PlanPhaseCompleted           PlanPhase = "Completed"
	PlanPhaseCompletedWithErrors PlanPhase = "CompletedWithErrors"
	PlanPhaseFailed              PlanPhase = "Failed"
	PlanPhaseDrifted             PlanPhase = "Drifted"
)

// +kubebuilder:validation:Enum=Pending;InProgress;Completed;Failed
// ItemState defines the state of a single configuration item.
type ItemState string

const (
	ItemStatePending    ItemState = "Pending"
	ItemStateInProgress ItemState = "InProgress"
	ItemStateCompleted  ItemState = "Completed"
	ItemStateFailed     ItemState = "Failed"
)

// ConfigurationPlanItem represents a single unit of work (e.g., one Descheduler patch).
type ConfigurationPlanItem struct {
	// Name is a human-readable identifier for this step (e.g., "enable-psi-metrics").
	Name string `json:"name"`

	// TargetRef identifies the specific resource being acted upon.
	TargetRef ObjectReference `json:"targetRef"`

	// ImpactSeverity indicates the risk level (e.g., "None", "High (Reboot)").
	ImpactSeverity string `json:"impactSeverity,omitempty"`

	// Diff contains the Unified Diff (Git style) showing the proposed changes.
	// Generated during the DryRun phase.
	// +optional
	Diff string `json:"diff,omitempty"`

	// State tracks the execution progress of this specific item.
	// +kubebuilder:default="Pending"
	State ItemState `json:"state,omitempty"`

	// Message provides real-time feedback (e.g., "Waiting for MCP update").
	// +optional
	Message string `json:"message,omitempty"`

	// LastTransitionTime is the last time the state transitioned.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
}

// ObjectReference identifies a Kubernetes object.
type ObjectReference struct {
	// APIVersion of the target resource.
	APIVersion string `json:"apiVersion"`
	// Kind of the target resource.
	Kind string `json:"kind"`
	// Name of the target resource.
	Name string `json:"name"`
	// Namespace of the target resource.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ConfigurationPlanStatus defines the observed state of ConfigurationPlan
type ConfigurationPlanStatus struct {
	// Phase is the high-level summary of the plan's lifecycle.
	Phase PlanPhase `json:"phase,omitempty"`

	// SourceSnapshotHash is the SHA256 hash of the target objects' specs
	// at the moment the plan was generated. Used for Optimistic Locking.
	// +optional
	SourceSnapshotHash string `json:"sourceSnapshotHash,omitempty"`

	// OperatorVersion tracks the version of the operator logic used to generate
	// the current items. Used to detect upgrade opportunities.
	// +optional
	OperatorVersion string `json:"operatorVersion,omitempty"`

	// Items is the ordered list of configuration steps to be executed.
	// +listType=map
	// +listMapKey=name
	Items []ConfigurationPlanItem `json:"items,omitempty"`

	// Conditions represents the latest available observations of the object's state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Profile",type="string",JSONPath=".spec.profile"
// +kubebuilder:printcolumn:name="Action",type="string",JSONPath=".spec.action"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:validation:XValidation:rule="self.metadata.name == self.spec.profile",message="To ensure a singleton pattern, the ConfigurationPlan name must exactly match the spec.profile."

// ConfigurationPlan is the Schema for the configurationplans API
type ConfigurationPlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigurationPlanSpec   `json:"spec,omitempty"`
	Status ConfigurationPlanStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ConfigurationPlanList contains a list of ConfigurationPlan
type ConfigurationPlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConfigurationPlan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConfigurationPlan{}, &ConfigurationPlanList{})
}
