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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
	"github.com/kubevirt/virt-advisor-operator/internal/discovery"
	"github.com/kubevirt/virt-advisor-operator/internal/profiles"
	"github.com/kubevirt/virt-advisor-operator/internal/util"
)

// This file contains watch setup and predicate functions for the VirtPlatformConfig controller.
// It manages dynamic watches on managed resources and HCO configuration.

// ==================== Managed Resource Watches ====================

// managedResourcePredicate returns a predicate that filters events to only managed resources.
// This optimizes memory by only caching resources that are referenced in VirtPlatformConfig items.
func (r *VirtPlatformConfigReconciler) managedResourcePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return r.isManagedResource(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return r.isManagedResource(e.ObjectNew)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return r.isManagedResource(e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return r.isManagedResource(e.Object)
		},
	}
}

// isManagedResource checks if a resource is referenced in any VirtPlatformConfig items.

func (r *VirtPlatformConfigReconciler) isManagedResource(obj client.Object) bool {
	ctx := context.Background()

	// List all VirtPlatformConfigs
	configList := &advisorv1alpha1.VirtPlatformConfigList{}
	if err := r.List(ctx, configList); err != nil {
		return false
	}

	// Get the object's GVK, name, and namespace
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return false
	}

	objGVK := u.GroupVersionKind()
	objName := u.GetName()
	objNamespace := u.GetNamespace()

	// Check if this object is referenced in any VirtPlatformConfig items
	for _, config := range configList.Items {
		for _, item := range config.Status.Items {
			// Parse the targetRef apiVersion to get group and version
			ref := item.TargetRef
			if ref.Kind == objGVK.Kind && ref.Name == objName {
				// Check if apiVersion matches (handle group.io/v1 format)
				if ref.APIVersion == objGVK.GroupVersion().String() {
					// Check namespace (empty means cluster-scoped)
					if ref.Namespace == objNamespace {
						return true
					}
				}
			}
		}
	}

	return false
}

// enqueueManagedResourceOwners returns an event handler that maps managed resource events

func (r *VirtPlatformConfigReconciler) enqueueManagedResourceOwners() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		logger := log.FromContext(ctx)

		// Get the object's GVK, name, and namespace
		u, ok := obj.(*unstructured.Unstructured)
		if !ok {
			return nil
		}

		objGVK := u.GroupVersionKind()
		objName := u.GetName()
		objNamespace := u.GetNamespace()

		logger.V(1).Info("Managed resource changed, finding owner VirtPlatformConfigs",
			"gvk", objGVK.String(),
			"name", objName,
			"namespace", objNamespace)

		// List all VirtPlatformConfigs to find which ones manage this resource
		configList := &advisorv1alpha1.VirtPlatformConfigList{}
		if err := r.List(ctx, configList); err != nil {
			logger.Error(err, "Failed to list VirtPlatformConfigs for managed resource event")
			return nil
		}

		var requests []reconcile.Request
		for _, config := range configList.Items {
			for _, item := range config.Status.Items {
				ref := item.TargetRef
				if ref.Kind == objGVK.Kind && ref.Name == objName {
					// Check if apiVersion matches
					if ref.APIVersion == objGVK.GroupVersion().String() {
						// Check namespace
						if ref.Namespace == objNamespace {
							logger.Info("Enqueuing VirtPlatformConfig for managed resource change",
								"virtplatformconfig", config.Name,
								"managedResource", fmt.Sprintf("%s/%s", objGVK.Kind, objName))
							requests = append(requests, reconcile.Request{
								NamespacedName: types.NamespacedName{
									Name: config.Name,
									// VirtPlatformConfig is cluster-scoped
								},
							})
						}
					}
				}
			}
		}

		return requests
	})
}

// shouldRecreateAdvertisedProfile checks if a deleted VirtPlatformConfig should be recreated
// because it represents an advertisable profile.
func (r *VirtPlatformConfigReconciler) shouldRecreateAdvertisedProfile(profileName string) bool {
	// Check if this profile exists in the registry
	profile, err := profiles.DefaultRegistry.Get(profileName)
	if err != nil {
		// Not a registered profile, don't recreate
		return false
	}

	// Only recreate if it's advertisable
	return profile.IsAdvertisable()
}

