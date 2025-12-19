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

package higherdensity

import (
	"testing"

	"github.com/kubevirt/virt-advisor-operator/internal/profiles/profileutils"
)

func TestVirtHigherDensityProfile_GetName(t *testing.T) {
	profile := NewVirtHigherDensityProfile()

	if got := profile.GetName(); got != ProfileNameVirtHigherDensity {
		t.Errorf("GetName() = %q, want %q", got, ProfileNameVirtHigherDensity)
	}

	if got := profile.GetName(); got != "virt-higher-density" {
		t.Errorf("GetName() = %q, want %q", got, "virt-higher-density")
	}
}

func TestVirtHigherDensityProfile_GetDescription(t *testing.T) {
	profile := NewVirtHigherDensityProfile()

	description := profile.GetDescription()
	if description == "" {
		t.Error("GetDescription() returned empty string")
	}

	// Description should mention higher VM density and swap
	if !contains(description, "higher") && !contains(description, "density") {
		t.Errorf("GetDescription() = %q, should mention higher density", description)
	}
}

func TestVirtHigherDensityProfile_GetCategory(t *testing.T) {
	profile := NewVirtHigherDensityProfile()

	if got := profile.GetCategory(); got != "density" {
		t.Errorf("GetCategory() = %q, want %q", got, "density")
	}
}

func TestVirtHigherDensityProfile_GetImpactSummary(t *testing.T) {
	profile := NewVirtHigherDensityProfile()

	impact := profile.GetImpactSummary()
	if impact == "" {
		t.Error("GetImpactSummary() returned empty string")
	}

	// Impact should mention swap and CNV_SWAP partition
	if !contains(impact, "swap") {
		t.Errorf("GetImpactSummary() = %q, should mention swap", impact)
	}

	if !contains(impact, "CNV_SWAP") {
		t.Errorf("GetImpactSummary() = %q, should mention CNV_SWAP partition requirement", impact)
	}
}

func TestVirtHigherDensityProfile_IsAdvertisable(t *testing.T) {
	profile := NewVirtHigherDensityProfile()

	if !profile.IsAdvertisable() {
		t.Error("IsAdvertisable() = false, want true - virt-higher-density should be advertisable")
	}
}

func TestVirtHigherDensityProfile_GetPrerequisites(t *testing.T) {
	profile := NewVirtHigherDensityProfile()

	prereqs := profile.GetPrerequisites()

	// Should require both MachineConfig and HyperConverged CRDs
	if len(prereqs) < 2 {
		t.Fatalf("GetPrerequisites() returned %d items, expected at least 2 (MachineConfig and HyperConverged)", len(prereqs))
	}

	foundMachineConfig := false
	foundHyperConverged := false
	for _, prereq := range prereqs {
		if prereq.GVK == profileutils.MachineConfigGVK {
			foundMachineConfig = true
			if !contains(prereq.Description, "MachineConfig") {
				t.Errorf("MachineConfig prerequisite description = %q, should mention MachineConfig", prereq.Description)
			}
			if !contains(prereq.Description, "OpenShift") {
				t.Errorf("MachineConfig prerequisite description = %q, should mention OpenShift", prereq.Description)
			}
		}
		if prereq.GVK == profileutils.HyperConvergedGVK {
			foundHyperConverged = true
			if prereq.Description == "" {
				t.Error("HyperConverged prerequisite should have a description")
			}
		}
	}

	if !foundMachineConfig {
		t.Error("GetPrerequisites() should include MachineConfig CRD requirement")
	}
	if !foundHyperConverged {
		t.Error("GetPrerequisites() should include HyperConverged CRD requirement")
	}
}

