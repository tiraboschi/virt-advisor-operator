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

// Integration tests for operator upgrade detection
// These specs are picked up by the main TestControllers suite in suite_test.go

var _ = Describe("Operator Upgrade Detection", Serial, func() {
	Context("When operator version changes", func() {
		ctx := context.Background()
		const timeout = 30 * time.Second
		const interval = 250 * time.Millisecond
		const vpcName = "example-profile" // Must match spec.profile

		AfterEach(func() {
			vpc := &advisorv1alpha1.VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{Name: vpcName},
			}
			_ = k8sClient.Delete(ctx, vpc)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: vpcName}, vpc)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("should transition to CompletedWithUpgrade when profile logic changes", func() {
			By("Creating a VirtPlatformConfig")
			vpc := &advisorv1alpha1.VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{Name: vpcName},
				Spec: advisorv1alpha1.VirtPlatformConfigSpec{
					Profile: "example-profile",
					Action:  advisorv1alpha1.PlanActionApply,
				},
			}
			Expect(k8sClient.Create(ctx, vpc)).To(Succeed())

			By("Manually setting Completed phase with OLD version plan items")
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: vpcName}, vpc); err != nil {
					return err
				}
				vpc.Status.Phase = advisorv1alpha1.PlanPhaseCompleted
				vpc.Status.OperatorVersion = OperatorVersion
				vpc.Status.ObservedGeneration = vpc.Generation
				// Items represent what the OLD operator version generated
				// Different from what current example-profile generates (different item name)
				vpc.Status.Items = []advisorv1alpha1.VirtPlatformConfigItem{
					{
						Name: "configure-old-component", // OLD: different name
						TargetRef: advisorv1alpha1.ObjectReference{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       "example-config",
							Namespace:  "openshift-cnv",
						},
						ImpactSeverity: advisorv1alpha1.ImpactLow,
						State:          advisorv1alpha1.ItemStatePending,
						Message:        "Waiting to apply configuration",
					},
				}
				return k8sClient.Status().Update(ctx, vpc)
			}, timeout, interval).Should(Succeed())

			By("Simulating operator upgrade")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: vpcName}, vpc)).To(Succeed())
			vpc.Status.OperatorVersion = "v0.1.30"
			Expect(k8sClient.Status().Update(ctx, vpc)).To(Succeed())

			By("Manually calling reconciler to trigger upgrade detection")
			// Status changes don't auto-trigger reconciliation in tests, so call it manually
			controllerReconciler := &VirtPlatformConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: vpcName},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying transition to CompletedWithUpgrade")
			Eventually(func() advisorv1alpha1.PlanPhase {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: vpcName}, vpc)
				return vpc.Status.Phase
			}, timeout, interval).Should(Equal(advisorv1alpha1.PlanPhaseCompletedWithUpgrade))

			By("Verifying UpgradeAvailable condition is set")
			Eventually(func() bool {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: vpcName}, vpc)
				for _, c := range vpc.Status.Conditions {
					if c.Type == ConditionTypeUpgrade && c.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		It("should silently update version when profile logic is identical", func() {
			By("Creating a VirtPlatformConfig")
			vpc := &advisorv1alpha1.VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{Name: vpcName},
				Spec: advisorv1alpha1.VirtPlatformConfigSpec{
					Profile: "example-profile",
					Action:  advisorv1alpha1.PlanActionApply,
				},
			}
			Expect(k8sClient.Create(ctx, vpc)).To(Succeed())

			By("Manually setting Completed phase with items matching current profile logic")
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: vpcName}, vpc); err != nil {
					return err
				}
				vpc.Status.Phase = advisorv1alpha1.PlanPhaseCompleted
				vpc.Status.OperatorVersion = OperatorVersion
				vpc.Status.ObservedGeneration = vpc.Generation
				// Items EXACTLY match what the current example-profile generates
				// This simulates both old and new operator versions generating identical config

				// Create the DesiredState matching what example-profile generates
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
					return err
				}

				vpc.Status.Items = []advisorv1alpha1.VirtPlatformConfigItem{
					{
						Name: "configure-example-component", // NEW: same as current profile
						TargetRef: advisorv1alpha1.ObjectReference{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       "example-config",
							Namespace:  "openshift-cnv",
						},
						ImpactSeverity: advisorv1alpha1.ImpactLow,
						State:          advisorv1alpha1.ItemStatePending,
						Message:        "Waiting to apply configuration",
						// These fields must match what example-profile generates
						Diff:         "--- example-config (ConfigMap)\n+++ example-config (ConfigMap)\n+  data:\n+    example-key: example-value\n",
						DesiredState: &runtime.RawExtension{Raw: rawJSON},
					},
				}
				return k8sClient.Status().Update(ctx, vpc)
			}, timeout, interval).Should(Succeed())

			By("Simulating upgrade with same logic")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: vpcName}, vpc)).To(Succeed())
			vpc.Status.OperatorVersion = "v0.1.30"
			Expect(k8sClient.Status().Update(ctx, vpc)).To(Succeed())

			By("Manually calling reconciler to trigger upgrade detection")
			controllerReconciler := &VirtPlatformConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: vpcName},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying silent version update")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: vpcName}, vpc)
				return vpc.Status.OperatorVersion
			}, timeout, interval).Should(Equal(OperatorVersion))

			By("Verifying phase stays Completed")
			Consistently(func() advisorv1alpha1.PlanPhase {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: vpcName}, vpc)
				return vpc.Status.Phase
			}, "5s", interval).Should(Equal(advisorv1alpha1.PlanPhaseCompleted))
		})

		It("should allow user to adopt upgrade", func() {
			By("Creating a VirtPlatformConfig")
			vpc := &advisorv1alpha1.VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{Name: vpcName},
				Spec: advisorv1alpha1.VirtPlatformConfigSpec{
					Profile: "example-profile",
					Action:  advisorv1alpha1.PlanActionApply,
				},
			}
			Expect(k8sClient.Create(ctx, vpc)).To(Succeed())

			By("Manually setting Completed phase with OLD version plan items")
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: vpcName}, vpc); err != nil {
					return err
				}
				vpc.Status.Phase = advisorv1alpha1.PlanPhaseCompleted
				vpc.Status.OperatorVersion = OperatorVersion
				vpc.Status.ObservedGeneration = vpc.Generation
				// Items represent what the OLD operator version generated (different name)
				vpc.Status.Items = []advisorv1alpha1.VirtPlatformConfigItem{
					{
						Name: "configure-old-component", // OLD: different name
						TargetRef: advisorv1alpha1.ObjectReference{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       "example-config",
							Namespace:  "openshift-cnv",
						},
						ImpactSeverity: advisorv1alpha1.ImpactLow,
						State:          advisorv1alpha1.ItemStatePending,
						Message:        "Waiting to apply configuration",
					},
				}
				return k8sClient.Status().Update(ctx, vpc)
			}, timeout, interval).Should(Succeed())

			By("Simulating upgrade")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: vpcName}, vpc)).To(Succeed())
			vpc.Status.OperatorVersion = "v0.1.30"
			Expect(k8sClient.Status().Update(ctx, vpc)).To(Succeed())

			By("Manually calling reconciler to trigger upgrade detection")
			controllerReconciler := &VirtPlatformConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: vpcName},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for CompletedWithUpgrade")
			Eventually(func() advisorv1alpha1.PlanPhase {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: vpcName}, vpc)
				return vpc.Status.Phase
			}, timeout, interval).Should(Equal(advisorv1alpha1.PlanPhaseCompletedWithUpgrade))

			By("User regenerates plan by changing action to DryRun")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: vpcName}, vpc)).To(Succeed())
			vpc.Spec.Action = advisorv1alpha1.PlanActionDryRun
			Expect(k8sClient.Update(ctx, vpc)).To(Succeed())

			By("Manually calling reconciler to process spec change")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: vpcName},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying plan regenerates")
			Eventually(func() bool {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: vpcName}, vpc)
				return vpc.Status.Phase == advisorv1alpha1.PlanPhasePending ||
					vpc.Status.Phase == advisorv1alpha1.PlanPhaseDrafting ||
					vpc.Status.Phase == advisorv1alpha1.PlanPhaseReviewRequired
			}, timeout, interval).Should(BeTrue())
		})
	})
})
