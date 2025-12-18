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

package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
)

// Helper to validate VirtPlatformConfig using Kubernetes validation
func validateVirtPlatformConfig(vpc *VirtPlatformConfig) field.ErrorList {
	// In real Kubernetes, CEL validation happens server-side
	// For unit tests, we simulate the validation logic
	var allErrs field.ErrorList

	// Rule 1: Singleton pattern - name must match profile
	if vpc.Name != vpc.Spec.Profile {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("metadata").Child("name"),
			vpc.Name,
			"To ensure a singleton pattern, the VirtPlatformConfig name must exactly match the spec.profile.",
		))
	}

	// Rule 2: Profile must be in allowed list (unless bypass annotation is set)
	skipValidation := false
	if vpc.Annotations != nil {
		if val, ok := vpc.Annotations["advisor.kubevirt.io/skip-profile-validation"]; ok && val == "true" {
			skipValidation = true
		}
	}

	if !skipValidation {
		validProfiles := []string{"load-aware-rebalancing", "virt-higher-density", "example-profile"}
		valid := false
		for _, p := range validProfiles {
			if vpc.Spec.Profile == p {
				valid = true
				break
			}
		}
		if !valid {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec").Child("profile"),
				vpc.Spec.Profile,
				"spec.profile must be one of: load-aware-rebalancing, virt-higher-density, example-profile",
			))
		}
	}

	// Rule 3: When profile is load-aware-rebalancing, only options.loadAware may be set
	if vpc.Spec.Profile == "load-aware-rebalancing" && vpc.Spec.Options != nil {
		if vpc.Spec.Options.VirtHigherDensity != nil {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec").Child("options").Child("virtHigherDensity"),
				vpc.Spec.Options.VirtHigherDensity,
				"When profile is 'load-aware-rebalancing', only options.loadAware may be set",
			))
		}
	}

	// Rule 4: When profile is virt-higher-density, only options.virtHigherDensity may be set
	if vpc.Spec.Profile == "virt-higher-density" && vpc.Spec.Options != nil {
		if vpc.Spec.Options.LoadAware != nil {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec").Child("options").Child("loadAware"),
				vpc.Spec.Options.LoadAware,
				"When profile is 'virt-higher-density', only options.virtHigherDensity may be set",
			))
		}
	}

	// Rule 5: When options.loadAware is set, profile must be load-aware-rebalancing
	if vpc.Spec.Options != nil && vpc.Spec.Options.LoadAware != nil {
		if vpc.Spec.Profile != "load-aware-rebalancing" {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec").Child("options").Child("loadAware"),
				vpc.Spec.Options.LoadAware,
				"options.loadAware can only be used with profile 'load-aware-rebalancing'",
			))
		}
	}

	// Rule 6: When options.virtHigherDensity is set, profile must be virt-higher-density
	if vpc.Spec.Options != nil && vpc.Spec.Options.VirtHigherDensity != nil {
		if vpc.Spec.Profile != "virt-higher-density" {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec").Child("options").Child("virtHigherDensity"),
				vpc.Spec.Options.VirtHigherDensity,
				"options.virtHigherDensity can only be used with profile 'virt-higher-density'",
			))
		}
	}

	return allErrs
}

func TestVirtPlatformConfigValidation_ValidProfiles(t *testing.T) {
	tests := []struct {
		name string
		vpc  *VirtPlatformConfig
	}{
		{
			name: "valid load-aware-rebalancing profile",
			vpc: &VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "load-aware-rebalancing",
				},
				Spec: VirtPlatformConfigSpec{
					Profile: "load-aware-rebalancing",
					Action:  PlanActionDryRun,
				},
			},
		},
		{
			name: "valid virt-higher-density profile",
			vpc: &VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "virt-higher-density",
				},
				Spec: VirtPlatformConfigSpec{
					Profile: "virt-higher-density",
					Action:  PlanActionDryRun,
				},
			},
		},
		{
			name: "valid load-aware with options",
			vpc: &VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "load-aware-rebalancing",
				},
				Spec: VirtPlatformConfigSpec{
					Profile: "load-aware-rebalancing",
					Action:  PlanActionDryRun,
					Options: &ProfileOptions{
						LoadAware: &LoadAwareConfig{
							DeschedulingIntervalSeconds: ptr.To[int32](120),
						},
					},
				},
			},
		},
		{
			name: "valid virt-higher-density with options",
			vpc: &VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "virt-higher-density",
				},
				Spec: VirtPlatformConfigSpec{
					Profile: "virt-higher-density",
					Action:  PlanActionDryRun,
					Options: &ProfileOptions{
						VirtHigherDensity: &VirtHigherDensityConfig{
							EnableSwap: ptr.To(true),
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateVirtPlatformConfig(tt.vpc)
			if len(errs) > 0 {
				t.Errorf("Expected no validation errors, but got: %v", errs)
			}
		})
	}
}

