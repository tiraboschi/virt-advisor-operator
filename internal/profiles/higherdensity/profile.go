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
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
	"github.com/kubevirt/virt-advisor-operator/internal/discovery"
	"github.com/kubevirt/virt-advisor-operator/internal/plan"
	"github.com/kubevirt/virt-advisor-operator/internal/profiles/profileutils"
)

const (
	// ProfileNameVirtHigherDensity is the name of the virt-higher-density profile
	ProfileNameVirtHigherDensity = "virt-higher-density"
	// defaultMemoryToRequestRatio is the default memory overcommit percentage (50% overcommit)
	defaultMemoryToRequestRatio = 150
)

// VirtHigherDensityProfile implements higher density configurations for virtualization workloads
type VirtHigherDensityProfile struct {
	name string
}

// NewVirtHigherDensityProfile creates a new virt-higher-density profile
func NewVirtHigherDensityProfile() *VirtHigherDensityProfile {
	return &VirtHigherDensityProfile{
		name: ProfileNameVirtHigherDensity,
	}
}

// GetName returns the profile name
func (p *VirtHigherDensityProfile) GetName() string {
	return p.name
}

// GetDescription returns a human-readable description
func (p *VirtHigherDensityProfile) GetDescription() string {
	return "Enables higher VM density through swap and other optimizations"
}

// GetCategory returns the profile category for grouping
func (p *VirtHigherDensityProfile) GetCategory() string {
	return "density"
}

// GetImpactSummary returns a summary of the profile's impact
func (p *VirtHigherDensityProfile) GetImpactSummary() string {
	return "Enables swap for kubelet to allow memory overcommit and higher VM density. Requires nodes with swap partition labeled CNV_SWAP."
}

// GetImpactLevel returns the aggregate risk level
func (p *VirtHigherDensityProfile) GetImpactLevel() advisorv1alpha1.Impact {
	return advisorv1alpha1.ImpactHigh
}

// IsAdvertisable indicates this profile should be auto-created for discovery
func (p *VirtHigherDensityProfile) IsAdvertisable() bool {
	return true
}

// GetPrerequisites returns the CRDs required by this profile
func (p *VirtHigherDensityProfile) GetPrerequisites() []discovery.Prerequisite {
	return []discovery.Prerequisite{
		{
			GVK:         profileutils.MachineConfigGVK,
			Description: "MachineConfig CRD is required for swap configuration (available on OpenShift)",
		},
		{
			GVK:         profileutils.HyperConvergedGVK,
			Description: "Install OpenShift Virtualization via OLM",
		},
	}
}

// GetManagedResourceTypes returns the GVKs that this profile manages
func (p *VirtHigherDensityProfile) GetManagedResourceTypes() []schema.GroupVersionKind {
	return []schema.GroupVersionKind{
		profileutils.MachineConfigGVK,
		profileutils.HyperConvergedGVK,
	}
}

// Validate checks if the provided config overrides are valid
func (p *VirtHigherDensityProfile) Validate(configOverrides map[string]string) error {
	// No string-based overrides supported for this profile
	// Configuration is done via typed Options.VirtHigherDensity
	return nil
}

// GeneratePlanItems creates the configuration plan for higher density
func (p *VirtHigherDensityProfile) GeneratePlanItems(ctx context.Context, c client.Client, configOverrides map[string]string) ([]advisorv1alpha1.VirtPlatformConfigItem, error) {
	var items []advisorv1alpha1.VirtPlatformConfigItem

	// Get swap setting from config (default: true)
	swapEnabled := profileutils.GetBoolConfig(configOverrides, "enableSwap", true)

	if swapEnabled {
		item, err := p.generateSwapMachineConfigItem(ctx, c)
		if err != nil {
			return nil, fmt.Errorf("failed to generate swap machine config item: %w", err)
		}
		items = append(items, item)
	}

	// Always generate HCO configuration item for memory overcommit and KSM
	hcoItem, err := p.generateHCOItem(ctx, c, configOverrides)
	if err != nil {
		return nil, fmt.Errorf("failed to generate HCO item: %w", err)
	}
	items = append(items, hcoItem)

	return items, nil
}

