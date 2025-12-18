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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
	"github.com/kubevirt/virt-advisor-operator/internal/profiles"
)

var _ = Describe("Profile Advertising Initialization", func() {
	var reconciler *VirtPlatformConfigReconciler

	BeforeEach(func() {
		reconciler = &VirtPlatformConfigReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	AfterEach(func() {
		// Clean up created VirtPlatformConfigs
		vpcs := &advisorv1alpha1.VirtPlatformConfigList{}
		Expect(k8sClient.List(ctx, vpcs)).To(Succeed())
		for _, vpc := range vpcs.Items {
			Expect(k8sClient.Delete(ctx, &vpc)).To(Succeed())
		}
	})

	It("should create VirtPlatformConfig for all advertisable profiles", func() {
		logger := logf.Log.WithName("test")

		reconciler.initializeAdvertisedProfiles(ctx, k8sClient, logger)

		// Verify load-aware-rebalancing was created
		vpc := &advisorv1alpha1.VirtPlatformConfig{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: "load-aware-rebalancing"}, vpc)
		Expect(err).NotTo(HaveOccurred())

		// Verify it has correct action
		Expect(vpc.Spec.Action).To(Equal(advisorv1alpha1.PlanActionIgnore))

		// Verify it has correct profile
		Expect(vpc.Spec.Profile).To(Equal("load-aware-rebalancing"))

		// Verify it has annotations
		Expect(vpc.Annotations["advisor.kubevirt.io/description"]).NotTo(BeEmpty())
		Expect(vpc.Annotations["advisor.kubevirt.io/impact-summary"]).NotTo(BeEmpty())
		Expect(vpc.Annotations["advisor.kubevirt.io/auto-created"]).To(Equal("true"))

		// Verify it has labels
		Expect(vpc.Labels["advisor.kubevirt.io/category"]).NotTo(BeEmpty())
	})

	It("should not create VirtPlatformConfig for non-advertisable profiles", func() {
		logger := logf.Log.WithName("test")

		reconciler.initializeAdvertisedProfiles(ctx, k8sClient, logger)

		// Verify example-profile was NOT created
		vpc := &advisorv1alpha1.VirtPlatformConfig{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: "example-profile"}, vpc)
		Expect(err).To(HaveOccurred())
	})

	It("should not overwrite existing VirtPlatformConfig", func() {
		logger := logf.Log.WithName("test")

		// Create existing VirtPlatformConfig with different action
		existing := &advisorv1alpha1.VirtPlatformConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: "load-aware-rebalancing",
			},
			Spec: advisorv1alpha1.VirtPlatformConfigSpec{
				Profile: "load-aware-rebalancing",
				Action:  advisorv1alpha1.PlanActionDryRun, // User modified
			},
		}
		Expect(k8sClient.Create(ctx, existing)).To(Succeed())

		// Get resource version before initialization
		vpc := &advisorv1alpha1.VirtPlatformConfig{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "load-aware-rebalancing"}, vpc)).To(Succeed())
		resourceVersionBefore := vpc.ResourceVersion

		// Run initialization
		reconciler.initializeAdvertisedProfiles(ctx, k8sClient, logger)

		// Verify it wasn't modified
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "load-aware-rebalancing"}, vpc)).To(Succeed())
		Expect(vpc.Spec.Action).To(Equal(advisorv1alpha1.PlanActionDryRun), "Action should not be modified")
		Expect(vpc.ResourceVersion).To(Equal(resourceVersionBefore), "Resource should not be updated")
	})

	It("should be idempotent when run multiple times", func() {
		logger := logf.Log.WithName("test")

		// Run initialization first time
		reconciler.initializeAdvertisedProfiles(ctx, k8sClient, logger)

		// Get created object
		vpc1 := &advisorv1alpha1.VirtPlatformConfig{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "load-aware-rebalancing"}, vpc1)).To(Succeed())
		resourceVersion1 := vpc1.ResourceVersion

		// Run initialization second time
		reconciler.initializeAdvertisedProfiles(ctx, k8sClient, logger)

		// Get object again
		vpc2 := &advisorv1alpha1.VirtPlatformConfig{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "load-aware-rebalancing"}, vpc2)).To(Succeed())

		// Verify it wasn't modified
		Expect(vpc2.ResourceVersion).To(Equal(resourceVersion1), "Resource should not be updated on second run")
	})

	It("should populate metadata from profile", func() {
		logger := logf.Log.WithName("test")

		reconciler.initializeAdvertisedProfiles(ctx, k8sClient, logger)

		// Get created VirtPlatformConfig
		vpc := &advisorv1alpha1.VirtPlatformConfig{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "load-aware-rebalancing"}, vpc)).To(Succeed())

		// Get the profile to compare
		profile, err := profiles.DefaultRegistry.Get("load-aware-rebalancing")
		Expect(err).NotTo(HaveOccurred())

		// Verify annotations match profile metadata
		Expect(vpc.Annotations["advisor.kubevirt.io/description"]).To(Equal(profile.GetDescription()))
		Expect(vpc.Annotations["advisor.kubevirt.io/impact-summary"]).To(Equal(profile.GetImpactSummary()))

		// Verify labels match profile metadata
		Expect(vpc.Labels["advisor.kubevirt.io/category"]).To(Equal(profile.GetCategory()))
	})

	It("should recreate deleted advertised profile during reconciliation", func() {
		logger := logf.Log.WithName("test")

		// First, initialize the advertised profiles
		reconciler.initializeAdvertisedProfiles(ctx, k8sClient, logger)

		// Verify the profile was created
		vpc := &advisorv1alpha1.VirtPlatformConfig{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: "load-aware-rebalancing"}, vpc)
		Expect(err).NotTo(HaveOccurred())

		// Delete the advertised profile
		Expect(k8sClient.Delete(ctx, vpc)).To(Succeed())

		// Verify it's deleted
		err = k8sClient.Get(ctx, client.ObjectKey{Name: "load-aware-rebalancing"}, vpc)
		Expect(err).To(HaveOccurred())

		// Simulate deletion reconciliation
		// This should recreate the profile
		Expect(reconciler.shouldRecreateAdvertisedProfile("load-aware-rebalancing")).To(BeTrue())
		created, err := reconciler.createAdvertisedProfile(ctx, k8sClient, "load-aware-rebalancing", logger)
		Expect(err).NotTo(HaveOccurred())
		Expect(created).To(BeTrue())

		// Verify the profile was recreated
		recreated := &advisorv1alpha1.VirtPlatformConfig{}
		err = k8sClient.Get(ctx, client.ObjectKey{Name: "load-aware-rebalancing"}, recreated)
		Expect(err).NotTo(HaveOccurred())
		Expect(recreated.Spec.Action).To(Equal(advisorv1alpha1.PlanActionIgnore))
		Expect(recreated.Annotations["advisor.kubevirt.io/auto-created"]).To(Equal("true"))
	})

	It("should not recreate profile that doesn't exist in registry", func() {
		// Try to recreate a profile name that doesn't exist in the registry
		Expect(reconciler.shouldRecreateAdvertisedProfile("non-existent-profile")).To(BeFalse())

		// Try to create it - should fail because it's not in the registry
		logger := logf.Log.WithName("test")
		created, err := reconciler.createAdvertisedProfile(ctx, k8sClient, "non-existent-profile", logger)
		Expect(err).To(HaveOccurred())
		Expect(created).To(BeFalse())
	})

	It("should not recreate user-modified advertised profile", func() {
		logger := logf.Log.WithName("test")

		// Create a user-modified advertised profile (user changed action from Ignore to Apply)
		vpc := &advisorv1alpha1.VirtPlatformConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: "load-aware-rebalancing",
				Annotations: map[string]string{
					"advisor.kubevirt.io/auto-created": "true", // Was auto-created but user modified it
				},
			},
			Spec: advisorv1alpha1.VirtPlatformConfigSpec{
				Profile: "load-aware-rebalancing",
				Action:  advisorv1alpha1.PlanActionApply, // User changed from Ignore to Apply
			},
		}
		Expect(k8sClient.Create(ctx, vpc)).To(Succeed())

		// Delete it
		Expect(k8sClient.Delete(ctx, vpc)).To(Succeed())

		// Even though it was deleted, shouldRecreateAdvertisedProfile should return true
		// because load-aware-rebalancing is an advertisable profile
		Expect(reconciler.shouldRecreateAdvertisedProfile("load-aware-rebalancing")).To(BeTrue())

		// And it should be recreated with default Ignore action
		created, err := reconciler.createAdvertisedProfile(ctx, k8sClient, "load-aware-rebalancing", logger)
		Expect(err).NotTo(HaveOccurred())
		Expect(created).To(BeTrue())

		// Verify it was recreated with Ignore action (not the user's Apply action)
		recreated := &advisorv1alpha1.VirtPlatformConfig{}
		err = k8sClient.Get(ctx, client.ObjectKey{Name: "load-aware-rebalancing"}, recreated)
		Expect(err).NotTo(HaveOccurred())
		Expect(recreated.Spec.Action).To(Equal(advisorv1alpha1.PlanActionIgnore))
	})
})
