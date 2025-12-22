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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
	"github.com/kubevirt/virt-advisor-operator/internal/discovery"
	"github.com/kubevirt/virt-advisor-operator/internal/plan"
	"github.com/kubevirt/virt-advisor-operator/internal/profiles"
	"github.com/kubevirt/virt-advisor-operator/internal/util"
)

const (
	// OperatorVersion is the current version of the operator logic
	OperatorVersion = "v0.1.31"
)

// Condition types
const (
	ConditionTypePending      = "Pending"
	ConditionTypeDrafting     = "Drafting"
	ConditionTypeInProgress   = "InProgress"
	ConditionTypeCompleted    = "Completed"
	ConditionTypeDrifted      = "Drifted"
	ConditionTypeFailed       = "Failed"
	ConditionTypeReviewReq    = "ReviewRequired"
	ConditionTypePrereqFail   = "PrerequisiteFailed"
	ConditionTypeCompletedErr = "CompletedWithErrors"
	ConditionTypeIgnored      = "Ignored"
	ConditionTypeUpgrade      = "UpgradeAvailable"
)

// Condition reasons
const (
	ReasonDrafting             = "Drafting"
	ReasonNotDrafting          = "NotDrafting"
	ReasonInProgress           = "InProgress"
	ReasonNotInProgress        = "NotInProgress"
	ReasonCompleted            = "Completed"
	ReasonNotCompleted         = "NotCompleted"
	ReasonReviewRequired       = "ReviewRequired"
	ReasonNoDrift              = "NoDrift"
	ReasonDriftDetected        = "DriftDetected"
	ReasonConfigurationDrifted = "ConfigurationDrifted"
	ReasonFailed               = "Failed"
	ReasonCompletedWithErrors  = "CompletedWithErrors"
	ReasonPrerequisitesFailed  = "PrerequisitesFailed"
	ReasonIgnored              = "Ignored"
	ReasonNotIgnored           = "NotIgnored"
)

// Condition messages
const (
	MessageDrafting                  = "Generating plan from profile"
	MessageNotDrafting               = "Plan generation completed"
	MessageNotDraftingFailed         = "Plan generation completed or failed"
	MessageNotDraftingPrereqFail     = "Cannot generate plan - prerequisites not met"
	MessageInProgress                = "Applying plan configuration"
	MessageNotInProgress             = "Plan is not being applied"
	MessageNotInProgressCompleted    = "Plan application completed"
	MessageNotInProgressCompletedErr = "Plan application completed with errors"
	MessageNotInProgressFailed       = "Plan application failed"
	MessageNotInProgressPrereqFail   = "Cannot apply - prerequisites not met"
	MessageNotInProgressDrifted      = "Waiting for manual intervention or aggressive remediation"
	MessageReviewRequired            = "Plan ready for review. Change action to 'Apply' to execute."
	MessageCompletedSuccess          = "Configuration successfully applied and in sync"
	MessageCompletedNotYetApplied    = "Plan not yet applied"
	MessageCompletedDrifted          = "Configuration is no longer in sync - drift detected"
	MessageCompletedWithErrors       = "Plan completed but some items failed"
	MessageCompletedFailed           = "Plan execution failed"
	MessageCompletedPrereqFailed     = "Required CRDs not installed"
	MessageNoDrift                   = "Configuration is in sync with desired state"
	MessageDriftDetected             = "Configuration has drifted from desired state"
	MessageIgnored                   = "Configuration is ignored - no management actions will be performed. Resources remain in their current state and are now under manual control. Change action to 'DryRun' or 'Apply' to resume operator management."
	MessageNotIgnored                = "Configuration is being managed"
	MessageNotApplicableIgnored      = "Not applicable - configuration is ignored"
)

