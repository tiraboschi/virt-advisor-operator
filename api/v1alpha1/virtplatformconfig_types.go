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

// VirtPlatformConfigSpec defines the desired state of VirtPlatformConfig
// +kubebuilder:validation:XValidation:rule="!has(self.options) || (self.profile == 'load-aware-rebalancing' ? has(self.options.loadAware) : true)",message="When profile is 'load-aware-rebalancing', only options.loadAware may be set"
type VirtPlatformConfigSpec struct {
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

	// Options holds tunable parameters for the selected profile.
	// Only the field matching the chosen Profile is allowed.
	// This field is optional - if omitted, profile defaults will be used.
	// +optional
	Options *ProfileOptions `json:"options,omitempty"`

	// BypassOptimisticLock disables staleness checks before execution.
	// WARNING: This is dangerous! When true, the controller will apply configurations
	// even if target resources have been modified since plan generation, potentially
	// overwriting manual changes or applying outdated configurations.
	// Only use this in emergency recovery scenarios or dev/test environments.
	// Defaults to false.
	// +optional
	// +kubebuilder:default=false
	BypassOptimisticLock bool `json:"bypassOptimisticLock,omitempty"`

	// WaitTimeout specifies the maximum time to wait for resources to become healthy
	// after applying configuration changes. This is particularly important for
	// high-impact changes like MachineConfig rollouts that can take hours or days
	// on large clusters with many nodes and VMs to migrate.
	//
	// If not set, the controller will wait indefinitely (checking on each reconciliation)
	// until resources become healthy. This is the recommended setting for production
	// clusters where rollout times are unpredictable.
	//
	// If set, the controller will fail items that don't become healthy within this duration.
	// The timeout is measured from when the item enters InProgress state.
	//
	// Examples: "30m", "2h", "24h"
	// +optional
	WaitTimeout *metav1.Duration `json:"waitTimeout,omitempty"`
}

// ProfileOptions is a union of all possible profile configurations.
// New profiles simply add a new field here.
// CEL validation ensures only the field matching spec.profile is set.
type ProfileOptions struct {
	// LoadAware config (only valid when profile=load-aware-rebalancing)
	// +optional
	LoadAware *LoadAwareConfig `json:"loadAware,omitempty"`

	// Future profiles will add their config types here:
	// +optional
	// HighDensity *HighDensityConfig `json:"highDensity,omitempty"`
}

// LoadAwareConfig contains typed configuration for the load-aware-rebalancing profile.
type LoadAwareConfig struct {
	// DeschedulingIntervalSeconds controls how often descheduling runs.
	// Minimum: 60 (1 minute)
	// Maximum: 86400 (24 hours)
	// +kubebuilder:default=60
	// +kubebuilder:validation:Minimum=60
	// +kubebuilder:validation:Maximum=86400
	// +optional
	DeschedulingIntervalSeconds *int32 `json:"deschedulingIntervalSeconds,omitempty"`

	// EnablePSIMetrics controls whether to configure PSI kernel parameter via MachineConfig.
	// When enabled, creates a MachineConfig with psi=1 kernel argument.
	// When disabled, the MachineConfig item is not included in the plan.
	// +kubebuilder:default=true
	// +optional
	EnablePSIMetrics *bool `json:"enablePSIMetrics,omitempty"`

	// DevDeviationThresholds sets the load deviation sensitivity for descheduling.
	// Controls how aggressive the descheduler is when detecting imbalanced nodes.
	// +kubebuilder:default="AsymmetricLow"
	// +kubebuilder:validation:Enum=Low;AsymmetricLow;High
	// +optional
	DevDeviationThresholds *string `json:"devDeviationThresholds,omitempty"`
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

// VirtPlatformConfigItem represents a single unit of work (e.g., one Descheduler patch).
type VirtPlatformConfigItem struct {
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

	// ManagedFields specifies which spec fields this plan item manages.
	// Used for drift detection to only monitor fields we control.
	// Format: JSON path like "spec.profiles", "spec.kernelArguments"
	// +optional
	ManagedFields []string `json:"managedFields,omitempty"`
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

// VirtPlatformConfigStatus defines the observed state of VirtPlatformConfig
type VirtPlatformConfigStatus struct {
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

	// AppliedOptions stores the profile options that were used to generate
	// the current plan items. Used to detect when spec.options changes.
	// +optional
	AppliedOptions *ProfileOptions `json:"appliedOptions,omitempty"`

	// Items is the ordered list of configuration steps to be executed.
	// +listType=map
	// +listMapKey=name
	Items []VirtPlatformConfigItem `json:"items,omitempty"`

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
// +kubebuilder:validation:XValidation:rule="self.metadata.name == self.spec.profile",message="To ensure a singleton pattern, the VirtPlatformConfig name must exactly match the spec.profile."

// VirtPlatformConfig is the Schema for the virtplatformconfigs API
type VirtPlatformConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtPlatformConfigSpec   `json:"spec,omitempty"`
	Status VirtPlatformConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VirtPlatformConfigList contains a list of VirtPlatformConfig
type VirtPlatformConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtPlatformConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VirtPlatformConfig{}, &VirtPlatformConfigList{})
}
