package profiles

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"

	hcov1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
)

var (
	integrationCfg       *rest.Config
	integrationK8sClient client.Client
	integrationTestEnv   *envtest.Environment
	integrationCtx       context.Context
	integrationCancel    context.CancelFunc
)

func TestLoadAwareProfileIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "LoadAwareRebalancingProfile Integration Suite")
}

var _ = BeforeSuite(func() {
	integrationCtx, integrationCancel = context.WithCancel(context.TODO())

	By("bootstrapping integration test environment")
	integrationTestEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: false,
	}

	var err error
	integrationCfg, err = integrationTestEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(integrationCfg).NotTo(BeNil())

	scheme := runtime.NewScheme()
	err = hcov1alpha1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	// Register core v1 scheme for Namespace operations
	err = corev1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	// Register apiextensions scheme for CRD operations
	err = apiextensionsv1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	integrationK8sClient, err = client.New(integrationCfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(integrationK8sClient).NotTo(BeNil())

	By("creating KubeDescheduler namespace")
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "openshift-kube-descheduler-operator",
		},
	}
	err = integrationK8sClient.Create(integrationCtx, ns)
	Expect(err).NotTo(HaveOccurred())

	By("loading KubeDescheduler CRD from file")
	kubedeschedulerCRD := loadKubeDeschedulerCRDFromFile()
	err = integrationK8sClient.Create(integrationCtx, kubedeschedulerCRD)
	Expect(err).NotTo(HaveOccurred())

	By("loading MachineConfig CRD from file")
	machineconfigCRD := loadMachineConfigCRDFromFile()
	err = integrationK8sClient.Create(integrationCtx, machineconfigCRD)
	Expect(err).NotTo(HaveOccurred())

	By("loading MachineConfigPool CRD from file")
	machineconfigpoolCRD := loadMachineConfigPoolCRDFromFile()
	err = integrationK8sClient.Create(integrationCtx, machineconfigpoolCRD)
	Expect(err).NotTo(HaveOccurred())

	// Wait for CRDs to be established
	time.Sleep(2 * time.Second)
})