// VirtPlatformConfigReconciler reconciles a VirtPlatformConfig object
type VirtPlatformConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=advisor.kubevirt.io,resources=virtplatformconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=advisor.kubevirt.io,resources=virtplatformconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=advisor.kubevirt.io,resources=virtplatformconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups=operator.openshift.io,resources=kubedeschedulers,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=machineconfiguration.openshift.io,resources=machineconfigs,verbs=get;list;watch;create;delete;patch
// +kubebuilder:rbac:groups=machineconfiguration.openshift.io,resources=machineconfigpools,verbs=get;list;watch
// +kubebuilder:rbac:groups=hco.kubevirt.io,resources=hyperconvergeds,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/reconcile
//
//nolint:gocognit // Reconcile is inherently complex due to state machine logic
func (r *VirtPlatformConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the VirtPlatformConfig
	configPlan := &advisorv1alpha1.VirtPlatformConfig{}
	if err := r.Get(ctx, req.NamespacedName, configPlan); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("VirtPlatformConfig resource not found", "name", req.Name)

			// Check if this was an advertisable profile that should be recreated
			if r.shouldRecreateAdvertisedProfile(req.Name) {
				logger.Info("Recreating deleted advertised profile", "profile", req.Name)
				if _, err := r.createAdvertisedProfile(ctx, r.Client, req.Name, logger); err != nil {
					logger.Error(err, "Failed to recreate advertised profile", "profile", req.Name)
					// Don't return error - this is a best-effort operation
				}
			}

			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get VirtPlatformConfig")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling VirtPlatformConfig",
		"profile", configPlan.Spec.Profile,
		"action", configPlan.Spec.Action,
		"phase", configPlan.Status.Phase)

	// Handle Ignore action: skip all management operations
	// IMPORTANT: Ignore does NOT rollback or undo changes previously applied by this profile.
	// Resources remain in their current state, and the cluster admin assumes manual control.
	// We simply stop all reconciliation (no drift detection, no plan generation, no apply).
	if configPlan.Spec.Action == advisorv1alpha1.PlanActionIgnore {
		// Set impact level in status even for ignored configs (needed for kubectl display)
		if err := r.ensureImpactLevel(ctx, configPlan); err != nil {
			return ctrl.Result{}, err
		}

		if configPlan.Status.Phase != advisorv1alpha1.PlanPhaseIgnored {
			logger.Info("Action is Ignore, transitioning to Ignored phase")
			return ctrl.Result{}, r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseIgnored,
				"Configuration ignored by administrator")
		}
		// Already in Ignored phase, nothing to do
		logger.V(1).Info("Configuration is ignored, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// If we were in Ignored phase but action changed, reset to Pending to regenerate plan
	if configPlan.Status.Phase == advisorv1alpha1.PlanPhaseIgnored {
		logger.Info("Action changed from Ignore, resetting to Pending to regenerate plan")
		return ctrl.Result{}, r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhasePending,
			"Action changed from Ignore, regenerating plan")
	}

	// Handle the reconciliation based on current phase
	var err error
	switch configPlan.Status.Phase {
	case "", advisorv1alpha1.PlanPhasePending:
		err = r.handlePendingPhase(ctx, configPlan)
	case advisorv1alpha1.PlanPhaseDrafting:
		err = r.handleDraftingPhase(ctx, configPlan)
	case advisorv1alpha1.PlanPhaseReviewRequired:
		err = r.handleReviewRequiredPhase(ctx, configPlan)
	case advisorv1alpha1.PlanPhaseInProgress:
		err = r.handleInProgressPhase(ctx, configPlan)
	case advisorv1alpha1.PlanPhaseCompleted:
		err = r.handleCompletedPhase(ctx, configPlan)
	case advisorv1alpha1.PlanPhaseCompletedWithErrors:
		err = r.handleCompletedPhase(ctx, configPlan)
	case advisorv1alpha1.PlanPhaseCompletedWithUpgrade:
		err = r.handleCompletedWithUpgradePhase(ctx, configPlan)
	case advisorv1alpha1.PlanPhaseDrifted:
		// Check if spec has changed (e.g., user changed action to DryRun for regeneration)
		if configPlan.Status.ObservedGeneration != configPlan.Generation {
			logger.Info("Spec changed since drift detection, regenerating plan",
				"observedGeneration", configPlan.Status.ObservedGeneration,
				"currentGeneration", configPlan.Generation)
			err = r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhasePending, "Regenerating after spec change")
		} else {
			err = r.handleDriftedPhase(ctx, configPlan)
		}
	case advisorv1alpha1.PlanPhaseFailed:
		// Check if spec has changed since failure (retry on spec change)
		if configPlan.Status.ObservedGeneration != configPlan.Generation {
			logger.Info("Spec changed since failure, retrying plan generation",
				"observedGeneration", configPlan.Status.ObservedGeneration,
				"currentGeneration", configPlan.Generation)
			err = r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhasePending, "Retrying after spec change")
		} else {
			// Failed plans don't automatically recover unless spec changes
			logger.Info("Plan is in Failed state, manual intervention required or change spec to retry")
			return ctrl.Result{}, nil
		}
	case advisorv1alpha1.PlanPhasePrerequisiteFailed:
		// Check if spec has changed since prerequisite failure (retry on spec change)
		if configPlan.Status.ObservedGeneration != configPlan.Generation {
			logger.Info("Spec changed since prerequisite failure, retrying",
				"observedGeneration", configPlan.Status.ObservedGeneration,
				"currentGeneration", configPlan.Generation)
			err = r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhasePending, "Retrying after spec change")
		} else {
			// Check if prerequisites are now available (may have been installed since last check)
			logger.V(1).Info("Checking if prerequisites are now available")
			profile, profileErr := profiles.DefaultRegistry.Get(configPlan.Spec.Profile)
			if profileErr == nil {
				checker := discovery.NewCRDChecker(r.Client)
				missing, checkErr := checker.CheckPrerequisites(ctx, profile.GetPrerequisites())
				if checkErr == nil {
					if len(missing) == 0 {
						// Prerequisites are now available! Reset to Pending to regenerate plan
						logger.Info("Prerequisites now available, retrying plan generation")
						err = r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhasePending, "Prerequisites now available")
						return ctrl.Result{}, err
					}
					// Some prerequisites still missing - update condition message to reflect current state
					logger.Info("Prerequisites still not met, updating condition message", "missing", len(missing))
					err = r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhasePrerequisiteFailed,
						discovery.FormatMissingPrerequisites(missing))
					return ctrl.Result{RequeueAfter: 2 * time.Minute}, err
				}
			}
			// Error checking prerequisites or profile not found, keep existing message
			logger.Info("Prerequisites not met, will retry periodically")
			return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
		}
	default:
		logger.Info("Unknown phase, resetting to Pending", "phase", configPlan.Status.Phase)
		err = r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhasePending, "Resetting unknown phase")
	}

	if err != nil {
		logger.Error(err, "Error during reconciliation")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// compareImpactLevels returns true if level1 is worse (higher) than level2
func compareImpactLevels(level1, level2 advisorv1alpha1.Impact) bool {
	severity := map[advisorv1alpha1.Impact]int{
		advisorv1alpha1.ImpactLow:    1,
		advisorv1alpha1.ImpactMedium: 2,
		advisorv1alpha1.ImpactHigh:   3,
	}
	return severity[level1] > severity[level2]
}

// aggregateImpactFromItems computes the worst impact level from all plan items
func aggregateImpactFromItems(items []advisorv1alpha1.VirtPlatformConfigItem) advisorv1alpha1.Impact {
	if len(items) == 0 {
		return ""
	}

	worstImpact := advisorv1alpha1.ImpactLow
	for _, item := range items {
		if item.ImpactSeverity == "" {
			continue
		}
		if compareImpactLevels(item.ImpactSeverity, worstImpact) {
			worstImpact = item.ImpactSeverity
		}
	}
	return worstImpact
}

// ensureImpactLevel updates the impact level in status based on plan items.
// If items exist, aggregates the worst impact from them.
// If no items yet, falls back to profile-level impact.
func (r *VirtPlatformConfigReconciler) ensureImpactLevel(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig) error {
	logger := log.FromContext(ctx)

	var impactLevel advisorv1alpha1.Impact

	// If we have plan items, compute impact from them
	if len(configPlan.Status.Items) > 0 {
		impactLevel = aggregateImpactFromItems(configPlan.Status.Items)
		logger.V(1).Info("Computed impact from plan items", "impact", impactLevel, "itemCount", len(configPlan.Status.Items))
	} else {
		// No items yet - fall back to profile-level impact
		profile, err := profiles.DefaultRegistry.Get(configPlan.Spec.Profile)
		if err != nil {
			// Profile not found - skip impact update (will fail later in drafting phase)
			return nil
		}
		impactLevel = profile.GetImpactLevel()
		logger.V(1).Info("Using profile-level impact (no items yet)", "impact", impactLevel)
	}

	if configPlan.Status.ImpactSeverity != impactLevel {
		configPlan.Status.ImpactSeverity = impactLevel
		if err := r.Status().Update(ctx, configPlan); err != nil {
			logger.Error(err, "Failed to update impact level in status")
			return err
		}
	}
	return nil
}

// isNoChangeDiff checks if a diff string indicates no actual changes
// Returns true if the diff shows "(no changes)" pattern
func isNoChangeDiff(diff string) bool {
	// Common patterns for "no changes" diffs:
	// - "(no changes)"
	// - Empty diff
	// - Only header lines with no actual changes
	if diff == "" {
		return true
	}
	if strings.Contains(diff, "(no changes)") {
		return true
	}
	// If diff has no + or - lines (only --- and +++ headers), no changes
	lines := strings.Split(diff, "\n")
	hasChanges := false
	for _, line := range lines {
		if len(line) > 0 && (line[0] == '+' || line[0] == '-') {
			// Exclude header lines (---, +++)
			if !strings.HasPrefix(line, "---") && !strings.HasPrefix(line, "+++") {
				hasChanges = true
				break
			}
		}
	}
	return !hasChanges
}

// areAllItemsPending checks if all items in the plan are still in pending state
func areAllItemsPending(items []advisorv1alpha1.VirtPlatformConfigItem) bool {
	for _, item := range items {
		if item.State != advisorv1alpha1.ItemStatePending {
			return false
		}
	}
	return true
}

// checkOptimisticLock verifies that target resources haven't changed since plan generation
func (r *VirtPlatformConfigReconciler) checkOptimisticLock(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig) error {
	logger := log.FromContext(ctx)

	// Only check on the FIRST entry to InProgress (when all items are still Pending)
	if !areAllItemsPending(configPlan.Status.Items) || configPlan.Status.SourceSnapshotHash == "" {
		return nil
	}

	// Check if bypass flag is set
	if configPlan.Spec.BypassOptimisticLock {
		logger.Info("WARNING: Optimistic lock check bypassed via spec.bypassOptimisticLock=true. "+
			"Applying configuration without verifying cluster state has not changed since plan generation. "+
			"This may overwrite manual changes or apply outdated configurations.",
			"bypassOptimisticLock", true)
		return nil
	}

	// Perform optimistic lock check
	currentHash, err := plan.ComputeCurrentStateHash(ctx, r.Client, configPlan.Status.Items)
	if err != nil {
		logger.Error(err, "Failed to compute current state hash for optimistic locking check")
		if updateErr := r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseFailed,
			fmt.Sprintf("Failed to verify cluster state before execution: %v", err)); updateErr != nil {
			return updateErr
		}
		// Return an error to stop further execution even if phase update succeeded
		return fmt.Errorf("hash computation failed - execution aborted: %w", err)
	}

	if currentHash != configPlan.Status.SourceSnapshotHash {
		logger.Info("Plan is stale! Target resources have been modified since plan generation",
			"stored_hash", configPlan.Status.SourceSnapshotHash,
			"current_hash", currentHash)
		if err := r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseFailed,
			"Plan is stale: target resources have been modified since plan generation. "+
				"Please regenerate the plan by changing action to DryRun, then review and re-approve."); err != nil {
			return err
		}
		// Return an error to stop further execution even if phase update succeeded
		return fmt.Errorf("plan is stale - execution aborted to prevent applying outdated configuration")
	}

	logger.V(1).Info("Optimistic lock verification passed - cluster state unchanged")
	return nil
}

