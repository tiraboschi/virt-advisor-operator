package profiles

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	hcov1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
)

// ExampleProfile is a simple demonstration profile.
// In a real implementation, this would configure actual cluster resources.
type ExampleProfile struct {
	name string
}

// NewExampleProfile creates a new example profile.
func NewExampleProfile() *ExampleProfile {
	return &ExampleProfile{
		name: "example-profile",
	}
}

// GetName returns the profile name.
func (p *ExampleProfile) GetName() string {
	return p.name
}

// Validate checks if the provided config overrides are valid.
func (p *ExampleProfile) Validate(configOverrides map[string]string) error {
	// Example: validate supported overrides
	supportedKeys := map[string]bool{
		"replicas": true,
		"enabled":  true,
	}

	for key := range configOverrides {
		if !supportedKeys[key] {
			return fmt.Errorf("unsupported config override: %q", key)
		}
	}

	return nil
}

// GeneratePlanItems creates the configuration items for this profile.
func (p *ExampleProfile) GeneratePlanItems(ctx context.Context, c client.Client, configOverrides map[string]string) ([]hcov1alpha1.VirtPlatformConfigItem, error) {
	// For now, return a simple example item
	// In a real implementation, this would:
	// 1. Read current cluster state
	// 2. Determine what changes are needed
	// 3. Generate plan items with diffs

	items := []hcov1alpha1.VirtPlatformConfigItem{
		{
			Name: "configure-example-component",
			TargetRef: hcov1alpha1.ObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "example-component",
				Namespace:  "default",
			},
			ImpactSeverity: "Low",
			State:          hcov1alpha1.ItemStatePending,
			Message:        "Waiting to apply configuration",
		},
	}

	return items, nil
}

func init() {
	// Register this profile in the default registry
	if err := DefaultRegistry.Register(NewExampleProfile()); err != nil {
		panic(fmt.Sprintf("failed to register example profile: %v", err))
	}
}
