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

package profiles

import (
	"testing"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
	"github.com/kubevirt/virt-advisor-operator/internal/profiles/loadaware"
)

// TestOptionsToMap tests the OptionsToMap conversion function
func TestOptionsToMap(t *testing.T) {
	tests := []struct {
		name        string
		profileName string
		options     *advisorv1alpha1.ProfileOptions
		expected    map[string]string
	}{
		{
			name:        "Nil options",
			profileName: loadaware.ProfileNameLoadAware,
			options:     nil,
			expected:    map[string]string{},
		},
		{
			name:        "Empty LoadAware options",
			profileName: loadaware.ProfileNameLoadAware,
			options: &advisorv1alpha1.ProfileOptions{
				LoadAware: nil,
			},
			expected: map[string]string{},
		},
		{
			name:        "LoadAware with all fields set",
			profileName: loadaware.ProfileNameLoadAware,
			options: &advisorv1alpha1.ProfileOptions{
				LoadAware: &advisorv1alpha1.LoadAwareConfig{
					DeschedulingIntervalSeconds: int32Ptr(120),
					EnablePSIMetrics:            boolPtr(true),
					DevDeviationThresholds:      stringPtr("High"),
				},
			},
			expected: map[string]string{
				"deschedulingIntervalSeconds": "120",
				"enablePSIMetrics":            "true",
				"devDeviationThresholds":      "High",
			},
		},
		{
			name:        "LoadAware with partial fields (interval only)",
			profileName: loadaware.ProfileNameLoadAware,
			options: &advisorv1alpha1.ProfileOptions{
				LoadAware: &advisorv1alpha1.LoadAwareConfig{
					DeschedulingIntervalSeconds: int32Ptr(60),
				},
			},
			expected: map[string]string{
				"deschedulingIntervalSeconds": "60",
			},
		},
		{
			name:        "LoadAware with PSI disabled",
			profileName: loadaware.ProfileNameLoadAware,
			options: &advisorv1alpha1.ProfileOptions{
				LoadAware: &advisorv1alpha1.LoadAwareConfig{
					EnablePSIMetrics: boolPtr(false),
				},
			},
			expected: map[string]string{
				"enablePSIMetrics": "false",
			},
		},
		{
			name:        "LoadAware with zero interval value",
			profileName: loadaware.ProfileNameLoadAware,
			options: &advisorv1alpha1.ProfileOptions{
				LoadAware: &advisorv1alpha1.LoadAwareConfig{
					DeschedulingIntervalSeconds: int32Ptr(0),
				},
			},
			expected: map[string]string{
				"deschedulingIntervalSeconds": "0",
			},
		},
		{
			name:        "Unknown profile name",
			profileName: "unknown-profile",
			options: &advisorv1alpha1.ProfileOptions{
				LoadAware: &advisorv1alpha1.LoadAwareConfig{
					DeschedulingIntervalSeconds: int32Ptr(120),
				},
			},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := OptionsToMap(tt.profileName, tt.options)

			// Check that result matches expected
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d entries, got %d. Expected: %v, Got: %v",
					len(tt.expected), len(result), tt.expected, result)
				return
			}

			for key, expectedValue := range tt.expected {
				if actualValue, ok := result[key]; !ok {
					t.Errorf("Expected key %q not found in result", key)
				} else if actualValue != expectedValue {
					t.Errorf("For key %q: expected %q, got %q", key, expectedValue, actualValue)
				}
			}
		})
	}
}

