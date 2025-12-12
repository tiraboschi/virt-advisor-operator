/*
Copyright 2025.

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
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	hcov1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
	"github.com/kubevirt/virt-advisor-operator/internal/plan"
	"github.com/kubevirt/virt-advisor-operator/internal/profiles"

	// Import profiles to register them
	_ "github.com/kubevirt/virt-advisor-operator/internal/profiles"
)

const (
	// OperatorVersion is the current version of the operator logic
	OperatorVersion = "v0.1.0"
)

// ConfigurationPlanReconciler reconciles a ConfigurationPlan object
type ConfigurationPlanReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=hco.kubevirt.io,resources=configurationplans,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hco.kubevirt.io,resources=configurationplans/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=hco.kubevirt.io,resources=configurationplans/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/reconcile
func (r *ConfigurationPlanReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ConfigurationPlan
	configPlan := &hcov1alpha1.ConfigurationPlan{}
	if err := r.Get(ctx, req.NamespacedName, configPlan); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("ConfigurationPlan resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ConfigurationPlan")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling ConfigurationPlan",
		"profile", configPlan.Spec.Profile,
		"action", configPlan.Spec.Action,
		"phase", configPlan.Status.Phase)

	// Handle the reconciliation based on current phase
	var err error
	switch configPlan.Status.Phase {
	case "", hcov1alpha1.PlanPhasePending:
		err = r.handlePendingPhase(ctx, configPlan)
	case hcov1alpha1.PlanPhaseDrafting:
		err = r.handleDraftingPhase(ctx, configPlan)
	case hcov1alpha1.PlanPhaseReviewRequired:
		err = r.handleReviewRequiredPhase(ctx, configPlan)
	case hcov1alpha1.PlanPhaseInProgress:
		err = r.handleInProgressPhase(ctx, configPlan)
	case hcov1alpha1.PlanPhaseCompleted:
		err = r.handleCompletedPhase(ctx, configPlan)
	case hcov1alpha1.PlanPhaseCompletedWithErrors:
		err = r.handleCompletedPhase(ctx, configPlan)
	case hcov1alpha1.PlanPhaseDrifted:
		err = r.handleDriftedPhase(ctx, configPlan)
	case hcov1alpha1.PlanPhaseFailed:
		// Failed plans don't automatically recover
		logger.Info("Plan is in Failed state, manual intervention required")
		return ctrl.Result{}, nil
	default:
		logger.Info("Unknown phase, resetting to Pending", "phase", configPlan.Status.Phase)
		err = r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhasePending, "Resetting unknown phase")
	}

	if err != nil {
		logger.Error(err, "Error during reconciliation")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// handlePendingPhase initializes a new ConfigurationPlan
func (r *ConfigurationPlanReconciler) handlePendingPhase(ctx context.Context, configPlan *hcov1alpha1.ConfigurationPlan) error {
	logger := log.FromContext(ctx)
	logger.Info("Handling Pending phase")

	// Transition to Drafting phase to generate the plan
	return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhaseDrafting, "Starting plan generation")
}

// handleDraftingPhase generates the plan items from the profile
func (r *ConfigurationPlanReconciler) handleDraftingPhase(ctx context.Context, configPlan *hcov1alpha1.ConfigurationPlan) error {
	logger := log.FromContext(ctx)
	logger.Info("Handling Drafting phase", "profile", configPlan.Spec.Profile)

	// Look up the profile
	profile, err := profiles.DefaultRegistry.Get(configPlan.Spec.Profile)
	if err != nil {
		return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhasePrerequisiteFailed,
			fmt.Sprintf("Profile not found: %v", err))
	}

	// Validate profile options match the selected profile
	if err := profiles.ValidateOptions(configPlan.Spec.Profile, configPlan.Spec.Options); err != nil {
		return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhasePrerequisiteFailed,
			fmt.Sprintf("Invalid profile options: %v", err))
	}

	// Convert ProfileOptions to map for backward compatibility with existing Profile interface
	// TODO: Update Profile interface to accept typed config directly
	configMap := profiles.OptionsToMap(configPlan.Spec.Profile, configPlan.Spec.Options)

	// Validate config
	if err := profile.Validate(configMap); err != nil {
		return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhasePrerequisiteFailed,
			fmt.Sprintf("Invalid config: %v", err))
	}

	// Generate plan items
	items, err := profile.GeneratePlanItems(ctx, r.Client, configMap)
	if err != nil {
		return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhaseFailed,
			fmt.Sprintf("Failed to generate plan items: %v", err))
	}

	// Compute snapshot hash of the CURRENT cluster state (for optimistic locking)
	// This ensures we detect if someone modifies the target resources between
	// plan generation and execution (TOCTOU protection)
	snapshotHash, err := plan.ComputeCurrentStateHash(ctx, r.Client, items)
	if err != nil {
		return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhaseFailed,
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

	// Move to ReviewRequired phase (for DryRun) or InProgress (for Apply)
	if configPlan.Spec.Action == hcov1alpha1.PlanActionDryRun {
		return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhaseReviewRequired,
			"Plan ready for review. Change action to 'Apply' to execute.")
	}

	return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhaseInProgress,
		"Starting plan execution")
}

// handleReviewRequiredPhase waits for user to approve by changing action to Apply
func (r *ConfigurationPlanReconciler) handleReviewRequiredPhase(ctx context.Context, configPlan *hcov1alpha1.ConfigurationPlan) error {
	logger := log.FromContext(ctx)
	logger.Info("Handling ReviewRequired phase")

	// Check if user has changed action to Apply
	if configPlan.Spec.Action == hcov1alpha1.PlanActionApply {
		return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhaseInProgress,
			"User approved, starting execution")
	}

	// Still in DryRun mode, nothing to do
	logger.Info("Waiting for user approval (action must be changed to Apply)")
	return nil
}

// handleInProgressPhase applies the configuration items
func (r *ConfigurationPlanReconciler) handleInProgressPhase(ctx context.Context, configPlan *hcov1alpha1.ConfigurationPlan) error {
	logger := log.FromContext(ctx)
	logger.Info("Handling InProgress phase")

	// OPTIMISTIC LOCKING: Verify that the target resources haven't changed
	// since the plan was generated (TOCTOU protection)
	//
	// Only check on the FIRST entry to InProgress (when all items are still Pending).
	// Once we start executing items, we're modifying the cluster ourselves, so the
	// hash will naturally change - that's expected and not a stale plan scenario.
	allPending := true
	for _, item := range configPlan.Status.Items {
		if item.State != hcov1alpha1.ItemStatePending {
			allPending = false
			break
		}
	}

	if allPending && configPlan.Status.SourceSnapshotHash != "" {
		// Check if bypass flag is set
		if configPlan.Spec.BypassOptimisticLock {
			logger.Info("WARNING: Optimistic lock check bypassed via spec.bypassOptimisticLock=true. "+
				"Applying configuration without verifying cluster state has not changed since plan generation. "+
				"This may overwrite manual changes or apply outdated configurations.",
				"bypassOptimisticLock", true)
		} else {
			// Perform optimistic lock check
			currentHash, err := plan.ComputeCurrentStateHash(ctx, r.Client, configPlan.Status.Items)
			if err != nil {
				logger.Error(err, "Failed to compute current state hash for optimistic locking check")
				return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhaseFailed,
					fmt.Sprintf("Failed to verify cluster state before execution: %v", err))
			}

			if currentHash != configPlan.Status.SourceSnapshotHash {
				logger.Info("Plan is stale! Target resources have been modified since plan generation",
					"stored_hash", configPlan.Status.SourceSnapshotHash,
					"current_hash", currentHash)
				return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhaseFailed,
					"Plan is stale: target resources have been modified since plan generation. "+
						"Please regenerate the plan by changing action to DryRun, then review and re-approve.")
			}
			logger.V(1).Info("Optimistic lock verification passed - cluster state unchanged")
		}
	}

	// Apply each pending item
	allCompleted := true
	hasFailures := false

	for i := range configPlan.Status.Items {
		item := &configPlan.Status.Items[i]

		// Skip already processed items
		if item.State == hcov1alpha1.ItemStateCompleted || item.State == hcov1alpha1.ItemStateFailed {
			if item.State == hcov1alpha1.ItemStateFailed {
				hasFailures = true
			}
			continue
		}

		// Mark as in progress
		if item.State == hcov1alpha1.ItemStatePending {
			item.State = hcov1alpha1.ItemStateInProgress
			item.Message = "Applying configuration..."
			now := metav1.Now()
			item.LastTransitionTime = &now

			if err := r.Status().Update(ctx, configPlan); err != nil {
				logger.Error(err, "Failed to update item to InProgress", "item", item.Name)
			}
		}

		// Execute the item
		logger.Info("Executing plan item", "item", item.Name, "target", fmt.Sprintf("%s/%s", item.TargetRef.Kind, item.TargetRef.Name))
		err := plan.ExecuteItem(ctx, r.Client, item)

		if err != nil {
			logger.Error(err, "Failed to execute plan item", "item", item.Name)
			now := metav1.Now()
			item.LastTransitionTime = &now
			item.State = hcov1alpha1.ItemStateFailed
			item.Message = fmt.Sprintf("Failed to apply: %v", err)
			hasFailures = true

			// Update status with failure
			if updateErr := r.Status().Update(ctx, configPlan); updateErr != nil {
				logger.Error(updateErr, "Failed to update item failure status")
			}

			// Check failure policy
			if configPlan.Spec.FailurePolicy == hcov1alpha1.FailurePolicyAbort {
				logger.Info("Aborting due to failure policy", "policy", configPlan.Spec.FailurePolicy)
				return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhaseFailed,
					fmt.Sprintf("Item '%s' failed: %v", item.Name, err))
			}

			// Continue policy - keep going
			allCompleted = false
			continue
		}

		// Item applied successfully - now check health (single check, no blocking)
		logger.V(1).Info("Item applied, checking health", "item", item.Name)

		waitManager := plan.NewWaitStrategyManager()
		progressMsg, isHealthy, needsWait, healthErr := waitManager.CheckHealthy(ctx, r.Client, item)

		if healthErr != nil {
			logger.Error(healthErr, "Failed to check resource health", "item", item.Name)
			now := metav1.Now()
			item.LastTransitionTime = &now
			item.State = hcov1alpha1.ItemStateFailed
			item.Message = fmt.Sprintf("Health check failed: %v", healthErr)
			hasFailures = true

			// Update status with failure
			if updateErr := r.Status().Update(ctx, configPlan); updateErr != nil {
				logger.Error(updateErr, "Failed to update item failure status")
			}

			// Check failure policy
			if configPlan.Spec.FailurePolicy == hcov1alpha1.FailurePolicyAbort {
				logger.Info("Aborting due to failure policy", "policy", configPlan.Spec.FailurePolicy)
				return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhaseFailed,
					fmt.Sprintf("Item '%s' health check failed: %v", item.Name, healthErr))
			}

			// Continue policy - keep going
			allCompleted = false
			continue
		}

		if needsWait && !isHealthy {
			// Resource needs more time to become healthy
			// Check if we've exceeded the optional waitTimeout
			if configPlan.Spec.WaitTimeout != nil && item.LastTransitionTime != nil {
				elapsed := time.Since(item.LastTransitionTime.Time)
				timeout := configPlan.Spec.WaitTimeout.Duration

				if elapsed > timeout {
					// Timeout exceeded
					logger.Info("Resource wait timeout exceeded", "item", item.Name, "elapsed", elapsed, "timeout", timeout)
					now := metav1.Now()
					item.LastTransitionTime = &now
					item.State = hcov1alpha1.ItemStateFailed
					item.Message = fmt.Sprintf("Timeout waiting for resource to become healthy (waited %v, timeout %v): %s",
						elapsed.Round(time.Second), timeout, progressMsg)
					hasFailures = true

					// Update status with failure
					if updateErr := r.Status().Update(ctx, configPlan); updateErr != nil {
						logger.Error(updateErr, "Failed to update item timeout status")
					}

					// Check failure policy
					if configPlan.Spec.FailurePolicy == hcov1alpha1.FailurePolicyAbort {
						logger.Info("Aborting due to failure policy", "policy", configPlan.Spec.FailurePolicy)
						return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhaseFailed,
							fmt.Sprintf("Item '%s' timeout after %v", item.Name, elapsed.Round(time.Second)))
					}

					// Continue policy - keep going
					allCompleted = false
					continue
				}
			}

			// Still within timeout (or no timeout set) - update status and requeue
			logger.Info("Resource not yet healthy, will recheck on next reconciliation", "item", item.Name, "message", progressMsg)
			item.Message = progressMsg
			// Keep state as InProgress
			// Don't update LastTransitionTime - we want to track total wait time

			if err := r.Status().Update(ctx, configPlan); err != nil {
				logger.Error(err, "Failed to update item progress status")
			}

			// Mark as not completed so we requeue
			allCompleted = false
			continue
		}

		// Resource is healthy (or doesn't need waiting)
		now := metav1.Now()
		item.LastTransitionTime = &now
		item.State = hcov1alpha1.ItemStateCompleted
		item.Message = progressMsg

		if err := r.Status().Update(ctx, configPlan); err != nil {
			return fmt.Errorf("failed to update item success status: %w", err)
		}

		logger.Info("Successfully completed plan item", "item", item.Name, "message", progressMsg)
	}

	// Check overall completion status
	for i := range configPlan.Status.Items {
		if configPlan.Status.Items[i].State != hcov1alpha1.ItemStateCompleted &&
			configPlan.Status.Items[i].State != hcov1alpha1.ItemStateFailed {
			allCompleted = false
			break
		}
	}

	if allCompleted {
		// Compute snapshot hash of current cluster state for drift detection
		currentStateHash, err := plan.ComputeCurrentStateHash(ctx, r.Client, configPlan.Status.Items)
		if err != nil {
			logger.Error(err, "Failed to compute current state hash for drift detection")
			// Continue anyway - drift detection is not critical for completion
		} else {
			configPlan.Status.SourceSnapshotHash = currentStateHash
			logger.Info("Updated snapshot hash for drift detection", "hash", currentStateHash)
		}

		if hasFailures {
			return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhaseCompletedWithErrors,
				"Plan completed with some errors")
		}
		return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhaseCompleted,
			"All items applied successfully")
	}

	return nil
}

// handleCompletedPhase monitors for drift
func (r *ConfigurationPlanReconciler) handleCompletedPhase(ctx context.Context, configPlan *hcov1alpha1.ConfigurationPlan) error {
	logger := log.FromContext(ctx)
	logger.Info("Handling Completed phase - monitoring for drift")

	// Check if options have changed - if so, regenerate plan
	if optionsChanged(configPlan.Spec.Options, configPlan.Status.AppliedOptions) {
		logger.Info("Options have changed, regenerating plan")
		return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhaseDrafting,
			"Options changed - regenerating plan")
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
		return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhaseDrifted,
			"Configuration drift detected - cluster state has changed from applied configuration")
	}

	logger.V(1).Info("No drift detected, configuration is in sync")
	return nil
}

// handleDriftedPhase handles the drifted state
func (r *ConfigurationPlanReconciler) handleDriftedPhase(ctx context.Context, configPlan *hcov1alpha1.ConfigurationPlan) error {
	logger := log.FromContext(ctx)
	logger.Info("Handling Drifted phase")

	// In Drifted phase, we monitor whether drift has been corrected manually
	// or if the user wants to re-apply the configuration

	// Check if drift still exists
	drifted, err := plan.DetectDrift(ctx, r.Client, configPlan.Status.Items, configPlan.Status.SourceSnapshotHash)
	if err != nil {
		logger.Error(err, "Failed to detect drift")
		return nil // Don't fail the reconciliation
	}

	if !drifted {
		// Drift has been manually corrected
		logger.Info("Drift has been corrected, transitioning back to Completed")
		return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhaseCompleted,
			"Drift has been corrected - configuration is back in sync")
	}

	// If action is set to Apply, user wants to re-apply and fix the drift
	if configPlan.Spec.Action == hcov1alpha1.PlanActionApply {
		logger.Info("User requested re-apply to fix drift")
		// Reset all items to Pending
		for i := range configPlan.Status.Items {
			configPlan.Status.Items[i].State = hcov1alpha1.ItemStatePending
			configPlan.Status.Items[i].Message = "Re-applying to fix drift"
		}
		return r.updatePhase(ctx, configPlan, hcov1alpha1.PlanPhaseInProgress,
			"Re-applying configuration to fix drift")
	}

	logger.Info("Drift still present, waiting for manual correction or re-apply")
	return nil
}

// updatePhase updates the phase and adds a condition
func (r *ConfigurationPlanReconciler) updatePhase(ctx context.Context, configPlan *hcov1alpha1.ConfigurationPlan, newPhase hcov1alpha1.PlanPhase, message string) error {
	logger := log.FromContext(ctx)
	logger.Info("Updating phase", "from", configPlan.Status.Phase, "to", newPhase, "message", message)

	configPlan.Status.Phase = newPhase

	// Add a condition
	condition := metav1.Condition{
		Type:               string(newPhase),
		Status:             metav1.ConditionTrue,
		Reason:             string(newPhase),
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	// Update or append condition
	found := false
	for i, c := range configPlan.Status.Conditions {
		if c.Type == condition.Type {
			configPlan.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		configPlan.Status.Conditions = append(configPlan.Status.Conditions, condition)
	}

	return r.Status().Update(ctx, configPlan)
}

// optionsChanged compares two ProfileOptions to detect changes
func optionsChanged(current, applied *hcov1alpha1.ProfileOptions) bool {
	// Both nil - no change
	if current == nil && applied == nil {
		return false
	}

	// One is nil, other is not - changed
	if (current == nil) != (applied == nil) {
		return true
	}

	// Compare LoadAware config
	if (current.LoadAware == nil) != (applied.LoadAware == nil) {
		return true
	}

	if current.LoadAware != nil && applied.LoadAware != nil {
		// Compare fields
		if !int32PtrEqual(current.LoadAware.DeschedulingIntervalSeconds, applied.LoadAware.DeschedulingIntervalSeconds) {
			return true
		}
		if !boolPtrEqual(current.LoadAware.EnablePSIMetrics, applied.LoadAware.EnablePSIMetrics) {
			return true
		}
		if !stringPtrEqual(current.LoadAware.DevDeviationThresholds, applied.LoadAware.DevDeviationThresholds) {
			return true
		}
	}

	// Future: compare other profile configs (HighDensity, etc.)

	return false
}

// Helper functions for pointer comparison
func int32PtrEqual(a, b *int32) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func boolPtrEqual(a, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func stringPtrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConfigurationPlanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hcov1alpha1.ConfigurationPlan{}).
		Named("configurationplan").
		Complete(r)
}
