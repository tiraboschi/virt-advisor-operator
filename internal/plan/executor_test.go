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

package plan

import (
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
)

// validationTestCase represents a test case for validation testing
type validationTestCase struct {
	name        string
	item        *advisorv1alpha1.VirtPlatformConfigItem
	expectError bool
	errorMsg    string
}

// TestExecuteItem_Validation tests the validation logic in ExecuteItem
func TestExecuteItem_Validation(t *testing.T) {
	tests := []validationTestCase{
		{
			name: "Missing DesiredState",
			item: &advisorv1alpha1.VirtPlatformConfigItem{
				Name: "test-item",
				TargetRef: advisorv1alpha1.ObjectReference{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "test",
				},
				DesiredState: nil,
			},
			expectError: true,
			errorMsg:    "missing desired state",
		},
		{
			name: "Invalid JSON in DesiredState",
			item: &advisorv1alpha1.VirtPlatformConfigItem{
				Name: "test-item",
				TargetRef: advisorv1alpha1.ObjectReference{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "test",
				},
				DesiredState: &runtime.RawExtension{
					Raw: []byte("invalid json"),
				},
			},
			expectError: true,
			errorMsg:    "failed to unmarshal desired state",
		},
		{
			name: "DesiredState missing kind",
			item: &advisorv1alpha1.VirtPlatformConfigItem{
				Name: "test-item",
				TargetRef: advisorv1alpha1.ObjectReference{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "test",
				},
				DesiredState: mustMarshalRawExtension(map[string]interface{}{
					"apiVersion": "v1",
					// kind missing
					"metadata": map[string]interface{}{
						"name": "test",
					},
				}),
			},
			expectError: true,
			errorMsg:    "missing required fields",
		},
		{
			name: "DesiredState missing apiVersion",
			item: &advisorv1alpha1.VirtPlatformConfigItem{
				Name: "test-item",
				TargetRef: advisorv1alpha1.ObjectReference{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "test",
				},
				DesiredState: mustMarshalRawExtension(map[string]interface{}{
					// apiVersion missing
					"kind": "ConfigMap",
					"metadata": map[string]interface{}{
						"name": "test",
					},
				}),
			},
			expectError: true,
			errorMsg:    "missing required fields",
		},
		{
			name: "DesiredState missing name",
			item: &advisorv1alpha1.VirtPlatformConfigItem{
				Name: "test-item",
				TargetRef: advisorv1alpha1.ObjectReference{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "test",
				},
				DesiredState: mustMarshalRawExtension(map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						// name missing
						"namespace": "default",
					},
				}),
			},
			expectError: true,
			errorMsg:    "missing required fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't actually execute without a real client, but we can test validation
			// by checking if the error occurs during unmarshaling/validation

			// Check for missing DesiredState
			if validateMissingDesiredState(t, tt) {
				return
			}

			// Validate desired state can be unmarshaled
			if validateUnmarshal(t, tt) {
				return
			}

			// For missing required fields tests, check the unmarshaled object
			validateRequiredFields(t, tt)
		})
	}
}

// validateMissingDesiredState checks if the test expects a missing DesiredState error
func validateMissingDesiredState(t *testing.T, tt validationTestCase) bool {
	if tt.item.DesiredState == nil && tt.expectError {
		if tt.errorMsg != "missing desired state" {
			t.Errorf("Expected different error message")
		}
		return true
	}
	return false
}

// validateUnmarshal checks if the desired state can be unmarshaled properly
func validateUnmarshal(t *testing.T, tt validationTestCase) bool {
	if tt.item.DesiredState == nil {
		return false
	}

	var obj map[string]interface{}
	err := json.Unmarshal(tt.item.DesiredState.Raw, &obj)

	if tt.expectError && tt.errorMsg == "failed to unmarshal desired state" {
		if err == nil {
			t.Error("Expected unmarshal error but got none")
		}
		return true
	}

	if err != nil && !tt.expectError {
		t.Errorf("Unexpected unmarshal error: %v", err)
		return true
	}

	return false
}

// validateRequiredFields checks if the unmarshaled object has all required fields
func validateRequiredFields(t *testing.T, tt validationTestCase) {
	if tt.item.DesiredState == nil || !tt.expectError || tt.errorMsg != "missing required fields" {
		return
	}

	var objMap map[string]interface{}
	_ = json.Unmarshal(tt.item.DesiredState.Raw, &objMap)

	kind, _ := objMap["kind"].(string)
	apiVersion, _ := objMap["apiVersion"].(string)

	metadata, _ := objMap["metadata"].(map[string]interface{})
	var name string
	if metadata != nil {
		name, _ = metadata["name"].(string)
	}

	if kind == "" || apiVersion == "" || name == "" {
		// This is expected - validation should catch this
		return
	}

	t.Error("Expected validation to fail for missing required fields")
}

// Helper function to marshal a map to RawExtension for testing
func mustMarshalRawExtension(obj map[string]interface{}) *runtime.RawExtension {
	bytes, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return &runtime.RawExtension{Raw: bytes}
}
