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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
	"github.com/kubevirt/virt-advisor-operator/internal/discovery"
	"github.com/kubevirt/virt-advisor-operator/internal/plan"
	"github.com/kubevirt/virt-advisor-operator/internal/profiles"
)

// This file contains all phase handling logic extracted from the main controller.
// Phase handlers orchestrate the state machine transitions for VirtPlatformConfig resources.

// ==================== Phase Handlers ====================

// handlePendingPhase initializes a new VirtPlatformConfig
func (r *VirtPlatformConfigReconciler) handlePendingPhase(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig) error {
	logger := log.FromContext(ctx)
	logger.Info("Handling Pending phase")

	// Transition to Drafting phase to generate the plan
	return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseDrafting, "Starting plan generation")
}

// handleDraftingPhase generates the plan items from the profile
//
//nolint:gocognit // Drafting phase is inherently complex due to state validation logic
func (r *VirtPlatformConfigReconciler) handleDraftingPhase(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig) error {
	logger := log.FromContext(ctx)
	logger.Info("Handling Drafting phase", "profile", configPlan.Spec.Profile)

	// Look up the profile
	profile, err := profiles.DefaultRegistry.Get(configPlan.Spec.Profile)
	if err != nil {
		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhasePrerequisiteFailed,
			fmt.Sprintf("Profile not found: %v", err))
	}

	// Check if required CRDs exist
	checker := discovery.NewCRDChecker(r.Client)
	missing, err := checker.CheckPrerequisites(ctx, profile.GetPrerequisites())
	if err != nil {
		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseFailed,
			fmt.Sprintf("Failed to check prerequisites: %v", err))
	}
	if len(missing) > 0 {
		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhasePrerequisiteFailed,
			discovery.FormatMissingPrerequisites(missing))
	}

	// Validate profile options match the selected profile
	if err := profiles.ValidateOptions(configPlan.Spec.Profile, configPlan.Spec.Options); err != nil {
		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhasePrerequisiteFailed,
			fmt.Sprintf("Invalid profile options: %v", err))
	}

	// Convert ProfileOptions to map for backward compatibility with existing Profile interface
	// TODO: Update Profile interface to accept typed config directly
	configMap := profiles.OptionsToMap(configPlan.Spec.Profile, configPlan.Spec.Options)

	// Validate config
	if err := profile.Validate(configMap); err != nil {
		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhasePrerequisiteFailed,
			fmt.Sprintf("Invalid config: %v", err))
	}

	// Generate plan items
	items, err := profile.GeneratePlanItems(ctx, r.Client, configMap)
	if err != nil {
		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseFailed,
			fmt.Sprintf("Failed to generate plan items: %v", err))
	}

	// Compute snapshot hash of the CURRENT cluster state (for optimistic locking)
	// This ensures we detect if someone modifies the target resources between
	// plan generation and execution (TOCTOU protection)
	snapshotHash, err := plan.ComputeCurrentStateHash(ctx, r.Client, items)
	if err != nil {
		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseFailed,
			fmt.Sprintf("Failed to compute snapshot hash of current cluster state: %v", err))
	}

	// Update status with plan items
	configPlan.Status.Items = items
	configPlan.Status.SourceSnapshotHash = snapshotHash
	configPlan.Status.OperatorVersion = OperatorVersion
	// Store the options used to generate this plan
	configPlan.Status.AppliedOptions = configPlan.Spec.Options

	if err := r.Status().Update(ctx, configPlan); err != nil {
		return fmt.Errorf("failed to update status with plan items: %w", err)
	}

	// Compute and set impact level from generated plan items
	if err := r.ensureImpactLevel(ctx, configPlan); err != nil {
		return err
	}

	// Check if there are any actual changes to apply
	// If all diffs show "(no changes)", the cluster is already in desired state
	hasChanges := false
	for _, item := range configPlan.Status.Items {
		// Check if diff indicates actual changes
		// Diffs showing "(no changes)" mean the resource is already in desired state
		if !isNoChangeDiff(item.Diff) {
			hasChanges = true
			break
		}
	}

	// If no changes needed, check if resources are healthy and skip to Completed
	if !hasChanges {
		logger.Info("No configuration changes needed - cluster already in desired state")

		// For resources that require health monitoring (e.g., MachineConfig),
		// verify they're actually healthy before marking as completed
		allHealthy := true
		waitManager := plan.NewWaitStrategyManager()

		for i := range configPlan.Status.Items {
			item := &configPlan.Status.Items[i]

			// Check health for items that need it
			msg, healthy, needsWait, healthErr := waitManager.CheckHealthy(ctx, r.Client, item)
			if healthErr != nil {
				logger.Error(healthErr, "Failed to check health for no-change item", "item", item.Name)
				// Don't fail the whole plan, but mark this item as needing attention
				allHealthy = false
				item.State = advisorv1alpha1.ItemStatePending
				item.Message = fmt.Sprintf("Health check failed: %v", healthErr)
				continue
			}

			if needsWait && !healthy {
				logger.Info("Resource exists but not yet healthy", "item", item.Name, "status", msg)
				allHealthy = false
				item.State = advisorv1alpha1.ItemStateInProgress
				item.Message = msg
			} else {
				// Resource is healthy or doesn't need health monitoring
				item.State = advisorv1alpha1.ItemStateCompleted
				item.Message = "Already in desired state"
			}
		}

		// Update status with health check results
		if err := r.Status().Update(ctx, configPlan); err != nil {
			return fmt.Errorf("failed to update status with health checks: %w", err)
		}

		// If all resources are healthy, go straight to Completed
		if allHealthy {
			return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseCompleted,
				"Configuration already in desired state - no changes needed")
		}

		// Some resources exist but aren't healthy yet (e.g., MCP still rolling out)
		// Go to InProgress to monitor them
		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseInProgress,
			"Resources exist but still converging to desired state")
	}

	// Check if this regeneration was triggered by input dependency drift
	inputDriftTriggered := false
	for _, cond := range configPlan.Status.Conditions {
		if cond.Type == "InputDependencyDrift" && cond.Status == metav1.ConditionTrue {
			inputDriftTriggered = true
			break
		}
	}

	// There are changes to apply - follow normal flow
	// Move to ReviewRequired phase (for DryRun) or InProgress (for Apply)
	// IMPORTANT: If regeneration was triggered by input drift, ALWAYS require review
	if configPlan.Spec.Action == advisorv1alpha1.PlanActionDryRun || inputDriftTriggered {
		// Keep the InputDependencyDrift marker - it will be cleared in ReviewRequired after user acknowledges
		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseReviewRequired,
			"Plan ready for review. Change action to 'Apply' to execute.")
	}

	return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseInProgress,
		"Starting plan execution")
}

