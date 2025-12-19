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

package loadaware

import (
	"testing"

	"github.com/kubevirt/virt-advisor-operator/internal/profiles/profileutils"
)

func TestLoadAwareProfile_GetName(t *testing.T) {
	profile := NewLoadAwareRebalancingProfile()

	if got := profile.GetName(); got != ProfileNameLoadAware {
		t.Errorf("GetName() = %q, want %q", got, ProfileNameLoadAware)
	}

	if got := profile.GetName(); got != "load-aware-rebalancing" {
		t.Errorf("GetName() = %q, want %q", got, "load-aware-rebalancing")
	}
}

func TestLoadAwareProfile_Validate(t *testing.T) {
	profile := NewLoadAwareRebalancingProfile()

	tests := []struct {
		name            string
		configOverrides map[string]string
		wantErr         bool
		errContains     string
	}{
		// Valid cases
		{
			name:            "empty overrides",
			configOverrides: map[string]string{},
			wantErr:         false,
		},
		{
			name: "deschedulingIntervalSeconds only",
			configOverrides: map[string]string{
				"deschedulingIntervalSeconds": "3600",
			},
			wantErr: false,
		},
		{
			name: "enablePSIMetrics only",
			configOverrides: map[string]string{
				"enablePSIMetrics": "true",
			},
			wantErr: false,
		},
		{
			name: "both valid keys",
			configOverrides: map[string]string{
				"deschedulingIntervalSeconds": "7200",
				"enablePSIMetrics":            "false",
			},
			wantErr: false,
		},
		{
			name: "valid with numeric string for interval",
			configOverrides: map[string]string{
				"deschedulingIntervalSeconds": "900",
			},
			wantErr: false,
		},
		{
			name: "valid with psi enabled string variations",
			configOverrides: map[string]string{
				"enablePSIMetrics": "True",
			},
			wantErr: false,
		},
		{
			name: "valid with devDeviationThresholds",
			configOverrides: map[string]string{
				"devDeviationThresholds": "High",
			},
			wantErr: false,
		},
		{
			name: "valid with all supported keys",
			configOverrides: map[string]string{
				"deschedulingIntervalSeconds": "3600",
				"enablePSIMetrics":            "true",
				"devDeviationThresholds":      "Medium",
			},
			wantErr: false,
		},

		// Invalid cases
		{
			name: "unknown key",
			configOverrides: map[string]string{
				"unknownKey": "value",
			},
			wantErr:     true,
			errContains: "unsupported config override",
		},
		{
			name: "invalid key with valid key",
			configOverrides: map[string]string{
				"deschedulingIntervalSeconds": "1800",
				"invalidKey":                  "value",
			},
			wantErr:     true,
			errContains: "unsupported config override",
		},
		{
			name: "typo in key name",
			configOverrides: map[string]string{
				"deschedulingInterval": "1800", // Missing 'Seconds'
			},
			wantErr:     true,
			errContains: "unsupported config override",
		},
		{
			name: "case sensitive key check",
			configOverrides: map[string]string{
				"DeschedulingIntervalSeconds": "1800", // Wrong case
			},
			wantErr:     true,
			errContains: "unsupported config override",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := profile.Validate(tt.configOverrides)

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil && tt.errContains != "" {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("Validate() error = %v, should contain %q", err, tt.errContains)
				}
			}
		})
	}
}