// createAdvertisedProfile creates a single VirtPlatformConfig for an advertisable profile.
// This is used both during initialization and when recreating deleted advertised profiles.
// Returns (created, error) where created is true if a new object was created.
func (r *VirtPlatformConfigReconciler) createAdvertisedProfile(ctx context.Context, c client.Client, profileName string, logger logr.Logger) (bool, error) {
	profile, err := profiles.DefaultRegistry.Get(profileName)
	if err != nil {
		return false, fmt.Errorf("profile not found in registry: %w", err)
	}

	// Check if VirtPlatformConfig already exists
	existing := &advisorv1alpha1.VirtPlatformConfig{}
	err = c.Get(ctx, types.NamespacedName{Name: profileName}, existing)

	if err == nil {
		// Object exists - don't overwrite user modifications
		logger.V(1).Info("VirtPlatformConfig already exists, skipping creation",
			"profile", profileName)
		return false, nil
	}

	if !errors.IsNotFound(err) {
		return false, fmt.Errorf("failed to check for existing VirtPlatformConfig: %w", err)
	}

	// Create new VirtPlatformConfig with Ignore action
	vpc := &advisorv1alpha1.VirtPlatformConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: profileName,
			Annotations: map[string]string{
				"advisor.kubevirt.io/description":    profile.GetDescription(),
				"advisor.kubevirt.io/impact-summary": profile.GetImpactSummary(),
				"advisor.kubevirt.io/auto-created":   "true",
			},
			Labels: map[string]string{
				"advisor.kubevirt.io/category": profile.GetCategory(),
			},
		},
		Spec: advisorv1alpha1.VirtPlatformConfigSpec{
			Profile: profileName,
			Action:  advisorv1alpha1.PlanActionIgnore, // Safe by default
		},
	}

	if err := c.Create(ctx, vpc); err != nil {
		return false, fmt.Errorf("failed to create VirtPlatformConfig: %w", err)
	}

	logger.Info("Created advertised VirtPlatformConfig",
		"profile", profileName,
		"category", profile.GetCategory(),
		"description", profile.GetDescription())

	return true, nil
}

// initializeAdvertisedProfiles creates VirtPlatformConfig objects for all advertisable profiles
// with action=Ignore to enable profile discovery via kubectl get.
// This function is idempotent - it will not overwrite existing VirtPlatformConfigs.
// All errors are logged and handled internally - the function never fails.
func (r *VirtPlatformConfigReconciler) initializeAdvertisedProfiles(ctx context.Context, c client.Client, logger logr.Logger) {
	logger.Info("Initializing advertised profiles for discoverability")

	createdCount := 0
	skippedCount := 0

	// Get all registered profile names
	for _, profileName := range profiles.DefaultRegistry.List() {
		profile, err := profiles.DefaultRegistry.Get(profileName)
		if err != nil {
			logger.Error(err, "Failed to get profile from registry", "profile", profileName)
			continue
		}

		// Skip non-advertisable profiles (e.g., example-profile)
		if !profile.IsAdvertisable() {
			logger.V(1).Info("Skipping non-advertisable profile", "profile", profileName)
			skippedCount++
			continue
		}

		// Use the helper function to create the profile
		created, err := r.createAdvertisedProfile(ctx, c, profileName, logger)
		if err != nil {
			logger.Error(err, "Failed to create advertised VirtPlatformConfig", "profile", profileName)
			// Continue with other profiles instead of failing completely
			continue
		}

		if created {
			createdCount++
		} else {
			skippedCount++
		}
	}

	logger.Info("Profile advertising initialization complete",
		"created", createdCount,
		"skipped", skippedCount)
}

// enqueueMachineConfigPoolOwners returns an event handler that maps MachineConfigPool events