func TestVirtPlatformConfigValidation_InvalidProfiles(t *testing.T) {
	tests := []struct {
		name        string
		vpc         *VirtPlatformConfig
		wantErrPath string
		wantErrMsg  string
	}{
		{
			name: "invalid profile name",
			vpc: &VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "unknown-profile",
				},
				Spec: VirtPlatformConfigSpec{
					Profile: "unknown-profile",
					Action:  PlanActionDryRun,
				},
			},
			wantErrPath: "spec.profile",
			wantErrMsg:  "spec.profile must be one of",
		},
		{
			name: "singleton pattern violation",
			vpc: &VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "wrong-name",
				},
				Spec: VirtPlatformConfigSpec{
					Profile: "load-aware-rebalancing",
					Action:  PlanActionDryRun,
				},
			},
			wantErrPath: "metadata.name",
			wantErrMsg:  "singleton pattern",
		},
		{
			name: "load-aware profile with virt-higher-density options",
			vpc: &VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "load-aware-rebalancing",
				},
				Spec: VirtPlatformConfigSpec{
					Profile: "load-aware-rebalancing",
					Action:  PlanActionDryRun,
					Options: &ProfileOptions{
						VirtHigherDensity: &VirtHigherDensityConfig{
							EnableSwap: ptr.To(true),
						},
					},
				},
			},
			wantErrPath: "spec.options.virtHigherDensity",
			wantErrMsg:  "", // Multiple rules can trigger - just check we get an error for this field
		},
		{
			name: "virt-higher-density profile with load-aware options",
			vpc: &VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "virt-higher-density",
				},
				Spec: VirtPlatformConfigSpec{
					Profile: "virt-higher-density",
					Action:  PlanActionDryRun,
					Options: &ProfileOptions{
						LoadAware: &LoadAwareConfig{
							DeschedulingIntervalSeconds: ptr.To[int32](120),
						},
					},
				},
			},
			wantErrPath: "spec.options.loadAware",
			wantErrMsg:  "", // Multiple rules can trigger - just check we get an error for this field
		},
		{
			name: "loadAware options with wrong profile",
			vpc: &VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "virt-higher-density",
				},
				Spec: VirtPlatformConfigSpec{
					Profile: "virt-higher-density",
					Action:  PlanActionDryRun,
					Options: &ProfileOptions{
						LoadAware: &LoadAwareConfig{
							DeschedulingIntervalSeconds: ptr.To[int32](120),
						},
					},
				},
			},
			wantErrPath: "spec.options.loadAware",
			wantErrMsg:  "", // Multiple rules can trigger - just check we get an error for this field
		},
		{
			name: "virtHigherDensity options with wrong profile",
			vpc: &VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "load-aware-rebalancing",
				},
				Spec: VirtPlatformConfigSpec{
					Profile: "load-aware-rebalancing",
					Action:  PlanActionDryRun,
					Options: &ProfileOptions{
						VirtHigherDensity: &VirtHigherDensityConfig{
							EnableSwap: ptr.To(true),
						},
					},
				},
			},
			wantErrPath: "spec.options.virtHigherDensity",
			wantErrMsg:  "", // Multiple rules can trigger - just check we get an error for this field
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateVirtPlatformConfig(tt.vpc)
			if len(errs) == 0 {
				t.Errorf("Expected validation errors, but got none")
				return
			}

			// Check that we got an error for the expected field path
			found := false
			for _, err := range errs {
				if err.Field == tt.wantErrPath {
					found = true
					if tt.wantErrMsg != "" && !contains(err.Error(), tt.wantErrMsg) {
						t.Errorf("Expected error message to contain %q, but got: %v", tt.wantErrMsg, err)
					}
				}
			}

			if !found {
				t.Errorf("Expected error for field %q, but got errors: %v", tt.wantErrPath, errs)
			}
		})
	}
}

func TestVirtPlatformConfigValidation_ExampleProfile(t *testing.T) {
	tests := []struct {
		name        string
		vpc         *VirtPlatformConfig
		shouldError bool
	}{
		{
			name: "example-profile is valid (for testing)",
			vpc: &VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "example-profile",
				},
				Spec: VirtPlatformConfigSpec{
					Profile: "example-profile",
					Action:  PlanActionDryRun,
				},
			},
			shouldError: false,
		},
		{
			name: "unknown profile is invalid",
			vpc: &VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "unknown-profile",
				},
				Spec: VirtPlatformConfigSpec{
					Profile: "unknown-profile",
					Action:  PlanActionDryRun,
				},
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateVirtPlatformConfig(tt.vpc)
			hasError := len(errs) > 0

			if hasError != tt.shouldError {
				if tt.shouldError {
					t.Errorf("Expected validation errors, but got none")
				} else {
					t.Errorf("Expected no validation errors, but got: %v", errs)
				}
			}
		})
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && hasSubstring(s, substr)))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