// handleReviewRequiredPhase waits for user to approve by changing action to Apply
func (r *VirtPlatformConfigReconciler) handleReviewRequiredPhase(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig) error {
	logger := log.FromContext(ctx)
	logger.Info("Handling ReviewRequired phase")

	// Check if this ReviewRequired was triggered by input dependency drift
	inputDriftTriggered := false
	for _, cond := range configPlan.Status.Conditions {
		if cond.Type == "InputDependencyDrift" && cond.Status == metav1.ConditionTrue {
			inputDriftTriggered = true
			break
		}
	}

	// If triggered by input drift, check if drift still exists or has been resolved
	if inputDriftTriggered {
		// Only check drift resolution if we have a Descheduler item to verify against
		// (without it, we can't actually check if HCO matches what's applied)
		deschedulerItem := r.findDeschedulerItem(configPlan)
		if deschedulerItem != nil {
			// Check if HCO dependencies have changed again (could have been reverted)
			changed, reason := r.hcoInputDependenciesChanged(ctx, configPlan)
			if !changed {
				// Also check virt-higher-density profile
				changed, reason = r.hcoHigherDensityConfigChanged(ctx, configPlan)
			}

			if !changed {
				// HCO is now back in sync with what's applied - drift resolved
				// Regenerate plan to reflect current state before transitioning to Completed
				logger.Info("Input dependency drift resolved - HCO now matches applied configuration, regenerating plan")
				r.setCondition(configPlan, "InputDependencyDrift", metav1.ConditionFalse, "DriftResolved",
					"Input dependency drift resolved - external changes reverted")
				if err := r.Status().Update(ctx, configPlan); err != nil {
					return err
				}
				return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhasePending,
					"Regenerating plan after drift resolution")
			}

			// HCO still different from what's applied - continue waiting for acknowledgment
			logger.Info("Input dependency drift detected - waiting for user acknowledgment",
				"reason", reason,
				"hint", "Please review the updated plan and toggle action (DryRun->Apply) or modify spec to acknowledge")
		} else {
			// No Descheduler item - can't verify drift resolution, wait for user acknowledgment
			logger.Info("Input dependency drift detected - waiting for user acknowledgment",
				"hint", "Please review the updated plan and toggle action (DryRun->Apply) or modify spec to acknowledge")
		}

		// Require user acknowledgment by checking if spec has changed
		// User must change action to DryRun->Apply OR modify any spec field to bump generation
		if configPlan.Status.ObservedGeneration == configPlan.Generation {
			return nil
		}

		// User has modified spec (generation changed) - clear the drift marker and proceed
		logger.Info("User acknowledged input dependency drift, clearing marker")
		r.setCondition(configPlan, "InputDependencyDrift", metav1.ConditionFalse, "Acknowledged",
			"User acknowledged input dependency change")
		if err := r.Status().Update(ctx, configPlan); err != nil {
			logger.Error(err, "Failed to clear InputDependencyDrift marker")
			return err
		}
	}

	// Check if user has changed action to Apply
	if configPlan.Spec.Action == advisorv1alpha1.PlanActionApply {
		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseInProgress,
			"User approved, starting execution")
	}

	// Still in DryRun mode, nothing to do
	logger.Info("Waiting for user approval (action must be changed to Apply)")
	return nil
}