// handleItemFailure updates item state to failed and checks failure policy
func (r *VirtPlatformConfigReconciler) handleItemFailure(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig, item *advisorv1alpha1.VirtPlatformConfigItem, errMsg string, err error) (bool, error) {
	logger := log.FromContext(ctx)

	now := metav1.Now()
	item.LastTransitionTime = &now
	item.State = advisorv1alpha1.ItemStateFailed
	item.Message = errMsg

	if updateErr := r.Status().Update(ctx, configPlan); updateErr != nil {
		logger.Error(updateErr, "Failed to update item failure status")
	}

	if configPlan.Spec.FailurePolicy == advisorv1alpha1.FailurePolicyAbort {
		logger.Info("Aborting due to failure policy", "policy", configPlan.Spec.FailurePolicy)
		return true, r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseFailed,
			fmt.Sprintf("Item '%s' failed: %v", item.Name, err))
	}

	return false, nil
}

// processItemExecution executes a plan item and handles the result
func (r *VirtPlatformConfigReconciler) processItemExecution(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig, item *advisorv1alpha1.VirtPlatformConfigItem) (shouldAbort bool, shouldContinue bool, err error) {
	logger := log.FromContext(ctx)

	// Mark as in progress
	if item.State == advisorv1alpha1.ItemStatePending {
		item.State = advisorv1alpha1.ItemStateInProgress
		item.Message = "Applying configuration..."
		now := metav1.Now()
		item.LastTransitionTime = &now

		if updateErr := r.Status().Update(ctx, configPlan); updateErr != nil {
			logger.Error(updateErr, "Failed to update item to InProgress", "item", item.Name)
		}
	}

	// Execute the item
	logger.Info("Executing plan item", "item", item.Name, "target", fmt.Sprintf("%s/%s", item.TargetRef.Kind, item.TargetRef.Name))
	execErr := plan.ExecuteItem(ctx, r.Client, item)

	if execErr != nil {
		logger.Error(execErr, "Failed to execute plan item", "item", item.Name)
		abort, err := r.handleItemFailure(ctx, configPlan, item, fmt.Sprintf("Failed to apply: %v", execErr), execErr)
		return abort, !abort, err
	}

	return false, false, nil
}