func TestLoadAwareProfile_Constants(t *testing.T) {
	// Verify profile constants are correctly defined
	if ProfileNameLoadAware != "load-aware-rebalancing" {
		t.Errorf("ProfileNameLoadAware = %q, want %q", ProfileNameLoadAware, "load-aware-rebalancing")
	}

	if defaultDeschedulingInterval != 60 {
		t.Errorf("defaultDeschedulingInterval = %d, want %d", defaultDeschedulingInterval, 60)
	}

	if psiKernelArg != "psi=1" {
		t.Errorf("psiKernelArg = %q, want %q", psiKernelArg, "psi=1")
	}

	if kubeDeschedulerCRD != "kubedeschedulers.operator.openshift.io" {
		t.Errorf("kubeDeschedulerCRD = %q, want %q", kubeDeschedulerCRD, "kubedeschedulers.operator.openshift.io")
	}

	if machineConfigCRD != "machineconfigs.machineconfiguration.openshift.io" {
		t.Errorf("machineConfigCRD = %q, want %q", machineConfigCRD, "machineconfigs.machineconfiguration.openshift.io")
	}

	if kubeDeschedulerNamespace != "openshift-kube-descheduler-operator" {
		t.Errorf("kubeDeschedulerNamespace = %q, want %q", kubeDeschedulerNamespace, "openshift-kube-descheduler-operator")
	}

	if kubeDeschedulerName != "cluster" {
		t.Errorf("kubeDeschedulerName = %q, want %q", kubeDeschedulerName, "cluster")
	}

	// Verify descheduler profile name constants
	if profileKubeVirtRelieveAndMigrate != "KubeVirtRelieveAndMigrate" {
		t.Errorf("profileKubeVirtRelieveAndMigrate = %q, want %q", profileKubeVirtRelieveAndMigrate, "KubeVirtRelieveAndMigrate")
	}

	if profileDevKubeVirtRelieveAndMigrate != "DevKubeVirtRelieveAndMigrate" {
		t.Errorf("profileDevKubeVirtRelieveAndMigrate = %q, want %q", profileDevKubeVirtRelieveAndMigrate, "DevKubeVirtRelieveAndMigrate")
	}

	if profileLongLifecycle != "LongLifecycle" {
		t.Errorf("profileLongLifecycle = %q, want %q", profileLongLifecycle, "LongLifecycle")
	}

	if defaultDeviationThresholds != "AsymmetricLow" {
		t.Errorf("defaultDeviationThresholds = %q, want %q", defaultDeviationThresholds, "AsymmetricLow")
	}
}

func TestLoadAwareProfile_GVKDefinitions(t *testing.T) {
	// Verify GroupVersionKind definitions
	if KubeDeschedulerGVK.Group != "operator.openshift.io" {
		t.Errorf("KubeDeschedulerGVK.Group = %q, want %q", KubeDeschedulerGVK.Group, "operator.openshift.io")
	}
	if KubeDeschedulerGVK.Version != "v1" {
		t.Errorf("KubeDeschedulerGVK.Version = %q, want %q", KubeDeschedulerGVK.Version, "v1")
	}
	if KubeDeschedulerGVK.Kind != "KubeDescheduler" {
		t.Errorf("KubeDeschedulerGVK.Kind = %q, want %q", KubeDeschedulerGVK.Kind, "KubeDescheduler")
	}

	if profileutils.MachineConfigGVK.Group != "machineconfiguration.openshift.io" {
		t.Errorf("MachineConfigGVK.Group = %q, want %q", profileutils.MachineConfigGVK.Group, "machineconfiguration.openshift.io")
	}
	if profileutils.MachineConfigGVK.Version != "v1" {
		t.Errorf("MachineConfigGVK.Version = %q, want %q", profileutils.MachineConfigGVK.Version, "v1")
	}
	if profileutils.MachineConfigGVK.Kind != "MachineConfig" {
		t.Errorf("MachineConfigGVK.Kind = %q, want %q", profileutils.MachineConfigGVK.Kind, "MachineConfig")
	}
}