func (r *VirtPlatformConfigReconciler) enqueueMachineConfigPoolOwners() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		logger := log.FromContext(ctx)

		// Get the MachineConfigPool name (e.g., "worker", "master")
		poolName := obj.GetName()

		logger.V(1).Info("MachineConfigPool changed, finding related VirtPlatformConfigs",
			"pool", poolName)

		// List all VirtPlatformConfigs to find which ones have MachineConfig items for this pool
		configList := &advisorv1alpha1.VirtPlatformConfigList{}
		if err := r.List(ctx, configList); err != nil {
			logger.Error(err, "Failed to list VirtPlatformConfigs for MachineConfigPool event")
			return nil
		}

		var requests []reconcile.Request
		for _, config := range configList.Items {
			// Skip if not in a phase that cares about MCP status
			if config.Status.Phase != advisorv1alpha1.PlanPhaseInProgress &&
				config.Status.Phase != advisorv1alpha1.PlanPhaseCompleted &&
				config.Status.Phase != advisorv1alpha1.PlanPhaseDrafting {
				continue
			}

			// Check if any item is a MachineConfig targeting this pool
			for _, item := range config.Status.Items {
				if item.TargetRef.Kind == "MachineConfig" {
					// MachineConfigs are labeled with the pool they target
					// e.g., "machineconfiguration.openshift.io/role: worker"
					// We need to check if this MC's desired state has a label matching the pool

					// For simplicity, we can check if the item is InProgress or recently completed
					// and enqueue the VirtPlatformConfig to let it check health
					if item.State == advisorv1alpha1.ItemStateInProgress ||
						item.State == advisorv1alpha1.ItemStateCompleted {
						logger.Info("Enqueuing VirtPlatformConfig due to MachineConfigPool change",
							"virtplatformconfig", config.Name,
							"pool", poolName,
							"machineConfigItem", item.Name)
						requests = append(requests, reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name: config.Name,
								// VirtPlatformConfig is cluster-scoped
							},
						})
						break // Only enqueue once per VirtPlatformConfig
					}
				}
			}
		}

		return requests
	})
}

// ==================== HCO Configuration Watches ====================

// hcoConfigPredicate returns a predicate that triggers on HCO configuration changes.
// Watches: liveMigrationConfig, higherWorkloadDensity, ksmConfiguration.
// This filters out status updates and other spec changes to minimize reconciliation noise.
func (r *VirtPlatformConfigReconciler) hcoConfigPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// New HCO CR - always trigger to pick up initial config
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldObj, ok := e.ObjectOld.(*unstructured.Unstructured)
			if !ok {
				return false
			}
			newObj, ok := e.ObjectNew.(*unstructured.Unstructured)
			if !ok {
				return false
			}

			configChanged := false

			// Check liveMigrationConfig (load-aware-rebalancing profile)
			oldMig, _, _ := util.GetNestedMap(oldObj, "spec", "liveMigrationConfig")
			newMig, _, _ := util.GetNestedMap(newObj, "spec", "liveMigrationConfig")
			oldMigJSON, _ := json.Marshal(oldMig)
			newMigJSON, _ := json.Marshal(newMig)
			if string(oldMigJSON) != string(newMigJSON) {
				configChanged = true
			}

			// Check higherWorkloadDensity (virt-higher-density profile)
			oldDens, _, _ := util.GetNestedMap(oldObj, "spec", "higherWorkloadDensity")
			newDens, _, _ := util.GetNestedMap(newObj, "spec", "higherWorkloadDensity")
			oldDensJSON, _ := json.Marshal(oldDens)
			newDensJSON, _ := json.Marshal(newDens)
			if string(oldDensJSON) != string(newDensJSON) {
				configChanged = true
			}

			// Check ksmConfiguration (virt-higher-density profile)
			oldKSM, _, _ := util.GetNestedMap(oldObj, "spec", "ksmConfiguration")
			newKSM, _, _ := util.GetNestedMap(newObj, "spec", "ksmConfiguration")
			oldKSMJSON, _ := json.Marshal(oldKSM)
			newKSMJSON, _ := json.Marshal(newKSM)
			if string(oldKSMJSON) != string(newKSMJSON) {
				configChanged = true
			}

			return configChanged
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// HCO deleted - don't trigger (profiles will fail prerequisite checks anyway)
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}

