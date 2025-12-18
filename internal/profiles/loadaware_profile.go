package profiles

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
	"github.com/kubevirt/virt-advisor-operator/internal/discovery"
	"github.com/kubevirt/virt-advisor-operator/internal/plan"
)

const (
	// ProfileNameLoadAware is the name of the load-aware rebalancing profile
	ProfileNameLoadAware = "load-aware-rebalancing"

	// Default descheduling interval for load-aware profile (1 minute)
	defaultDeschedulingInterval = 60

	// PSI kernel argument to enable pressure stall information
	psiKernelArg = "psi=1"

	// CRD names
	kubeDeschedulerCRD = "kubedeschedulers.operator.openshift.io"
	machineConfigCRD   = "machineconfigs.machineconfiguration.openshift.io"

	// KubeDescheduler resource details
	kubeDeschedulerNamespace = "openshift-kube-descheduler-operator"
	kubeDeschedulerName      = "cluster"

	// KubeDescheduler profile names (in order of preference)
	profileKubeVirtRelieveAndMigrate    = "KubeVirtRelieveAndMigrate"
	profileDevKubeVirtRelieveAndMigrate = "DevKubeVirtRelieveAndMigrate"
	profileLongLifecycle                = "LongLifecycle"

	// Profiles that should be preserved if they exist
	profileAffinityAndTaints         = "AffinityAndTaints"
	profileSoftTopologyAndDuplicates = "SoftTopologyAndDuplicates"

	// Default value for devDeviationThresholds in profileCustomization
	defaultDeviationThresholds = "AsymmetricLow"

	// Default mode for descheduler
	defaultMode = "Automatic"
)

var (
	// KubeDeschedulerGVK is the GroupVersionKind for KubeDescheduler
	KubeDeschedulerGVK = schema.GroupVersionKind{
		Group:   "operator.openshift.io",
		Version: "v1",
		Kind:    "KubeDescheduler",
	}

	// MachineConfigGVK is the GroupVersionKind for MachineConfig
	MachineConfigGVK = schema.GroupVersionKind{
		Group:   "machineconfiguration.openshift.io",
		Version: "v1",
		Kind:    "MachineConfig",
	}
)

// LoadAwareRebalancingProfile implements the load-aware rebalancing capability
// from the VEP. It configures:
// 1. KubeDescheduler to enable load-aware scheduling
// 2. MachineConfig to enable PSI (Pressure Stall Information) metrics
//
// NOTE: This profile uses dynamic/unstructured objects to avoid requiring
// the third-party CRDs to be registered in the operator's scheme.
// This allows the operator to start even if the CRDs don't exist yet.
type LoadAwareRebalancingProfile struct {
	name string
}

// NewLoadAwareRebalancingProfile creates a new load-aware rebalancing profile
func NewLoadAwareRebalancingProfile() *LoadAwareRebalancingProfile {
	return &LoadAwareRebalancingProfile{
		name: ProfileNameLoadAware,
	}
}

// GetName returns the profile name
func (p *LoadAwareRebalancingProfile) GetName() string {
	return p.name
}

// GetPrerequisites returns the CRDs required by this profile
func (p *LoadAwareRebalancingProfile) GetPrerequisites() []discovery.Prerequisite {
	return []discovery.Prerequisite{
		{
			GVK:         KubeDeschedulerGVK,
			Description: "Please install the Descheduler Operator via OLM",
		},
		{
			GVK:         MachineConfigGVK,
			Description: "MachineConfig CRD is required for PSI metrics configuration (typically available on OpenShift)",
		},
	}
}

// GetManagedResourceTypes returns the resource types this profile manages for drift detection
func (p *LoadAwareRebalancingProfile) GetManagedResourceTypes() []schema.GroupVersionKind {
	return []schema.GroupVersionKind{
		KubeDeschedulerGVK,
		MachineConfigGVK,
	}
}

// Validate checks if the provided config overrides are valid
func (p *LoadAwareRebalancingProfile) Validate(configOverrides map[string]string) error {
	supportedKeys := map[string]bool{
		"deschedulingIntervalSeconds": true,
		"enablePSIMetrics":            true,
		"devDeviationThresholds":      true,
	}

	for key := range configOverrides {
		if !supportedKeys[key] {
			return fmt.Errorf("unsupported config override: %q (supported: deschedulingIntervalSeconds, enablePSIMetrics, devDeviationThresholds)", key)
		}
	}

	return nil
}

