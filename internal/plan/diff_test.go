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

package plan

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestSanitizeForDiff tests the sanitizeForDiff function
func TestSanitizeForDiff(t *testing.T) {
	tests := []struct {
		name     string
		input    *unstructured.Unstructured
		expected map[string]interface{}
	}{
		{
			name: "Remove all noise fields",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":              "test",
						"namespace":         "default",
						"resourceVersion":   "12345",
						"generation":        int64(2),
						"creationTimestamp": "2024-01-01T00:00:00Z",
						"uid":               "abc-123",
						"selfLink":          "/api/v1/namespaces/default/configmaps/test",
						"managedFields":     []interface{}{},
					},
					"data": map[string]interface{}{
						"key": "value",
					},
					"status": map[string]interface{}{
						"condition": "Ready",
					},
				},
			},
			expected: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "test",
					"namespace": "default",
				},
				"data": map[string]interface{}{
					"key": "value",
				},
			},
		},
		{
			name: "Already clean object",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name": "test",
					},
					"data": map[string]interface{}{
						"key": "value",
					},
				},
			},
			expected: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "test",
				},
				"data": map[string]interface{}{
					"key": "value",
				},
			},
		},
		{
			name: "Object with only metadata noise",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":            "test",
						"resourceVersion": "999",
					},
				},
			},
			expected: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "test",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeForDiff(tt.input)

			// Verify expected fields are present
			if !deepEqualUnstructured(result.Object, tt.expected) {
				t.Errorf("Sanitized object doesn't match expected.\nGot: %+v\nExpected: %+v",
					result.Object, tt.expected)
			}

			// Verify noise fields are removed
			if _, found, _ := unstructured.NestedString(result.Object, "metadata", "resourceVersion"); found {
				t.Error("resourceVersion should be removed")
			}
			if _, found, _ := unstructured.NestedString(result.Object, "metadata", "uid"); found {
				t.Error("uid should be removed")
			}
			if _, found, _ := unstructured.NestedFieldNoCopy(result.Object, "status"); found {
				t.Error("status should be removed")
			}
			if _, found, _ := unstructured.NestedFieldNoCopy(result.Object, "metadata", "managedFields"); found {
				t.Error("managedFields should be removed")
			}
		})
	}
}

// TestGenerateCreationDiff tests the generateCreationDiff function
func TestGenerateCreationDiff(t *testing.T) {
	tests := []struct {
		name            string
		obj             *unstructured.Unstructured
		expectedStrings []string
	}{
		{
			name: "Simple ConfigMap creation",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-config",
						"namespace": "default",
					},
					"data": map[string]interface{}{
						"key": "value",
					},
				},
			},
			expectedStrings: []string{
				"--- /dev/null",
				"+++ test-config (ConfigMap)",
				"@@",
				"+apiVersion: v1",
				"+kind: ConfigMap",
				"+metadata:",
				"+  name: test-config",
				"+data:",
			},
		},
		{
			name: "Cluster-scoped resource",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Namespace",
					"metadata": map[string]interface{}{
						"name": "test-namespace",
					},
				},
			},
			expectedStrings: []string{
				"--- /dev/null",
				"+++ test-namespace (Namespace)",
				"+apiVersion: v1",
				"+kind: Namespace",
				"+metadata:",
				"+  name: test-namespace",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff, err := generateCreationDiff(tt.obj)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			for _, expected := range tt.expectedStrings {
				if !strings.Contains(diff, expected) {
					t.Errorf("Expected diff to contain %q, but it didn't.\nDiff:\n%s", expected, diff)
				}
			}

			// Verify unified diff format
			if !strings.HasPrefix(diff, "---") {
				t.Error("Diff should start with '---'")
			}
			if !strings.Contains(diff, "+++") {
				t.Error("Diff should contain '+++'")
			}
			if !strings.Contains(diff, "@@") {
				t.Error("Diff should contain hunk header '@@'")
			}
		})
	}
}

