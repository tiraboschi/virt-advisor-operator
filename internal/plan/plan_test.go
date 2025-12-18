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
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
)

// TestParseFieldPath tests the parseFieldPath utility function
func TestParseFieldPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []string
	}{
		{
			name:     "Simple path",
			path:     "spec.profiles",
			expected: []string{"spec", "profiles"},
		},
		{
			name:     "Deep nested path",
			path:     "spec.template.spec.containers",
			expected: []string{"spec", "template", "spec", "containers"},
		},
		{
			name:     "Single field",
			path:     "metadata",
			expected: []string{"metadata"},
		},
		{
			name:     "Empty string",
			path:     "",
			expected: nil,
		},
		{
			name:     "Path with trailing dot",
			path:     "spec.profiles.",
			expected: []string{"spec", "profiles"},
		},
		{
			name:     "Path with leading dot",
			path:     ".spec.profiles",
			expected: []string{"spec", "profiles"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFieldPath(tt.path)
			if !sliceEqual(result, tt.expected) {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestBuildResourceKey tests the buildResourceKey function
func TestBuildResourceKey(t *testing.T) {
	tests := []struct {
		name      string
		gvk       schema.GroupVersionKind
		resName   string
		namespace string
		expected  string
	}{
		{
			name: "Cluster-scoped resource",
			gvk: schema.GroupVersionKind{
				Group:   "operator.openshift.io",
				Version: "v1",
				Kind:    "KubeDescheduler",
			},
			resName:   "cluster",
			namespace: "",
			expected:  "operator.openshift.io/v1/KubeDescheduler/cluster",
		},
		{
			name: "Namespaced resource",
			gvk: schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
			},
			resName:   "myapp",
			namespace: "default",
			expected:  "apps/v1/Deployment/default/myapp",
		},
		{
			name: "Core API resource",
			gvk: schema.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "ConfigMap",
			},
			resName:   "config",
			namespace: "kube-system",
			expected:  "/v1/ConfigMap/kube-system/config",
		},
		{
			name: "Resource with empty group (core API, cluster-scoped)",
			gvk: schema.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "Node",
			},
			resName:   "node-1",
			namespace: "",
			expected:  "/v1/Node/node-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildResourceKey(tt.gvk, tt.resName, tt.namespace)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestExtractManagedState tests the extractManagedState function
func TestExtractManagedState(t *testing.T) {
	tests := []struct {
		name          string
		obj           *unstructured.Unstructured
		item          *advisorv1alpha1.VirtPlatformConfigItem
		expectedState map[string]interface{}
		expectError   bool
	}{
		{
			name: "Extract specific managed fields",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "operator.openshift.io/v1",
					"kind":       "KubeDescheduler",
					"metadata": map[string]interface{}{
						"name": "cluster",
					},
					"spec": map[string]interface{}{
						"deschedulingIntervalSeconds": int64(60),
						"profiles":                    []interface{}{"LoadAware"},
						"otherField":                  "should-not-be-extracted",
					},
				},
			},
			item: &advisorv1alpha1.VirtPlatformConfigItem{
				TargetRef: advisorv1alpha1.ObjectReference{
					APIVersion: "operator.openshift.io/v1",
					Kind:       "KubeDescheduler",
					Name:       "cluster",
				},
				ManagedFields: []string{
					"spec.deschedulingIntervalSeconds",
					"spec.profiles",
				},
			},
			expectedState: map[string]interface{}{
				"spec.deschedulingIntervalSeconds": int64(60),
				"spec.profiles":                    []interface{}{"LoadAware"},
			},
			expectError: false,
		},
		{
			name: "Extract entire spec when no managed fields specified",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "operator.openshift.io/v1",
					"kind":       "KubeDescheduler",
					"spec": map[string]interface{}{
						"deschedulingIntervalSeconds": int64(60),
						"profiles":                    []interface{}{"LoadAware"},
					},
				},
			},
			item: &advisorv1alpha1.VirtPlatformConfigItem{
				TargetRef: advisorv1alpha1.ObjectReference{
					APIVersion: "operator.openshift.io/v1",
					Kind:       "KubeDescheduler",
					Name:       "cluster",
				},
				ManagedFields: []string{}, // Empty managed fields
			},
			expectedState: map[string]interface{}{
				"spec": map[string]interface{}{
					"deschedulingIntervalSeconds": int64(60),
					"profiles":                    []interface{}{"LoadAware"},
				},
			},
			expectError: false,
		},
		{
			name: "Handle missing managed field gracefully",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "operator.openshift.io/v1",
					"kind":       "KubeDescheduler",
					"spec": map[string]interface{}{
						"deschedulingIntervalSeconds": int64(60),
					},
				},
			},
			item: &advisorv1alpha1.VirtPlatformConfigItem{
				TargetRef: advisorv1alpha1.ObjectReference{
					APIVersion: "operator.openshift.io/v1",
					Kind:       "KubeDescheduler",
					Name:       "cluster",
				},
				ManagedFields: []string{
					"spec.deschedulingIntervalSeconds",
					"spec.nonexistent", // Field doesn't exist
				},
			},
			expectedState: map[string]interface{}{
				"spec.deschedulingIntervalSeconds": int64(60),
				// nonexistent field should not be in result
			},
			expectError: false,
		},
		{
			name: "Extract nested field",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "operator.openshift.io/v1",
					"kind":       "KubeDescheduler",
					"spec": map[string]interface{}{
						"profileCustomizations": map[string]interface{}{
							"devEnableEvictionsInBackground": true,
							"devDeviationThresholds":         "High",
						},
					},
				},
			},
			item: &advisorv1alpha1.VirtPlatformConfigItem{
				TargetRef: advisorv1alpha1.ObjectReference{
					APIVersion: "operator.openshift.io/v1",
					Kind:       "KubeDescheduler",
					Name:       "cluster",
				},
				ManagedFields: []string{
					"spec.profileCustomizations.devEnableEvictionsInBackground",
					"spec.profileCustomizations.devDeviationThresholds",
				},
			},
			expectedState: map[string]interface{}{
				"spec.profileCustomizations.devEnableEvictionsInBackground": true,
				"spec.profileCustomizations.devDeviationThresholds":         "High",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractManagedState(tt.obj, tt.item)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Check if all expected keys are present with correct values
			for key, expectedValue := range tt.expectedState {
				actualValue, exists := result[key]
				if !exists {
					t.Errorf("Expected key %q not found in result", key)
					continue
				}
				if !deepEqual(actualValue, expectedValue) {
					t.Errorf("For key %q: expected %v, got %v", key, expectedValue, actualValue)
				}
			}

			// Check that no unexpected keys are present
			for key := range result {
				if _, expected := tt.expectedState[key]; !expected {
					t.Errorf("Unexpected key %q found in result with value %v", key, result[key])
				}
			}
		})
	}
}

// Helper functions

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// deepEqual performs a simple deep equality check for testing
// This is a simplified version - for production use reflect.DeepEqual
func deepEqual(a, b interface{}) bool {
	switch av := a.(type) {
	case int64:
		if bv, ok := b.(int64); ok {
			return av == bv
		}
		return false
	case string:
		if bv, ok := b.(string); ok {
			return av == bv
		}
		return false
	case bool:
		if bv, ok := b.(bool); ok {
			return av == bv
		}
		return false
	case []interface{}:
		bv, ok := b.([]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !deepEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		bv, ok := b.(map[string]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if bVal, exists := bv[k]; !exists || !deepEqual(v, bVal) {
				return false
			}
		}
		return true
	}
	return false
}
