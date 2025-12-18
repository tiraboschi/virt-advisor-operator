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
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
	"github.com/kubevirt/virt-advisor-operator/internal/discovery"
)

// Profile defines the interface for a configuration profile.
// Each profile represents a named capability or feature set that can be applied to the cluster.
//
// IMPORTANT: When implementing GeneratePlanItems(), you MUST use PlanItemBuilder
// to construct VirtPlatformConfigItems. This ensures diffs are generated using
// Server-Side Apply (SSA) dry-run, which leverages API server validation, defaulting,
// CEL rules, and webhooks as required by the VEP.
//
// Example:
//
//	func (p *MyProfile) GeneratePlanItems(ctx context.Context, c client.Client, overrides map[string]string) ([]advisorv1alpha1.VirtPlatformConfigItem, error) {
//	    desired := plan.CreateUnstructured(MyGVK, "my-resource", "")
//	    // ... configure desired object ...
//
//	    item, err := NewPlanItemBuilder(ctx, c, "virt-advisor-operator").
//	        ForResource(desired, "my-item-name").
//	        WithManagedFields([]string{"spec.field1", "spec.field2"}).
//	        Build()
//	    if err != nil {
//	        return nil, err
//	    }
//	    return []advisorv1alpha1.VirtPlatformConfigItem{item}, nil
//	}
type Profile interface {
	// GetName returns the unique name of this profile.
	GetName() string

	// GetPrerequisites returns the list of CRDs required by this profile.
	// The controller will check these before generating the plan.
	GetPrerequisites() []discovery.Prerequisite

	// GetManagedResourceTypes returns the list of resource types (GVKs) that this profile manages.
	// The controller will automatically watch these resources for drift detection.
	// This enables a plugin mechanism where each profile declares what it needs to monitor.
	GetManagedResourceTypes() []schema.GroupVersionKind

	// Validate checks if the provided config overrides are valid for this profile.
	// Returns an error if any override is invalid or unsupported.
	Validate(configOverrides map[string]string) error

	// GeneratePlanItems creates the ordered list of configuration items to apply.
	// The items describe what resources will be modified and how.
	//
	// CRITICAL: You MUST use PlanItemBuilder to construct items, not manually.
	// See the Profile interface documentation for usage example.
	GeneratePlanItems(ctx context.Context, c client.Client, configOverrides map[string]string) ([]advisorv1alpha1.VirtPlatformConfigItem, error)

	// GetDescription returns a human-readable one-line description of this profile.
	// Used for profile advertising annotations.
	GetDescription() string

	// GetCategory returns the category this profile belongs to.
	// Examples: "scheduling", "performance", "storage", "networking"
	// Used for profile advertising labels.
	GetCategory() string

	// GetImpactSummary returns a summary of the impact of enabling this profile.
	// Examples: "Low - Configuration only", "Medium - May require node reboots"
	// Used for profile advertising annotations.
	GetImpactSummary() string

	// GetImpactLevel returns the aggregate risk level for kubectl output.
	// Used in VirtPlatformConfig status.impact field for kubectl get display.
	// Returns the enum value from advisorv1alpha1.Impact (Low, Medium, High).
	GetImpactLevel() advisorv1alpha1.Impact

	// IsAdvertisable returns true if this profile should be auto-created as a
	// VirtPlatformConfig with action=Ignore on operator startup for discoverability.
	// Returns false for example/dev/internal profiles.
	IsAdvertisable() bool
}

// Registry manages the collection of available profiles.
type Registry struct {
	profiles map[string]Profile
}

// NewRegistry creates a new profile registry.
func NewRegistry() *Registry {
	return &Registry{
		profiles: make(map[string]Profile),
	}
}

// Register adds a profile to the registry.
// Returns an error if a profile with the same name is already registered.
func (r *Registry) Register(p Profile) error {
	name := p.GetName()
	if _, exists := r.profiles[name]; exists {
		return fmt.Errorf("profile %q is already registered", name)
	}
	r.profiles[name] = p
	return nil
}

// Get retrieves a profile by name.
// Returns an error if the profile is not found.
func (r *Registry) Get(name string) (Profile, error) {
	p, exists := r.profiles[name]
	if !exists {
		return nil, fmt.Errorf("profile %q not found", name)
	}
	return p, nil
}

// List returns the names of all registered profiles.
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.profiles))
	for name := range r.profiles {
		names = append(names, name)
	}
	return names
}

// GetAllManagedResourceTypes returns the unique set of resource types (GVKs)
// that are managed by all registered profiles. This is used by the controller
// to dynamically register watches for drift detection.
func (r *Registry) GetAllManagedResourceTypes() []schema.GroupVersionKind {
	gvkSet := make(map[schema.GroupVersionKind]bool)

	// Collect all GVKs from all profiles
	for _, profile := range r.profiles {
		for _, gvk := range profile.GetManagedResourceTypes() {
			gvkSet[gvk] = true
		}
	}

	// Convert set to slice
	gvks := make([]schema.GroupVersionKind, 0, len(gvkSet))
	for gvk := range gvkSet {
		gvks = append(gvks, gvk)
	}

	return gvks
}

// DefaultRegistry is the global profile registry.
// Profiles should register themselves during init().
var DefaultRegistry = NewRegistry()