// GeneratePlanItems creates the configuration items for this profile
func (p *LoadAwareRebalancingProfile) GeneratePlanItems(ctx context.Context, c client.Client, configOverrides map[string]string) ([]advisorv1alpha1.VirtPlatformConfigItem, error) {
	// First, check if required CRDs exist
	checker := plan.NewCRDChecker(c)
	requiredCRDs := []string{kubeDeschedulerCRD}

	// Determine configuration values
	enablePSI := true // Default to enabled
	if val, ok := configOverrides["enablePSIMetrics"]; ok && val == "false" {
		enablePSI = false
	}

	if enablePSI {
		requiredCRDs = append(requiredCRDs, machineConfigCRD)
	}

	// Check for missing CRDs
	missingCRDs, err := checker.CheckRequiredCRDs(ctx, requiredCRDs)
	if err != nil {
		return nil, fmt.Errorf("failed to check required CRDs: %w", err)
	}

	if len(missingCRDs) > 0 {
		return nil, fmt.Errorf("required CRDs not found in cluster: %v (install them first)", missingCRDs)
	}

	var items []advisorv1alpha1.VirtPlatformConfigItem

	deschedulingInterval := int32(defaultDeschedulingInterval)
	if val, ok := configOverrides["deschedulingIntervalSeconds"]; ok {
		var parsed int32
		if _, err := fmt.Sscanf(val, "%d", &parsed); err == nil && parsed > 0 {
			deschedulingInterval = parsed
		}
	}

	// Item 1: Configure KubeDescheduler
	deschedulerItem, err := p.generateDeschedulerItem(ctx, c, deschedulingInterval, configOverrides)
	if err != nil {
		return nil, fmt.Errorf("failed to generate descheduler item: %w", err)
	}
	items = append(items, deschedulerItem)

	// Item 2: Configure MachineConfig for PSI metrics (if enabled)
	if enablePSI {
		mcItem, err := p.generateMachineConfigItem(ctx, c)
		if err != nil {
			return nil, fmt.Errorf("failed to generate machine config item: %w", err)
		}
		items = append(items, mcItem)
	}

	return items, nil
}

// selectDeschedulerProfile determines the best available descheduler profile to use.
// It follows the preference order:
// 1. KubeVirtRelieveAndMigrate (primary)
// 2. DevKubeVirtRelieveAndMigrate (fallback if primary not available)
// 3. LongLifecycle (fallback if neither primary nor dev preview available)
//
// The function checks the existing KubeDescheduler CRD schema to determine which
// profiles are available in the cluster.
func (p *LoadAwareRebalancingProfile) selectDeschedulerProfile(ctx context.Context, c client.Client) string {
	// Define preference order
	preferredProfiles := []string{
		profileKubeVirtRelieveAndMigrate,
		profileDevKubeVirtRelieveAndMigrate,
		profileLongLifecycle,
	}

	// Try to fetch the KubeDescheduler CRD to check available profiles
	crdGVK := schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	}

	crd, err := plan.GetUnstructured(ctx, c, crdGVK, kubeDeschedulerCRD, "")
	if err != nil {
		// If we can't fetch the CRD, default to the primary profile
		return profileKubeVirtRelieveAndMigrate
	}

	// Extract available profiles from the CRD schema
	// The profiles are typically defined as an enum in the spec validation
	availableProfiles := extractAvailableProfiles(crd)

	// Select the first profile from our preference list that's available
	for _, preferred := range preferredProfiles {
		if stringInSlice(preferred, availableProfiles) {
			return preferred
		}
	}

	// If no preferred profiles are found, default to primary
	return profileKubeVirtRelieveAndMigrate
}