// checkItemHealth checks item health and handles wait/timeout logic
// Returns (shouldContinue, error) where shouldContinue indicates whether to continue to next item
func (r *VirtPlatformConfigReconciler) checkItemHealth(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig, item *advisorv1alpha1.VirtPlatformConfigItem) (bool, error) {
	logger := log.FromContext(ctx)

	waitManager := plan.NewWaitStrategyManager()
	progressMsg, isHealthy, needsWait, healthErr := waitManager.CheckHealthy(ctx, r.Client, item)

	if healthErr != nil {
		logger.Error(healthErr, "Failed to check resource health", "item", item.Name)
		abort, err := r.handleItemFailure(ctx, configPlan, item, fmt.Sprintf("Health check failed: %v", healthErr), healthErr)
		if abort {
			return false, err
		}
		return true, nil
	}

	if needsWait && !isHealthy {
		abort, shouldContinue, err := r.handleItemWaitTimeout(ctx, configPlan, item, progressMsg)
		if abort {
			return false, err
		}
		return shouldContinue, nil
	}

	// Resource is healthy (or doesn't need waiting)
	now := metav1.Now()
	item.LastTransitionTime = &now
	item.State = advisorv1alpha1.ItemStateCompleted
	item.Message = progressMsg

	// Regenerate diff to show current state vs. desired state
	// After successful apply, this should be empty (or show any drift that occurred)
	if err := r.refreshItemDiff(ctx, item); err != nil {
		logger.Error(err, "Failed to refresh diff after completion", "item", item.Name)
		// Non-fatal - continue with completion
	}

	if updateErr := r.Status().Update(ctx, configPlan); updateErr != nil {
		return false, fmt.Errorf("failed to update item success status: %w", updateErr)
	}

	logger.Info("Successfully completed plan item", "item", item.Name, "message", progressMsg)
	return false, nil
}

// handleItemWaitTimeout handles timeout logic for items waiting to become healthy
func (r *VirtPlatformConfigReconciler) handleItemWaitTimeout(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig, item *advisorv1alpha1.VirtPlatformConfigItem, progressMsg string) (shouldAbort bool, shouldContinue bool, err error) {
	logger := log.FromContext(ctx)

	// Check if we've exceeded the optional waitTimeout
	if configPlan.Spec.WaitTimeout != nil && item.LastTransitionTime != nil {
		elapsed := time.Since(item.LastTransitionTime.Time)
		timeout := configPlan.Spec.WaitTimeout.Duration

		if elapsed > timeout {
			logger.Info("Resource wait timeout exceeded", "item", item.Name, "elapsed", elapsed, "timeout", timeout)
			abort, err := r.handleItemFailure(ctx, configPlan, item,
				fmt.Sprintf("Timeout waiting for resource to become healthy (waited %v, timeout %v): %s",
					elapsed.Round(time.Second), timeout, progressMsg),
				fmt.Errorf("timeout after %v", elapsed.Round(time.Second)))
			return abort, !abort, err
		}
	}

	// Still within timeout (or no timeout set) - update status and requeue
	logger.Info("Resource not yet healthy, will recheck on next reconciliation", "item", item.Name, "message", progressMsg)
	item.Message = progressMsg

	if updateErr := r.Status().Update(ctx, configPlan); updateErr != nil {
		logger.Error(updateErr, "Failed to update item progress status")
	}

	return false, true, nil
}

// areAllItemsCompleted checks if all items are either completed or failed
func areAllItemsCompleted(items []advisorv1alpha1.VirtPlatformConfigItem) bool {
	for _, item := range items {
		if item.State != advisorv1alpha1.ItemStateCompleted && item.State != advisorv1alpha1.ItemStateFailed {
			return false
		}
	}
	return true
}

// finalizeInProgressPhase handles final completion logic with drift detection
func (r *VirtPlatformConfigReconciler) finalizeInProgressPhase(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig, hasFailures bool) error {
	logger := log.FromContext(ctx)

	// Compute snapshot hash of current cluster state for drift detection
	currentStateHash, err := plan.ComputeCurrentStateHash(ctx, r.Client, configPlan.Status.Items)
	if err != nil {
		logger.Error(err, "Failed to compute current state hash for drift detection")
		// Continue anyway - drift detection is not critical for completion
	} else {
		configPlan.Status.SourceSnapshotHash = currentStateHash
		logger.Info("Updated snapshot hash for drift detection", "hash", currentStateHash)
	}

	// Check if any completed items have drifted
	hasDrift := false
	for i := range configPlan.Status.Items {
		item := &configPlan.Status.Items[i]
		if item.State == advisorv1alpha1.ItemStateCompleted &&
			item.Message == "Configuration has drifted from desired state" {
			hasDrift = true
			logger.Info("Item has drifted", "item", item.Name)
			break
		}
	}

	if hasFailures {
		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseCompletedWithErrors,
			"Plan completed with some errors")
	}

	if hasDrift {
		return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseDrifted,
			"Configuration drift detected - one or more items have drifted from desired state")
	}

	return r.updatePhase(ctx, configPlan, advisorv1alpha1.PlanPhaseCompleted,
		"All items applied successfully")
}