// enqueueHCODependentConfigs returns an event handler that enqueues VirtPlatformConfigs
// that depend on HCO configuration (migration limits, memory overcommit, KSM).
//
//nolint:gocognit // Complex logic required to determine which configs depend on HCO
func (r *VirtPlatformConfigReconciler) enqueueHCODependentConfigs() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		logger := log.FromContext(ctx)

		logger.V(1).Info("HyperConverged config changed, finding dependent VirtPlatformConfigs")

		// List all VirtPlatformConfigs
		configList := &advisorv1alpha1.VirtPlatformConfigList{}
		if err := r.List(ctx, configList); err != nil {
			logger.Error(err, "Failed to list VirtPlatformConfigs for HyperConverged event")
			return nil
		}

		var requests []reconcile.Request
		for _, config := range configList.Items {
			shouldEnqueue := false

			// Only care about configs in active management or waiting for review
			// Skip Pending, Drafting, Failed, etc. - they'll pick up new config on next plan generation
			// Include ReviewRequired to enable drift auto-resolution when HCO changes back
			if config.Status.Phase != advisorv1alpha1.PlanPhaseCompleted &&
				config.Status.Phase != advisorv1alpha1.PlanPhaseCompletedWithErrors &&
				config.Status.Phase != advisorv1alpha1.PlanPhaseReviewRequired {
				continue
			}

			// Handle load-aware-rebalancing profile
			if config.Spec.Profile == "load-aware-rebalancing" {
				hasExplicitLimits := false
				if config.Spec.Options != nil && config.Spec.Options.LoadAware != nil {
					if config.Spec.Options.LoadAware.EvictionLimitTotal != nil ||
						config.Spec.Options.LoadAware.EvictionLimitNode != nil {
						hasExplicitLimits = true
					}
				}
				if !hasExplicitLimits {
					shouldEnqueue = true
					logger.V(1).Info("Enqueuing load-aware profile (relies on HCO eviction limits)",
						"virtplatformconfig", config.Name)
				}
			}

			// Handle virt-higher-density profile
			if config.Spec.Profile == "virt-higher-density" {
				hasExplicitConfig := false
				if config.Spec.Options != nil && config.Spec.Options.VirtHigherDensity != nil {
					cfg := config.Spec.Options.VirtHigherDensity
					if cfg.KSMConfiguration != nil || cfg.MemoryToRequestRatio != nil {
						hasExplicitConfig = true
					}
				}
				if !hasExplicitConfig {
					shouldEnqueue = true
					logger.V(1).Info("Enqueuing virt-higher-density profile (relies on HCO defaults)",
						"virtplatformconfig", config.Name)
				}
			}

			if shouldEnqueue {
				logger.Info("Enqueuing VirtPlatformConfig due to HyperConverged config change",
					"virtplatformconfig", config.Name,
					"profile", config.Spec.Profile)
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name: config.Name,
						// VirtPlatformConfig is cluster-scoped
					},
				})
			}
		}

		return requests
	})
}

// ==================== Controller Setup ====================

