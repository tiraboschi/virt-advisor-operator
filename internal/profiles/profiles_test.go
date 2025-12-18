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
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
	"github.com/kubevirt/virt-advisor-operator/internal/discovery"
)

// mockProfile is a simple mock implementation for testing.
type mockProfile struct {
	name string
}

func (m *mockProfile) GetName() string {
	return m.name
}

func (m *mockProfile) GetPrerequisites() []discovery.Prerequisite {
	return []discovery.Prerequisite{}
}

func (m *mockProfile) GetManagedResourceTypes() []schema.GroupVersionKind {
	return []schema.GroupVersionKind{}
}

func (m *mockProfile) Validate(configOverrides map[string]string) error {
	return nil
}

func (m *mockProfile) GeneratePlanItems(ctx context.Context, c client.Client, configOverrides map[string]string) ([]advisorv1alpha1.VirtPlatformConfigItem, error) {
	return []advisorv1alpha1.VirtPlatformConfigItem{}, nil
}

func (m *mockProfile) GetDescription() string {
	return "Mock profile for testing"
}

func (m *mockProfile) GetCategory() string {
	return "test"
}

func (m *mockProfile) GetImpactSummary() string {
	return "None - Testing only"
}

func (m *mockProfile) GetImpactLevel() advisorv1alpha1.Impact {
	return advisorv1alpha1.ImpactLow
}

func (m *mockProfile) IsAdvertisable() bool {
	return false
}

func TestRegistry_Register(t *testing.T) {
	tests := []struct {
		name        string
		profiles    []Profile
		wantErr     bool
		errContains string
	}{
		{
			name: "register single profile",
			profiles: []Profile{
				&mockProfile{name: "test-profile"},
			},
			wantErr: false,
		},
		{
			name: "register multiple profiles",
			profiles: []Profile{
				&mockProfile{name: "profile-1"},
				&mockProfile{name: "profile-2"},
			},
			wantErr: false,
		},
		{
			name: "register duplicate profile",
			profiles: []Profile{
				&mockProfile{name: "duplicate"},
				&mockProfile{name: "duplicate"},
			},
			wantErr:     true,
			errContains: "already registered",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()

			var err error
			for _, p := range tt.profiles {
				err = registry.Register(p)
				if err != nil {
					break
				}
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("Register() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				if tt.errContains != "" {
					if !contains(err.Error(), tt.errContains) {
						t.Errorf("Register() error = %v, should contain %q", err, tt.errContains)
					}
				}
			}
		})
	}
}

// validateGetError checks that an error matches expectations
func validateGetError(t *testing.T, err error, errContains string) {
	t.Helper()
	if errContains != "" && !contains(err.Error(), errContains) {
		t.Errorf("Get() error = %v, should contain %q", err, errContains)
	}
}

// validateGetSuccess checks that a successful Get returns the expected profile
func validateGetSuccess(t *testing.T, got Profile, expectedName string) {
	t.Helper()
	if got == nil {
		t.Error("Get() returned nil profile but expected non-nil")
		return
	}
	if got.GetName() != expectedName {
		t.Errorf("Get() returned profile %q, expected %q", got.GetName(), expectedName)
	}
}

func TestRegistry_Get(t *testing.T) {
	tests := []struct {
		name        string
		register    []Profile
		getProfile  string
		wantErr     bool
		errContains string
	}{
		{
			name: "get existing profile",
			register: []Profile{
				&mockProfile{name: "test-profile"},
			},
			getProfile: "test-profile",
			wantErr:    false,
		},
		{
			name: "get non-existent profile",
			register: []Profile{
				&mockProfile{name: "other-profile"},
			},
			getProfile:  "missing-profile",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()

			for _, p := range tt.register {
				if err := registry.Register(p); err != nil {
					t.Fatalf("failed to register profile: %v", err)
				}
			}

			got, err := registry.Get(tt.getProfile)

			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				validateGetError(t, err, tt.errContains)
				return
			}

			if !tt.wantErr {
				validateGetSuccess(t, got, tt.getProfile)
			}
		})
	}
}