// ensureCompletedPhaseConditions updates stale conditions when staying in Completed phase
func (r *VirtPlatformConfigReconciler) ensureCompletedPhaseConditions(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig) error {
	logger := log.FromContext(ctx)
	needsUpdate := false

	for i, c := range configPlan.Status.Conditions {
		switch c.Type {
		case ConditionTypeDrifted:
			if c.Status == metav1.ConditionTrue {
				configPlan.Status.Conditions[i].Status = metav1.ConditionFalse
				configPlan.Status.Conditions[i].Reason = ReasonNoDrift
				configPlan.Status.Conditions[i].Message = MessageNoDrift
				configPlan.Status.Conditions[i].LastTransitionTime = metav1.Now()
				needsUpdate = true
			}
		case ConditionTypeDrafting:
			if c.Status == metav1.ConditionTrue {
				configPlan.Status.Conditions[i].Status = metav1.ConditionFalse
				configPlan.Status.Conditions[i].Reason = ReasonNotDrafting
				configPlan.Status.Conditions[i].Message = MessageNotDrafting
				configPlan.Status.Conditions[i].LastTransitionTime = metav1.Now()
				needsUpdate = true
			}
		case ConditionTypeInProgress:
			if c.Status == metav1.ConditionTrue {
				configPlan.Status.Conditions[i].Status = metav1.ConditionFalse
				configPlan.Status.Conditions[i].Reason = ReasonNotInProgress
				configPlan.Status.Conditions[i].Message = MessageNotInProgressCompleted
				configPlan.Status.Conditions[i].LastTransitionTime = metav1.Now()
				needsUpdate = true
			}
		case ConditionTypeCompleted:
			if c.Status == metav1.ConditionFalse {
				configPlan.Status.Conditions[i].Status = metav1.ConditionTrue
				configPlan.Status.Conditions[i].Reason = ReasonCompleted
				configPlan.Status.Conditions[i].Message = MessageCompletedSuccess
				configPlan.Status.Conditions[i].LastTransitionTime = metav1.Now()
				needsUpdate = true
			}
		}
	}

	if needsUpdate {
		logger.Info("Updating phase-related conditions to reflect Completed state")
		return r.Status().Update(ctx, configPlan)
	}

	return nil
}

// checkItemDrift checks if a completed item has drifted from its desired state.
// If drift is detected, updates the item's diff and message to reflect the current state.
func (r *VirtPlatformConfigReconciler) checkItemDrift(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig, item *advisorv1alpha1.VirtPlatformConfigItem) error {
	logger := log.FromContext(ctx)

	// Refresh the diff to check for drift
	oldDiff := item.Diff
	if err := r.refreshItemDiff(ctx, item); err != nil {
		return fmt.Errorf("failed to refresh diff for drift detection: %w", err)
	}

	// Check if diff changed (indicating drift)
	hasDrift := item.Diff != oldDiff && item.Diff != "--- "+item.TargetRef.Name+" ("+item.TargetRef.Kind+")\n+++ "+item.TargetRef.Name+" ("+item.TargetRef.Kind+")\n(no changes)\n"

	if hasDrift {
		logger.Info("Drift detected on completed item", "item", item.Name, "target", fmt.Sprintf("%s/%s", item.TargetRef.Kind, item.TargetRef.Name))
		item.Message = "Configuration has drifted from desired state"

		// Update status to reflect drift
		if updateErr := r.Status().Update(ctx, configPlan); updateErr != nil {
			logger.Error(updateErr, "Failed to update item with drift status", "item", item.Name)
		}
	}

	return nil
}

// refreshItemDiff regenerates the diff for an item based on current cluster state vs. desired state.
// This is useful after applying changes to show if the apply succeeded or if drift has occurred.
func (r *VirtPlatformConfigReconciler) refreshItemDiff(ctx context.Context, item *advisorv1alpha1.VirtPlatformConfigItem) error {
	logger := log.FromContext(ctx)

	// Skip if no desired state (shouldn't happen, but be defensive)
	if item.DesiredState == nil {
		return fmt.Errorf("cannot refresh diff: item has no desired state")
	}

	// Convert desired state to unstructured
	desired := &unstructured.Unstructured{}
	if err := json.Unmarshal(item.DesiredState.Raw, &desired.Object); err != nil {
		return fmt.Errorf("failed to unmarshal desired state: %w", err)
	}

	// Regenerate the diff using SSA dry-run
	// This shows current state vs. desired state
	// If apply succeeded and there's no drift, diff should be empty or minimal
	newDiff, err := plan.GenerateSSADiff(ctx, r.Client, desired, "virt-advisor-operator")
	if err != nil {
		return fmt.Errorf("failed to regenerate diff: %w", err)
	}

	// Update the item's diff
	item.Diff = newDiff
	logger.V(1).Info("Refreshed diff for item", "item", item.Name, "diffLength", len(newDiff))

	return nil
}

// updatePhase updates the phase and adds a condition
func (r *VirtPlatformConfigReconciler) updatePhase(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig, newPhase advisorv1alpha1.PlanPhase, message string) error {
	logger := log.FromContext(ctx)
	logger.Info("Updating phase", "from", configPlan.Status.Phase, "to", newPhase, "message", message)

	configPlan.Status.Phase = newPhase

	// Update observedGeneration to track which spec version we're processing
	// This enables automatic retry when spec changes
	configPlan.Status.ObservedGeneration = configPlan.Generation

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

	// Set all phase-related conditions using the declarative helper method
	// Pass the custom message for error states (Failed, PrerequisiteFailed)
	r.setPhaseConditions(configPlan, newPhase, message)

	return r.Status().Update(ctx, configPlan)
}

