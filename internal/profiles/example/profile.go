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

package example

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
	"github.com/kubevirt/virt-advisor-operator/internal/discovery"
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

// GetPrerequisites returns the CRDs required by this profile.
// Example profile has no prerequisites as it's just a demonstration.
func (p *ExampleProfile) GetPrerequisites() []discovery.Prerequisite {
	return []discovery.Prerequisite{}
}

// GetManagedResourceTypes returns the resource types this profile manages for drift detection.
// Example profile doesn't manage any real resources, so returns empty.
func (p *ExampleProfile) GetManagedResourceTypes() []schema.GroupVersionKind {
	return []schema.GroupVersionKind{}
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
func (p *ExampleProfile) GeneratePlanItems(ctx context.Context, c client.Client, configOverrides map[string]string) ([]advisorv1alpha1.VirtPlatformConfigItem, error) {
	// For now, return a simple example item
	// In a real implementation, this would:
	// 1. Read current cluster state
	// 2. Determine what changes are needed
	// 3. Generate plan items with diffs

	// Create a simple ConfigMap as the desired state
	desiredState := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "example-config",
			"namespace": "openshift-cnv",
		},
		"data": map[string]interface{}{
			"example-key": "example-value",
		},
	}
	rawJSON, err := json.Marshal(desiredState)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal desired state: %w", err)
	}

	items := []advisorv1alpha1.VirtPlatformConfigItem{
		{
			Name: "configure-example-component",
			TargetRef: advisorv1alpha1.ObjectReference{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "example-config",
				Namespace:  "openshift-cnv",
			},
			ImpactSeverity: advisorv1alpha1.ImpactLow,
			State:          advisorv1alpha1.ItemStatePending,
			Message:        "Waiting to apply configuration",
			// Generate a fake diff for testing purposes
			// Real profiles would call plan.GenerateSSADiff() to create this
			Diff:         "--- example-config (ConfigMap)\n+++ example-config (ConfigMap)\n+  data:\n+    example-key: example-value\n",
			DesiredState: &runtime.RawExtension{Raw: rawJSON},
		},
	}

	return items, nil
}

// GetDescription returns a human-readable description of this profile
func (p *ExampleProfile) GetDescription() string {
	return "Example profile for demonstration purposes only"
}

// GetCategory returns the category this profile belongs to
func (p *ExampleProfile) GetCategory() string {
	return "example"
}

// GetImpactSummary returns a summary of the impact of enabling this profile
func (p *ExampleProfile) GetImpactSummary() string {
	return "None - Example profile with no real resources"
}

// GetImpactLevel returns the aggregate risk level
func (p *ExampleProfile) GetImpactLevel() advisorv1alpha1.Impact {
	return advisorv1alpha1.ImpactLow
}

// IsAdvertisable returns false since this is an example profile
func (p *ExampleProfile) IsAdvertisable() bool {
	return false
}