// TestGetIntConfig tests integer config extraction
func TestGetIntConfig(t *testing.T) {
	tests := []struct {
		name         string
		configMap    map[string]string
		key          string
		defaultValue int
		expected     int
	}{
		{
			name:         "Key exists with valid integer",
			configMap:    map[string]string{"interval": "120"},
			key:          "interval",
			defaultValue: 60,
			expected:     120,
		},
		{
			name:         "Key missing, use default",
			configMap:    map[string]string{},
			key:          "interval",
			defaultValue: 60,
			expected:     60,
		},
		{
			name:         "Key exists with invalid integer, use default",
			configMap:    map[string]string{"interval": "invalid"},
			key:          "interval",
			defaultValue: 60,
			expected:     60,
		},
		{
			name:         "Key exists with zero value",
			configMap:    map[string]string{"interval": "0"},
			key:          "interval",
			defaultValue: 60,
			expected:     0,
		},
		{
			name:         "Key exists with negative value",
			configMap:    map[string]string{"interval": "-100"},
			key:          "interval",
			defaultValue: 60,
			expected:     -100,
		},
		{
			name:         "Key exists with large value",
			configMap:    map[string]string{"interval": "86400"},
			key:          "interval",
			defaultValue: 60,
			expected:     86400,
		},
		{
			name:         "Empty config map",
			configMap:    map[string]string{},
			key:          "interval",
			defaultValue: 60,
			expected:     60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetIntConfig(tt.configMap, tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestGetBoolConfig tests boolean config extraction
func TestGetBoolConfig(t *testing.T) {
	tests := []struct {
		name         string
		configMap    map[string]string
		key          string
		defaultValue bool
		expected     bool
	}{
		{
			name:         "Key exists with 'true'",
			configMap:    map[string]string{"enabled": "true"},
			key:          "enabled",
			defaultValue: false,
			expected:     true,
		},
		{
			name:         "Key exists with 'false'",
			configMap:    map[string]string{"enabled": "false"},
			key:          "enabled",
			defaultValue: true,
			expected:     false,
		},
		{
			name:         "Key missing, use default true",
			configMap:    map[string]string{},
			key:          "enabled",
			defaultValue: true,
			expected:     true,
		},
		{
			name:         "Key missing, use default false",
			configMap:    map[string]string{},
			key:          "enabled",
			defaultValue: false,
			expected:     false,
		},
		{
			name:         "Key exists with 'True' (capitalized)",
			configMap:    map[string]string{"enabled": "True"},
			key:          "enabled",
			defaultValue: false,
			expected:     true,
		},
		{
			name:         "Key exists with '1' (truthy)",
			configMap:    map[string]string{"enabled": "1"},
			key:          "enabled",
			defaultValue: false,
			expected:     true,
		},
		{
			name:         "Key exists with 'yes' (truthy)",
			configMap:    map[string]string{"enabled": "yes"},
			key:          "enabled",
			defaultValue: false,
			expected:     true,
		},
		{
			name:         "Key exists with empty string (truthy, not 'false')",
			configMap:    map[string]string{"enabled": ""},
			key:          "enabled",
			defaultValue: false,
			expected:     true,
		},
		{
			name:         "Key exists with 'FALSE' (case matters)",
			configMap:    map[string]string{"enabled": "FALSE"},
			key:          "enabled",
			defaultValue: false,
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetBoolConfig(tt.configMap, tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestGetStringConfig tests string config extraction
func TestGetStringConfig(t *testing.T) {
	tests := []struct {
		name         string
		configMap    map[string]string
		key          string
		defaultValue string
		expected     string
	}{
		{
			name:         "Key exists",
			configMap:    map[string]string{"threshold": "High"},
			key:          "threshold",
			defaultValue: "Low",
			expected:     "High",
		},
		{
			name:         "Key missing, use default",
			configMap:    map[string]string{},
			key:          "threshold",
			defaultValue: "Low",
			expected:     "Low",
		},
		{
			name:         "Key exists with empty value",
			configMap:    map[string]string{"threshold": ""},
			key:          "threshold",
			defaultValue: "Low",
			expected:     "",
		},
		{
			name:         "Key exists with whitespace",
			configMap:    map[string]string{"threshold": "  High  "},
			key:          "threshold",
			defaultValue: "Low",
			expected:     "  High  ",
		},
		{
			name:         "Empty config map",
			configMap:    map[string]string{},
			key:          "threshold",
			defaultValue: "Medium",
			expected:     "Medium",
		},
		{
			name:         "Multiple keys, get specific one",
			configMap:    map[string]string{"threshold": "High", "mode": "Auto"},
			key:          "threshold",
			defaultValue: "Low",
			expected:     "High",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetStringConfig(tt.configMap, tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestValidateOptions tests the ValidateOptions function
func TestValidateOptions(t *testing.T) {
	tests := []struct {
		name        string
		profileName string
		options     *advisorv1alpha1.ProfileOptions
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Nil options (valid)",
			profileName: loadaware.ProfileNameLoadAware,
			options:     nil,
			expectError: false,
		},
		{
			name:        "LoadAware profile with LoadAware options",
			profileName: loadaware.ProfileNameLoadAware,
			options: &advisorv1alpha1.ProfileOptions{
				LoadAware: &advisorv1alpha1.LoadAwareConfig{
					DeschedulingIntervalSeconds: int32Ptr(120),
				},
			},
			expectError: false,
		},
		{
			name:        "LoadAware profile with nil LoadAware (valid, use defaults)",
			profileName: loadaware.ProfileNameLoadAware,
			options: &advisorv1alpha1.ProfileOptions{
				LoadAware: nil,
			},
			expectError: false,
		},
		{
			name:        "Unknown profile name",
			profileName: "unknown-profile",
			options: &advisorv1alpha1.ProfileOptions{
				LoadAware: &advisorv1alpha1.LoadAwareConfig{},
			},
			expectError: true,
			errorMsg:    "unknown profile",
		},
		{
			name:        "Empty profile name with options",
			profileName: "",
			options: &advisorv1alpha1.ProfileOptions{
				LoadAware: &advisorv1alpha1.LoadAwareConfig{},
			},
			expectError: true,
			errorMsg:    "unknown profile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOptions(tt.profileName, tt.options)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				if tt.errorMsg != "" && !stringContains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error to contain %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// Helper functions

func int32Ptr(i int32) *int32 {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

func stringPtr(s string) *string {
	return &s
}

func stringContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || stringContainsHelper(s, substr))
}

func stringContainsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
