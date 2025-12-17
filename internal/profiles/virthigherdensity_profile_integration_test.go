package profiles

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
)

var _ = Describe("VirtHigherDensityProfile Integration Tests", func() {
	var profile *VirtHigherDensityProfile

	BeforeEach(func() {
		profile = NewVirtHigherDensityProfile()
	})

	Describe("GeneratePlanItems with swap enabled (default)", func() {
		It("should generate 1 plan item (MachineConfig for swap)", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(HaveLen(1), "should generate MachineConfig for swap")
		})

		It("should create MachineConfig item with correct properties", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			mcItem := items[0]
			Expect(mcItem.Name).To(Equal("enable-kubelet-swap"))
			Expect(mcItem.TargetRef.Kind).To(Equal("MachineConfig"))
			Expect(mcItem.TargetRef.APIVersion).To(Equal("machineconfiguration.openshift.io/v1"))
			Expect(mcItem.TargetRef.Name).To(Equal("90-worker-kubelet-swap"))
			Expect(mcItem.TargetRef.Namespace).To(BeEmpty(), "MachineConfig is cluster-scoped")
			Expect(mcItem.State).To(Equal(advisorv1alpha1.ItemStatePending))
		})

		It("should generate SSA-based unified diff for MachineConfig", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			mcItem := items[0]
			Expect(mcItem.Diff).NotTo(BeEmpty(), "diff should be generated")
			Expect(mcItem.Diff).To(ContainSubstring("---"), "should be unified diff format")
			Expect(mcItem.Diff).To(ContainSubstring("+++"), "should be unified diff format")
			Expect(mcItem.Diff).To(MatchRegexp(`@@.*@@`), "should have diff hunk headers")
		})

		It("should include swap configuration in MachineConfig diff", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			mcItem := items[0]
			// Should show ignition config
			Expect(mcItem.Diff).To(ContainSubstring("ignition"), "should show ignition config")
			Expect(mcItem.Diff).To(ContainSubstring("3.2.0"), "should show ignition version")

			// Should show storage files
			Expect(mcItem.Diff).To(ContainSubstring("storage"), "should show storage config")
			Expect(mcItem.Diff).To(ContainSubstring("files"), "should show files")

			// Should show systemd units
			Expect(mcItem.Diff).To(ContainSubstring("systemd"), "should show systemd config")
			Expect(mcItem.Diff).To(ContainSubstring("units"), "should show systemd units")

			// Should show the swap-provision service
			Expect(mcItem.Diff).To(ContainSubstring("swap-provision.service"), "should show swap-provision service")

			// Should show cgroup-system-slice-config service
			Expect(mcItem.Diff).To(ContainSubstring("cgroup-system-slice-config.service"), "should show cgroup config service")

			// Should show role label
			Expect(mcItem.Diff).To(ContainSubstring("machineconfiguration.openshift.io/role"), "should show role label")
			Expect(mcItem.Diff).To(ContainSubstring("worker"), "should target worker nodes")
		})

		It("should set managed fields correctly", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			mcItem := items[0]
			Expect(mcItem.ManagedFields).To(ConsistOf(
				"spec.config",
			))
		})

		It("should set appropriate message", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			mcItem := items[0]
			Expect(mcItem.Message).To(ContainSubstring("MachineConfig"))
			Expect(mcItem.Message).To(ContainSubstring("90-worker-kubelet-swap"))
			Expect(mcItem.Message).To(ContainSubstring("swap"))
		})

		It("should set impact severity to High for new MachineConfig", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			mcItem := items[0]
			Expect(mcItem.ImpactSeverity).To(ContainSubstring("High"), "MachineConfig requires node reboot")
			Expect(mcItem.ImpactSeverity).To(ContainSubstring("reboot"), "should mention reboot requirement")
		})
	})

	Describe("GeneratePlanItems with swap explicitly enabled", func() {
		It("should generate MachineConfig when enableSwap=true", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{
				"enableSwap": "true",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(HaveLen(1), "should generate MachineConfig when enableSwap=true")
		})

		It("should handle various truthy values for enableSwap", func() {
			truthyValues := []string{"true", "True", "TRUE", "yes", "1"}

			for _, value := range truthyValues {
				items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{
					"enableSwap": value,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(items).To(HaveLen(1), "value %q should enable swap", value)
			}
		})
	})

	Describe("GeneratePlanItems with swap disabled", func() {
		It("should generate no items when enableSwap=false", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{
				"enableSwap": "false",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(BeEmpty(), "should not generate any items when swap is disabled")
		})
	})

	Describe("Item structure validation", func() {
		It("should have all required fields on all items", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			for _, item := range items {
				Expect(item.Name).NotTo(BeEmpty(), "name should be set")
				Expect(item.TargetRef.APIVersion).NotTo(BeEmpty(), "apiVersion should be set")
				Expect(item.TargetRef.Kind).NotTo(BeEmpty(), "kind should be set")
				Expect(item.TargetRef.Name).NotTo(BeEmpty(), "resource name should be set")
				Expect(item.ImpactSeverity).NotTo(BeEmpty(), "impact severity should be set")
				Expect(item.Diff).NotTo(BeEmpty(), "diff should be set")
				Expect(item.State).To(Equal(advisorv1alpha1.ItemStatePending), "state should be Pending")
				Expect(item.Message).NotTo(BeEmpty(), "message should be set")
				Expect(item.ManagedFields).NotTo(BeEmpty(), "managed fields should be set")
			}
		})
	})

	Describe("Swap configuration details", func() {
		It("should include find-swap-partition script", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			mcItem := items[0]
			// The script is base64 encoded in the MachineConfig, so we check for the file path
			Expect(mcItem.Diff).To(ContainSubstring("/etc/find-swap-partition"),
				"should include swap partition detection script")
		})

		It("should include kubelet swap configuration", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			mcItem := items[0]
			// The config is base64 encoded, so we check for the file path
			Expect(mcItem.Diff).To(ContainSubstring("/etc/openshift/kubelet.conf.d/90-swap.conf"),
				"should include kubelet swap configuration file")
		})

		It("should include swap-provision systemd unit", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			mcItem := items[0]
			Expect(mcItem.Diff).To(ContainSubstring("swap-provision.service"),
				"should include swap-provision systemd service")
			Expect(mcItem.Diff).To(MatchRegexp("Description.*swap"),
				"service should have description mentioning swap")
		})

		It("should include cgroup-system-slice-config systemd unit", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			mcItem := items[0]
			Expect(mcItem.Diff).To(ContainSubstring("cgroup-system-slice-config.service"),
				"should include cgroup-system-slice-config service")
			Expect(mcItem.Diff).To(MatchRegexp("Description.*system slice"),
				"service should have description mentioning system slice")
		})

		It("should enable both systemd units", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			mcItem := items[0]
			// Both services should be enabled (enabled: true)
			Expect(mcItem.Diff).To(ContainSubstring("swap-provision.service"),
				"should include swap-provision service")
			Expect(mcItem.Diff).To(ContainSubstring("cgroup-system-slice-config.service"),
				"should include cgroup-system-slice-config service")
			// Check that enabled is set to true for both services
			Expect(mcItem.Diff).To(MatchRegexp(`enabled:\s+true`),
				"services should be enabled")
		})
	})

	Describe("MachineConfig structure validation", func() {
		It("should set correct ignition version", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			mcItem := items[0]
			Expect(mcItem.Diff).To(ContainSubstring("3.2.0"), "should use Ignition 3.2.0")
		})

		It("should target worker nodes via label", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			mcItem := items[0]
			Expect(mcItem.Diff).To(ContainSubstring("machineconfiguration.openshift.io/role"),
				"should have role label")
			Expect(mcItem.Diff).To(ContainSubstring("worker"),
				"should target worker role")
		})

		It("should set file permissions correctly", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			mcItem := items[0]
			// Script should be executable (755 = 493 decimal)
			Expect(mcItem.Diff).To(MatchRegexp("mode.*755|mode.*493"),
				"swap detection script should be executable")
		})
	})

	Describe("Configuration override handling", func() {
		It("should respect enableSwap override", func() {
			// Test with swap enabled
			itemsEnabled, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{
				"enableSwap": "true",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(itemsEnabled).To(HaveLen(1), "should generate item when enabled")

			// Test with swap disabled
			itemsDisabled, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{
				"enableSwap": "false",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(itemsDisabled).To(BeEmpty(), "should not generate items when disabled")
		})

		It("should default to swap enabled when not specified", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(HaveLen(1), "should default to swap enabled")
		})

		It("should default to swap enabled when config is nil", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(HaveLen(1), "should default to swap enabled with nil config")
		})
	})
})
