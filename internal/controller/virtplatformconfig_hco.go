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

package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
	"github.com/kubevirt/virt-advisor-operator/internal/profiles"
	"github.com/kubevirt/virt-advisor-operator/internal/profiles/higherdensity"
	"github.com/kubevirt/virt-advisor-operator/internal/profiles/loadaware"
)

// This file contains HCO (HyperConverged Operator) dependency tracking logic.
// These functions detect when HCO configuration changes require regenerating plans.

// ==================== HCO Input Dependency Detection ====================

func (r *VirtPlatformConfigReconciler) hcoInputDependenciesChanged(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig) (bool, string) {
	logger := log.FromContext(ctx)

	// Only applies to load-aware-rebalancing profile
	if configPlan.Spec.Profile != "load-aware-rebalancing" {
		return false, ""
	}

	// If user provided explicit eviction limits, HCO changes don't matter
	hasExplicitLimits := false
	if configPlan.Spec.Options != nil && configPlan.Spec.Options.LoadAware != nil {
		if configPlan.Spec.Options.LoadAware.EvictionLimitTotal != nil ||
			configPlan.Spec.Options.LoadAware.EvictionLimitNode != nil {
			hasExplicitLimits = true
		}
	}

	if hasExplicitLimits {
		return false, ""
	}

	// User relies on HCO defaults - check if they changed
	// Get the profile to access HCO limit computation
	profile, err := profiles.DefaultRegistry.Get(configPlan.Spec.Profile)
	if err != nil {
		logger.Error(err, "Failed to get profile for HCO dependency check")
		return false, ""
	}

	loadAwareProfile, ok := profile.(*loadaware.LoadAwareRebalancingProfile)
	if !ok {
		return false, ""
	}

	// Read current HCO limits and compute scaled values
	configOverrides := profiles.OptionsToMap(configPlan.Spec.Profile, configPlan.Spec.Options)
	currentTotal, currentNode, err := loadAwareProfile.GetEvictionLimitsForTesting(ctx, r.Client, configOverrides)
	if err != nil {
		logger.V(1).Info("Could not read current HCO limits, skipping dependency check", "error", err)
		return false, ""
	}

	// Read eviction limits from the applied descheduler configuration
	deschedulerItem := r.findDeschedulerItem(configPlan)
	if deschedulerItem == nil {
		logger.V(1).Info("No KubeDescheduler item found, skipping HCO dependency check")
		return false, ""
	}

	appliedTotal, appliedNode, err := r.extractAppliedEvictionLimits(ctx, deschedulerItem)
	if err != nil {
		logger.V(1).Info("Could not extract applied eviction limits", "error", err)
		return false, ""
	}

	// Compare current HCO-derived limits with what's applied
	if currentTotal != appliedTotal {
		return true, fmt.Sprintf("HCO parallelMigrationsPerCluster changed (scaled %d -> %d)", appliedTotal, currentTotal)
	}

	if currentNode != appliedNode {
		return true, fmt.Sprintf("HCO parallelOutboundMigrationsPerNode changed (scaled %d -> %d)", appliedNode, currentNode)
	}

	return false, ""
}

// hcoHigherDensityConfigChanged checks if HCO configuration relevant to virt-higher-density has drifted.
// Returns (changed bool, reason string).

func (r *VirtPlatformConfigReconciler) hcoHigherDensityConfigChanged(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig) (bool, string) {
	logger := log.FromContext(ctx)

	// Only applies to virt-higher-density profile
	if configPlan.Spec.Profile != "virt-higher-density" {
		return false, ""
	}

	// Skip if user has explicit config overrides
	hasExplicitConfig := false
	if configPlan.Spec.Options != nil && configPlan.Spec.Options.VirtHigherDensity != nil {
		cfg := configPlan.Spec.Options.VirtHigherDensity
		// Check if user explicitly set KSM or memory ratio
		if cfg.KSMConfiguration != nil || cfg.MemoryToRequestRatio != nil {
			hasExplicitConfig = true
		}
	}
	if hasExplicitConfig {
		logger.V(1).Info("User has explicit virt-higher-density config, skipping HCO drift check")
		return false, ""
	}

	// Get the profile to access HCO config reader
	profile, err := profiles.DefaultRegistry.Get(configPlan.Spec.Profile)
	if err != nil {
		logger.Error(err, "Failed to get virt-higher-density profile")
		return false, ""
	}

	hdProfile, ok := profile.(*higherdensity.VirtHigherDensityProfile)
	if !ok {
		logger.Error(fmt.Errorf("profile is not VirtHigherDensityProfile"), "Type assertion failed")
		return false, ""
	}

	// Get current HCO configuration
	currentKSM, currentMemory, err := hdProfile.GetHCOConfigForTesting(ctx, r.Client)
	if err != nil {
		logger.V(1).Info("Could not read current HCO config for drift detection", "error", err)
		return false, ""
	}

	// Find the HCO item in the applied plan
	hcoItem := r.findHCOItem(configPlan)
	if hcoItem == nil {
		logger.V(1).Info("No HCO item found in plan, cannot detect drift")
		return false, ""
	}

	// Extract what was applied
	appliedKSM, appliedMemory, err := r.extractAppliedHCOConfig(ctx, hcoItem)
	if err != nil {
		logger.V(1).Info("Could not extract applied HCO config", "error", err)
		return false, ""
	}

	// Compare KSM configuration
	if currentKSM != appliedKSM {
		return true, fmt.Sprintf("HCO KSM configuration changed (was %v, now %v)", appliedKSM, currentKSM)
	}

	// Compare memory overcommit percentage
	if currentMemory != appliedMemory {
		return true, fmt.Sprintf("HCO memoryOvercommitPercentage changed (%d -> %d)", appliedMemory, currentMemory)
	}

	return false, ""
}

