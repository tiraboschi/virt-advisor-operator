package profiles

import (
	"context"
	"fmt"

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

	// Validate checks if the provided config overrides are valid for this profile.
	// Returns an error if any override is invalid or unsupported.
	Validate(configOverrides map[string]string) error

	// GeneratePlanItems creates the ordered list of configuration items to apply.
	// The items describe what resources will be modified and how.
	//
	// CRITICAL: You MUST use PlanItemBuilder to construct items, not manually.
	// See the Profile interface documentation for usage example.
	GeneratePlanItems(ctx context.Context, c client.Client, configOverrides map[string]string) ([]advisorv1alpha1.VirtPlatformConfigItem, error)
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

// DefaultRegistry is the global profile registry.
// Profiles should register themselves during init().
var DefaultRegistry = NewRegistry()