// handleInProgressPhase applies the configuration items
func (r *VirtPlatformConfigReconciler) handleInProgressPhase(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig) error {
	logger := log.FromContext(ctx)
	logger.Info("Handling InProgress phase")

	// OPTIMISTIC LOCKING: Verify that the target resources haven't changed
	if err := r.checkOptimisticLock(ctx, configPlan); err != nil {
		return err
	}

	// Apply each pending item
	hasFailures := false

	for i := range configPlan.Status.Items {
		item := &configPlan.Status.Items[i]

		// For completed items, check for drift while other items are still in progress
		if item.State == advisorv1alpha1.ItemStateCompleted {
			if err := r.checkItemDrift(ctx, configPlan, item); err != nil {
				logger.Error(err, "Failed to check drift for completed item", "item", item.Name)
				// Non-fatal - continue with other items
			}
			continue
		}

		// Skip failed items
		if item.State == advisorv1alpha1.ItemStateFailed {
			hasFailures = true
			continue
		}

		// Execute the item
		shouldAbort, shouldContinue, err := r.processItemExecution(ctx, configPlan, item)
		if err != nil {
			return err
		}
		if shouldAbort {
			return err
		}
		if shouldContinue {
			hasFailures = true
			continue
		}

		// Check health
		logger.V(1).Info("Item applied, checking health", "item", item.Name)
		//nolint:staticcheck // SA4006: needsRetry is used on line below, linter false positive
		needsRetry, healthErr := r.checkItemHealth(ctx, configPlan, item)
		if healthErr != nil {
			return healthErr
		}
		if needsRetry {
			continue
		}
	}

	// Check overall completion status
	if areAllItemsCompleted(configPlan.Status.Items) {
		return r.finalizeInProgressPhase(ctx, configPlan, hasFailures)
	}

	return nil
}