// findHCOItem finds the HyperConverged item in the plan

// ==================== HCO Item Lookup Helpers ====================

func (r *VirtPlatformConfigReconciler) findHCOItem(configPlan *advisorv1alpha1.VirtPlatformConfig) *advisorv1alpha1.VirtPlatformConfigItem {
	for i := range configPlan.Status.Items {
		item := &configPlan.Status.Items[i]
		if item.TargetRef.Kind == "HyperConverged" {
			return item
		}
	}
	return nil
}

// extractAppliedHCOConfig reads the currently applied HCO CR and extracts KSM and memory config.
// Returns (ksmEnabled bool, memoryPct int32, err error).

func (r *VirtPlatformConfigReconciler) extractAppliedHCOConfig(ctx context.Context, item *advisorv1alpha1.VirtPlatformConfigItem) (bool, int32, error) {
	hco := &unstructured.Unstructured{}
	hco.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "hco.kubevirt.io",
		Version: "v1beta1",
		Kind:    "HyperConverged",
	})

	key := types.NamespacedName{
		Name:      item.TargetRef.Name,
		Namespace: item.TargetRef.Namespace,
	}

	if err := r.Get(ctx, key, hco); err != nil {
		return false, 0, fmt.Errorf("failed to get HCO CR: %w", err)
	}

	// Check if KSM is configured
	_, ksmFound, err := unstructured.NestedMap(hco.Object, "spec", "ksmConfiguration")
	if err != nil {
		return false, 0, fmt.Errorf("error reading ksmConfiguration: %w", err)
	}

	// Extract memory overcommit percentage
	memoryPct, found, err := unstructured.NestedInt64(hco.Object, "spec", "higherWorkloadDensity", "memoryOvercommitPercentage")
	if err != nil || !found {
		return false, 0, fmt.Errorf("memoryOvercommitPercentage not found: %w", err)
	}

	return ksmFound, int32(memoryPct), nil
}

// findDeschedulerItem finds the KubeDescheduler item in the plan

func (r *VirtPlatformConfigReconciler) findDeschedulerItem(configPlan *advisorv1alpha1.VirtPlatformConfig) *advisorv1alpha1.VirtPlatformConfigItem {
	for i := range configPlan.Status.Items {
		item := &configPlan.Status.Items[i]
		if item.TargetRef.Kind == "KubeDescheduler" {
			return item
		}
	}
	return nil
}

// extractAppliedEvictionLimits extracts eviction limits from the applied descheduler configuration

func (r *VirtPlatformConfigReconciler) extractAppliedEvictionLimits(ctx context.Context, item *advisorv1alpha1.VirtPlatformConfigItem) (int32, int32, error) {
	// Read the current KubeDescheduler resource
	descheduler := &unstructured.Unstructured{}
	descheduler.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operator.openshift.io",
		Version: "v1",
		Kind:    "KubeDescheduler",
	})

	key := types.NamespacedName{
		Name:      item.TargetRef.Name,
		Namespace: item.TargetRef.Namespace,
	}

	if err := r.Get(ctx, key, descheduler); err != nil {
		return 0, 0, fmt.Errorf("failed to get descheduler: %w", err)
	}

	// Extract evictionLimits from spec.evictionLimits (not profileCustomizations)
	total, _, err := unstructured.NestedInt64(descheduler.Object, "spec", "evictionLimits", "total")
	if err != nil {
		return 0, 0, fmt.Errorf("failed to extract evictionLimits.total: %w", err)
	}

	// Note: node field may not exist in older Descheduler CRD versions
	node, found, err := unstructured.NestedInt64(descheduler.Object, "spec", "evictionLimits", "node")
	if err != nil {
		return 0, 0, fmt.Errorf("failed to extract evictionLimits.node: %w", err)
	}
	if !found {
		// Older CRD versions don't have the node field, default to 0
		node = 0
	}

	return int32(total), int32(node), nil
}
