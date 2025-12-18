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

package profileutils

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestParseKernelArg(t *testing.T) {
	tests := []struct {
		name          string
		arg           string
		expectedKey   string
		expectedValue string
	}{
		{
			name:          "key-value pair",
			arg:           "psi=1",
			expectedKey:   "psi",
			expectedValue: "1",
		},
		{
			name:          "flag without value",
			arg:           "debug",
			expectedKey:   "debug",
			expectedValue: "",
		},
		{
			name:          "empty string",
			arg:           "",
			expectedKey:   "",
			expectedValue: "",
		},
		{
			name:          "multiple equals",
			arg:           "foo=bar=baz",
			expectedKey:   "foo",
			expectedValue: "bar=baz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, value := ParseKernelArg(tt.arg)
			if key != tt.expectedKey {
				t.Errorf("ParseKernelArg(%q) key = %q, want %q", tt.arg, key, tt.expectedKey)
			}
			if value != tt.expectedValue {
				t.Errorf("ParseKernelArg(%q) value = %q, want %q", tt.arg, value, tt.expectedValue)
			}
		})
	}
}

func TestCheckKernelArg(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		target   string
		expected bool
	}{
		{
			name:     "exact match - single arg",
			args:     []string{"psi=1"},
			target:   "psi=1",
			expected: true,
		},
		{
			name:     "not present",
			args:     []string{"debug"},
			target:   "psi=1",
			expected: false,
		},
		{
			name:     "last wins - present",
			args:     []string{"psi=0", "psi=1"},
			target:   "psi=1",
			expected: true,
		},
		{
			name:     "last wins - absent",
			args:     []string{"psi=1", "psi=0"},
			target:   "psi=1",
			expected: false,
		},
		{
			name:     "flag without value - match",
			args:     []string{"debug"},
			target:   "debug",
			expected: true,
		},
		{
			name:     "flag without value - no match",
			args:     []string{"debug"},
			target:   "verbose",
			expected: false,
		},
		{
			name:     "multiple occurrences - last wins",
			args:     []string{"psi=0", "other=value", "psi=1", "another=arg"},
			target:   "psi=1",
			expected: true,
		},
		{
			name:     "empty args list",
			args:     []string{},
			target:   "psi=1",
			expected: false,
		},
		{
			name:     "different value for same key",
			args:     []string{"psi=0"},
			target:   "psi=1",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CheckKernelArg(tt.args, tt.target)
			if result != tt.expected {
				t.Errorf("CheckKernelArg(%v, %q) = %v, want %v", tt.args, tt.target, result, tt.expected)
			}
		})
	}
}

func TestAddKernelArg(t *testing.T) {
	tests := []struct {
		name     string
		existing []string
		arg      string
		expected []string
	}{
		{
			name:     "add to empty list",
			existing: []string{},
			arg:      "psi=1",
			expected: []string{"psi=1"},
		},
		{
			name:     "add new arg",
			existing: []string{"debug"},
			arg:      "psi=1",
			expected: []string{"debug", "psi=1"},
		},
		{
			name:     "duplicate - exact match already exists",
			existing: []string{"psi=1"},
			arg:      "psi=1",
			expected: []string{"psi=1"},
		},
		{
			name:     "same key different value - should add",
			existing: []string{"psi=0"},
			arg:      "psi=1",
			expected: []string{"psi=0", "psi=1"},
		},
		{
			name:     "add flag without value",
			existing: []string{"psi=1"},
			arg:      "debug",
			expected: []string{"psi=1", "debug"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AddKernelArg(tt.existing, tt.arg)
			if len(result) != len(tt.expected) {
				t.Errorf("AddKernelArg(%v, %q) length = %d, want %d", tt.existing, tt.arg, len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("AddKernelArg(%v, %q)[%d] = %q, want %q", tt.existing, tt.arg, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestRemoveKernelArg(t *testing.T) {
	tests := []struct {
		name     string
		existing []string
		arg      string
		expected []string
	}{
		{
			name:     "remove exact match",
			existing: []string{"psi=1", "debug"},
			arg:      "psi=1",
			expected: []string{"debug"},
		},
		{
			name:     "remove prefix match",
			existing: []string{"psi=0", "debug", "psi=1"},
			arg:      "psi=",
			expected: []string{"debug"},
		},
		{
			name:     "remove non-existent",
			existing: []string{"debug"},
			arg:      "psi=1",
			expected: []string{"debug"},
		},
		{
			name:     "remove from empty list",
			existing: []string{},
			arg:      "psi=1",
			expected: []string{},
		},
		{
			name:     "remove all occurrences",
			existing: []string{"psi=1", "debug", "psi=1"},
			arg:      "psi=1",
			expected: []string{"debug"},
		},
		{
			name:     "prefix match removes multiple",
			existing: []string{"psi=0", "other=1", "psi=1", "psi=2"},
			arg:      "psi=",
			expected: []string{"other=1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RemoveKernelArg(tt.existing, tt.arg)
			if len(result) != len(tt.expected) {
				t.Errorf("RemoveKernelArg(%v, %q) length = %d, want %d", tt.existing, tt.arg, len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("RemoveKernelArg(%v, %q)[%d] = %q, want %q", tt.existing, tt.arg, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestGetRenderedMachineConfigName(t *testing.T) {
	tests := []struct {
		name        string
		pool        *unstructured.Unstructured
		expected    string
		expectError bool
	}{
		{
			name: "success - rendered config present",
			pool: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "worker",
					},
					"status": map[string]interface{}{
						"configuration": map[string]interface{}{
							"name": "rendered-worker-abc123",
						},
					},
				},
			},
			expected:    "rendered-worker-abc123",
			expectError: false,
		},
		{
			name: "error - no configuration in status",
			pool: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "worker",
					},
					"status": map[string]interface{}{},
				},
			},
			expected:    "",
			expectError: true,
		},
		{
			name: "error - empty rendered config name",
			pool: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "worker",
					},
					"status": map[string]interface{}{
						"configuration": map[string]interface{}{
							"name": "",
						},
					},
				},
			},
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetRenderedMachineConfigName(tt.pool)
			if tt.expectError && err == nil {
				t.Errorf("GetRenderedMachineConfigName() expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("GetRenderedMachineConfigName() unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("GetRenderedMachineConfigName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetEffectiveKernelArgs(t *testing.T) {
	tests := []struct {
		name        string
		mc          *unstructured.Unstructured
		expected    []string
		expectError bool
	}{
		{
			name: "success - kernel args present",
			mc: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"kernelArguments": []interface{}{"psi=1", "debug"},
					},
				},
			},
			expected:    []string{"psi=1", "debug"},
			expectError: false,
		},
		{
			name: "success - no kernel args (empty slice)",
			mc: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{},
				},
			},
			expected:    []string{},
			expectError: false,
		},
		{
			name: "success - empty kernel args array",
			mc: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"kernelArguments": []interface{}{},
					},
				},
			},
			expected:    []string{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetEffectiveKernelArgs(tt.mc)
			if tt.expectError && err == nil {
				t.Errorf("GetEffectiveKernelArgs() expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("GetEffectiveKernelArgs() unexpected error: %v", err)
			}
			if len(result) != len(tt.expected) {
				t.Errorf("GetEffectiveKernelArgs() length = %d, want %d", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("GetEffectiveKernelArgs()[%d] = %q, want %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}
