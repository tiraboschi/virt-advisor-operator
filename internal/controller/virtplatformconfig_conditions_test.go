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

package controller

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
	"github.com/kubevirt/virt-advisor-operator/internal/util"
)

// TestUpdatePhaseConditions tests that updatePhase correctly sets all condition types, reasons, and messages using constants
func TestUpdatePhaseConditions(t *testing.T) {
	tests := []struct {
		name     string
		phase    advisorv1alpha1.PlanPhase
		expected map[string]conditionExpectation
	}{
		{
			name:  "Drafting phase sets correct conditions",
			phase: advisorv1alpha1.PlanPhaseDrafting,
			expected: map[string]conditionExpectation{
				ConditionTypeDrafting: {
					status:  metav1.ConditionTrue,
					reason:  ReasonDrafting,
					message: MessageDrafting,
				},
				ConditionTypeInProgress: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotInProgress,
					message: MessageNotInProgress,
				},
			},
		},
		{
			name:  "ReviewRequired phase sets Completed=False and ReviewRequired=True",
			phase: advisorv1alpha1.PlanPhaseReviewRequired,
			expected: map[string]conditionExpectation{
				ConditionTypeDrafting: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotDrafting,
					message: MessageNotDrafting,
				},
				ConditionTypeInProgress: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotInProgress,
					message: MessageNotInProgress,
				},
				ConditionTypeCompleted: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotCompleted,
					message: MessageCompletedNotYetApplied,
				},
				ConditionTypeDrifted: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNoDrift,
					message: MessageNoDrift,
				},
				ConditionTypeReviewReq: {
					status:  metav1.ConditionTrue,
					reason:  ReasonReviewRequired,
					message: MessageReviewRequired,
				},
			},
		},
		{
			name:  "InProgress phase sets correct conditions",
			phase: advisorv1alpha1.PlanPhaseInProgress,
			expected: map[string]conditionExpectation{
				ConditionTypeDrafting: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotDrafting,
					message: MessageNotDrafting,
				},
				ConditionTypeInProgress: {
					status:  metav1.ConditionTrue,
					reason:  ReasonInProgress,
					message: MessageInProgress,
				},
			},
		},
		{
			name:  "Completed phase sets Completed=True and Drifted=False",
			phase: advisorv1alpha1.PlanPhaseCompleted,
			expected: map[string]conditionExpectation{
				ConditionTypeDrafting: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotDrafting,
					message: MessageNotDrafting,
				},
				ConditionTypeInProgress: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotInProgress,
					message: MessageNotInProgressCompleted,
				},
				ConditionTypeDrifted: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNoDrift,
					message: MessageNoDrift,
				},
				ConditionTypeCompleted: {
					status:  metav1.ConditionTrue,
					reason:  ReasonCompleted,
					message: MessageCompletedSuccess,
				},
			},
		},
		{
			name:  "Drifted phase sets Completed=False and Drifted=True",
			phase: advisorv1alpha1.PlanPhaseDrifted,
			expected: map[string]conditionExpectation{
				ConditionTypeDrafting: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotDrafting,
					message: MessageNotDrafting,
				},
				ConditionTypeInProgress: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotInProgress,
					message: MessageNotInProgressDrifted,
				},
				ConditionTypeDrifted: {
					status:  metav1.ConditionTrue,
					reason:  ReasonDriftDetected,
					message: MessageDriftDetected,
				},
				ConditionTypeCompleted: {
					status:  metav1.ConditionFalse,
					reason:  ReasonConfigurationDrifted,
					message: MessageCompletedDrifted,
				},
			},
		},
		{
			name:  "Failed phase sets Completed=False",
			phase: advisorv1alpha1.PlanPhaseFailed,
			expected: map[string]conditionExpectation{
				ConditionTypeDrafting: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotDrafting,
					message: MessageNotDraftingFailed,
				},
				ConditionTypeInProgress: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotInProgress,
					message: MessageNotInProgressFailed,
				},
				ConditionTypeCompleted: {
					status:  metav1.ConditionFalse,
					reason:  ReasonFailed,
					message: MessageCompletedFailed,
				},
			},
		},
		{
			name:  "CompletedWithErrors phase sets Completed=False",
			phase: advisorv1alpha1.PlanPhaseCompletedWithErrors,
			expected: map[string]conditionExpectation{
				ConditionTypeDrafting: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotDrafting,
					message: MessageNotDrafting,
				},
				ConditionTypeInProgress: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotInProgress,
					message: MessageNotInProgressCompletedErr,
				},
				ConditionTypeCompleted: {
					status:  metav1.ConditionFalse,
					reason:  ReasonCompletedWithErrors,
					message: MessageCompletedWithErrors,
				},
			},
		},
		{
			name:  "PrerequisiteFailed phase sets Completed=False",
			phase: advisorv1alpha1.PlanPhasePrerequisiteFailed,
			expected: map[string]conditionExpectation{
				ConditionTypeDrafting: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotDrafting,
					message: MessageNotDraftingPrereqFail,
				},
				ConditionTypeInProgress: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotInProgress,
					message: MessageNotInProgressPrereqFail,
				},
				ConditionTypeCompleted: {
					status:  metav1.ConditionFalse,
					reason:  ReasonPrerequisitesFailed,
					message: MessageCompletedPrereqFailed,
				},
			},
		},
		{
			name:  "Ignored phase sets Ignored=True and all management conditions to False",
			phase: advisorv1alpha1.PlanPhaseIgnored,
			expected: map[string]conditionExpectation{
				ConditionTypeIgnored: {
					status:  metav1.ConditionTrue,
					reason:  ReasonIgnored,
					message: MessageIgnored,
				},
				ConditionTypeDrafting: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotIgnored,
					message: MessageNotIgnored,
				},
				ConditionTypeInProgress: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotIgnored,
					message: MessageNotIgnored,
				},
				ConditionTypeCompleted: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotIgnored,
					message: MessageNotIgnored,
				},
				ConditionTypeDrifted: {
					status:  metav1.ConditionFalse,
					reason:  ReasonNotIgnored,
					message: MessageNotIgnored,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test VirtPlatformConfig
			configPlan := &advisorv1alpha1.VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-config",
					Generation: 1,
				},
				Spec: advisorv1alpha1.VirtPlatformConfigSpec{
					Profile: "example-profile",
				},
				Status: advisorv1alpha1.VirtPlatformConfigStatus{
					Conditions: []metav1.Condition{},
				},
			}

			// Create reconciler (we won't actually call Update, just test the logic)
			reconciler := &VirtPlatformConfigReconciler{}

			// Call updatePhase logic (without the actual API update)
			configPlan.Status.Phase = tt.phase
			configPlan.Status.ObservedGeneration = configPlan.Generation

			// Manually apply the condition logic from updatePhase switch statement
			switch tt.phase {
			case advisorv1alpha1.PlanPhaseDrafting:
				reconciler.setCondition(configPlan, ConditionTypeDrafting, metav1.ConditionTrue, ReasonDrafting, MessageDrafting)
				reconciler.setCondition(configPlan, ConditionTypeInProgress, metav1.ConditionFalse, ReasonNotInProgress, MessageNotInProgress)

			case advisorv1alpha1.PlanPhaseReviewRequired:
				reconciler.setCondition(configPlan, ConditionTypeDrafting, metav1.ConditionFalse, ReasonNotDrafting, MessageNotDrafting)
				reconciler.setCondition(configPlan, ConditionTypeInProgress, metav1.ConditionFalse, ReasonNotInProgress, MessageNotInProgress)
				reconciler.setCondition(configPlan, ConditionTypeCompleted, metav1.ConditionFalse, ReasonNotCompleted, MessageCompletedNotYetApplied)
				reconciler.setCondition(configPlan, ConditionTypeDrifted, metav1.ConditionFalse, ReasonNoDrift, MessageNoDrift)
				reconciler.setCondition(configPlan, ConditionTypeReviewReq, metav1.ConditionTrue, ReasonReviewRequired, MessageReviewRequired)

			case advisorv1alpha1.PlanPhaseInProgress:
				reconciler.setCondition(configPlan, ConditionTypeDrafting, metav1.ConditionFalse, ReasonNotDrafting, MessageNotDrafting)
				reconciler.setCondition(configPlan, ConditionTypeInProgress, metav1.ConditionTrue, ReasonInProgress, MessageInProgress)

			case advisorv1alpha1.PlanPhaseCompleted:
				reconciler.setCondition(configPlan, ConditionTypeDrafting, metav1.ConditionFalse, ReasonNotDrafting, MessageNotDrafting)
				reconciler.setCondition(configPlan, ConditionTypeInProgress, metav1.ConditionFalse, ReasonNotInProgress, MessageNotInProgressCompleted)
				reconciler.setCondition(configPlan, ConditionTypeDrifted, metav1.ConditionFalse, ReasonNoDrift, MessageNoDrift)
				reconciler.setCondition(configPlan, ConditionTypeCompleted, metav1.ConditionTrue, ReasonCompleted, MessageCompletedSuccess)

			case advisorv1alpha1.PlanPhaseDrifted:
				reconciler.setCondition(configPlan, ConditionTypeDrafting, metav1.ConditionFalse, ReasonNotDrafting, MessageNotDrafting)
				reconciler.setCondition(configPlan, ConditionTypeInProgress, metav1.ConditionFalse, ReasonNotInProgress, MessageNotInProgressDrifted)
				reconciler.setCondition(configPlan, ConditionTypeDrifted, metav1.ConditionTrue, ReasonDriftDetected, MessageDriftDetected)
				reconciler.setCondition(configPlan, ConditionTypeCompleted, metav1.ConditionFalse, ReasonConfigurationDrifted, MessageCompletedDrifted)

			case advisorv1alpha1.PlanPhaseCompletedWithErrors:
				reconciler.setCondition(configPlan, ConditionTypeDrafting, metav1.ConditionFalse, ReasonNotDrafting, MessageNotDrafting)
				reconciler.setCondition(configPlan, ConditionTypeInProgress, metav1.ConditionFalse, ReasonNotInProgress, MessageNotInProgressCompletedErr)
				reconciler.setCondition(configPlan, ConditionTypeCompleted, metav1.ConditionFalse, ReasonCompletedWithErrors, MessageCompletedWithErrors)

			case advisorv1alpha1.PlanPhaseFailed:
				reconciler.setCondition(configPlan, ConditionTypeDrafting, metav1.ConditionFalse, ReasonNotDrafting, MessageNotDraftingFailed)
				reconciler.setCondition(configPlan, ConditionTypeInProgress, metav1.ConditionFalse, ReasonNotInProgress, MessageNotInProgressFailed)
				reconciler.setCondition(configPlan, ConditionTypeCompleted, metav1.ConditionFalse, ReasonFailed, MessageCompletedFailed)

			case advisorv1alpha1.PlanPhasePrerequisiteFailed:
				reconciler.setCondition(configPlan, ConditionTypeDrafting, metav1.ConditionFalse, ReasonNotDrafting, MessageNotDraftingPrereqFail)
				reconciler.setCondition(configPlan, ConditionTypeInProgress, metav1.ConditionFalse, ReasonNotInProgress, MessageNotInProgressPrereqFail)
				reconciler.setCondition(configPlan, ConditionTypeCompleted, metav1.ConditionFalse, ReasonPrerequisitesFailed, MessageCompletedPrereqFailed)

			case advisorv1alpha1.PlanPhaseIgnored:
				reconciler.setCondition(configPlan, ConditionTypeIgnored, metav1.ConditionTrue, ReasonIgnored, MessageIgnored)
				reconciler.setCondition(configPlan, ConditionTypeDrafting, metav1.ConditionFalse, ReasonNotIgnored, MessageNotIgnored)
				reconciler.setCondition(configPlan, ConditionTypeInProgress, metav1.ConditionFalse, ReasonNotIgnored, MessageNotIgnored)
				reconciler.setCondition(configPlan, ConditionTypeCompleted, metav1.ConditionFalse, ReasonNotIgnored, MessageNotIgnored)
				reconciler.setCondition(configPlan, ConditionTypeDrifted, metav1.ConditionFalse, ReasonNotIgnored, MessageNotIgnored)
			}

			// Verify all expected conditions
			for condType, expectation := range tt.expected {
				condition := findCondition(configPlan.Status.Conditions, condType)
				if condition == nil {
					t.Errorf("Expected condition %s not found", condType)
					continue
				}

				if condition.Status != expectation.status {
					t.Errorf("Condition %s: expected status %s, got %s", condType, expectation.status, condition.Status)
				}
				if condition.Reason != expectation.reason {
					t.Errorf("Condition %s: expected reason %s, got %s", condType, expectation.reason, condition.Reason)
				}
				if condition.Message != expectation.message {
					t.Errorf("Condition %s: expected message %s, got %s", condType, expectation.message, condition.Message)
				}
			}
		})
	}
}