func TestRegistry_List(t *testing.T) {
	tests := []struct {
		name         string
		register     []Profile
		wantProfiles []string
	}{
		{
			name:         "empty registry",
			register:     []Profile{},
			wantProfiles: []string{},
		},
		{
			name: "single profile",
			register: []Profile{
				&mockProfile{name: "profile-1"},
			},
			wantProfiles: []string{"profile-1"},
		},
		{
			name: "multiple profiles",
			register: []Profile{
				&mockProfile{name: "profile-1"},
				&mockProfile{name: "profile-2"},
				&mockProfile{name: "profile-3"},
			},
			wantProfiles: []string{"profile-1", "profile-2", "profile-3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()

			for _, p := range tt.register {
				if err := registry.Register(p); err != nil {
					t.Fatalf("failed to register profile: %v", err)
				}
			}

			got := registry.List()

			if len(got) != len(tt.wantProfiles) {
				t.Errorf("List() returned %d profiles, expected %d", len(got), len(tt.wantProfiles))
				return
			}

			// Convert to map for easier checking (order doesn't matter)
			gotMap := make(map[string]bool)
			for _, name := range got {
				gotMap[name] = true
			}

			for _, want := range tt.wantProfiles {
				if !gotMap[want] {
					t.Errorf("List() missing expected profile %q", want)
				}
			}
		})
	}
}

func TestExampleProfile_GetName(t *testing.T) {
	profile := NewExampleProfile()
	if profile.GetName() != "example-profile" {
		t.Errorf("GetName() = %q, expected %q", profile.GetName(), "example-profile")
	}
}

func TestExampleProfile_Validate(t *testing.T) {
	tests := []struct {
		name            string
		configOverrides map[string]string
		wantErr         bool
		errContains     string
	}{
		{
			name:            "valid overrides",
			configOverrides: map[string]string{"replicas": "3", "enabled": "true"},
			wantErr:         false,
		},
		{
			name:            "empty overrides",
			configOverrides: map[string]string{},
			wantErr:         false,
		},
		{
			name:            "invalid override key",
			configOverrides: map[string]string{"invalid-key": "value"},
			wantErr:         true,
			errContains:     "unsupported config override",
		},
	}

	profile := NewExampleProfile()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := profile.Validate(tt.configOverrides)

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				if tt.errContains != "" {
					if !contains(err.Error(), tt.errContains) {
						t.Errorf("Validate() error = %v, should contain %q", err, tt.errContains)
					}
				}
			}
		})
	}
}

func TestExampleProfile_GeneratePlanItems(t *testing.T) {
	profile := NewExampleProfile()

	items, err := profile.GeneratePlanItems(context.TODO(), nil, nil)
	if err != nil {
		t.Fatalf("GeneratePlanItems() error = %v", err)
	}

	if len(items) == 0 {
		t.Error("GeneratePlanItems() returned no items, expected at least one")
	}

	if len(items) > 0 {
		item := items[0]
		if item.Name == "" {
			t.Error("Plan item has empty name")
		}
		if item.TargetRef.Kind == "" {
			t.Error("Plan item has empty target kind")
		}
		if item.State != advisorv1alpha1.ItemStatePending {
			t.Errorf("Plan item state = %v, expected Pending", item.State)
		}
	}
}

func TestDefaultRegistry(t *testing.T) {
	// The example profile should be registered in init()
	profile, err := DefaultRegistry.Get("example-profile")
	if err != nil {
		t.Fatalf("DefaultRegistry.Get() error = %v", err)
	}

	if profile == nil {
		t.Fatal("DefaultRegistry.Get() returned nil profile")
	}

	if profile.GetName() != "example-profile" {
		t.Errorf("Profile name = %q, expected %q", profile.GetName(), "example-profile")
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
