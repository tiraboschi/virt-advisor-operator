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

package discovery

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TestToLower tests the toLower utility function
func TestToLower(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "All uppercase",
			input:    "KUBEDESCHEDULER",
			expected: "kubedescheduler",
		},
		{
			name:     "Mixed case",
			input:    "KubeDescheduler",
			expected: "kubedescheduler",
		},
		{
			name:     "All lowercase",
			input:    "kubedescheduler",
			expected: "kubedescheduler",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "With numbers",
			input:    "Config123",
			expected: "config123",
		},
		{
			name:     "CamelCase",
			input:    "MachineConfigPool",
			expected: "machineconfigpool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toLower(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestGVKToCRDName tests the GVKToCRDName function
func TestGVKToCRDName(t *testing.T) {
	tests := []struct {
		name     string
		gvk      schema.GroupVersionKind
		expected string
	}{
		{
			name: "KubeDescheduler",
			gvk: schema.GroupVersionKind{
				Group:   "operator.openshift.io",
				Version: "v1",
				Kind:    "KubeDescheduler",
			},
			expected: "kubedeschedulers.operator.openshift.io",
		},
		{
			name: "MachineConfig",
			gvk: schema.GroupVersionKind{
				Group:   "machineconfiguration.openshift.io",
				Version: "v1",
				Kind:    "MachineConfig",
			},
			expected: "machineconfigs.machineconfiguration.openshift.io",
		},
		{
			name: "MachineConfigPool",
			gvk: schema.GroupVersionKind{
				Group:   "machineconfiguration.openshift.io",
				Version: "v1",
				Kind:    "MachineConfigPool",
			},
			expected: "machineconfigpools.machineconfiguration.openshift.io",
		},
		{
			name: "Generic resource (simple pluralization)",
			gvk: schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
			},
			expected: "deployments.apps",
		},
		{
			name: "Core API resource (empty group)",
			gvk: schema.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "ConfigMap",
			},
			expected: "configmaps.",
		},
		{
			name: "Custom resource",
			gvk: schema.GroupVersionKind{
				Group:   "advisor.kubevirt.io",
				Version: "v1alpha1",
				Kind:    "VirtPlatformConfig",
			},
			expected: "virtplatformconfigs.advisor.kubevirt.io",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GVKToCRDName(tt.gvk)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestFormatMissingPrerequisites tests the FormatMissingPrerequisites function
func TestFormatMissingPrerequisites(t *testing.T) {
	tests := []struct {
		name             string
		missing          []Prerequisite
		expectedContains []string
	}{
		{
			name:             "Empty list",
			missing:          []Prerequisite{},
			expectedContains: []string{},
		},
		{
			name: "Single missing prerequisite",
			missing: []Prerequisite{
				{
					GVK: schema.GroupVersionKind{
						Group:   "operator.openshift.io",
						Version: "v1",
						Kind:    "KubeDescheduler",
					},
					Description: "OpenShift KubeDescheduler Operator",
				},
			},
			expectedContains: []string{
				"Missing required dependencies:",
				"KubeDescheduler",
				"operator.openshift.io.v1",
				"OpenShift KubeDescheduler Operator",
			},
		},
		{
			name: "Multiple missing prerequisites",
			missing: []Prerequisite{
				{
					GVK: schema.GroupVersionKind{
						Group:   "operator.openshift.io",
						Version: "v1",
						Kind:    "KubeDescheduler",
					},
					Description: "OpenShift KubeDescheduler Operator",
				},
				{
					GVK: schema.GroupVersionKind{
						Group:   "machineconfiguration.openshift.io",
						Version: "v1",
						Kind:    "MachineConfig",
					},
					Description: "OpenShift Machine Config Operator",
				},
			},
			expectedContains: []string{
				"Missing required dependencies:",
				"KubeDescheduler",
				"operator.openshift.io.v1",
				"OpenShift KubeDescheduler Operator",
				"MachineConfig",
				"machineconfiguration.openshift.io.v1",
				"OpenShift Machine Config Operator",
			},
		},
		{
			name: "Prerequisite with empty description",
			missing: []Prerequisite{
				{
					GVK: schema.GroupVersionKind{
						Group:   "custom.io",
						Version: "v1",
						Kind:    "CustomResource",
					},
					Description: "",
				},
			},
			expectedContains: []string{
				"Missing required dependencies:",
				"CustomResource",
				"custom.io.v1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatMissingPrerequisites(tt.missing)

			if len(tt.missing) == 0 {
				if result != "" {
					t.Errorf("Expected empty string for empty list, got %q", result)
				}
				return
			}

			for _, expected := range tt.expectedContains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected result to contain %q, but it didn't.\nResult: %s", expected, result)
				}
			}
		})
	}
}

// TestFormatMissingPrerequisitesStructure tests the structure of the formatted message
func TestFormatMissingPrerequisitesStructure(t *testing.T) {
	missing := []Prerequisite{
		{
			GVK: schema.GroupVersionKind{
				Group:   "operator.openshift.io",
				Version: "v1",
				Kind:    "KubeDescheduler",
			},
			Description: "OpenShift KubeDescheduler Operator",
		},
		{
			GVK: schema.GroupVersionKind{
				Group:   "machineconfiguration.openshift.io",
				Version: "v1",
				Kind:    "MachineConfig",
			},
			Description: "OpenShift Machine Config Operator",
		},
	}

	result := FormatMissingPrerequisites(missing)

	// Check that the message has a header
	if !strings.HasPrefix(result, "Missing required dependencies:") {
		t.Errorf("Expected message to start with header, got: %s", result)
	}

	// Check that each prerequisite has its own line
	lines := strings.Split(result, "\n")
	if len(lines) < 3 {
		t.Errorf("Expected at least 3 lines (header + 2 prereqs), got %d lines", len(lines))
	}

	// Check that each prerequisite line starts with "  - "
	for i := 1; i < len(lines)-1; i++ { // Skip header and last empty line
		if !strings.HasPrefix(lines[i], "  - ") {
			t.Errorf("Expected line %d to start with '  - ', got: %q", i, lines[i])
		}
	}
}