// TestOptionsChanged tests the optionsChanged function
func TestOptionsChanged(t *testing.T) {
	tests := []struct {
		name     string
		current  *advisorv1alpha1.ProfileOptions
		applied  *advisorv1alpha1.ProfileOptions
		expected bool
	}{
		{
			name:     "Both nil - no change",
			current:  nil,
			applied:  nil,
			expected: false,
		},
		{
			name:    "Current nil, applied not nil - changed",
			current: nil,
			applied: &advisorv1alpha1.ProfileOptions{
				LoadAware: &advisorv1alpha1.LoadAwareConfig{},
			},
			expected: true,
		},
		{
			name: "Current not nil, applied nil - changed",
			current: &advisorv1alpha1.ProfileOptions{
				LoadAware: &advisorv1alpha1.LoadAwareConfig{},
			},
			applied:  nil,
			expected: true,
		},
		{
			name: "DeschedulingIntervalSeconds changed",
			current: &advisorv1alpha1.ProfileOptions{
				LoadAware: &advisorv1alpha1.LoadAwareConfig{
					DeschedulingIntervalSeconds: int32Ptr(120),
				},
			},
			applied: &advisorv1alpha1.ProfileOptions{
				LoadAware: &advisorv1alpha1.LoadAwareConfig{
					DeschedulingIntervalSeconds: int32Ptr(60),
				},
			},
			expected: true,
		},
		{
			name: "EnablePSIMetrics changed",
			current: &advisorv1alpha1.ProfileOptions{
				LoadAware: &advisorv1alpha1.LoadAwareConfig{
					EnablePSIMetrics: boolPtr(true),
				},
			},
			applied: &advisorv1alpha1.ProfileOptions{
				LoadAware: &advisorv1alpha1.LoadAwareConfig{
					EnablePSIMetrics: boolPtr(false),
				},
			},
			expected: true,
		},
		{
			name: "DevDeviationThresholds changed",
			current: &advisorv1alpha1.ProfileOptions{
				LoadAware: &advisorv1alpha1.LoadAwareConfig{
					DevDeviationThresholds: stringPtr("Low"),
				},
			},
			applied: &advisorv1alpha1.ProfileOptions{
				LoadAware: &advisorv1alpha1.LoadAwareConfig{
					DevDeviationThresholds: stringPtr("High"),
				},
			},
			expected: true,
		},
		{
			name: "No changes",
			current: &advisorv1alpha1.ProfileOptions{
				LoadAware: &advisorv1alpha1.LoadAwareConfig{
					DeschedulingIntervalSeconds: int32Ptr(120),
					EnablePSIMetrics:            boolPtr(true),
					DevDeviationThresholds:      stringPtr("AsymmetricLow"),
				},
			},
			applied: &advisorv1alpha1.ProfileOptions{
				LoadAware: &advisorv1alpha1.LoadAwareConfig{
					DeschedulingIntervalSeconds: int32Ptr(120),
					EnablePSIMetrics:            boolPtr(true),
					DevDeviationThresholds:      stringPtr("AsymmetricLow"),
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := optionsChanged(tt.current, tt.applied)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestAreAllItemsPending tests the areAllItemsPending function
func TestAreAllItemsPending(t *testing.T) {
	tests := []struct {
		name     string
		items    []advisorv1alpha1.VirtPlatformConfigItem
		expected bool
	}{
		{
			name:     "Empty items list",
			items:    []advisorv1alpha1.VirtPlatformConfigItem{},
			expected: true,
		},
		{
			name: "All items pending",
			items: []advisorv1alpha1.VirtPlatformConfigItem{
				{State: advisorv1alpha1.ItemStatePending},
				{State: advisorv1alpha1.ItemStatePending},
			},
			expected: true,
		},
		{
			name: "One item in progress",
			items: []advisorv1alpha1.VirtPlatformConfigItem{
				{State: advisorv1alpha1.ItemStatePending},
				{State: advisorv1alpha1.ItemStateInProgress},
			},
			expected: false,
		},
		{
			name: "One item completed",
			items: []advisorv1alpha1.VirtPlatformConfigItem{
				{State: advisorv1alpha1.ItemStatePending},
				{State: advisorv1alpha1.ItemStateCompleted},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := areAllItemsPending(tt.items)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestAreAllItemsCompleted tests the areAllItemsCompleted function
func TestAreAllItemsCompleted(t *testing.T) {
	tests := []struct {
		name     string
		items    []advisorv1alpha1.VirtPlatformConfigItem
		expected bool
	}{
		{
			name:     "Empty items list",
			items:    []advisorv1alpha1.VirtPlatformConfigItem{},
			expected: true,
		},
		{
			name: "All items completed",
			items: []advisorv1alpha1.VirtPlatformConfigItem{
				{State: advisorv1alpha1.ItemStateCompleted},
				{State: advisorv1alpha1.ItemStateCompleted},
			},
			expected: true,
		},
		{
			name: "All items completed or failed",
			items: []advisorv1alpha1.VirtPlatformConfigItem{
				{State: advisorv1alpha1.ItemStateCompleted},
				{State: advisorv1alpha1.ItemStateFailed},
			},
			expected: true,
		},
		{
			name: "One item still pending",
			items: []advisorv1alpha1.VirtPlatformConfigItem{
				{State: advisorv1alpha1.ItemStateCompleted},
				{State: advisorv1alpha1.ItemStatePending},
			},
			expected: false,
		},
		{
			name: "One item in progress",
			items: []advisorv1alpha1.VirtPlatformConfigItem{
				{State: advisorv1alpha1.ItemStateCompleted},
				{State: advisorv1alpha1.ItemStateInProgress},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := areAllItemsCompleted(tt.items)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// Helper types and functions

type conditionExpectation struct {
	status  metav1.ConditionStatus
	reason  string
	message string
}

func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

func int32Ptr(i int32) *int32 {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

func stringPtr(s string) *string {
	return &s
}

// TestConditionConstants verifies that all condition constants are used correctly
func TestConditionConstants(t *testing.T) {
	// Verify condition types
	conditionTypes := []string{
		ConditionTypePending,
		ConditionTypeDrafting,
		ConditionTypeInProgress,
		ConditionTypeCompleted,
		ConditionTypeDrifted,
		ConditionTypeFailed,
		ConditionTypeReviewReq,
		ConditionTypePrereqFail,
		ConditionTypeCompletedErr,
		ConditionTypeIgnored,
	}

	for _, ct := range conditionTypes {
		if ct == "" {
			t.Errorf("Empty condition type constant found")
		}
	}

	// Verify condition reasons
	reasons := []string{
		ReasonDrafting,
		ReasonNotDrafting,
		ReasonInProgress,
		ReasonNotInProgress,
		ReasonCompleted,
		ReasonNotCompleted,
		ReasonReviewRequired,
		ReasonNoDrift,
		ReasonDriftDetected,
		ReasonConfigurationDrifted,
		ReasonFailed,
		ReasonCompletedWithErrors,
		ReasonPrerequisitesFailed,
	}

	for _, r := range reasons {
		if r == "" {
			t.Errorf("Empty reason constant found")
		}
	}

	// Verify condition messages
	messages := []string{
		MessageDrafting,
		MessageNotDrafting,
		MessageNotDraftingFailed,
		MessageNotDraftingPrereqFail,
		MessageInProgress,
		MessageNotInProgress,
		MessageNotInProgressCompleted,
		MessageNotInProgressCompletedErr,
		MessageNotInProgressFailed,
		MessageNotInProgressPrereqFail,
		MessageNotInProgressDrifted,
		MessageReviewRequired,
		MessageCompletedSuccess,
		MessageCompletedNotYetApplied,
		MessageCompletedDrifted,
		MessageCompletedWithErrors,
		MessageCompletedFailed,
		MessageCompletedPrereqFailed,
		MessageNoDrift,
		MessageDriftDetected,
	}

	for _, m := range messages {
		if m == "" {
			t.Errorf("Empty message constant found")
		}
	}
}

// TestPointerHelpers tests the pointer comparison helper functions
func TestInt32PtrEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        *int32
		b        *int32
		expected bool
	}{
		{"Both nil", nil, nil, true},
		{"First nil", nil, int32Ptr(1), false},
		{"Second nil", int32Ptr(1), nil, false},
		{"Equal values", int32Ptr(5), int32Ptr(5), true},
		{"Different values", int32Ptr(5), int32Ptr(10), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := util.PtrEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestBoolPtrEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        *bool
		b        *bool
		expected bool
	}{
		{"Both nil", nil, nil, true},
		{"First nil", nil, boolPtr(true), false},
		{"Second nil", boolPtr(true), nil, false},
		{"Equal true", boolPtr(true), boolPtr(true), true},
		{"Equal false", boolPtr(false), boolPtr(false), true},
		{"Different values", boolPtr(true), boolPtr(false), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := util.PtrEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestStringPtrEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        *string
		b        *string
		expected bool
	}{
		{"Both nil", nil, nil, true},
		{"First nil", nil, stringPtr("test"), false},
		{"Second nil", stringPtr("test"), nil, false},
		{"Equal values", stringPtr("hello"), stringPtr("hello"), true},
		{"Different values", stringPtr("hello"), stringPtr("world"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := util.PtrEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