var _ = AfterSuite(func() {
	integrationCancel()
	By("tearing down the integration test environment")
	err := integrationTestEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// Helper function to load KubeDescheduler CRD from file
func loadKubeDeschedulerCRDFromFile() *apiextensionsv1.CustomResourceDefinition {
	// Load the actual KubeDescheduler CRD from the mocks directory
	crdPath := filepath.Join("..", "..", "config", "crd", "mocks", "descheduler_crd_v532.yaml")
	crdBytes, err := os.ReadFile(crdPath)
	Expect(err).NotTo(HaveOccurred(), "Failed to read KubeDescheduler CRD file")

	var crd apiextensionsv1.CustomResourceDefinition
	err = yaml.Unmarshal(crdBytes, &crd)
	Expect(err).NotTo(HaveOccurred(), "Failed to unmarshal KubeDescheduler CRD")

	return &crd
}

// Helper function to load MachineConfig CRD from file
func loadMachineConfigCRDFromFile() *apiextensionsv1.CustomResourceDefinition {
	// Load the actual MachineConfig CRD from the mocks directory
	crdPath := filepath.Join("..", "..", "config", "crd", "mocks", "machineconfig_crd.yaml")
	crdBytes, err := os.ReadFile(crdPath)
	Expect(err).NotTo(HaveOccurred(), "Failed to read MachineConfig CRD file")

	var crd apiextensionsv1.CustomResourceDefinition
	err = yaml.Unmarshal(crdBytes, &crd)
	Expect(err).NotTo(HaveOccurred(), "Failed to unmarshal MachineConfig CRD")

	return &crd
}

// Helper function to load MachineConfigPool CRD from file
func loadMachineConfigPoolCRDFromFile() *apiextensionsv1.CustomResourceDefinition {
	// Load the actual MachineConfigPool CRD from the mocks directory
	crdPath := filepath.Join("..", "..", "config", "crd", "mocks", "machineconfigpool_crd.yaml")
	crdBytes, err := os.ReadFile(crdPath)
	Expect(err).NotTo(HaveOccurred(), "Failed to read MachineConfigPool CRD file")

	var crd apiextensionsv1.CustomResourceDefinition
	err = yaml.Unmarshal(crdBytes, &crd)
	Expect(err).NotTo(HaveOccurred(), "Failed to unmarshal MachineConfigPool CRD")

	return &crd
}

var _ = Describe("LoadAwareRebalancingProfile Integration Tests", func() {
	var profile *LoadAwareRebalancingProfile

	BeforeEach(func() {
		profile = NewLoadAwareRebalancingProfile()
	})

	Describe("GeneratePlanItems with default configuration", func() {
		It("should generate 2 plan items (KubeDescheduler + MachineConfig)", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(HaveLen(2), "should generate KubeDescheduler and MachineConfig items")
		})

		It("should create KubeDescheduler item with correct properties", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Name).To(Equal("enable-load-aware-descheduling"))
			Expect(deschedulerItem.TargetRef.Kind).To(Equal("KubeDescheduler"))
			Expect(deschedulerItem.TargetRef.APIVersion).To(Equal("operator.openshift.io/v1"))
			Expect(deschedulerItem.TargetRef.Name).To(Equal("cluster"))
			Expect(deschedulerItem.TargetRef.Namespace).To(Equal("openshift-kube-descheduler-operator"), "KubeDescheduler is namespaced")
			Expect(deschedulerItem.State).To(Equal(hcov1alpha1.ItemStatePending))
		})

		It("should create MachineConfig item with correct properties", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			mcItem := items[1]
			Expect(mcItem.Name).To(Equal("enable-psi-metrics"))
			Expect(mcItem.TargetRef.Kind).To(Equal("MachineConfig"))
			Expect(mcItem.TargetRef.APIVersion).To(Equal("machineconfiguration.openshift.io/v1"))
			Expect(mcItem.TargetRef.Name).To(Equal("99-worker-psi-karg"))
			Expect(mcItem.TargetRef.Namespace).To(BeEmpty(), "MachineConfig is cluster-scoped")
			Expect(mcItem.State).To(Equal(hcov1alpha1.ItemStatePending))
		})

		It("should generate SSA-based unified diff for KubeDescheduler", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Diff).NotTo(BeEmpty(), "diff should be generated")
			Expect(deschedulerItem.Diff).To(ContainSubstring("---"), "should be unified diff format")
			Expect(deschedulerItem.Diff).To(ContainSubstring("+++"), "should be unified diff format")
			Expect(deschedulerItem.Diff).To(MatchRegexp(`@@.*@@`), "should have diff hunk headers")
		})

		It("should include correct fields in KubeDescheduler diff", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Diff).To(ContainSubstring("deschedulingIntervalSeconds"), "should show interval field")
			Expect(deschedulerItem.Diff).To(ContainSubstring("1800"), "should show default interval value")
			Expect(deschedulerItem.Diff).To(ContainSubstring("profiles"), "should show profiles field")
			// Should show one of the preferred profiles (exact profile depends on CRD schema)
			Expect(deschedulerItem.Diff).To(MatchRegexp("KubeVirtRelieveAndMigrate|DevKubeVirtRelieveAndMigrate|LongLifecycle"), "should show a KubeVirt profile")
		})

		It("should include correct fields in MachineConfig diff", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			mcItem := items[1]
			Expect(mcItem.Diff).To(ContainSubstring("kernelArguments"), "should show kernel arguments field")
			Expect(mcItem.Diff).To(ContainSubstring("psi=1"), "should show PSI kernel arg")
			Expect(mcItem.Diff).To(ContainSubstring("machineconfiguration.openshift.io/role"), "should show role label")
			Expect(mcItem.Diff).To(ContainSubstring("worker"), "should show worker role")
		})

		It("should set managed fields correctly", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			// For KubeVirt profiles, profileCustomizations is also managed
			Expect(deschedulerItem.ManagedFields).To(ContainElement("spec.deschedulingIntervalSeconds"))
			Expect(deschedulerItem.ManagedFields).To(ContainElement("spec.profiles"))
			Expect(deschedulerItem.ManagedFields).To(ContainElement("spec.profileCustomizations"))

			mcItem := items[1]
			Expect(mcItem.ManagedFields).To(ConsistOf(
				"spec.kernelArguments",
			))
		})

		It("should set appropriate messages", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Message).To(ContainSubstring("KubeDescheduler"))
			Expect(deschedulerItem.Message).To(ContainSubstring("profile"))

			mcItem := items[1]
			Expect(mcItem.Message).To(ContainSubstring("PSI metrics"))
		})

		It("should set impact severity for new resources", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			// Both resources don't exist yet, so should indicate creation
			for _, item := range items {
				Expect(item.ImpactSeverity).NotTo(BeEmpty())
			}

			mcItem := items[1]
			Expect(mcItem.ImpactSeverity).To(ContainSubstring("High"), "MachineConfig requires node reboot")
			Expect(mcItem.ImpactSeverity).To(ContainSubstring("reboot"), "should mention reboot requirement")
		})
	})

	Describe("GeneratePlanItems with custom descheduling interval", func() {
		It("should use custom interval value", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{
				"deschedulingIntervalSeconds": "3600",
			})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Diff).To(ContainSubstring("3600"), "should use custom interval")
		})

		It("should handle various valid intervals", func() {
			testCases := []string{"60", "900", "7200"}

			for _, interval := range testCases {
				items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{
					"deschedulingIntervalSeconds": interval,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(items[0].Diff).To(ContainSubstring(interval), "should use interval: %s", interval)
			}
		})

		It("should fall back to default on invalid interval", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{
				"deschedulingIntervalSeconds": "invalid",
			})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Diff).To(ContainSubstring("1800"), "should fall back to default 1800")
		})

		It("should fall back to default on negative interval", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{
				"deschedulingIntervalSeconds": "-100",
			})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Diff).To(ContainSubstring("1800"), "should fall back to default 1800")
		})
	})

	Describe("GeneratePlanItems with PSI metrics disabled", func() {
		It("should generate only 1 item (KubeDescheduler only)", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{
				"enablePSIMetrics": "false",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(HaveLen(1), "should only generate KubeDescheduler item")
		})

		It("should generate only KubeDescheduler item", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{
				"enablePSIMetrics": "false",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(items[0].TargetRef.Kind).To(Equal("KubeDescheduler"))
		})

		It("should treat non-'false' values as enabled", func() {
			enabledValues := []string{"true", "True", "yes", "1", ""}

			for _, value := range enabledValues {
				items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{
					"enablePSIMetrics": value,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(items).To(HaveLen(2), "value %q should enable PSI", value)
			}
		})
	})

	Describe("GeneratePlanItems with combined overrides", func() {
		It("should apply both custom interval and disabled PSI", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{
				"deschedulingIntervalSeconds": "5400",
				"enablePSIMetrics":            "false",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(items).To(HaveLen(1))
			Expect(items[0].Diff).To(ContainSubstring("5400"))
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
				Expect(item.State).To(Equal(hcov1alpha1.ItemStatePending), "state should be Pending")
				Expect(item.Message).NotTo(BeEmpty(), "message should be set")
				Expect(item.ManagedFields).NotTo(BeEmpty(), "managed fields should be set")
			}
		})
	})

	Describe("ProfileCustomization fields", func() {
		It("should set profileCustomizations fields for KubeVirt profiles", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			// Should include profileCustomizations fields in the diff
			Expect(deschedulerItem.Diff).To(ContainSubstring("profileCustomizations"), "should set profileCustomizations")
			Expect(deschedulerItem.Diff).To(ContainSubstring("devEnableEvictionsInBackground"), "should set devEnableEvictionsInBackground")
			Expect(deschedulerItem.Diff).To(ContainSubstring("devEnableSoftTainter"), "should set devEnableSoftTainter")
		})

		It("should set devEnableEvictionsInBackground to true", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Diff).To(MatchRegexp(`devEnableEvictionsInBackground.*true`), "should set to true")
		})

		It("should set devEnableSoftTainter to true", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Diff).To(MatchRegexp(`devEnableSoftTainter.*true`), "should set to true")
		})

		It("should set devDeviationThresholds to default AsymmetricLow", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Diff).To(ContainSubstring("devDeviationThresholds"), "should include devDeviationThresholds field")
			Expect(deschedulerItem.Diff).To(ContainSubstring("AsymmetricLow"), "should use default value")
		})

		It("should override devDeviationThresholds when provided", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{
				"devDeviationThresholds": "High",
			})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Diff).To(ContainSubstring("devDeviationThresholds"), "should include devDeviationThresholds field")
			Expect(deschedulerItem.Diff).To(ContainSubstring("High"), "should use provided value")
			Expect(deschedulerItem.Diff).NotTo(ContainSubstring("AsymmetricLow"), "should not use default value")
		})

		It("should include profileCustomizations in managed fields", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.ManagedFields).To(ContainElement("spec.profileCustomizations"), "should manage profileCustomizations")
		})

		It("should handle multiple config overrides including devDeviationThresholds", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{
				"deschedulingIntervalSeconds": "7200",
				"devDeviationThresholds":      "Medium",
			})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Diff).To(ContainSubstring("7200"), "should use custom interval")
			Expect(deschedulerItem.Diff).To(ContainSubstring("Medium"), "should use custom deviation thresholds")
		})
	})

	Describe("Profile preservation", func() {
		var existingDescheduler *unstructured.Unstructured

		BeforeEach(func() {
			// Clean up any existing KubeDescheduler resource
			existingDescheduler = &unstructured.Unstructured{}
			existingDescheduler.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "operator.openshift.io",
				Version: "v1",
				Kind:    "KubeDescheduler",
			})
			existingDescheduler.SetName("cluster")
			existingDescheduler.SetNamespace("openshift-kube-descheduler-operator")
			_ = integrationK8sClient.Delete(integrationCtx, existingDescheduler)
		})

		It("should add only KubeVirt profile when no existing KubeDescheduler", func() {
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			// Should show adding the KubeVirt profile
			Expect(deschedulerItem.Diff).To(MatchRegexp(`KubeVirtRelieveAndMigrate|DevKubeVirtRelieveAndMigrate|LongLifecycle`))
			// Should not have any other profiles
			Expect(deschedulerItem.Diff).NotTo(ContainSubstring("AffinityAndTaints"))
			Expect(deschedulerItem.Diff).NotTo(ContainSubstring("SoftTopologyAndDuplicates"))
		})

		It("should preserve AffinityAndTaints if it exists", func() {
			// Create existing KubeDescheduler with AffinityAndTaints
			err := unstructured.SetNestedStringSlice(existingDescheduler.Object, []string{"AffinityAndTaints"}, "spec", "profiles")
			Expect(err).NotTo(HaveOccurred())
			err = integrationK8sClient.Create(integrationCtx, existingDescheduler)
			Expect(err).NotTo(HaveOccurred())

			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Diff).To(ContainSubstring("AffinityAndTaints"), "should preserve AffinityAndTaints")
			Expect(deschedulerItem.Diff).To(MatchRegexp(`KubeVirtRelieveAndMigrate|DevKubeVirtRelieveAndMigrate|LongLifecycle`), "should add KubeVirt profile")
		})

		It("should preserve SoftTopologyAndDuplicates if it exists", func() {
			// Create existing KubeDescheduler with SoftTopologyAndDuplicates
			err := unstructured.SetNestedStringSlice(existingDescheduler.Object, []string{"SoftTopologyAndDuplicates"}, "spec", "profiles")
			Expect(err).NotTo(HaveOccurred())
			err = integrationK8sClient.Create(integrationCtx, existingDescheduler)
			Expect(err).NotTo(HaveOccurred())

			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Diff).To(ContainSubstring("SoftTopologyAndDuplicates"), "should preserve SoftTopologyAndDuplicates")
			Expect(deschedulerItem.Diff).To(MatchRegexp(`KubeVirtRelieveAndMigrate|DevKubeVirtRelieveAndMigrate|LongLifecycle`), "should add KubeVirt profile")
		})

		It("should preserve both AffinityAndTaints and SoftTopologyAndDuplicates", func() {
			// Create existing KubeDescheduler with both preserved profiles
			err := unstructured.SetNestedStringSlice(existingDescheduler.Object, []string{"AffinityAndTaints", "SoftTopologyAndDuplicates"}, "spec", "profiles")
			Expect(err).NotTo(HaveOccurred())
			err = integrationK8sClient.Create(integrationCtx, existingDescheduler)
			Expect(err).NotTo(HaveOccurred())

			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Diff).To(ContainSubstring("AffinityAndTaints"), "should preserve AffinityAndTaints")
			Expect(deschedulerItem.Diff).To(ContainSubstring("SoftTopologyAndDuplicates"), "should preserve SoftTopologyAndDuplicates")
			Expect(deschedulerItem.Diff).To(MatchRegexp(`KubeVirtRelieveAndMigrate|DevKubeVirtRelieveAndMigrate|LongLifecycle`), "should add KubeVirt profile")
		})

		It("should remove other profiles like LifecycleAndUtilization", func() {
			// Create existing KubeDescheduler with profiles that should be removed
			err := unstructured.SetNestedStringSlice(existingDescheduler.Object, []string{"LifecycleAndUtilization", "TopologyAndDuplicates"}, "spec", "profiles")
			Expect(err).NotTo(HaveOccurred())
			err = integrationK8sClient.Create(integrationCtx, existingDescheduler)
			Expect(err).NotTo(HaveOccurred())

			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			// Should show removing the old profiles
			Expect(deschedulerItem.Diff).To(MatchRegexp(`-.*LifecycleAndUtilization`), "should remove LifecycleAndUtilization")
			Expect(deschedulerItem.Diff).To(MatchRegexp(`-.*TopologyAndDuplicates`), "should remove TopologyAndDuplicates")
			// Should add KubeVirt profile
			Expect(deschedulerItem.Diff).To(MatchRegexp(`KubeVirtRelieveAndMigrate|DevKubeVirtRelieveAndMigrate|LongLifecycle`))
		})

		It("should preserve AffinityAndTaints but remove other profiles", func() {
			// Create existing KubeDescheduler with mix of preserved and removed profiles
			err := unstructured.SetNestedStringSlice(existingDescheduler.Object, []string{"AffinityAndTaints", "LifecycleAndUtilization", "TopologyAndDuplicates"}, "spec", "profiles")
			Expect(err).NotTo(HaveOccurred())
			err = integrationK8sClient.Create(integrationCtx, existingDescheduler)
			Expect(err).NotTo(HaveOccurred())

			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			// Should preserve AffinityAndTaints
			Expect(deschedulerItem.Diff).To(ContainSubstring("AffinityAndTaints"))
			// Should remove other profiles
			Expect(deschedulerItem.Diff).To(MatchRegexp(`-.*LifecycleAndUtilization`), "should remove LifecycleAndUtilization")
			Expect(deschedulerItem.Diff).To(MatchRegexp(`-.*TopologyAndDuplicates`), "should remove TopologyAndDuplicates")
			// Should add KubeVirt profile
			Expect(deschedulerItem.Diff).To(MatchRegexp(`KubeVirtRelieveAndMigrate|DevKubeVirtRelieveAndMigrate|LongLifecycle`))
		})

		It("should preserve SoftTopologyAndDuplicates but remove other profiles", func() {
			// Create existing KubeDescheduler with mix of preserved and removed profiles
			err := unstructured.SetNestedStringSlice(existingDescheduler.Object, []string{"SoftTopologyAndDuplicates", "LifecycleAndUtilization"}, "spec", "profiles")
			Expect(err).NotTo(HaveOccurred())
			err = integrationK8sClient.Create(integrationCtx, existingDescheduler)
			Expect(err).NotTo(HaveOccurred())

			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			// Should preserve SoftTopologyAndDuplicates
			Expect(deschedulerItem.Diff).To(ContainSubstring("SoftTopologyAndDuplicates"))
			// Should remove other profiles
			Expect(deschedulerItem.Diff).To(MatchRegexp(`-.*LifecycleAndUtilization`), "should remove LifecycleAndUtilization")
			// Should add KubeVirt profile
			Expect(deschedulerItem.Diff).To(MatchRegexp(`KubeVirtRelieveAndMigrate|DevKubeVirtRelieveAndMigrate|LongLifecycle`))
		})
	})

	Describe("Profile selection across CRD versions", func() {
		// Helper to load a specific CRD version and update it in the cluster
		loadAndUpdateCRD := func(version string) {
			crdPath := filepath.Join("..", "..", "config", "crd", "mocks", fmt.Sprintf("descheduler_crd_%s.yaml", version))
			crdBytes, err := os.ReadFile(crdPath)
			Expect(err).NotTo(HaveOccurred(), "Failed to read CRD file for version %s", version)

			var crd apiextensionsv1.CustomResourceDefinition
			err = yaml.Unmarshal(crdBytes, &crd)
			Expect(err).NotTo(HaveOccurred(), "Failed to unmarshal CRD for version %s", version)

			// Update the CRD in the cluster
			existingCRD := &apiextensionsv1.CustomResourceDefinition{}
			existingCRD.SetName(kubeDeschedulerCRD)
			err = integrationK8sClient.Get(integrationCtx, client.ObjectKeyFromObject(existingCRD), existingCRD)
			Expect(err).NotTo(HaveOccurred())

			existingCRD.Spec = crd.Spec
			err = integrationK8sClient.Update(integrationCtx, existingCRD)
			Expect(err).NotTo(HaveOccurred())

			// Wait for the update to take effect
			time.Sleep(1 * time.Second)
		}

		AfterEach(func() {
			// Restore v532 CRD for subsequent tests
			loadAndUpdateCRD("v532")
		})

		It("should select LongLifecycle for OCP 5.10 (no KubeVirt profiles)", func() {
			loadAndUpdateCRD("v510")

			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Diff).To(ContainSubstring("LongLifecycle"), "should use LongLifecycle profile")
			Expect(deschedulerItem.Diff).NotTo(ContainSubstring("KubeVirtRelieveAndMigrate"), "should not have KubeVirtRelieveAndMigrate")
			Expect(deschedulerItem.Diff).NotTo(ContainSubstring("DevKubeVirtRelieveAndMigrate"), "should not have DevKubeVirtRelieveAndMigrate")

			// LongLifecycle should NOT have profileCustomizations
			Expect(deschedulerItem.Diff).NotTo(ContainSubstring("profileCustomizations"), "LongLifecycle should not set profileCustomizations")
			Expect(deschedulerItem.ManagedFields).NotTo(ContainElement("spec.profileCustomizations"), "should not manage profileCustomizations for LongLifecycle")
		})

		It("should select DevKubeVirtRelieveAndMigrate for OCP 5.14", func() {
			loadAndUpdateCRD("v514")

			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Diff).To(ContainSubstring("DevKubeVirtRelieveAndMigrate"), "should use DevKubeVirtRelieveAndMigrate profile")
			Expect(deschedulerItem.Diff).NotTo(MatchRegexp(`- KubeVirtRelieveAndMigrate\s*$`), "should not have KubeVirtRelieveAndMigrate GA (not available in v514)")

			// DevKubeVirtRelieveAndMigrate should have profileCustomizations
			Expect(deschedulerItem.Diff).To(ContainSubstring("profileCustomizations"), "should set profileCustomizations")
			Expect(deschedulerItem.Diff).To(ContainSubstring("devEnableEvictionsInBackground"), "should set devEnableEvictionsInBackground")
			Expect(deschedulerItem.Diff).To(ContainSubstring("devEnableSoftTainter"), "should set devEnableSoftTainter")
			Expect(deschedulerItem.Diff).To(ContainSubstring("AsymmetricLow"), "should set default devDeviationThresholds")
			Expect(deschedulerItem.ManagedFields).To(ContainElement("spec.profileCustomizations"), "should manage profileCustomizations")
		})

		It("should select DevKubeVirtRelieveAndMigrate for OCP 5.21", func() {
			loadAndUpdateCRD("v521")

			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Diff).To(ContainSubstring("DevKubeVirtRelieveAndMigrate"), "should use DevKubeVirtRelieveAndMigrate profile")
			Expect(deschedulerItem.Diff).NotTo(MatchRegexp(`- KubeVirtRelieveAndMigrate\s*$`), "should not have KubeVirtRelieveAndMigrate GA (not available in v521)")

			// DevKubeVirtRelieveAndMigrate should have profileCustomizations
			Expect(deschedulerItem.Diff).To(ContainSubstring("profileCustomizations"), "should set profileCustomizations")
			Expect(deschedulerItem.ManagedFields).To(ContainElement("spec.profileCustomizations"), "should manage profileCustomizations")
		})

		It("should select KubeVirtRelieveAndMigrate for OCP 5.32 (GA)", func() {
			loadAndUpdateCRD("v532")

			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Diff).To(MatchRegexp(`- KubeVirtRelieveAndMigrate\s*$`), "should use KubeVirtRelieveAndMigrate profile (GA)")
			Expect(deschedulerItem.Diff).NotTo(MatchRegexp(`- DevKubeVirtRelieveAndMigrate\s*$`), "should not use dev preview version when GA is available")

			// KubeVirtRelieveAndMigrate should have profileCustomizations
			Expect(deschedulerItem.Diff).To(ContainSubstring("profileCustomizations"), "should set profileCustomizations")
			Expect(deschedulerItem.Diff).To(ContainSubstring("devEnableEvictionsInBackground"), "should set devEnableEvictionsInBackground")
			Expect(deschedulerItem.Diff).To(ContainSubstring("devEnableSoftTainter"), "should set devEnableSoftTainter")
			Expect(deschedulerItem.Diff).To(ContainSubstring("AsymmetricLow"), "should set default devDeviationThresholds")
			Expect(deschedulerItem.ManagedFields).To(ContainElement("spec.profileCustomizations"), "should manage profileCustomizations")
		})

		It("should respect custom devDeviationThresholds across all versions", func() {
			versions := []string{"v514", "v521", "v532"}
			customThreshold := "High"

			for _, version := range versions {
				loadAndUpdateCRD(version)

				items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{
					"devDeviationThresholds": customThreshold,
				})
				Expect(err).NotTo(HaveOccurred())

				deschedulerItem := items[0]
				Expect(deschedulerItem.Diff).To(ContainSubstring(customThreshold), "version %s should use custom devDeviationThresholds", version)
				Expect(deschedulerItem.Diff).NotTo(ContainSubstring("AsymmetricLow"), "version %s should not use default value", version)
			}
		})

		It("should not set profileCustomizations for LongLifecycle even with config override", func() {
			loadAndUpdateCRD("v510")

			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{
				"devDeviationThresholds": "High",
			})
			Expect(err).NotTo(HaveOccurred())

			deschedulerItem := items[0]
			Expect(deschedulerItem.Diff).To(ContainSubstring("LongLifecycle"), "should use LongLifecycle")
			// Even with override, LongLifecycle should not get profileCustomizations
			Expect(deschedulerItem.Diff).NotTo(ContainSubstring("profileCustomizations"), "should not set profileCustomizations for LongLifecycle")
			Expect(deschedulerItem.Diff).NotTo(ContainSubstring("High"), "should not set devDeviationThresholds for LongLifecycle")
		})
	})

	Describe("Effect-based validation of PSI kernel argument", func() {
		var (
			workerPoolName       = "worker"
			renderedConfigName   = "rendered-worker-12345"
			machineConfigName    = "99-worker-psi-karg"
			machineConfigPoolGVK schema.GroupVersionKind
			machineConfigGVK     schema.GroupVersionKind
		)

		BeforeEach(func() {
			machineConfigPoolGVK = schema.GroupVersionKind{
				Group:   "machineconfiguration.openshift.io",
				Version: "v1",
				Kind:    "MachineConfigPool",
			}
			machineConfigGVK = schema.GroupVersionKind{
				Group:   "machineconfiguration.openshift.io",
				Version: "v1",
				Kind:    "MachineConfig",
			}
		})

		It("should detect PSI already effective in pool and set impact to None", func() {
			// Create a MachineConfigPool with a rendered config that includes psi=1
			pool := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "machineconfiguration.openshift.io/v1",
					"kind":       "MachineConfigPool",
					"metadata": map[string]interface{}{
						"name": workerPoolName,
					},
					"status": map[string]interface{}{
						"configuration": map[string]interface{}{
							"name": renderedConfigName,
						},
					},
				},
			}
			pool.SetGroupVersionKind(machineConfigPoolGVK)
			Expect(integrationK8sClient.Create(integrationCtx, pool)).To(Succeed())

			// Create the rendered MachineConfig with psi=1
			renderedMC := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "machineconfiguration.openshift.io/v1",
					"kind":       "MachineConfig",
					"metadata": map[string]interface{}{
						"name": renderedConfigName,
					},
					"spec": map[string]interface{}{
						"kernelArguments": []interface{}{"psi=1"},
					},
				},
			}
			renderedMC.SetGroupVersionKind(machineConfigGVK)
			Expect(integrationK8sClient.Create(integrationCtx, renderedMC)).To(Succeed())

			// Generate plan items
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(HaveLen(2)) // KubeDescheduler + MachineConfig

			// Find the MachineConfig item
			var mcItem *hcov1alpha1.VirtPlatformConfigItem
			for i := range items {
				if items[i].TargetRef.Name == machineConfigName {
					mcItem = &items[i]
					break
				}
			}
			Expect(mcItem).NotTo(BeNil(), "MachineConfig item should be generated")

			// Verify effect-based validation detected PSI is already present
			Expect(mcItem.ImpactSeverity).To(Equal("None - PSI metrics already enabled in pool"))
			Expect(mcItem.Message).To(ContainSubstring("PSI metrics already effective"))
			Expect(mcItem.Message).To(ContainSubstring(workerPoolName))

			// Cleanup
			Expect(integrationK8sClient.Delete(integrationCtx, pool)).To(Succeed())
			Expect(integrationK8sClient.Delete(integrationCtx, renderedMC)).To(Succeed())
		})

		It("should detect PSI not in rendered config and set impact to High", func() {
			// Create a MachineConfigPool with a rendered config WITHOUT psi=1
			pool := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "machineconfiguration.openshift.io/v1",
					"kind":       "MachineConfigPool",
					"metadata": map[string]interface{}{
						"name": workerPoolName,
					},
					"status": map[string]interface{}{
						"configuration": map[string]interface{}{
							"name": renderedConfigName,
						},
					},
				},
			}
			pool.SetGroupVersionKind(machineConfigPoolGVK)
			Expect(integrationK8sClient.Create(integrationCtx, pool)).To(Succeed())

			// Create the rendered MachineConfig WITHOUT psi=1
			renderedMC := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "machineconfiguration.openshift.io/v1",
					"kind":       "MachineConfig",
					"metadata": map[string]interface{}{
						"name": renderedConfigName,
					},
					"spec": map[string]interface{}{
						"kernelArguments": []interface{}{"debug", "other=value"},
					},
				},
			}
			renderedMC.SetGroupVersionKind(machineConfigGVK)
			Expect(integrationK8sClient.Create(integrationCtx, renderedMC)).To(Succeed())

			// Generate plan items
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(HaveLen(2))

			// Find the MachineConfig item
			var mcItem *hcov1alpha1.VirtPlatformConfigItem
			for i := range items {
				if items[i].TargetRef.Name == machineConfigName {
					mcItem = &items[i]
					break
				}
			}
			Expect(mcItem).NotTo(BeNil())

			// Verify PSI is NOT detected, so reboot is required
			Expect(mcItem.ImpactSeverity).To(Equal("High - Node reboot required for kernel arguments"))
			Expect(mcItem.Message).To(ContainSubstring("will be configured to enable PSI metrics"))

			// Cleanup
			Expect(integrationK8sClient.Delete(integrationCtx, pool)).To(Succeed())
			Expect(integrationK8sClient.Delete(integrationCtx, renderedMC)).To(Succeed())
		})

		It("should handle last occurrence wins for PSI (psi=0 then psi=1)", func() {
			// Create pool and rendered config with psi=0 first, then psi=1 (last wins)
			pool := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "machineconfiguration.openshift.io/v1",
					"kind":       "MachineConfigPool",
					"metadata": map[string]interface{}{
						"name": workerPoolName,
					},
					"status": map[string]interface{}{
						"configuration": map[string]interface{}{
							"name": renderedConfigName,
						},
					},
				},
			}
			pool.SetGroupVersionKind(machineConfigPoolGVK)
			Expect(integrationK8sClient.Create(integrationCtx, pool)).To(Succeed())

			renderedMC := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "machineconfiguration.openshift.io/v1",
					"kind":       "MachineConfig",
					"metadata": map[string]interface{}{
						"name": renderedConfigName,
					},
					"spec": map[string]interface{}{
						"kernelArguments": []interface{}{"psi=0", "debug", "psi=1"},
					},
				},
			}
			renderedMC.SetGroupVersionKind(machineConfigGVK)
			Expect(integrationK8sClient.Create(integrationCtx, renderedMC)).To(Succeed())

			// Generate plan items
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			// Find the MachineConfig item
			var mcItem *hcov1alpha1.VirtPlatformConfigItem
			for i := range items {
				if items[i].TargetRef.Name == machineConfigName {
					mcItem = &items[i]
					break
				}
			}
			Expect(mcItem).NotTo(BeNil())

			// Last occurrence is psi=1, so it should be detected as already effective
			Expect(mcItem.ImpactSeverity).To(Equal("None - PSI metrics already enabled in pool"))

			// Cleanup
			Expect(integrationK8sClient.Delete(integrationCtx, pool)).To(Succeed())
			Expect(integrationK8sClient.Delete(integrationCtx, renderedMC)).To(Succeed())
		})

		It("should handle last occurrence wins for PSI (psi=1 then psi=0)", func() {
			// Create pool and rendered config with psi=1 first, then psi=0 (last wins)
			pool := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "machineconfiguration.openshift.io/v1",
					"kind":       "MachineConfigPool",
					"metadata": map[string]interface{}{
						"name": workerPoolName,
					},
					"status": map[string]interface{}{
						"configuration": map[string]interface{}{
							"name": renderedConfigName,
						},
					},
				},
			}
			pool.SetGroupVersionKind(machineConfigPoolGVK)
			Expect(integrationK8sClient.Create(integrationCtx, pool)).To(Succeed())

			renderedMC := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "machineconfiguration.openshift.io/v1",
					"kind":       "MachineConfig",
					"metadata": map[string]interface{}{
						"name": renderedConfigName,
					},
					"spec": map[string]interface{}{
						"kernelArguments": []interface{}{"psi=1", "debug", "psi=0"},
					},
				},
			}
			renderedMC.SetGroupVersionKind(machineConfigGVK)
			Expect(integrationK8sClient.Create(integrationCtx, renderedMC)).To(Succeed())

			// Generate plan items
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			// Find the MachineConfig item
			var mcItem *hcov1alpha1.VirtPlatformConfigItem
			for i := range items {
				if items[i].TargetRef.Name == machineConfigName {
					mcItem = &items[i]
					break
				}
			}
			Expect(mcItem).NotTo(BeNil())

			// Last occurrence is psi=0, so psi=1 is NOT effective
			Expect(mcItem.ImpactSeverity).To(Equal("High - Node reboot required for kernel arguments"))

			// Cleanup
			Expect(integrationK8sClient.Delete(integrationCtx, pool)).To(Succeed())
			Expect(integrationK8sClient.Delete(integrationCtx, renderedMC)).To(Succeed())
		})

		It("should gracefully handle missing pool (log warning and continue)", func() {
			// Don't create the pool - it doesn't exist

			// Generate plan items should still succeed
			items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, map[string]string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(HaveLen(2))

			// Find the MachineConfig item
			var mcItem *hcov1alpha1.VirtPlatformConfigItem
			for i := range items {
				if items[i].TargetRef.Name == machineConfigName {
					mcItem = &items[i]
					break
				}
			}
			Expect(mcItem).NotTo(BeNil())

			// Should default to High impact since we can't validate
			Expect(mcItem.ImpactSeverity).To(Equal("High - Node reboot required for kernel arguments"))
		})
	})
})
