package profiles

import (
	"testing"
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

	// Should require MachineConfig CRD
	if len(prereqs) == 0 {
		t.Fatal("GetPrerequisites() returned empty list, expected MachineConfig CRD")
	}

	foundMachineConfig := false
	for _, prereq := range prereqs {
		if prereq.GVK == MachineConfigGVK {
			foundMachineConfig = true
			if !contains(prereq.Description, "MachineConfig") {
				t.Errorf("MachineConfig prerequisite description = %q, should mention MachineConfig", prereq.Description)
			}
			if !contains(prereq.Description, "OpenShift") {
				t.Errorf("MachineConfig prerequisite description = %q, should mention OpenShift", prereq.Description)
			}
		}
	}

	if !foundMachineConfig {
		t.Error("GetPrerequisites() should include MachineConfig CRD requirement")
	}
}

func TestVirtHigherDensityProfile_GetManagedResourceTypes(t *testing.T) {
	profile := NewVirtHigherDensityProfile()

	managedTypes := profile.GetManagedResourceTypes()

	// Should manage MachineConfig
	if len(managedTypes) == 0 {
		t.Fatal("GetManagedResourceTypes() returned empty list, expected MachineConfig")
	}

	foundMachineConfig := false
	for _, gvk := range managedTypes {
		if gvk == MachineConfigGVK {
			foundMachineConfig = true
		}
	}

	if !foundMachineConfig {
		t.Error("GetManagedResourceTypes() should include MachineConfig GVK")
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

func TestVirtHigherDensityProfile_Constants(t *testing.T) {
	// Verify profile constants are correctly defined
	if ProfileNameVirtHigherDensity != "virt-higher-density" {
		t.Errorf("ProfileNameVirtHigherDensity = %q, want %q", ProfileNameVirtHigherDensity, "virt-higher-density")
	}
}

func TestVirtHigherDensityProfile_Registration(t *testing.T) {
	// Verify the profile is registered in the default registry
	profile, err := DefaultRegistry.Get(ProfileNameVirtHigherDensity)
	if err != nil {
		t.Fatalf("DefaultRegistry.Get(%q) error = %v", ProfileNameVirtHigherDensity, err)
	}

	if profile == nil {
		t.Fatal("DefaultRegistry.Get() returned nil profile")
	}

	if profile.GetName() != ProfileNameVirtHigherDensity {
		t.Errorf("Registered profile name = %q, want %q", profile.GetName(), ProfileNameVirtHigherDensity)
	}

	// Verify it's listed in available profiles
	profiles := DefaultRegistry.List()
	found := false
	for _, name := range profiles {
		if name == ProfileNameVirtHigherDensity {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Profile %q not found in registry list %v", ProfileNameVirtHigherDensity, profiles)
	}
}