// handleCompletedPhase monitors for drift
//
//nolint:gocognit // Completed phase is inherently complex due to drift detection and health monitoring logic
func (r *VirtPlatformConfigReconciler) handleCompletedPhase(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig) error {
	logger := log.FromContext(ctx)
	logger.Info("Handling Completed phase - monitoring for drift")

	// Check if options have changed - if so, regenerate plan
	if optionsChanged(configPlan.Spec.Options, configPlan.Status.AppliedOptions) {
		logger.Info("Options have changed, regenerating plan")
		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseDrafting,
			"Options changed - regenerating plan")
	}

	// Check if operator version has changed (upgrade detected)
	if configPlan.Status.OperatorVersion != "" && configPlan.Status.OperatorVersion != OperatorVersion {
		logger.Info("Operator version changed - checking if profile logic changed",
			"appliedVersion", configPlan.Status.OperatorVersion,
			"currentVersion", OperatorVersion)

		// Regenerate plan items using NEW operator logic (in-memory, not persisted)
		newItems, err := r.generatePlanItemsWithNewLogic(ctx, configPlan)
		if err != nil {
			logger.Error(err, "Failed to generate new plan items for upgrade comparison")
			// Don't fail - just skip upgrade detection this cycle
		} else {
			// Compare new items vs currently applied items
			if planItemsDiffer(newItems, configPlan.Status.Items) {
				logger.Info("Upgrade would change configuration - transitioning to CompletedWithUpgrade phase",
					"appliedVersion", configPlan.Status.OperatorVersion,
					"currentVersion", OperatorVersion)

				// Set UpgradeAvailable condition
				r.setCondition(configPlan, ConditionTypeUpgrade, metav1.ConditionTrue, "LogicChanged",
					fmt.Sprintf("Operator upgraded from %s to %s with profile logic changes. "+
						"Regenerate plan (action=DryRun) to review proposed changes.",
						configPlan.Status.OperatorVersion, OperatorVersion))

				// Transition to CompletedWithUpgrade phase for visibility
				return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseCompletedWithUpgrade,
					fmt.Sprintf("Operator upgraded from %s to %s - profile logic changes detected",
						configPlan.Status.OperatorVersion, OperatorVersion))
			}

			// Upgrade detected but no changes - silently update version
			logger.Info("Upgrade detected but profile logic unchanged - no user action needed",
				"appliedVersion", configPlan.Status.OperatorVersion,
				"currentVersion", OperatorVersion)

			// Clear any stale UpgradeAvailable condition
			r.setCondition(configPlan, ConditionTypeUpgrade, metav1.ConditionFalse, "NoLogicChange",
				"Operator upgraded but profile logic unchanged")

			// Update stored version to suppress future checks
			configPlan.Status.OperatorVersion = OperatorVersion
			return r.Status().Update(ctx, configPlan)
		}
	}

	// Clear UpgradeAvailable if operator version matches (e.g., after user regenerated)
	if configPlan.Status.OperatorVersion == OperatorVersion {
		for _, c := range configPlan.Status.Conditions {
			if c.Type == ConditionTypeUpgrade && c.Status == metav1.ConditionTrue {
				r.setCondition(configPlan, ConditionTypeUpgrade, metav1.ConditionFalse, "Applied",
					"Upgrade changes applied")
				return r.Status().Update(ctx, configPlan)
			}
		}
	}

	// Check if HCO-derived input dependencies have changed
	changed, reason := r.hcoInputDependenciesChanged(ctx, configPlan)
	if !changed {
		// Also check virt-higher-density profile HCO dependencies
		changed, reason = r.hcoHigherDensityConfigChanged(ctx, configPlan)
	}
	if changed {
		logger.Info("HCO input dependencies changed, regenerating plan", "reason", reason)
		// Set a special marker to force review after regeneration
		r.setCondition(configPlan, "InputDependencyDrift", metav1.ConditionTrue, "InputChanged",
			fmt.Sprintf("Input dependency changed: %s", reason))
		if err := r.Status().Update(ctx, configPlan); err != nil {
			return err
		}
		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseDrafting,
			fmt.Sprintf("Regenerating plan due to input dependency change: %s", reason))
	}

	// Check health of MachineConfig items - the MCP might be updating due to other profiles
	// This prevents marking as Completed when MCP is still converging
	waitManager := plan.NewWaitStrategyManager()
	needsRequeue := false
	statusChanged := false

	for i := range configPlan.Status.Items {
		item := &configPlan.Status.Items[i]

		// Only check completed items with health monitoring (e.g., MachineConfig)
		if item.State != advisorv1alpha1.ItemStateCompleted {
			continue
		}

		// Check if this item needs health monitoring
		msg, healthy, needsWait, healthErr := waitManager.CheckHealthy(ctx, r.Client, item)
		if healthErr != nil {
			logger.Error(healthErr, "Failed to check health in Completed phase", "item", item.Name)
			continue
		}

		// If item needs health monitoring and is not healthy, transition back to InProgress
		if needsWait && !healthy {
			logger.Info("MachineConfig item not healthy in Completed phase - MCP is updating",
				"item", item.Name,
				"status", msg)
			item.State = advisorv1alpha1.ItemStateInProgress
			item.Message = msg
			needsRequeue = true
			statusChanged = true
		}
	}

	// If any items are not healthy, update status and transition to InProgress
	if needsRequeue {
		if err := r.Status().Update(ctx, configPlan); err != nil {
			return fmt.Errorf("failed to update item health status: %w", err)
		}
		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseInProgress,
			"MachineConfigPool is updating - waiting for convergence")
	}

	// Update status if health messages changed
	if statusChanged {
		if err := r.Status().Update(ctx, configPlan); err != nil {
			return err
		}
	}

	// First check if any items already have drift detected (from InProgress phase monitoring)
	for i := range configPlan.Status.Items {
		item := &configPlan.Status.Items[i]
		if item.State == advisorv1alpha1.ItemStateCompleted &&
			item.Message == "Configuration has drifted from desired state" {
			logger.Info("Item drift already detected, transitioning to Drifted phase", "item", item.Name)
			return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseDrifted,
				"Configuration drift detected - one or more items have drifted from desired state")
		}
	}

	// Check if we have a stored snapshot hash
	if configPlan.Status.SourceSnapshotHash == "" {
		logger.Info("No snapshot hash stored, computing current state")
		currentStateHash, err := plan.ComputeCurrentStateHash(ctx, r.Client, configPlan.Status.Items)
		if err != nil {
			logger.Error(err, "Failed to compute current state hash")
			return nil // Don't fail the reconciliation
		}
		configPlan.Status.SourceSnapshotHash = currentStateHash
		return r.Status().Update(ctx, configPlan)
	}

	// Detect drift by comparing current state with stored hash
	drifted, err := plan.DetectDrift(ctx, r.Client, configPlan.Status.Items, configPlan.Status.SourceSnapshotHash)
	if err != nil {
		logger.Error(err, "Failed to detect drift")
		// Don't transition to failed, just log and continue monitoring
		return nil
	}

	if drifted {
		logger.Info("Drift detected! Current cluster state differs from applied configuration")

		// Refresh diffs for all items to show what changed
		for i := range configPlan.Status.Items {
			item := &configPlan.Status.Items[i]
			if err := r.checkItemDrift(ctx, configPlan, item); err != nil {
				logger.Error(err, "Failed to refresh diff for drifted item", "item", item.Name)
				// Continue to check other items
			}
		}

		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseDrifted,
			"Configuration drift detected - cluster state has changed from applied configuration")
	}

	logger.V(1).Info("No drift detected, configuration is in sync")

	// Ensure phase-related conditions are properly set even when staying in Completed phase
	return r.ensureCompletedPhaseConditions(ctx, configPlan)
}