// stringInSlice checks if a string is present in a slice of strings
func stringInSlice(s string, slice []string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// extractAvailableProfiles parses the CRD schema to find available profile names
func extractAvailableProfiles(crd *unstructured.Unstructured) []string {
	// Navigate the CRD schema to find the profiles enum
	// Path: spec.versions[].schema.openAPIV3Schema.properties.spec.properties.profiles.items.enum
	versions, found, err := unstructured.NestedSlice(crd.Object, "spec", "versions")
	if !found || err != nil {
		return nil
	}

	for _, v := range versions {
		version, ok := v.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if this version has a schema
		schema, found, err := unstructured.NestedMap(version, "schema", "openAPIV3Schema", "properties", "spec", "properties", "profiles")
		if !found || err != nil {
			continue
		}

		// Check for items.enum (for array of strings)
		if enumValues, found, _ := unstructured.NestedSlice(schema, "items", "enum"); found {
			profiles := make([]string, 0, len(enumValues))
			for _, e := range enumValues {
				if s, ok := e.(string); ok {
					profiles = append(profiles, s)
				}
			}
			if len(profiles) > 0 {
				return profiles
			}
		}
	}

	return nil
}

// buildProfilesList constructs the desired profiles list by:
// 1. Preserving AffinityAndTaints and SoftTopologyAndDuplicates if they already exist
// 2. Adding the selected KubeVirt profile
// 3. Removing all other profiles
func (p *LoadAwareRebalancingProfile) buildProfilesList(ctx context.Context, c client.Client, kubeVirtProfile string) []interface{} {
	preservedProfiles := make([]interface{}, 0, 3)

	// Try to read the existing KubeDescheduler resource
	existing, err := plan.GetUnstructured(ctx, c, KubeDeschedulerGVK, kubeDeschedulerName, kubeDeschedulerNamespace)
	if err == nil {
		// Extract existing profiles
		currentProfiles, found, _ := unstructured.NestedSlice(existing.Object, "spec", "profiles")
		if found {
			// Keep only AffinityAndTaints and SoftTopologyAndDuplicates
			for _, p := range currentProfiles {
				if profileStr, ok := p.(string); ok {
					if profileStr == profileAffinityAndTaints || profileStr == profileSoftTopologyAndDuplicates {
						preservedProfiles = append(preservedProfiles, profileStr)
					}
				}
			}
		}
	}

	// Add the KubeVirt profile
	preservedProfiles = append(preservedProfiles, kubeVirtProfile)

	return preservedProfiles
}

// generateDeschedulerItem creates a plan item for KubeDescheduler configuration
func (p *LoadAwareRebalancingProfile) generateDeschedulerItem(ctx context.Context, c client.Client, interval int32, configOverrides map[string]string) (advisorv1alpha1.VirtPlatformConfigItem, error) {
	// Select the best available descheduler profile
	profile := p.selectDeschedulerProfile(ctx, c)

	// Build the list of profiles: preserve AffinityAndTaints and SoftTopologyAndDuplicates if they exist
	profiles := p.buildProfilesList(ctx, c, profile)

	// Build the desired KubeDescheduler object
	desired := plan.CreateUnstructured(KubeDeschedulerGVK, kubeDeschedulerName, kubeDeschedulerNamespace)

	// Default mode to Automatic
	mode := defaultMode
	if customMode, ok := configOverrides["mode"]; ok && customMode != "" {
		mode = customMode
	}

	// Set the spec with our desired configuration
	spec := map[string]interface{}{
		"deschedulingIntervalSeconds": int64(interval),
		"mode":                        mode,
		"profiles":                    profiles,
	}

	// Add profileCustomizations for KubeVirtRelieveAndMigrate and DevKubeVirtRelieveAndMigrate
	// These settings are NOT applicable to LongLifecycle
	// Note: The field name is "profileCustomizations" (plural) in the CRD schema
	if profile == profileKubeVirtRelieveAndMigrate || profile == profileDevKubeVirtRelieveAndMigrate {
		// Default devDeviationThresholds to AsymmetricLow
		devThresholds := defaultDeviationThresholds
		if customThresholds, ok := configOverrides["devDeviationThresholds"]; ok && customThresholds != "" {
			devThresholds = customThresholds
		}

		profileCustomizations := map[string]interface{}{
			"devEnableEvictionsInBackground": true,
			"devEnableSoftTainter":           true,
			"devDeviationThresholds":         devThresholds,
		}

		spec["profileCustomizations"] = profileCustomizations
	}

	if err := unstructured.SetNestedMap(desired.Object, spec, "spec"); err != nil {
		return advisorv1alpha1.VirtPlatformConfigItem{}, fmt.Errorf("failed to set spec: %w", err)
	}

	// Determine managed fields based on whether profileCustomizations is set
	managedFields := []string{"spec.deschedulingIntervalSeconds", "spec.mode", "spec.profiles"}
	if profile == profileKubeVirtRelieveAndMigrate || profile == profileDevKubeVirtRelieveAndMigrate {
		managedFields = append(managedFields, "spec.profileCustomizations")
	}

	// Use the builder to create the item with SSA-generated diff
	return NewPlanItemBuilder(ctx, c, "virt-advisor-operator").
		ForResource(desired, "enable-load-aware-descheduling").
		WithManagedFields(managedFields).
		WithMessage(fmt.Sprintf("KubeDescheduler 'cluster' will be configured with profile '%s'", profile)).
		Build()
}

// generateMachineConfigItem creates a plan item for MachineConfig PSI enablement
func (p *LoadAwareRebalancingProfile) generateMachineConfigItem(ctx context.Context, c client.Client) (advisorv1alpha1.VirtPlatformConfigItem, error) {
	mcName := "99-worker-psi-karg"

	// Build the desired MachineConfig object
	desired := plan.CreateUnstructured(MachineConfigGVK, mcName, "")

	// Set labels
	desired.SetLabels(map[string]string{
		"machineconfiguration.openshift.io/role": "worker",
	})

	// Set the spec with kernel arguments
	spec := map[string]interface{}{
		"kernelArguments": []interface{}{psiKernelArg},
	}

	if err := unstructured.SetNestedMap(desired.Object, spec, "spec"); err != nil {
		return advisorv1alpha1.VirtPlatformConfigItem{}, fmt.Errorf("failed to set spec: %w", err)
	}

	// Determine impact based on effect-based validation
	// Check if PSI is already effective in the merged MachineConfigPool config
	poolName := "worker"
	impact := advisorv1alpha1.ImpactHigh
	message := fmt.Sprintf("MachineConfig '%s' will be configured to enable PSI metrics", mcName)

	// EFFECT-BASED VALIDATION: Check if PSI is already effective in the pool
	isPresent, _, err := ValidateKernelArgInPool(ctx, c, poolName, psiKernelArg)
	if err != nil {
		// Pool might not exist in test environments - log but continue
		log.FromContext(ctx).V(1).Info("Could not validate kernel arg in pool", "pool", poolName, "error", err)
	} else if isPresent {
		// PSI is already effective in the merged config - no reboot needed
		impact = advisorv1alpha1.ImpactLow
		message = fmt.Sprintf("PSI metrics already effective in MachineConfigPool '%s'", poolName)
	}

	// Use the builder to create the item with SSA-generated diff
	return NewPlanItemBuilder(ctx, c, "virt-advisor-operator").
		ForResource(desired, "enable-psi-metrics").
		WithManagedFields([]string{"spec.kernelArguments"}).
		WithImpact(impact).
		WithMessage(message).
		Build()
}

// GetDescription returns a human-readable description of this profile
func (p *LoadAwareRebalancingProfile) GetDescription() string {
	return "Enables load-aware VM rebalancing and PSI metrics for intelligent scheduling"
}

// GetCategory returns the category this profile belongs to
func (p *LoadAwareRebalancingProfile) GetCategory() string {
	return "scheduling"
}

// GetImpactSummary returns a summary of the impact of enabling this profile
func (p *LoadAwareRebalancingProfile) GetImpactSummary() string {
	return "Medium - May require node reboots if PSI metrics are enabled"
}

// GetImpactLevel returns the aggregate risk level
func (p *LoadAwareRebalancingProfile) GetImpactLevel() advisorv1alpha1.Impact {
	return advisorv1alpha1.ImpactHigh
}

// IsAdvertisable returns true since this is a production profile
func (p *LoadAwareRebalancingProfile) IsAdvertisable() bool {
	return true
}

func init() {
	// Register this profile in the default registry
	if err := DefaultRegistry.Register(NewLoadAwareRebalancingProfile()); err != nil {
		panic(fmt.Sprintf("failed to register load-aware rebalancing profile: %v", err))
	}
}