// TestGenerateUnifiedDiff tests the generateUnifiedDiff function
func TestGenerateUnifiedDiff(t *testing.T) {
	tests := []struct {
		name            string
		live            *unstructured.Unstructured
		result          *unstructured.Unstructured
		expectedStrings []string
		expectNoChanges bool
	}{
		{
			name: "No changes",
			live: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name": "test",
					},
					"data": map[string]interface{}{
						"key": "value",
					},
				},
			},
			result: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name": "test",
					},
					"data": map[string]interface{}{
						"key": "value",
					},
				},
			},
			expectNoChanges: true,
			expectedStrings: []string{
				"--- test (ConfigMap)",
				"+++ test (ConfigMap)",
				"(no changes)",
			},
		},
		{
			name: "Data value changed",
			live: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name": "test",
					},
					"data": map[string]interface{}{
						"key": "old-value",
					},
				},
			},
			result: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name": "test",
					},
					"data": map[string]interface{}{
						"key": "new-value",
					},
				},
			},
			expectNoChanges: false,
			expectedStrings: []string{
				"--- test (ConfigMap)",
				"+++ test (ConfigMap)",
				"@@",
				"-  key: old-value",
				"+  key: new-value",
			},
		},
		{
			name: "New field added",
			live: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name": "test",
					},
					"data": map[string]interface{}{
						"key1": "value1",
					},
				},
			},
			result: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name": "test",
					},
					"data": map[string]interface{}{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			expectNoChanges: false,
			expectedStrings: []string{
				"@@",
				"+  key2: value2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff, err := generateUnifiedDiff(tt.live, tt.result, "test", "ConfigMap")
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			for _, expected := range tt.expectedStrings {
				if !strings.Contains(diff, expected) {
					t.Errorf("Expected diff to contain %q, but it didn't.\nDiff:\n%s", expected, diff)
				}
			}

			if tt.expectNoChanges {
				if !strings.Contains(diff, "(no changes)") {
					t.Errorf("Expected '(no changes)' in diff, got:\n%s", diff)
				}
			}
		})
	}
}

// TestPrettyPrintJSON tests the PrettyPrintJSON function
func TestPrettyPrintJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected []string
	}{
		{
			name: "Simple map",
			input: map[string]interface{}{
				"key": "value",
			},
			expected: []string{
				`"key": "value"`,
			},
		},
		{
			name: "Nested structure",
			input: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "value",
				},
			},
			expected: []string{
				`"outer"`,
				`"inner"`,
				`"value"`,
			},
		},
		{
			name: "Array",
			input: map[string]interface{}{
				"items": []string{"a", "b", "c"},
			},
			expected: []string{
				`"items"`,
				`"a"`,
				`"b"`,
				`"c"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := PrettyPrintJSON(tt.input)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			for _, expected := range tt.expected {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected JSON to contain %q, but it didn't.\nJSON:\n%s", expected, result)
				}
			}

			// Verify it's properly indented
			if !strings.Contains(result, "  ") {
				t.Error("Expected JSON to be indented with spaces")
			}
		})
	}
}

// Helper function for deep equality check on unstructured objects
func deepEqualUnstructured(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}

	for key, aVal := range a {
		bVal, exists := b[key]
		if !exists {
			return false
		}

		switch aTyped := aVal.(type) {
		case map[string]interface{}:
			bTyped, ok := bVal.(map[string]interface{})
			if !ok || !deepEqualUnstructured(aTyped, bTyped) {
				return false
			}
		case []interface{}:
			bTyped, ok := bVal.([]interface{})
			if !ok || len(aTyped) != len(bTyped) {
				return false
			}
			for i := range aTyped {
				// Simple comparison for test purposes
				if aTyped[i] != bTyped[i] {
					return false
				}
			}
		default:
			if aVal != bVal {
				return false
			}
		}
	}

	return true
}