// generateSwapMachineConfigItem creates a plan item for MachineConfig swap enablement
func (p *VirtHigherDensityProfile) generateSwapMachineConfigItem(ctx context.Context, c client.Client) (advisorv1alpha1.VirtPlatformConfigItem, error) {
	mcName := "90-worker-kubelet-swap"

	// Build the desired MachineConfig object
	desired := plan.CreateUnstructured(profileutils.MachineConfigGVK, mcName, "")

	// Set labels
	desired.SetLabels(map[string]string{
		"machineconfiguration.openshift.io/role": "worker",
	})

	// Build the ignition config spec
	spec := map[string]interface{}{
		"config": map[string]interface{}{
			"ignition": map[string]interface{}{
				"version": "3.2.0",
			},
			"storage": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{
						"contents": map[string]interface{}{
							"source": "data:text/plain;charset=utf-8;base64,IyEvdXNyL2Jpbi9lbnYgYmFzaApzZXQgLWV1byBwaXBlZmFpbAoKUEFSVExBQkVMPSJDTlZfU1dBUCIKREVWSUNFPSIvZGV2L2Rpc2svYnktcGFydGxhYmVsLyR7UEFSVExBQkVMfSIKCmlmIFtbICEgLWUgIiRERVZJQ0UiIF1dOyB0aGVuCiAgZWNobyAiU3dhcCBwYXJ0aXRpb24gd2l0aCBQQVJUTEFCRUw9JHtQQVJUTEFCRUx9IG5vdCBmb3VuZCIgPiYyCiAgZXhpdCAxCmZpCgppZiBzd2Fwb24gLS1zaG93PU5BTUUgfCBncmVwIC1xICIkREVWSUNFIjsgdGhlbgogIGVjaG8gIlN3YXAgYWxyZWFkeSBlbmFibGVkIG9uICRERVZJQ0UiID4mMgogIGV4aXQgMQpmaQoKZWNobyAiRW5hYmxpbmcgc3dhcCBvbiAkREVWSUNFIiA+JjIKc3dhcG9uICIkREVWSUNFIgp0b3VjaCAvdmFyL3RtcC9zd2FwZmlsZQ==",
						},
						"mode":      int64(0755),
						"overwrite": true,
						"path":      "/etc/find-swap-partition",
					},
					map[string]interface{}{
						"contents": map[string]interface{}{
							"source": "data:text/plain;charset=utf-8;base64,YXBpVmVyc2lvbjoga3ViZWxldC5jb25maWcuazhzLmlvL3YxYmV0YTEKa2luZDogS3ViZWxldENvbmZpZ3VyYXRpb24KbWVtb3J5U3dhcDoKICBzd2FwQmVoYXZpb3I6IExpbWl0ZWRTd2FwCg==",
						},
						"mode":      int64(420),
						"overwrite": true,
						"path":      "/etc/openshift/kubelet.conf.d/90-swap.conf",
					},
				},
			},
			"systemd": map[string]interface{}{
				"units": []interface{}{
					map[string]interface{}{
						"contents": "[Unit]\nDescription=Provision and enable swap\nConditionFirstBoot=no\nConditionPathExists=!/var/tmp/swapfile\n\n[Service]\nType=oneshot\nExecStart=/etc/find-swap-partition\n\n[Install]\nWantedBy=multi-user.target\nRequiredBy=kubelet-dependencies.target\n",
						"enabled":  true,
						"name":     "swap-provision.service",
					},
					map[string]interface{}{
						"contents": "[Unit]\nDescription=Restrict swap for system slice\nConditionFirstBoot=no\n\n[Service]\nType=oneshot\nExecStart=/bin/sh -c \"sudo systemctl set-property --runtime system.slice MemorySwapMax=0 IODeviceLatencyTargetSec=\\\"/ 50ms\\\"\"\n\n[Install]\nRequiredBy=kubelet-dependencies.target\n",
						"enabled":  true,
						"name":     "cgroup-system-slice-config.service",
					},
				},
			},
		},
	}

	if err := unstructured.SetNestedMap(desired.Object, spec, "spec"); err != nil {
		return advisorv1alpha1.VirtPlatformConfigItem{}, fmt.Errorf("failed to set spec: %w", err)
	}

	// Use the builder to create the item with SSA-generated diff
	return profileutils.NewPlanItemBuilder(ctx, c, "virt-advisor-operator").
		ForResource(desired, "enable-kubelet-swap").
		WithManagedFields([]string{"spec.config"}).
		WithImpact(advisorv1alpha1.ImpactHigh).
		WithMessage(fmt.Sprintf("MachineConfig '%s' will be configured to enable kubelet swap", mcName)).
		Build()
}

