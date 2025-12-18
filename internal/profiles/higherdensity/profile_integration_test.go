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

package higherdensity

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
)

var (
	integrationCfg       *rest.Config
	integrationK8sClient client.Client
	integrationTestEnv   *envtest.Environment
	integrationCtx       context.Context
	integrationCancel    context.CancelFunc
)

func TestVirtHigherDensityProfileIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VirtHigherDensityProfile Integration Suite")
}

var _ = BeforeSuite(func() {
	integrationCtx, integrationCancel = context.WithCancel(context.TODO())

	By("bootstrapping integration test environment")
	integrationTestEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: false,
	}

	var err error
	integrationCfg, err = integrationTestEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(integrationCfg).NotTo(BeNil())

	scheme := runtime.NewScheme()
	err = advisorv1alpha1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	// Register apiextensions scheme for CRD operations
	err = apiextensionsv1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	integrationK8sClient, err = client.New(integrationCfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(integrationK8sClient).NotTo(BeNil())

	By("loading MachineConfig CRD from file")
	machineconfigCRD := loadMachineConfigCRDFromFile()
	err = integrationK8sClient.Create(integrationCtx, machineconfigCRD)
	Expect(err).NotTo(HaveOccurred())

	// Wait for environment to be ready
	time.Sleep(1 * time.Second)
})

var _ = AfterSuite(func() {
	integrationCancel()
	By("tearing down the integration test environment")
	err := integrationTestEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

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
			Expect(mcItem.ImpactSeverity).To(Equal(advisorv1alpha1.ImpactHigh), "MachineConfig requires node reboot")
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

// Helper function to load MachineConfig CRD from file
func loadMachineConfigCRDFromFile() *apiextensionsv1.CustomResourceDefinition {
	// Load the actual MachineConfig CRD from the mocks directory
	crdPath := filepath.Join("..", "..", "..", "config", "crd", "mocks", "machineconfig_crd.yaml")
	crdBytes, err := os.ReadFile(crdPath)
	Expect(err).NotTo(HaveOccurred(), "Failed to read MachineConfig CRD file")

	var crd apiextensionsv1.CustomResourceDefinition
	err = yaml.Unmarshal(crdBytes, &crd)
	Expect(err).NotTo(HaveOccurred(), "Failed to unmarshal MachineConfig CRD")

	return &crd
}
