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

	"github.com/kubevirt/virt-advisor-operator/internal/profiles/example"
	"github.com/kubevirt/virt-advisor-operator/internal/profiles/loadaware"
)

// TestProfileMetadata verifies that all registered profiles implement metadata methods correctly
func TestProfileMetadata(t *testing.T) {
	// Get all registered profiles
	profileNames := DefaultRegistry.List()

	if len(profileNames) == 0 {
		t.Fatal("No profiles registered in DefaultRegistry")
	}

	for _, name := range profileNames {
		profile, err := DefaultRegistry.Get(name)
		if err != nil {
			t.Errorf("Failed to get profile %s: %v", name, err)
			continue
		}

		t.Run(name, func(t *testing.T) {
			// Test GetDescription
			desc := profile.GetDescription()
			if desc == "" {
				t.Error("GetDescription() returned empty string")
			}
			if len(desc) > 200 {
				t.Errorf("GetDescription() returned very long string (%d chars), should be concise", len(desc))
			}

			// Test GetCategory
			category := profile.GetCategory()
			if category == "" {
				t.Error("GetCategory() returned empty string")
			}
			// Validate category is reasonable
			validCategories := map[string]bool{
				"scheduling":  true,
				"performance": true,
				"storage":     true,
				"networking":  true,
				"example":     true, // For example profiles
			}
			if !validCategories[category] {
				t.Logf("Warning: category %q is not in common categories list", category)
			}

			// Test GetImpactSummary
			impact := profile.GetImpactSummary()
			if impact == "" {
				t.Error("GetImpactSummary() returned empty string")
			}

			// Test IsAdvertisable - just verify it returns a boolean (both values are valid)
			advertisable := profile.IsAdvertisable()
			t.Logf("Profile %s is advertisable: %v", name, advertisable)
		})
	}
}

// TestLoadAwareProfileMetadata specifically tests the LoadAwareProfile metadata
func TestLoadAwareProfileMetadata(t *testing.T) {
	profile := loadaware.NewLoadAwareRebalancingProfile()

	// Verify specific expected values
	if profile.GetName() != loadaware.ProfileNameLoadAware {
		t.Errorf("Expected name %s, got %s", loadaware.ProfileNameLoadAware, profile.GetName())
	}

	if profile.GetCategory() != "scheduling" {
		t.Errorf("Expected category 'scheduling', got %s", profile.GetCategory())
	}

	if !profile.IsAdvertisable() {
		t.Error("LoadAwareProfile should be advertisable")
	}

	desc := profile.GetDescription()
	if desc == "" {
		t.Error("Description should not be empty")
	}

	impact := profile.GetImpactSummary()
	if impact == "" {
		t.Error("Impact summary should not be empty")
	}
}

// TestExampleProfileNotAdvertisable verifies the example profile is not advertisable
func TestExampleProfileNotAdvertisable(t *testing.T) {
	profile := example.NewExampleProfile()

	if profile.IsAdvertisable() {
		t.Error("ExampleProfile should NOT be advertisable")
	}

	if profile.GetCategory() != "example" {
		t.Errorf("Expected category 'example', got %s", profile.GetCategory())
	}
}

// TestRegistryAdvertisableProfiles verifies the registry can filter advertisable profiles
func TestRegistryAdvertisableProfiles(t *testing.T) {
	advertisableCount := 0
	nonAdvertisableCount := 0

	for _, name := range DefaultRegistry.List() {
		profile, err := DefaultRegistry.Get(name)
		if err != nil {
			t.Fatalf("Failed to get profile %s: %v", name, err)
		}

		if profile.IsAdvertisable() {
			advertisableCount++
		} else {
			nonAdvertisableCount++
		}
	}

	t.Logf("Found %d advertisable profiles and %d non-advertisable profiles",
		advertisableCount, nonAdvertisableCount)

	// We expect at least one advertisable profile (load-aware-rebalancing)
	if advertisableCount == 0 {
		t.Error("Expected at least one advertisable profile")
	}

	// We expect at least one non-advertisable profile (example-profile)
	if nonAdvertisableCount == 0 {
		t.Error("Expected at least one non-advertisable profile (example)")
	}
}