// generateHCOItem creates a plan item for HyperConverged configuration
func (p *VirtHigherDensityProfile) generateHCOItem(ctx context.Context, c client.Client, configOverrides map[string]string) (advisorv1alpha1.VirtPlatformConfigItem, error) {
	logger := log.FromContext(ctx)

	// Create desired HCO state
	desired := plan.CreateUnstructured(profileutils.HyperConvergedGVK, profileutils.HyperConvergedName, profileutils.HyperConvergedNamespace)

	spec := make(map[string]interface{})
	managedFields := []string{}
	message := ""

	// Get memory overcommit ratio from config (default: 150)
	memoryRatio := int32(profileutils.GetIntConfig(configOverrides, "memoryToRequestRatio", defaultMemoryToRequestRatio))

	// Configure memory overcommit
	spec["higherWorkloadDensity"] = map[string]interface{}{
		"memoryOvercommitPercentage": int64(memoryRatio),
	}
	managedFields = append(managedFields, "spec.higherWorkloadDensity.memoryOvercommitPercentage")
	message = fmt.Sprintf("HyperConverged '%s' will be configured with memoryOvercommitPercentage=%d", profileutils.HyperConvergedName, memoryRatio)

	// Configure KSM if enabled (default: true)
	ksmEnabled := profileutils.GetBoolConfig(configOverrides, "ksmEnabled", true)
	if ksmEnabled {
		ksmConfig := make(map[string]interface{})

		// Parse node selector if provided
		if selectorJSON, ok := configOverrides["ksmNodeLabelSelector"]; ok && selectorJSON != "" {
			var selector metav1.LabelSelector
			if err := json.Unmarshal([]byte(selectorJSON), &selector); err != nil {
				logger.V(1).Info("Failed to parse KSM selector, using default (all nodes)", "error", err)
				ksmConfig["nodeLabelSelector"] = map[string]interface{}{}
			} else {
				ksmConfig["nodeLabelSelector"] = profileutils.LabelSelectorToMap(&selector)
			}
		} else {
			// Empty selector = all nodes
			ksmConfig["nodeLabelSelector"] = map[string]interface{}{}
		}

		// Set KSM configuration under spec
		spec["ksmConfiguration"] = ksmConfig
		managedFields = append(managedFields, "spec.ksmConfiguration")
		message += " and KSM enabled"
	}

	// Set the spec on the desired object
	if err := unstructured.SetNestedMap(desired.Object, spec, "spec"); err != nil {
		return advisorv1alpha1.VirtPlatformConfigItem{}, fmt.Errorf("failed to set spec: %w", err)
	}

	// Use the builder to create the item
	return profileutils.NewPlanItemBuilder(ctx, c, "virt-advisor-operator").
		ForResource(desired, "configure-higher-density").
		WithManagedFields(managedFields).
		WithImpact(advisorv1alpha1.ImpactMedium).
		WithMessage(message).
		Build()
}

// GetHCOConfigForTesting extracts the current HCO configuration.
// This method is exported for use by the controller for drift detection.
// Returns (ksmEnabled bool, memoryPct int32, err error).
func (p *VirtHigherDensityProfile) GetHCOConfigForTesting(ctx context.Context, c client.Client) (bool, int32, error) {
	logger := log.FromContext(ctx)

	hco, err := profileutils.GetHCO(ctx, c)
	if err != nil {
		logger.V(1).Info("Could not fetch HyperConverged CR", "error", err)
		return false, 0, err
	}

	// Check if KSM is enabled (spec.ksmConfiguration exists)
	ksmConfig, found, err := unstructured.NestedMap(hco.Object, "spec", "ksmConfiguration")
	ksmEnabled := found && err == nil && ksmConfig != nil

	// Extract memory overcommit percentage
	memoryPct, found, err := unstructured.NestedInt64(hco.Object, "spec", "higherWorkloadDensity", "memoryOvercommitPercentage")
	if err != nil || !found {
		return ksmEnabled, 0, fmt.Errorf("memoryOvercommitPercentage not found in HCO CR")
	}

	return ksmEnabled, int32(memoryPct), nil
}