// setCondition sets or updates a specific condition
func (r *VirtPlatformConfigReconciler) setCondition(configPlan *advisorv1alpha1.VirtPlatformConfig, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	// Update or append condition
	found := false
	for i, c := range configPlan.Status.Conditions {
		if c.Type == condition.Type {
			// Only update if status changed or message changed
			if c.Status != condition.Status || c.Message != condition.Message {
				configPlan.Status.Conditions[i] = condition
			}
			found = true
			break
		}
	}
	if !found {
		configPlan.Status.Conditions = append(configPlan.Status.Conditions, condition)
	}
}

// clearAllErrorConditions sets all error-related conditions to False
func (r *VirtPlatformConfigReconciler) clearAllErrorConditions(configPlan *advisorv1alpha1.VirtPlatformConfig, reason, message string) {
	r.setCondition(configPlan, ConditionTypePrereqFail, metav1.ConditionFalse, reason, message)
	r.setCondition(configPlan, ConditionTypeFailed, metav1.ConditionFalse, reason, message)
}

// clearAllActiveConditions sets all active/in-progress conditions to False
func (r *VirtPlatformConfigReconciler) clearAllActiveConditions(configPlan *advisorv1alpha1.VirtPlatformConfig, reason, message string) {
	r.setCondition(configPlan, ConditionTypeDrafting, metav1.ConditionFalse, reason, message)
	r.setCondition(configPlan, ConditionTypeInProgress, metav1.ConditionFalse, reason, message)
	r.setCondition(configPlan, ConditionTypeReviewReq, metav1.ConditionFalse, reason, message)
}

// clearAllCompletionConditions sets all completion-related conditions to False
func (r *VirtPlatformConfigReconciler) clearAllCompletionConditions(configPlan *advisorv1alpha1.VirtPlatformConfig, reason, message string) {
	r.setCondition(configPlan, ConditionTypeCompleted, metav1.ConditionFalse, reason, message)
	r.setCondition(configPlan, ConditionTypeDrifted, metav1.ConditionFalse, reason, message)
}