func TestVirtHigherDensityProfile_GetManagedResourceTypes(t *testing.T) {
	profile := NewVirtHigherDensityProfile()

	managedTypes := profile.GetManagedResourceTypes()

	// Should manage both MachineConfig and HyperConverged
	if len(managedTypes) < 2 {
		t.Fatalf("GetManagedResourceTypes() returned %d items, expected at least 2 (MachineConfig and HyperConverged)", len(managedTypes))
	}

	foundMachineConfig := false
	foundHyperConverged := false
	for _, gvk := range managedTypes {
		if gvk == profileutils.MachineConfigGVK {
			foundMachineConfig = true
		}
		if gvk == profileutils.HyperConvergedGVK {
			foundHyperConverged = true
		}
	}

	if !foundMachineConfig {
		t.Error("GetManagedResourceTypes() should include MachineConfig GVK")
	}
	if !foundHyperConverged {
		t.Error("GetManagedResourceTypes() should include HyperConverged GVK for drift detection")
	}
}

func TestVirtHigherDensityProfile_Validate(t *testing.T) {
	profile := NewVirtHigherDensityProfile()

	tests := []struct {
		name            string
		configOverrides map[string]string
		wantErr         bool
	}{
		{
			name:            "empty overrides",
			configOverrides: map[string]string{},
			wantErr:         false,
		},
		{
			name: "enableSwap true",
			configOverrides: map[string]string{
				"enableSwap": "true",
			},
			wantErr: false,
		},
		{
			name: "enableSwap false",
			configOverrides: map[string]string{
				"enableSwap": "false",
			},
			wantErr: false,
		},
		{
			name: "unknown key is accepted (no validation on string map)",
			configOverrides: map[string]string{
				"unknownKey": "value",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := profile.Validate(tt.configOverrides)

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVirtHigherDensityProfile_ConfigDefaults(t *testing.T) {
	// Test that default values are correctly applied for HCO configuration
	tests := []struct {
		name            string
		configOverrides map[string]string
		wantKSMEnabled  bool
		wantMemoryRatio int
	}{
		{
			name:            "empty config uses defaults",
			configOverrides: map[string]string{},
			wantKSMEnabled:  true,
			wantMemoryRatio: defaultMemoryToRequestRatio,
		},
		{
			name: "explicit KSM enabled",
			configOverrides: map[string]string{
				"ksmEnabled": "true",
			},
			wantKSMEnabled:  true,
			wantMemoryRatio: defaultMemoryToRequestRatio,
		},
		{
			name: "explicit KSM disabled",
			configOverrides: map[string]string{
				"ksmEnabled": "false",
			},
			wantKSMEnabled:  false,
			wantMemoryRatio: defaultMemoryToRequestRatio,
		},
		{
			name: "custom memory ratio",
			configOverrides: map[string]string{
				"memoryToRequestRatio": "200",
			},
			wantKSMEnabled:  true,
			wantMemoryRatio: 200,
		},
		{
			name: "all options specified",
			configOverrides: map[string]string{
				"ksmEnabled":           "false",
				"memoryToRequestRatio": "120",
			},
			wantKSMEnabled:  false,
			wantMemoryRatio: 120,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test KSM config extraction
			ksmEnabled := profileutils.GetBoolConfig(tt.configOverrides, "ksmEnabled", true)
			if ksmEnabled != tt.wantKSMEnabled {
				t.Errorf("KSM enabled = %v, want %v", ksmEnabled, tt.wantKSMEnabled)
			}

			// Test memory ratio extraction
			memoryRatio := profileutils.GetIntConfig(tt.configOverrides, "memoryToRequestRatio", defaultMemoryToRequestRatio)
			if memoryRatio != tt.wantMemoryRatio {
				t.Errorf("Memory ratio = %d, want %d", memoryRatio, tt.wantMemoryRatio)
			}
		})
	}
}

func TestVirtHigherDensityProfile_Constants(t *testing.T) {
	// Verify profile constants are correctly defined
	if ProfileNameVirtHigherDensity != "virt-higher-density" {
		t.Errorf("ProfileNameVirtHigherDensity = %q, want %q", ProfileNameVirtHigherDensity, "virt-higher-density")
	}

	// Verify default memory ratio constant
	if defaultMemoryToRequestRatio != 150 {
		t.Errorf("defaultMemoryToRequestRatio = %d, want 150", defaultMemoryToRequestRatio)
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
