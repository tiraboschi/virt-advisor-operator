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
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
)

var _ = Describe("VirtPlatformConfig Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "example-profile" // Must be a valid profile name per enum validation

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
			// No namespace - VirtPlatformConfig is cluster-scoped
		}
		configurationplan := &advisorv1alpha1.VirtPlatformConfig{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind VirtPlatformConfig")
			err := k8sClient.Get(ctx, typeNamespacedName, configurationplan)
			if err != nil && errors.IsNotFound(err) {
				resource := &advisorv1alpha1.VirtPlatformConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
						// No namespace - VirtPlatformConfig is cluster-scoped
					},
					Spec: advisorv1alpha1.VirtPlatformConfigSpec{
						Profile: resourceName, // Must match metadata.name per CEL validation
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("Cleanup the specific resource instance VirtPlatformConfig")
			resource := &advisorv1alpha1.VirtPlatformConfig{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &VirtPlatformConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When validating singleton pattern", func() {
		ctx := context.Background()

		AfterEach(func() {
			// Clean up any resources that might have been created
			resource := &advisorv1alpha1.VirtPlatformConfig{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "mismatched-name"}, resource)
			if err == nil {
				_ = k8sClient.Delete(ctx, resource)
			}
		})

		It("should reject VirtPlatformConfig when name does not match profile", func() {
			By("attempting to create a VirtPlatformConfig with mismatched name and profile")
			resource := &advisorv1alpha1.VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mismatched-name", // Name doesn't match profile
				},
				Spec: advisorv1alpha1.VirtPlatformConfigSpec{
					Profile: "example-profile", // Valid profile but name doesn't match
				},
			}

			err := k8sClient.Create(ctx, resource)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("To ensure a singleton pattern, the VirtPlatformConfig name must exactly match the spec.profile"))
		})

		It("should accept VirtPlatformConfig when name matches profile", func() {
			By("creating a VirtPlatformConfig with matching name and profile")
			const matchingName = "load-aware-rebalancing" // Must be a valid profile from enum
			resource := &advisorv1alpha1.VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: matchingName,
				},
				Spec: advisorv1alpha1.VirtPlatformConfigSpec{
					Profile: matchingName,
				},
			}

			err := k8sClient.Create(ctx, resource)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
	})

	Context("When handling PrerequisiteFailed phase", func() {
		const resourceName = "load-aware-rebalancing"
		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}

		BeforeEach(func() {
			By("creating a VirtPlatformConfig in PrerequisiteFailed phase")
			resource := &advisorv1alpha1.VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Spec: advisorv1alpha1.VirtPlatformConfigSpec{
					Profile: resourceName,
					Action:  advisorv1alpha1.PlanActionDryRun,
				},
			}

			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			// Set the resource to PrerequisiteFailed phase
			Eventually(func() error {
				err := k8sClient.Get(ctx, typeNamespacedName, resource)
				if err != nil {
					return err
				}
				resource.Status.Phase = advisorv1alpha1.PlanPhasePrerequisiteFailed
				resource.Status.ObservedGeneration = resource.Generation
				return k8sClient.Status().Update(ctx, resource)
			}).Should(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the VirtPlatformConfig")
			resource := &advisorv1alpha1.VirtPlatformConfig{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should automatically retry when prerequisites become available", func() {
			By("reconciling the resource in PrerequisiteFailed phase")
			controllerReconciler := &VirtPlatformConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconciliation should check prerequisites
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// If prerequisites are available (CRDs exist), it should transition to Pending
			// If prerequisites are missing, it should requeue after 5 minutes
			resource := &advisorv1alpha1.VirtPlatformConfig{}
			Eventually(func() advisorv1alpha1.PlanPhase {
				err := k8sClient.Get(ctx, typeNamespacedName, resource)
				if err != nil {
					return ""
				}
				return resource.Status.Phase
			}).Should(Or(
				Equal(advisorv1alpha1.PlanPhasePending),            // Prerequisites available
				Equal(advisorv1alpha1.PlanPhasePrerequisiteFailed), // Prerequisites still missing
			))

			// If prerequisites are available, verify it transitions through the phases
			if resource.Status.Phase == advisorv1alpha1.PlanPhasePending {
				By("verifying phase transitions after prerequisites become available")
				// Should have set requeue after to 0 (immediate retry)
				Expect(result.RequeueAfter).To(Equal(time.Duration(0)))
			} else {
				By("verifying requeue is set when prerequisites are still missing")
				// Should requeue after 5 minutes when prerequisites are missing
				Expect(result.RequeueAfter).To(Equal(5 * time.Minute))
			}
		})

		It("should not retry when spec generation has not changed", func() {
			By("ensuring ObservedGeneration matches current Generation")
			resource := &advisorv1alpha1.VirtPlatformConfig{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, resource)
				if err != nil {
					return false
				}
				resource.Status.ObservedGeneration = resource.Generation
				err = k8sClient.Status().Update(ctx, resource)
				return err == nil
			}).Should(BeTrue())

			By("reconciling without spec changes")
			controllerReconciler := &VirtPlatformConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Should either transition to Pending (if prerequisites available) or requeue
			err = k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			if resource.Status.Phase == advisorv1alpha1.PlanPhasePrerequisiteFailed {
				// Still in PrerequisiteFailed means prerequisites are missing
				Expect(result.RequeueAfter).To(Equal(5 * time.Minute))
			}
		})

		It("should retry immediately when spec generation changes", func() {
			By("changing the spec to trigger retry")
			resource := &advisorv1alpha1.VirtPlatformConfig{}
			Eventually(func() error {
				err := k8sClient.Get(ctx, typeNamespacedName, resource)
				if err != nil {
					return err
				}
				// Set ObservedGeneration to be out of sync
				resource.Status.ObservedGeneration = resource.Generation - 1
				return k8sClient.Status().Update(ctx, resource)
			}).Should(Succeed())

			By("reconciling with spec changes")
			controllerReconciler := &VirtPlatformConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Should transition to Pending when spec changes
			Eventually(func() advisorv1alpha1.PlanPhase {
				err := k8sClient.Get(ctx, typeNamespacedName, resource)
				if err != nil {
					return ""
				}
				return resource.Status.Phase
			}).Should(Equal(advisorv1alpha1.PlanPhasePending))
		})
	})

	Context("When optimistic locking detects stale plans", func() {
		const resourceName = "load-aware-rebalancing"
		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}

		// Helper function to create a realistic test item with proper TargetRef and DesiredState
		createTestItem := func(name string, state advisorv1alpha1.ItemState) advisorv1alpha1.VirtPlatformConfigItem {
			desiredState := map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "test-configmap",
					"namespace": "default",
				},
				"data": map[string]interface{}{
					"key": "value",
				},
			}
			rawJSON, _ := json.Marshal(desiredState)

			return advisorv1alpha1.VirtPlatformConfigItem{
				Name:  name,
				State: state,
				TargetRef: advisorv1alpha1.ObjectReference{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "test-configmap",
					Namespace:  "default",
				},
				DesiredState: &runtime.RawExtension{Raw: rawJSON},
				Message:      "Pending",
			}
		}

		BeforeEach(func() {
			By("creating a VirtPlatformConfig")
			resource := &advisorv1alpha1.VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: resourceName,
				},
				Spec: advisorv1alpha1.VirtPlatformConfigSpec{
					Profile: resourceName,
					Action:  advisorv1alpha1.PlanActionApply,
				},
			}

			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("cleaning up the VirtPlatformConfig")
			resource := &advisorv1alpha1.VirtPlatformConfig{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should abort execution when plan is stale", func() {
			By("setting up a plan with items in Pending state and a stored hash")
			resource := &advisorv1alpha1.VirtPlatformConfig{}
			Eventually(func() error {
				err := k8sClient.Get(ctx, typeNamespacedName, resource)
				if err != nil {
					return err
				}
				resource.Status.Phase = advisorv1alpha1.PlanPhaseInProgress
				resource.Status.SourceSnapshotHash = "original-hash-before-manual-changes"
				resource.Status.Items = []advisorv1alpha1.VirtPlatformConfigItem{
					createTestItem("test-item", advisorv1alpha1.ItemStatePending),
				}
				return k8sClient.Status().Update(ctx, resource)
			}).Should(Succeed())

			By("reconciling the InProgress phase with stale plan")
			controllerReconciler := &VirtPlatformConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// The reconciler should detect the hash mismatch and abort
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})

			// Should return an error about stale plan
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("plan is stale"))
			Expect(result).To(Equal(reconcile.Result{}))

			By("verifying the phase transitioned to Failed")
			Eventually(func() advisorv1alpha1.PlanPhase {
				err := k8sClient.Get(ctx, typeNamespacedName, resource)
				if err != nil {
					return ""
				}
				return resource.Status.Phase
			}).Should(Equal(advisorv1alpha1.PlanPhaseFailed))

			By("verifying the Failed condition has the detailed stale message")
			Eventually(func() string {
				err := k8sClient.Get(ctx, typeNamespacedName, resource)
				if err != nil {
					return ""
				}
				for _, cond := range resource.Status.Conditions {
					if cond.Type == "Failed" && cond.Status == metav1.ConditionTrue {
						return cond.Message
					}
				}
				return ""
			}).Should(ContainSubstring("Plan is stale: target resources have been modified"))

			By("verifying items were NOT executed (still in Pending state)")
			err = k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(resource.Status.Items).NotTo(BeEmpty())
			Expect(resource.Status.Items[0].State).To(Equal(advisorv1alpha1.ItemStatePending),
				"Item should still be Pending - execution should have been aborted")
		})

		It("should allow execution when bypass flag is set", func() {
			By("setting up a plan with bypass flag enabled")
			resource := &advisorv1alpha1.VirtPlatformConfig{}
			Eventually(func() error {
				err := k8sClient.Get(ctx, typeNamespacedName, resource)
				if err != nil {
					return err
				}
				resource.Spec.BypassOptimisticLock = true
				resource.Status.Phase = advisorv1alpha1.PlanPhaseInProgress
				resource.Status.SourceSnapshotHash = "original-hash-before-manual-changes"
				resource.Status.Items = []advisorv1alpha1.VirtPlatformConfigItem{
					createTestItem("test-item", advisorv1alpha1.ItemStatePending),
				}
				err = k8sClient.Update(ctx, resource)
				if err != nil {
					return err
				}
				return k8sClient.Status().Update(ctx, resource)
			}).Should(Succeed())

			By("reconciling with bypass flag set")
			controllerReconciler := &VirtPlatformConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// Should not return stale plan error (bypass is enabled)
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})

			// May have other errors (missing resources, etc.) but NOT the stale plan error
			if err != nil {
				Expect(err.Error()).NotTo(ContainSubstring("plan is stale"),
					"Bypass flag should prevent stale plan detection")
			}
		})

		It("should skip optimistic lock check when items are not all Pending", func() {
			By("setting up a plan where some items are already InProgress")
			resource := &advisorv1alpha1.VirtPlatformConfig{}
			Eventually(func() error {
				err := k8sClient.Get(ctx, typeNamespacedName, resource)
				if err != nil {
					return err
				}
				resource.Status.Phase = advisorv1alpha1.PlanPhaseInProgress
				resource.Status.SourceSnapshotHash = "some-hash"
				resource.Status.Items = []advisorv1alpha1.VirtPlatformConfigItem{
					createTestItem("item-1", advisorv1alpha1.ItemStateCompleted),
					createTestItem("item-2", advisorv1alpha1.ItemStatePending),
				}
				return k8sClient.Status().Update(ctx, resource)
			}).Should(Succeed())

			By("reconciling when not all items are Pending")
			controllerReconciler := &VirtPlatformConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// Should not abort with stale plan error
			// (optimistic lock only checks on FIRST entry to InProgress)
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})

			// May have other errors but NOT stale plan error
			if err != nil {
				Expect(err.Error()).NotTo(ContainSubstring("plan is stale"),
					"Optimistic lock should be skipped when items are already in progress")
			}
		})

		It("should skip optimistic lock check when SourceSnapshotHash is empty", func() {
			By("setting up a plan without a stored hash")
			resource := &advisorv1alpha1.VirtPlatformConfig{}
			Eventually(func() error {
				err := k8sClient.Get(ctx, typeNamespacedName, resource)
				if err != nil {
					return err
				}
				resource.Status.Phase = advisorv1alpha1.PlanPhaseInProgress
				resource.Status.SourceSnapshotHash = "" // No hash stored
				resource.Status.Items = []advisorv1alpha1.VirtPlatformConfigItem{
					createTestItem("test-item", advisorv1alpha1.ItemStatePending),
				}
				return k8sClient.Status().Update(ctx, resource)
			}).Should(Succeed())

			By("reconciling without a stored hash")
			controllerReconciler := &VirtPlatformConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// Should not abort with stale plan error
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})

			// May have other errors but NOT stale plan error
			if err != nil {
				Expect(err.Error()).NotTo(ContainSubstring("plan is stale"),
					"Optimistic lock should be skipped when no hash is stored")
			}
		})
	})
})