func (r *VirtPlatformConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := mgr.GetLogger().WithName("setup")
	ctx := context.Background()

	// Start building the controller
	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&advisorv1alpha1.VirtPlatformConfig{})

	// Dynamically register watches for all resource types managed by registered profiles
	// This implements a plugin mechanism where each profile declares what it needs to monitor
	managedGVKs := profiles.DefaultRegistry.GetAllManagedResourceTypes()

	logger.Info("Setting up dynamic watches for managed resources",
		"profileCount", len(profiles.DefaultRegistry.List()),
		"uniqueResourceTypes", len(managedGVKs))

	// IMPORTANT: Only register watches for resource types whose CRDs actually exist
	// This prevents controller-runtime startup failures when CRDs are missing
	// We need a non-cached client because the cache hasn't started yet during setup
	directClient, err := client.New(mgr.GetConfig(), client.Options{
		Scheme: mgr.GetScheme(),
	})
	if err != nil {
		return fmt.Errorf("failed to create direct client for CRD checking: %w", err)
	}

	crdChecker := discovery.NewCRDChecker(directClient)
	registeredWatchCount := 0
	skippedWatchCount := 0

	for _, gvk := range managedGVKs {
		// Check if the CRD exists before trying to watch it
		crdName := discovery.GVKToCRDName(gvk)
		exists, err := crdChecker.CRDExists(ctx, crdName)
		if err != nil {
			logger.Error(err, "Failed to check CRD existence, skipping watch",
				"crd", crdName,
				"group", gvk.Group,
				"version", gvk.Version,
				"kind", gvk.Kind)
			skippedWatchCount++
			continue
		}

		if !exists {
			logger.Info("Skipping watch for resource type - CRD not installed",
				"crd", crdName,
				"group", gvk.Group,
				"version", gvk.Version,
				"kind", gvk.Kind,
				"note", "Watch will not be registered. Profiles using this resource will fail prerequisite checks.")
			skippedWatchCount++
			continue
		}

		// CRD exists - safe to register watch
		resource := &unstructured.Unstructured{}
		resource.SetGroupVersionKind(gvk)

		logger.Info("Registering watch for resource type",
			"group", gvk.Group,
			"version", gvk.Version,
			"kind", gvk.Kind)

		// Add watch with predicate filtering and event handler
		controllerBuilder = controllerBuilder.Watches(
			resource,
			r.enqueueManagedResourceOwners(),
			builder.WithPredicates(r.managedResourcePredicate()),
		)
		registeredWatchCount++
	}

	logger.Info("Dynamic watch setup complete",
		"registeredWatches", registeredWatchCount,
		"skippedWatches", skippedWatchCount)

	// Register watch for MachineConfigPool (for health monitoring of MachineConfig items)
	// MachineConfigs don't directly expose their convergence status - we need to watch
	// the MachineConfigPool that processes them to know when nodes have been updated
	mcpGVK := schema.GroupVersionKind{
		Group:   "machineconfiguration.openshift.io",
		Version: "v1",
		Kind:    "MachineConfigPool",
	}
	mcpCRDName := discovery.GVKToCRDName(mcpGVK)
	mcpExists, err := crdChecker.CRDExists(ctx, mcpCRDName)
	if err == nil && mcpExists {
		logger.Info("Registering watch for MachineConfigPool (health monitoring)",
			"group", mcpGVK.Group,
			"version", mcpGVK.Version,
			"kind", mcpGVK.Kind)

		mcpResource := &unstructured.Unstructured{}
		mcpResource.SetGroupVersionKind(mcpGVK)

		controllerBuilder = controllerBuilder.Watches(
			mcpResource,
			r.enqueueMachineConfigPoolOwners(),
		)
	} else {
		logger.Info("MachineConfigPool CRD not available, skipping health monitoring watch",
			"note", "MachineConfig health checks will not auto-update")
	}

	// Register watch for HyperConverged (input dependency for load-aware profile)
	// When HCO migration limits change, we need to regenerate plans that rely on them
	hcoGVK := schema.GroupVersionKind{
		Group:   "hco.kubevirt.io",
		Version: "v1beta1",
		Kind:    "HyperConverged",
	}
	hcoCRDName := discovery.GVKToCRDName(hcoGVK)
	hcoExists, err := crdChecker.CRDExists(ctx, hcoCRDName)
	if err == nil && hcoExists {
		logger.Info("Registering watch for HyperConverged (input dependency)",
			"group", hcoGVK.Group,
			"version", hcoGVK.Version,
			"kind", hcoGVK.Kind)

		hcoResource := &unstructured.Unstructured{}
		hcoResource.SetGroupVersionKind(hcoGVK)

		controllerBuilder = controllerBuilder.Watches(
			hcoResource,
			r.enqueueHCODependentConfigs(),
			builder.WithPredicates(r.hcoConfigPredicate()),
		)
	} else {
		logger.Info("HyperConverged CRD not available, skipping input dependency watch",
			"note", "Changes to HCO migration limits will not trigger plan regeneration")
	}

	// Initialize advertised profiles using direct client (no cache dependency)
	// In production with proper RBAC, this works immediately during setup.
	// Note: In e2e tests there may be timing issues with RBAC propagation.
	r.initializeAdvertisedProfiles(ctx, directClient, logger)

	// Register controller with manager
	return controllerBuilder.Named("virtplatformconfig").Complete(r)
}
