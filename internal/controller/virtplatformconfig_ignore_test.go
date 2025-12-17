package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
)

var _ = Describe("Ignore Action Handling", func() {
	var reconciler *VirtPlatformConfigReconciler

	BeforeEach(func() {
		reconciler = &VirtPlatformConfigReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	AfterEach(func() {
		// Clean up VirtPlatformConfigs
		vpcs := &advisorv1alpha1.VirtPlatformConfigList{}
		Expect(k8sClient.List(ctx, vpcs)).To(Succeed())
		for _, vpc := range vpcs.Items {
			Expect(k8sClient.Delete(ctx, &vpc)).To(Succeed())
		}
	})

	It("should transition to Ignored phase when action is Ignore", func() {
		By("creating a VirtPlatformConfig with action=Ignore")
		vpc := &advisorv1alpha1.VirtPlatformConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: "example-profile",
			},
			Spec: advisorv1alpha1.VirtPlatformConfigSpec{
				Profile: "example-profile",
				Action:  advisorv1alpha1.PlanActionIgnore,
			},
		}
		Expect(k8sClient.Create(ctx, vpc)).To(Succeed())

		By("triggering reconciliation")
		_, err := reconciler.Reconcile(ctx, requestFor(vpc))
		Expect(err).NotTo(HaveOccurred())

		By("verifying it transitioned to Ignored phase")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "example-profile"}, vpc)).To(Succeed())
		Expect(vpc.Status.Phase).To(Equal(advisorv1alpha1.PlanPhaseIgnored))

		By("verifying Ignored condition is set")
		ignoredCondition := findConditionByType(vpc.Status.Conditions, ConditionTypeIgnored)
		Expect(ignoredCondition).NotTo(BeNil())
		Expect(ignoredCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(ignoredCondition.Reason).To(Equal(ReasonIgnored))
		Expect(ignoredCondition.Message).To(ContainSubstring("Resources remain in their current state"))
	})

	It("should stay in Ignored phase when already Ignored and action is still Ignore", func() {
		By("creating a VirtPlatformConfig with action=Ignore")
		vpc := &advisorv1alpha1.VirtPlatformConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: "load-aware-rebalancing",
			},
			Spec: advisorv1alpha1.VirtPlatformConfigSpec{
				Profile: "load-aware-rebalancing",
				Action:  advisorv1alpha1.PlanActionIgnore,
			},
		}
		Expect(k8sClient.Create(ctx, vpc)).To(Succeed())

		By("first reconciliation to enter Ignored phase")
		_, err := reconciler.Reconcile(ctx, requestFor(vpc))
		Expect(err).NotTo(HaveOccurred())

		By("getting resource version after first reconciliation")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "load-aware-rebalancing"}, vpc)).To(Succeed())
		Expect(vpc.Status.Phase).To(Equal(advisorv1alpha1.PlanPhaseIgnored))
		firstResourceVersion := vpc.ResourceVersion

		By("second reconciliation while still Ignored")
		_, err = reconciler.Reconcile(ctx, requestFor(vpc))
		Expect(err).NotTo(HaveOccurred())

		By("verifying status was not updated (no-op reconciliation)")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "load-aware-rebalancing"}, vpc)).To(Succeed())
		Expect(vpc.Status.Phase).To(Equal(advisorv1alpha1.PlanPhaseIgnored))
		// Resource version should be same since no status update occurred
		Expect(vpc.ResourceVersion).To(Equal(firstResourceVersion))
	})

	It("should exit Ignored phase when action changes from Ignore to DryRun", func() {
		By("creating a VirtPlatformConfig with action=Ignore")
		vpc := &advisorv1alpha1.VirtPlatformConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: "virt-higher-density",
			},
			Spec: advisorv1alpha1.VirtPlatformConfigSpec{
				Profile: "virt-higher-density",
				Action:  advisorv1alpha1.PlanActionIgnore,
			},
		}
		Expect(k8sClient.Create(ctx, vpc)).To(Succeed())

		By("reconciling to enter Ignored phase")
		_, err := reconciler.Reconcile(ctx, requestFor(vpc))
		Expect(err).NotTo(HaveOccurred())

		By("verifying it's in Ignored phase")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "virt-higher-density"}, vpc)).To(Succeed())
		Expect(vpc.Status.Phase).To(Equal(advisorv1alpha1.PlanPhaseIgnored))

		By("changing action to DryRun")
		vpc.Spec.Action = advisorv1alpha1.PlanActionDryRun
		Expect(k8sClient.Update(ctx, vpc)).To(Succeed())

		By("reconciling after action change")
		_, err = reconciler.Reconcile(ctx, requestFor(vpc))
		Expect(err).NotTo(HaveOccurred())

		By("verifying it exited Ignored phase and reset to Pending")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "virt-higher-density"}, vpc)).To(Succeed())
		Expect(vpc.Status.Phase).To(Equal(advisorv1alpha1.PlanPhasePending))

		By("verifying it's no longer in Ignored phase (condition may still exist)")
		// Note: The Ignored condition may still be True - what matters is the phase changed
		Expect(vpc.Status.Phase).NotTo(Equal(advisorv1alpha1.PlanPhaseIgnored),
			"Should have exited Ignored phase")
	})

	It("should exit Ignored phase when action changes from Ignore to Apply", func() {
		By("creating a VirtPlatformConfig with action=Ignore")
		vpc := &advisorv1alpha1.VirtPlatformConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: "example-profile",
			},
			Spec: advisorv1alpha1.VirtPlatformConfigSpec{
				Profile: "example-profile",
				Action:  advisorv1alpha1.PlanActionIgnore,
			},
		}
		Expect(k8sClient.Create(ctx, vpc)).To(Succeed())

		By("reconciling to enter Ignored phase")
		_, err := reconciler.Reconcile(ctx, requestFor(vpc))
		Expect(err).NotTo(HaveOccurred())

		By("verifying it's in Ignored phase")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "example-profile"}, vpc)).To(Succeed())
		Expect(vpc.Status.Phase).To(Equal(advisorv1alpha1.PlanPhaseIgnored))

		By("changing action to Apply")
		vpc.Spec.Action = advisorv1alpha1.PlanActionApply
		Expect(k8sClient.Update(ctx, vpc)).To(Succeed())

		By("reconciling after action change")
		_, err = reconciler.Reconcile(ctx, requestFor(vpc))
		Expect(err).NotTo(HaveOccurred())

		By("verifying it exited Ignored phase and reset to Pending")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "example-profile"}, vpc)).To(Succeed())
		Expect(vpc.Status.Phase).To(Equal(advisorv1alpha1.PlanPhasePending))
	})

	It("should not generate plan items when in Ignored phase", func() {
		By("creating a VirtPlatformConfig with action=Ignore for example-profile")
		vpc := &advisorv1alpha1.VirtPlatformConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: "example-profile",
			},
			Spec: advisorv1alpha1.VirtPlatformConfigSpec{
				Profile: "example-profile",
				Action:  advisorv1alpha1.PlanActionIgnore,
			},
		}
		Expect(k8sClient.Create(ctx, vpc)).To(Succeed())

		By("reconciling")
		_, err := reconciler.Reconcile(ctx, requestFor(vpc))
		Expect(err).NotTo(HaveOccurred())

		By("verifying no plan items were generated")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "example-profile"}, vpc)).To(Succeed())
		Expect(vpc.Status.Items).To(BeEmpty(), "Should not generate plan items in Ignored phase")
		Expect(vpc.Status.Phase).To(Equal(advisorv1alpha1.PlanPhaseIgnored))
	})

	It("should not perform drift detection when in Ignored phase", func() {
		By("creating a VirtPlatformConfig with action=Ignore")
		vpc := &advisorv1alpha1.VirtPlatformConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: "load-aware-rebalancing",
			},
			Spec: advisorv1alpha1.VirtPlatformConfigSpec{
				Profile: "load-aware-rebalancing",
				Action:  advisorv1alpha1.PlanActionIgnore,
			},
		}
		Expect(k8sClient.Create(ctx, vpc)).To(Succeed())

		By("reconciling to enter Ignored phase")
		_, err := reconciler.Reconcile(ctx, requestFor(vpc))
		Expect(err).NotTo(HaveOccurred())

		By("verifying it's in Ignored phase")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "load-aware-rebalancing"}, vpc)).To(Succeed())
		Expect(vpc.Status.Phase).To(Equal(advisorv1alpha1.PlanPhaseIgnored))

		By("verifying no plan items were generated")
		Expect(vpc.Status.Items).To(BeEmpty(), "Should not generate plan in Ignored phase")

		By("reconciling again while Ignored (simulating potential drift)")
		_, err = reconciler.Reconcile(ctx, requestFor(vpc))
		Expect(err).NotTo(HaveOccurred())

		By("verifying phase stayed Ignored (no drift detection)")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "load-aware-rebalancing"}, vpc)).To(Succeed())
		Expect(vpc.Status.Phase).To(Equal(advisorv1alpha1.PlanPhaseIgnored),
			"Should remain in Ignored phase - no drift detection should occur")

		By("verifying still no plan items")
		Expect(vpc.Status.Items).To(BeEmpty(),
			"Should not generate plan items even after multiple reconciliations")
	})

	It("should update observedGeneration when entering Ignored phase", func() {
		By("creating a VirtPlatformConfig with action=Ignore")
		vpc := &advisorv1alpha1.VirtPlatformConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: "virt-higher-density",
			},
			Spec: advisorv1alpha1.VirtPlatformConfigSpec{
				Profile: "virt-higher-density",
				Action:  advisorv1alpha1.PlanActionIgnore,
			},
		}
		Expect(k8sClient.Create(ctx, vpc)).To(Succeed())

		By("getting the generation before reconciliation")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "virt-higher-density"}, vpc)).To(Succeed())
		currentGeneration := vpc.Generation

		By("reconciling")
		_, err := reconciler.Reconcile(ctx, requestFor(vpc))
		Expect(err).NotTo(HaveOccurred())

		By("verifying observedGeneration was updated")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "virt-higher-density"}, vpc)).To(Succeed())
		Expect(vpc.Status.ObservedGeneration).To(Equal(currentGeneration),
			"ObservedGeneration should be updated when entering Ignored phase")
	})

	It("should allow multiple transitions between Ignored and other phases", func() {
		By("creating a VirtPlatformConfig with action=DryRun")
		vpc := &advisorv1alpha1.VirtPlatformConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: "example-profile",
			},
			Spec: advisorv1alpha1.VirtPlatformConfigSpec{
				Profile: "example-profile",
				Action:  advisorv1alpha1.PlanActionDryRun,
			},
		}
		Expect(k8sClient.Create(ctx, vpc)).To(Succeed())

		By("reconciling to generate plan")
		_, err := reconciler.Reconcile(ctx, requestFor(vpc))
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "example-profile"}, vpc)).To(Succeed())
		Expect(vpc.Status.Phase).NotTo(Equal(advisorv1alpha1.PlanPhaseIgnored))

		By("changing to Ignore")
		vpc.Spec.Action = advisorv1alpha1.PlanActionIgnore
		Expect(k8sClient.Update(ctx, vpc)).To(Succeed())
		_, err = reconciler.Reconcile(ctx, requestFor(vpc))
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "example-profile"}, vpc)).To(Succeed())
		Expect(vpc.Status.Phase).To(Equal(advisorv1alpha1.PlanPhaseIgnored))

		By("changing back to DryRun")
		vpc.Spec.Action = advisorv1alpha1.PlanActionDryRun
		Expect(k8sClient.Update(ctx, vpc)).To(Succeed())
		_, err = reconciler.Reconcile(ctx, requestFor(vpc))
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "example-profile"}, vpc)).To(Succeed())
		Expect(vpc.Status.Phase).To(Equal(advisorv1alpha1.PlanPhasePending))

		By("changing to Ignore again")
		vpc.Spec.Action = advisorv1alpha1.PlanActionIgnore
		Expect(k8sClient.Update(ctx, vpc)).To(Succeed())
		_, err = reconciler.Reconcile(ctx, requestFor(vpc))
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "example-profile"}, vpc)).To(Succeed())
		Expect(vpc.Status.Phase).To(Equal(advisorv1alpha1.PlanPhaseIgnored))

		By("changing to Apply")
		vpc.Spec.Action = advisorv1alpha1.PlanActionApply
		Expect(k8sClient.Update(ctx, vpc)).To(Succeed())
		_, err = reconciler.Reconcile(ctx, requestFor(vpc))
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "example-profile"}, vpc)).To(Succeed())
		Expect(vpc.Status.Phase).To(Equal(advisorv1alpha1.PlanPhasePending))
	})
})

// Helper function to find a condition by type
func findConditionByType(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

// Helper function to create a reconcile request for a VirtPlatformConfig
func requestFor(vpc *advisorv1alpha1.VirtPlatformConfig) ctrl.Request {
	return ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      vpc.Name,
			Namespace: vpc.Namespace,
		},
	}
}