func TestComputeScaledEvictionLimit(t *testing.T) {
	tests := []struct {
		name     string
		input    int32
		expected int32
	}{
		{
			name:     "value less than 10 - use as is (1)",
			input:    1,
			expected: 1,
		},
		{
			name:     "value less than 10 - use as is (5)",
			input:    5,
			expected: 5,
		},
		{
			name:     "value less than 10 - use as is (9)",
			input:    9,
			expected: 9,
		},
		{
			name:     "value exactly 10 - continuous at boundary",
			input:    10,
			expected: 10, // 10 + (10-10)*0.8 = 10
		},
		{
			name:     "value 15 - monotonic scaling",
			input:    15,
			expected: 14, // 10 + (15-10)*0.8 = 10 + 4 = 14
		},
		{
			name:     "value 20 - monotonic scaling",
			input:    20,
			expected: 18, // 10 + (20-10)*0.8 = 10 + 8 = 18
		},
		{
			name:     "value 25 - monotonic scaling",
			input:    25,
			expected: 22, // 10 + (25-10)*0.8 = 10 + 12 = 22
		},
		{
			name:     "value 50 - monotonic scaling",
			input:    50,
			expected: 42, // 10 + (50-10)*0.8 = 10 + 32 = 42
		},
		{
			name:     "value 100 - monotonic scaling",
			input:    100,
			expected: 82, // 10 + (100-10)*0.8 = 10 + 72 = 82
		},
		{
			name:     "value 11 - monotonic scaling with integer rounding",
			input:    11,
			expected: 10, // 10 + ((11-10)*4)/5 = 10 + 0 = 10 (integer division)
		},
		{
			name:     "value 13 - monotonic scaling with integer rounding",
			input:    13,
			expected: 12, // 10 + ((13-10)*4)/5 = 10 + 2 = 12 (integer division)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeScaledEvictionLimit(tt.input)
			if result != tt.expected {
				t.Errorf("computeScaledEvictionLimit(%d) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLoadAwareProfile_EvictionLimitDefaults(t *testing.T) {
	// Verify eviction limit constants are correctly defined
	// HCO defaults: parallelMigrationsPerCluster=5 -> evictionLimits.total
	//               parallelOutboundMigrationsPerNode=2 -> evictionLimits.node
	if defaultEvictionLimitsTotal != 5 {
		t.Errorf("defaultEvictionLimitsTotal = %d, want %d", defaultEvictionLimitsTotal, 5)
	}

	if defaultEvictionLimitsNode != 2 {
		t.Errorf("defaultEvictionLimitsNode = %d, want %d", defaultEvictionLimitsNode, 2)
	}
}

func TestLoadAwareProfile_HCOConstants(t *testing.T) {
	// Verify HyperConverged resource constants (now in profileutils)
	if profileutils.HyperConvergedNamespace != "openshift-cnv" {
		t.Errorf("HyperConvergedNamespace = %q, want %q", profileutils.HyperConvergedNamespace, "openshift-cnv")
	}

	if profileutils.HyperConvergedName != "kubevirt-hyperconverged" {
		t.Errorf("HyperConvergedName = %q, want %q", profileutils.HyperConvergedName, "kubevirt-hyperconverged")
	}

	if hyperConvergedCRD != "hyperconvergeds.hco.kubevirt.io" {
		t.Errorf("hyperConvergedCRD = %q, want %q", hyperConvergedCRD, "hyperconvergeds.hco.kubevirt.io")
	}

	// Verify HyperConvergedGVK (now in profileutils)
	if profileutils.HyperConvergedGVK.Group != "hco.kubevirt.io" {
		t.Errorf("HyperConvergedGVK.Group = %q, want %q", profileutils.HyperConvergedGVK.Group, "hco.kubevirt.io")
	}
	if profileutils.HyperConvergedGVK.Version != "v1beta1" {
		t.Errorf("HyperConvergedGVK.Version = %q, want %q", profileutils.HyperConvergedGVK.Version, "v1beta1")
	}
	if profileutils.HyperConvergedGVK.Kind != "HyperConverged" {
		t.Errorf("HyperConvergedGVK.Kind = %q, want %q", profileutils.HyperConvergedGVK.Kind, "HyperConverged")
	}
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