// setPhaseConditions sets all conditions for a given phase in a declarative way
// customMessage is used for error states (Failed, PrerequisiteFailed) to preserve detailed error information
func (r *VirtPlatformConfigReconciler) setPhaseConditions(configPlan *advisorv1alpha1.VirtPlatformConfig, phase advisorv1alpha1.PlanPhase, customMessage string) {
	switch phase {
	case advisorv1alpha1.PlanPhasePending:
		r.setCondition(configPlan, ConditionTypeIgnored, metav1.ConditionFalse, ReasonNotIgnored, MessageNotIgnored)
		r.clearAllActiveConditions(configPlan, ReasonNotIgnored, MessageNotIgnored)
		r.clearAllCompletionConditions(configPlan, ReasonNotIgnored, MessageNotIgnored)
		r.clearAllErrorConditions(configPlan, ReasonNotIgnored, MessageNotIgnored)

	case advisorv1alpha1.PlanPhaseDrafting:
		r.setCondition(configPlan, ConditionTypeIgnored, metav1.ConditionFalse, ReasonNotIgnored, MessageNotIgnored)
		r.setCondition(configPlan, ConditionTypeDrafting, metav1.ConditionTrue, ReasonDrafting, MessageDrafting)
		r.setCondition(configPlan, ConditionTypeInProgress, metav1.ConditionFalse, ReasonNotInProgress, MessageNotInProgress)
		r.setCondition(configPlan, ConditionTypeReviewReq, metav1.ConditionFalse, ReasonNotDrafting, MessageNotDrafting)
		r.clearAllCompletionConditions(configPlan, ReasonNotDrafting, MessageNotDrafting)
		r.clearAllErrorConditions(configPlan, ReasonNotDrafting, MessageNotDrafting)

	case advisorv1alpha1.PlanPhaseReviewRequired:
		r.setCondition(configPlan, ConditionTypeIgnored, metav1.ConditionFalse, ReasonNotIgnored, MessageNotIgnored)
		r.setCondition(configPlan, ConditionTypeDrafting, metav1.ConditionFalse, ReasonNotDrafting, MessageNotDrafting)
		r.setCondition(configPlan, ConditionTypeInProgress, metav1.ConditionFalse, ReasonNotInProgress, MessageNotInProgress)
		r.setCondition(configPlan, ConditionTypeReviewReq, metav1.ConditionTrue, ReasonReviewRequired, MessageReviewRequired)
		r.setCondition(configPlan, ConditionTypeCompleted, metav1.ConditionFalse, ReasonNotCompleted, MessageCompletedNotYetApplied)
		r.setCondition(configPlan, ConditionTypeDrifted, metav1.ConditionFalse, ReasonNoDrift, MessageNoDrift)
		r.clearAllErrorConditions(configPlan, ReasonReviewRequired, MessageReviewRequired)

	case advisorv1alpha1.PlanPhaseInProgress:
		r.setCondition(configPlan, ConditionTypeIgnored, metav1.ConditionFalse, ReasonNotIgnored, MessageNotIgnored)
		r.setCondition(configPlan, ConditionTypeDrafting, metav1.ConditionFalse, ReasonNotDrafting, MessageNotDrafting)
		r.setCondition(configPlan, ConditionTypeInProgress, metav1.ConditionTrue, ReasonInProgress, MessageInProgress)
		r.setCondition(configPlan, ConditionTypeReviewReq, metav1.ConditionFalse, ReasonInProgress, MessageInProgress)
		r.clearAllCompletionConditions(configPlan, ReasonInProgress, MessageInProgress)
		r.clearAllErrorConditions(configPlan, ReasonInProgress, MessageInProgress)

	case advisorv1alpha1.PlanPhaseCompleted:
		r.setCondition(configPlan, ConditionTypeIgnored, metav1.ConditionFalse, ReasonNotIgnored, MessageNotIgnored)
		r.setCondition(configPlan, ConditionTypePending, metav1.ConditionFalse, ReasonNotCompleted, MessageCompletedSuccess)
		r.setCondition(configPlan, ConditionTypeDrafting, metav1.ConditionFalse, ReasonNotDrafting, MessageNotDrafting)
		r.setCondition(configPlan, ConditionTypeInProgress, metav1.ConditionFalse, ReasonNotInProgress, MessageNotInProgressCompleted)
		r.setCondition(configPlan, ConditionTypeReviewReq, metav1.ConditionFalse, ReasonNotCompleted, MessageCompletedSuccess)
		r.setCondition(configPlan, ConditionTypeCompleted, metav1.ConditionTrue, ReasonCompleted, MessageCompletedSuccess)
		r.setCondition(configPlan, ConditionTypeDrifted, metav1.ConditionFalse, ReasonNoDrift, MessageNoDrift)
		r.setCondition(configPlan, ConditionTypePrereqFail, metav1.ConditionFalse, ReasonCompleted, "All prerequisites met")
		r.setCondition(configPlan, ConditionTypeFailed, metav1.ConditionFalse, ReasonCompleted, "No failures occurred")

	case advisorv1alpha1.PlanPhaseDrifted:
		r.setCondition(configPlan, ConditionTypeIgnored, metav1.ConditionFalse, ReasonNotIgnored, MessageNotIgnored)
		r.setCondition(configPlan, ConditionTypePending, metav1.ConditionFalse, ReasonDriftDetected, MessageDriftDetected)
		r.clearAllActiveConditions(configPlan, ReasonDriftDetected, MessageDriftDetected)
		r.setCondition(configPlan, ConditionTypeDrifted, metav1.ConditionTrue, ReasonDriftDetected, MessageDriftDetected)
		r.setCondition(configPlan, ConditionTypeCompleted, metav1.ConditionFalse, ReasonConfigurationDrifted, MessageCompletedDrifted)
		r.clearAllErrorConditions(configPlan, ReasonDriftDetected, MessageDriftDetected)

	case advisorv1alpha1.PlanPhaseCompletedWithErrors:
		r.setCondition(configPlan, ConditionTypeIgnored, metav1.ConditionFalse, ReasonNotIgnored, MessageNotIgnored)
		r.setCondition(configPlan, ConditionTypePending, metav1.ConditionFalse, ReasonCompletedWithErrors, MessageCompletedWithErrors)
		r.clearAllActiveConditions(configPlan, ReasonCompletedWithErrors, MessageCompletedWithErrors)
		r.setCondition(configPlan, ConditionTypeCompleted, metav1.ConditionFalse, ReasonCompletedWithErrors, MessageCompletedWithErrors)
		r.setCondition(configPlan, ConditionTypeDrifted, metav1.ConditionFalse, ReasonCompletedWithErrors, MessageCompletedWithErrors)
		r.setCondition(configPlan, ConditionTypePrereqFail, metav1.ConditionFalse, ReasonCompletedWithErrors, MessageCompletedWithErrors)
		// Note: Failed condition is NOT cleared here - CompletedWithErrors is a partial failure state

	case advisorv1alpha1.PlanPhaseCompletedWithUpgrade:
		r.setCondition(configPlan, ConditionTypeIgnored, metav1.ConditionFalse, ReasonNotIgnored, MessageNotIgnored)
		r.setCondition(configPlan, ConditionTypePending, metav1.ConditionFalse, ReasonNotCompleted, MessageCompletedSuccess)
		r.setCondition(configPlan, ConditionTypeDrafting, metav1.ConditionFalse, ReasonNotDrafting, MessageNotDrafting)
		r.setCondition(configPlan, ConditionTypeInProgress, metav1.ConditionFalse, ReasonNotInProgress, MessageNotInProgressCompleted)
		r.setCondition(configPlan, ConditionTypeReviewReq, metav1.ConditionFalse, ReasonNotCompleted, MessageCompletedSuccess)
		r.setCondition(configPlan, ConditionTypeCompleted, metav1.ConditionTrue, ReasonCompleted, MessageCompletedSuccess)
		r.setCondition(configPlan, ConditionTypeDrifted, metav1.ConditionFalse, ReasonNoDrift, MessageNoDrift)
		r.setCondition(configPlan, ConditionTypePrereqFail, metav1.ConditionFalse, ReasonCompleted, "All prerequisites met")
		r.setCondition(configPlan, ConditionTypeFailed, metav1.ConditionFalse, ReasonCompleted, "No failures occurred")
		// Note: UpgradeAvailable condition is set separately in handleCompletedPhase logic

	case advisorv1alpha1.PlanPhaseFailed:
		// Use customMessage if provided, otherwise use default
		failedMessage := MessageCompletedFailed
		if customMessage != "" {
			failedMessage = customMessage
		}
		r.setCondition(configPlan, ConditionTypeIgnored, metav1.ConditionFalse, ReasonNotIgnored, MessageNotIgnored)
		r.setCondition(configPlan, ConditionTypePending, metav1.ConditionFalse, ReasonFailed, failedMessage)
		r.clearAllActiveConditions(configPlan, ReasonFailed, failedMessage)
		r.clearAllCompletionConditions(configPlan, ReasonFailed, failedMessage)
		r.setCondition(configPlan, ConditionTypeFailed, metav1.ConditionTrue, ReasonFailed, failedMessage)
		r.setCondition(configPlan, ConditionTypePrereqFail, metav1.ConditionFalse, ReasonFailed, failedMessage)

	case advisorv1alpha1.PlanPhasePrerequisiteFailed:
		// Use customMessage if provided, otherwise use default
		prereqMessage := MessageCompletedPrereqFailed
		if customMessage != "" {
			prereqMessage = customMessage
		}
		r.setCondition(configPlan, ConditionTypeIgnored, metav1.ConditionFalse, ReasonNotIgnored, MessageNotIgnored)
		r.setCondition(configPlan, ConditionTypePending, metav1.ConditionFalse, ReasonPrerequisitesFailed, prereqMessage)
		r.clearAllActiveConditions(configPlan, ReasonPrerequisitesFailed, prereqMessage)
		r.clearAllCompletionConditions(configPlan, ReasonPrerequisitesFailed, prereqMessage)
		r.setCondition(configPlan, ConditionTypePrereqFail, metav1.ConditionTrue, ReasonPrerequisitesFailed, prereqMessage)
		r.setCondition(configPlan, ConditionTypeFailed, metav1.ConditionFalse, ReasonPrerequisitesFailed, prereqMessage)

	case advisorv1alpha1.PlanPhaseIgnored:
		r.setCondition(configPlan, ConditionTypeIgnored, metav1.ConditionTrue, ReasonIgnored, MessageIgnored)
		r.clearAllActiveConditions(configPlan, ReasonIgnored, MessageNotApplicableIgnored)
		r.clearAllCompletionConditions(configPlan, ReasonIgnored, MessageNotApplicableIgnored)
		r.clearAllErrorConditions(configPlan, ReasonIgnored, MessageNotApplicableIgnored)
	}
}