// handleCompletedWithUpgradePhase handles the state where configuration is applied successfully
// but a new operator version has different profile logic available.
// Implements auto-recovery: if cluster is manually aligned with new logic, transitions back to Completed.
func (r *VirtPlatformConfigReconciler) handleCompletedWithUpgradePhase(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig) error {
	logger := log.FromContext(ctx)
	logger.Info("Handling CompletedWithUpgrade phase - waiting for user to review upgrade or checking for auto-resolution")

	// Check if spec has changed (user regenerated plan to adopt upgrade)
	if configPlan.Status.ObservedGeneration != configPlan.Generation {
		logger.Info("Spec changed - user regenerating plan to adopt upgrade")
		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhasePending,
			"User regenerating plan to adopt operator upgrade")
	}

	// AUTO-RECOVERY: Re-check if upgrade would still cause changes
	// This handles the case where admin manually updated cluster to match new operator logic
	if configPlan.Status.OperatorVersion != "" && configPlan.Status.OperatorVersion != OperatorVersion {
		newItems, err := r.generatePlanItemsWithNewLogic(ctx, configPlan)
		if err != nil {
			logger.Error(err, "Failed to regenerate plan items for upgrade auto-recovery check")
			// Don't fail - just skip auto-recovery this cycle
			return nil
		}

		// Compare new items vs currently applied items
		if !planItemsDiffer(newItems, configPlan.Status.Items) {
			// Upgrade diff has disappeared! Cluster was manually aligned with new logic
			logger.Info("Upgrade drift resolved - cluster manually aligned with new operator logic, transitioning to Completed",
				"appliedVersion", configPlan.Status.OperatorVersion,
				"currentVersion", OperatorVersion)

			// Clear UpgradeAvailable condition
			r.setCondition(configPlan, ConditionTypeUpgrade, metav1.ConditionFalse, "ManuallyAligned",
				"Cluster manually aligned with new operator logic - upgrade no longer needed")

			// Silently update operator version
			configPlan.Status.OperatorVersion = OperatorVersion

			// Transition back to Completed
			return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseCompleted,
				"Upgrade auto-resolved - cluster aligned with new operator logic")
		}

		// Still different - stay in CompletedWithUpgrade and wait for user action
		logger.V(1).Info("Upgrade still needed - waiting for user to regenerate plan")
	}

	// Waiting for user to regenerate plan (change action to DryRun)
	logger.Info("Waiting for user to regenerate plan to adopt operator upgrade (change action to DryRun)")
	return nil
}

// handleDriftedPhase handles the drifted state
func (r *VirtPlatformConfigReconciler) handleDriftedPhase(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig) error {
	logger := log.FromContext(ctx)
	logger.Info("Handling Drifted phase")

	// In Drifted phase, we monitor whether drift has been corrected manually
	// or if the user wants to re-apply the configuration

	// First check for per-item drift messages (set during monitoring)
	hasItemDrift := false
	for i := range configPlan.Status.Items {
		item := &configPlan.Status.Items[i]
		if item.State == advisorv1alpha1.ItemStateCompleted &&
			item.Message == "Configuration has drifted from desired state" {
			hasItemDrift = true
			break
		}
	}

	// If no per-item drift messages, check hash-based drift detection
	var drifted bool
	var err error
	if !hasItemDrift {
		drifted, err = plan.DetectDrift(ctx, r.Client, configPlan.Status.Items, configPlan.Status.SourceSnapshotHash)
		if err != nil {
			logger.Error(err, "Failed to detect drift")
			return nil // Don't fail the reconciliation
		}
	}

	// If neither per-item nor hash-based drift exists, drift has been corrected
	if !hasItemDrift && !drifted {
		// Drift has been manually corrected
		logger.Info("Drift has been corrected, transitioning back to Completed")
		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseCompleted,
			"Drift has been corrected - configuration is back in sync")
	}

	// Only re-apply if bypassOptimisticLock is true (aggressive continuous reconciliation)
	// Otherwise, require manual intervention to avoid fighting with other controllers
	if configPlan.Spec.Action == advisorv1alpha1.PlanActionApply && configPlan.Spec.BypassOptimisticLock {
		logger.Info("Auto-correcting drift (bypassOptimisticLock=true)")
		// Reset all items to Pending
		for i := range configPlan.Status.Items {
			configPlan.Status.Items[i].State = advisorv1alpha1.ItemStatePending
			configPlan.Status.Items[i].Message = "Re-applying to fix drift (bypassOptimisticLock)"
		}
		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseInProgress,
			"Auto-correcting drift (bypassOptimisticLock enabled)")
	}

	// Drift detected but NOT auto-correcting to avoid fighting with other controllers
	// User must change action to DryRun (to regenerate plan) then back to Apply to retry
	logger.Info("Drift detected - manual intervention required. Change action to DryRun then Apply to retry, or set bypassOptimisticLock=true for continuous reconciliation")
	return nil
}