// optionsChanged compares two ProfileOptions to detect changes
func optionsChanged(current, applied *advisorv1alpha1.ProfileOptions) bool {
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
		if !util.PtrEqual(current.LoadAware.DeschedulingIntervalSeconds, applied.LoadAware.DeschedulingIntervalSeconds) {
			return true
		}
		if !util.PtrEqual(current.LoadAware.EnablePSIMetrics, applied.LoadAware.EnablePSIMetrics) {
			return true
		}
		if !util.PtrEqual(current.LoadAware.DevDeviationThresholds, applied.LoadAware.DevDeviationThresholds) {
			return true
		}
	}

	// Future: compare other profile configs (HighDensity, etc.)

	return false
}

// hcoInputDependenciesChanged checks if HCO-derived eviction limits have changed.
// Only applies to load-aware-rebalancing profile when user hasn't provided explicit limits.
// Returns (changed bool, reason string).
func (r *VirtPlatformConfigReconciler) generatePlanItemsWithNewLogic(ctx context.Context, configPlan *advisorv1alpha1.VirtPlatformConfig) ([]advisorv1alpha1.VirtPlatformConfigItem, error) {
	profile, err := profiles.DefaultRegistry.Get(configPlan.Spec.Profile)
	if err != nil {
		return nil, fmt.Errorf("profile not found: %w", err)
	}

	// Convert ProfileOptions to map for backward compatibility
	configMap := profiles.OptionsToMap(configPlan.Spec.Profile, configPlan.Spec.Options)

	// Generate items using current logic
	items, err := profile.GeneratePlanItems(ctx, r.Client, configMap)
	if err != nil {
		return nil, fmt.Errorf("failed to generate plan items: %w", err)
	}

	return items, nil
}

// planItemsDiffer compares two sets of plan items to detect meaningful changes.
// Returns true if the configuration differs (not just execution state).
func planItemsDiffer(current, applied []advisorv1alpha1.VirtPlatformConfigItem) bool {
	// Different number of items = definitely different
	if len(current) != len(applied) {
		return true
	}

	// Build a map of applied items for efficient lookup
	appliedMap := make(map[string]*advisorv1alpha1.VirtPlatformConfigItem)
	for i := range applied {
		appliedMap[applied[i].Name] = &applied[i]
	}

	// Compare each current item against applied
	for i := range current {
		currentItem := &current[i]
		appliedItem, exists := appliedMap[currentItem.Name]

		if !exists {
			// New item added
			return true
		}

		// Compare meaningful fields (ignore execution state/timestamps)
		if itemConfigDiffers(currentItem, appliedItem) {
			return true
		}
	}

	return false
}

// itemConfigDiffers compares the configuration aspects of two items.
// Ignores execution state (State, Message, LastTransitionTime).
func itemConfigDiffers(current, applied *advisorv1alpha1.VirtPlatformConfigItem) bool {
	// Compare target reference
	if current.TargetRef != applied.TargetRef {
		return true
	}

	// Compare impact severity
	if current.ImpactSeverity != applied.ImpactSeverity {
		return true
	}

	// Compare desired state (the actual configuration)
	if current.DesiredState == nil && applied.DesiredState != nil {
		return true
	}
	if current.DesiredState != nil && applied.DesiredState == nil {
		return true
	}
	if current.DesiredState != nil && applied.DesiredState != nil {
		// Deep compare the JSON
		currentJSON, _ := json.Marshal(current.DesiredState)
		appliedJSON, _ := json.Marshal(applied.DesiredState)
		if string(currentJSON) != string(appliedJSON) {
			return true
		}
	}

	// Compare managed fields
	if len(current.ManagedFields) != len(applied.ManagedFields) {
		return true
	}
	for i := range current.ManagedFields {
		if current.ManagedFields[i] != applied.ManagedFields[i] {
			return true
		}
	}

	return false
}

// This is used to filter watched resources and minimize cache memory usage.
// to reconciliation requests for their owning VirtPlatformConfigs.
// because it represents an advertisable profile.
// Returns (created, error) where created is true if a new object was created.
// All errors are logged and handled internally - the function never fails.
// to reconciliation requests for VirtPlatformConfigs that have MachineConfig items targeting that pool.

// SetupWithManager sets up the controller with the Manager.
