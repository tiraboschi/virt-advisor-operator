# Profile Development Guide

## Overview

This guide explains how to create new configuration profiles for the virt-advisor-operator. Profiles define reusable configuration capabilities that can be applied to the cluster with proper governance controls.

## Directory Structure

Profiles are organized in subdirectories under `internal/profiles/`. Each profile has its own package:

```
internal/profiles/
├── profiles.go              # Central registry and Profile interface
├── profileutils/            # Shared utilities
│   ├── constants.go         # Shared GVKs and constants
│   ├── config_helpers.go    # Configuration helper functions
│   └── builder.go           # PlanItemBuilder
├── loadaware/               # Load-aware rebalancing profile
│   ├── profile.go
│   └── profile_integration_test.go
├── higherdensity/           # Higher-density profile
│   ├── profile.go
│   └── profile_integration_test.go
└── example/                 # Example profile
    └── profile.go
```

**Key principles:**
- ✅ **One profile per subdirectory** - Enables CODEOWNERS protection
- ✅ **Package name matches directory** - Clear organization (e.g., `package loadaware`)
- ✅ **Integration tests in same directory** - Keep tests with implementation
- ✅ **Shared code in profileutils/** - Avoid circular dependencies

## CRITICAL REQUIREMENT: Always Use PlanItemBuilder

**You MUST use `PlanItemBuilder` when creating `VirtPlatformConfigItem` objects.** This ensures:

1. ✅ **API Server Validation**: Diffs reflect CEL validation rules
2. ✅ **Defaulting**: Shows actual defaults the API server will apply
3. ✅ **Webhook Processing**: Captures mutations from admission webhooks
4. ✅ **Accurate Diffs**: What you see is exactly what will be applied

### ❌ WRONG - Manual Diff Construction

```go
// DO NOT DO THIS - Bypasses API server validation!
func (p *MyProfile) GeneratePlanItems(ctx context.Context, c client.Client, overrides map[string]string) ([]advisorv1alpha1.VirtPlatformConfigItem, error) {
    diff := fmt.Sprintf(`--- my-resource
+++ my-resource
@@ -1,1 +1,1 @@
 spec:
-  oldValue: 1
+  newValue: 2
`)

    return []advisorv1alpha1.VirtPlatformConfigItem{{
        Name:       "my-item",
        Diff:       diff,  // ❌ Manual string - NO validation!
        TargetRef:  /* ... */,
        // ...
    }}, nil
}
```

**Problems with manual diffs:**
- ❌ No CEL validation - invalid configs pass through
- ❌ No defaulting - missing fields not shown
- ❌ No webhook processing - mutations not reflected
- ❌ Stale on API changes - breaks when CRD schema updates

### ✅ CORRECT - Using PlanItemBuilder

```go
func (p *MyProfile) GeneratePlanItems(ctx context.Context, c client.Client, overrides map[string]string) ([]advisorv1alpha1.VirtPlatformConfigItem, error) {
    // 1. Build the desired object
    desired := plan.CreateUnstructured(MyGVK, "my-resource", "")

    spec := map[string]interface{}{
        "newValue": 2,
    }

    if err := unstructured.SetNestedMap(desired.Object, spec, "spec"); err != nil {
        return nil, fmt.Errorf("failed to set spec: %w", err)
    }

    // 2. Use the builder to create the item with SSA-generated diff
    item, err := profileutils.NewPlanItemBuilder(ctx, c, "virt-advisor-operator").
        ForResource(desired, "my-item").
        WithManagedFields([]string{"spec.newValue"}).
        WithMessage("MyResource will be configured").
        Build()  // ✅ This performs SSA dry-run for accurate diff!

    if err != nil {
        return nil, err
    }

    return []advisorv1alpha1.VirtPlatformConfigItem{item}, nil
}
```

## PlanItemBuilder API

### Required Methods

**`ForResource(desired *unstructured.Unstructured, name string)`**
- Sets the desired resource configuration and item name
- The desired object is what you want the resource to look like after applying

### Optional Methods

**`WithManagedFields(fields []string)`**
- Specifies which spec fields this item manages
- Used for drift detection to only monitor fields we control
- Format: JSON path like `"spec.profiles"`, `"spec.kernelArguments"`

**`WithImpactSeverity(severity string)`**
- Sets the human-readable impact description
- If not set, auto-determined based on whether resource exists
- Examples: `"Low - Updates existing configuration"`, `"High - Node reboot required"`

**`WithMessage(message string)`**
- Sets the initial status message
- If not set, auto-generated from resource kind and name

### Build Process

When you call `.Build()`, the builder:

1. **Validates** required fields are set
2. **Auto-determines** impact severity if not manually set
3. **Performs SSA dry-run** with `DryRun=true`, `Force=true`
4. **Generates unified diff** comparing live vs. dry-run result
5. **Returns** complete `VirtPlatformConfigItem`

## Complete Example: LoadAwareRebalancing Profile

```go
package loadaware

import (
    "context"
    "fmt"

    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "sigs.k8s.io/controller-runtime/pkg/client"

    advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
    "github.com/kubevirt/virt-advisor-operator/internal/plan"
    "github.com/kubevirt/virt-advisor-operator/internal/profiles/profileutils"
)

const ProfileNameLoadAware = "load-aware-rebalancing"

type LoadAwareRebalancingProfile struct {
    name string
}

func NewLoadAwareRebalancingProfile() *LoadAwareRebalancingProfile {
    return &LoadAwareRebalancingProfile{
        name: ProfileNameLoadAware,
    }
}

func (p *LoadAwareRebalancingProfile) GetName() string {
    return p.name
}

func (p *LoadAwareRebalancingProfile) Validate(configOverrides map[string]string) error {
    // Validate config overrides
    return nil
}

func (p *LoadAwareRebalancingProfile) GeneratePlanItems(ctx context.Context, c client.Client, configOverrides map[string]string) ([]advisorv1alpha1.VirtPlatformConfigItem, error) {
    // Select the best available descheduler profile based on CRD schema
    profile := p.selectDeschedulerProfile(ctx, c)  // Returns KubeVirtRelieveAndMigrate, DevKubeVirtRelieveAndMigrate, or LongLifecycle

    // Build list of profiles, preserving AffinityAndTaints and SoftTopologyAndDuplicates
    profiles := p.buildProfilesList(ctx, c, profile)

    // Build desired configuration using shared GVK from profileutils
    desired := plan.CreateUnstructured(profileutils.KubeDeschedulerGVK, "cluster", "openshift-kube-descheduler-operator")

    spec := map[string]interface{}{
        "deschedulingIntervalSeconds": int64(1800),
        "profiles":                    profiles,
    }

    // Add profileCustomizations for KubeVirt profiles (not LongLifecycle)
    if profile == "KubeVirtRelieveAndMigrate" || profile == "DevKubeVirtRelieveAndMigrate" {
        devThresholds := "AsymmetricLow"  // Default
        if customThresholds, ok := configOverrides["devDeviationThresholds"]; ok && customThresholds != "" {
            devThresholds = customThresholds
        }

        spec["profileCustomizations"] = map[string]interface{}{
            "devEnableEvictionsInBackground": true,
            "devEnableSoftTainter":           true,
            "devDeviationThresholds":         devThresholds,
        }
    }

    if err := unstructured.SetNestedMap(desired.Object, spec, "spec"); err != nil {
        return nil, fmt.Errorf("failed to set spec: %w", err)
    }

    // Determine managed fields based on profile
    managedFields := []string{"spec.deschedulingIntervalSeconds", "spec.profiles"}
    if profile == "KubeVirtRelieveAndMigrate" || profile == "DevKubeVirtRelieveAndMigrate" {
        managedFields = append(managedFields, "spec.profileCustomizations")
    }

    // Use builder to create item with SSA-generated diff
    item, err := profileutils.NewPlanItemBuilder(ctx, c, "virt-advisor-operator").
        ForResource(desired, "enable-load-aware-descheduling").
        WithManagedFields(managedFields).
        WithMessage(fmt.Sprintf("KubeDescheduler 'cluster' will be configured with profile '%s'", profile)).
        Build()

    if err != nil {
        return nil, err
    }

    return []advisorv1alpha1.VirtPlatformConfigItem{item}, nil
}

// Note: Profile registration is handled by the parent profiles package in its init() function
// Parent directly instantiates: loadaware.NewLoadAwareRebalancingProfile()
```

## Creating a New Profile

Follow these steps to create a new profile:

### 1. Create a New Subdirectory

Create a new subdirectory under `internal/profiles/` with your profile name (e.g., `myprofile/`):

```bash
mkdir internal/profiles/myprofile
```

### 2. Create the Profile Implementation

Create `profile.go` in your subdirectory:

```go
package myprofile

import (
    "context"
    "fmt"

    "sigs.k8s.io/controller-runtime/pkg/client"

    advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
    "github.com/kubevirt/virt-advisor-operator/internal/profiles/profileutils"
)

const ProfileNameMyProfile = "my-profile"

type MyProfile struct {
    name string
}

func NewMyProfile() *MyProfile {
    return &MyProfile{
        name: ProfileNameMyProfile,
    }
}

func (p *MyProfile) GetName() string {
    return p.name
}

func (p *MyProfile) GetPrerequisites() []discovery.Prerequisite {
    // Return list of required CRDs
    return []discovery.Prerequisite{
        {
            GVK:         profileutils.SomeRequiredGVK,
            Description: "Please install the required operator via OLM",
        },
    }
}

func (p *MyProfile) GetManagedResourceTypes() []schema.GroupVersionKind {
    // Return list of resource types this profile manages
    return []schema.GroupVersionKind{
        profileutils.SomeRequiredGVK,
    }
}

func (p *MyProfile) Validate(configOverrides map[string]string) error {
    // Validate configuration overrides
    return nil
}

func (p *MyProfile) GeneratePlanItems(ctx context.Context, c client.Client, configOverrides map[string]string) ([]advisorv1alpha1.VirtPlatformConfigItem, error) {
    // Build your desired configuration
    desired := plan.CreateUnstructured(profileutils.SomeGVK, "resource-name", "namespace")

    // ... configure spec ...

    // Use builder to create plan item
    item, err := profileutils.NewPlanItemBuilder(ctx, c, "virt-advisor-operator").
        ForResource(desired, "my-config-item").
        WithManagedFields([]string{"spec.myField"}).
        WithMessage("My resource will be configured").
        Build()

    if err != nil {
        return nil, err
    }

    return []advisorv1alpha1.VirtPlatformConfigItem{item}, nil
}
```

### 3. Register in Parent Package

Add your profile to the parent package's `init()` function in `internal/profiles/profiles.go`:

```go
import (
    "github.com/kubevirt/virt-advisor-operator/internal/profiles/myprofile"
)

func init() {
    // ... existing profiles ...

    if err := DefaultRegistry.Register(myprofile.NewMyProfile()); err != nil {
        panic(fmt.Sprintf("failed to register my-profile: %v", err))
    }
}
```

### 4. Add Shared Constants (if needed)

If your profile needs to share GVKs or other constants with other profiles, add them to `internal/profiles/profileutils/constants.go`:

```go
var (
    MyNewGVK = schema.GroupVersionKind{
        Group:   "example.io",
        Version: "v1",
        Kind:    "MyResource",
    }
)
```

### 5. Create Integration Tests

Create `profile_integration_test.go` in your profile subdirectory:

```go
package myprofile

import (
    "context"
    "path/filepath"
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
    integrationTestEnv    *envtest.Environment
    integrationK8sClient  client.Client
    integrationCtx        context.Context
    integrationCancelFunc context.CancelFunc
)

var _ = BeforeSuite(func() {
    integrationCtx, integrationCancelFunc = context.WithCancel(context.Background())

    integrationTestEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "..", "config", "crd", "bases"),
            // Add mock CRDs if needed
        },
    }

    cfg, err := integrationTestEnv.Start()
    Expect(err).NotTo(HaveOccurred())

    integrationK8sClient, err = client.New(cfg, client.Options{})
    Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
    integrationCancelFunc()
    if integrationTestEnv != nil {
        Expect(integrationTestEnv.Stop()).To(Succeed())
    }
})

func TestMyProfileIntegration(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "MyProfile Integration Suite")
}

var _ = Describe("MyProfile Integration", func() {
    It("should generate plan items", func() {
        profile := NewMyProfile()
        items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, nil)
        Expect(err).NotTo(HaveOccurred())
        Expect(items).NotTo(BeEmpty())
    })
})
```

### 6. Add to CRD Validation

Update `api/v1alpha1/virtplatformconfig_types.go` to add your profile name to the enum:

```go
// +kubebuilder:validation:Enum=load-aware-rebalancing;virt-higher-density;my-profile
Profile string `json:"profile"`
```

### 7. CODEOWNERS Protection

Once your profile is in a subdirectory, you can add it to `.github/CODEOWNERS`:

```
/internal/profiles/myprofile/ @your-team
```

## Profile Interface Methods

All profiles must implement the `Profile` interface, which includes the following methods:

### GetName() string
Returns the unique identifier for the profile. This name must match the VirtPlatformConfig resource name (singleton pattern).

### GetPrerequisites() []discovery.Prerequisite
Returns the list of CRDs required by this profile. The controller checks these before generating the plan and moves to `PrerequisiteFailed` if any are missing.

```go
func (p *LoadAwareRebalancingProfile) GetPrerequisites() []discovery.Prerequisite {
    return []discovery.Prerequisite{
        {
            GVK:         KubeDeschedulerGVK,
            Description: "Please install the Descheduler Operator via OLM",
        },
        {
            GVK:         MachineConfigGVK,
            Description: "MachineConfig CRD is required for PSI metrics configuration",
        },
    }
}
```

### GetManagedResourceTypes() []schema.GroupVersionKind
**IMPORTANT**: This method enables the **plugin mechanism for drift detection**.

Returns the list of resource types (GVKs) that this profile manages. The controller **automatically watches** these resources for changes, enabling automatic drift detection without manual configuration.

**How it works:**
1. During startup, the controller queries all registered profiles
2. Collects all unique GVKs from `GetManagedResourceTypes()`
3. Dynamically creates watches for each resource type
4. When a managed resource changes, the controller automatically triggers reconciliation
5. Drift is detected and the VirtPlatformConfig transitions to `Drifted` phase

**Example:**
```go
func (p *LoadAwareRebalancingProfile) GetManagedResourceTypes() []schema.GroupVersionKind {
    return []schema.GroupVersionKind{
        KubeDeschedulerGVK,
        MachineConfigGVK,
    }
}
```

**Benefits:**
- ✅ **Automatic drift detection**: No manual watch configuration needed
- ✅ **Memory optimized**: Predicates filter to only cache managed resources
- ✅ **Plugin architecture**: Adding new profiles automatically adds their watches
- ✅ **Extensible**: Each profile declares what it needs to monitor

**Startup logs example (all CRDs present):**
```
INFO  setup  Setting up dynamic watches for managed resources
    {"profileCount": 2, "uniqueResourceTypes": 2}
INFO  setup  Registering watch for resource type
    {"group": "operator.openshift.io", "version": "v1", "kind": "KubeDescheduler"}
INFO  setup  Registering watch for resource type
    {"group": "machineconfiguration.openshift.io", "version": "v1", "kind": "MachineConfig"}
INFO  setup  Dynamic watch setup complete
    {"registeredWatches": 2, "skippedWatches": 0}
```

**Startup logs example (CRD missing):**
```
INFO  setup  Setting up dynamic watches for managed resources
    {"profileCount": 2, "uniqueResourceTypes": 2}
INFO  setup  Skipping watch for resource type - CRD not installed
    {"crd": "kubedeschedulers.operator.openshift.io", "group": "operator.openshift.io",
     "version": "v1", "kind": "KubeDescheduler",
     "note": "Watch will not be registered. Profiles using this resource will fail prerequisite checks."}
INFO  setup  Registering watch for resource type
    {"group": "machineconfiguration.openshift.io", "version": "v1", "kind": "MachineConfig"}
INFO  setup  Dynamic watch setup complete
    {"registeredWatches": 1, "skippedWatches": 1}
```

**Critical Implementation Detail:**
The controller uses a **non-cached direct client** during `SetupWithManager` to check CRD existence. This is necessary because the manager's cache hasn't started yet during setup. Without this:
- Trying to use the cached client would fail with "cache is not started"
- Trying to register watches for non-existent CRDs would cause controller-runtime startup failures

This design ensures:
- ✅ **Graceful degradation**: Missing CRDs don't prevent operator startup
- ✅ **Cross-platform support**: Works on vanilla Kubernetes (without OpenShift CRDs)
- ✅ **Clear feedback**: Logs show which watches were skipped and why

### Advanced: Profiles That Depend on HyperConverged (HCO) Configuration

If your profile needs to read values from the **HyperConverged (HCO) CR** in OpenShift Virtualization, the controller already has infrastructure to automatically trigger drift detection when HCO changes. You just need to integrate your profile with the existing mechanism.

**Examples of HCO-dependent profiles:**
- **LoadAwareRebalancing**: Reads `spec.liveMigrationConfig` to derive eviction limits
- **VirtHigherDensity**: Reads `spec.higherWorkloadDensity.memoryOvercommitPercentage` and `spec.configuration.ksmConfiguration`

#### What You Need to Do as a Profile Author

**1. Update the HCO predicate in the controller** (`internal/controller/virtplatformconfig_controller.go:2228-2249`)

Add a check for your HCO field in the `hcoConfigPredicate()` function:

```go
func (r *VirtPlatformConfigReconciler) hcoConfigPredicate() predicate.Predicate {
    return predicate.Funcs{
        UpdateFunc: func(e event.UpdateEvent) bool {
            oldObj := e.ObjectOld.(*unstructured.Unstructured)
            newObj := e.ObjectNew.(*unstructured.Unstructured)

            // ... existing checks ...

            // Add your field check here
            oldYourField, _, _ := unstructured.NestedInt64(oldObj.Object, "spec", "yourSection", "yourField")
            newYourField, _, _ := unstructured.NestedInt64(newObj.Object, "spec", "yourSection", "yourField")
            if oldYourField != newYourField {
                return true
            }

            return false
        },
    }
}
```

**2. Update the event handler** (`internal/controller/virtplatformconfig_controller.go:2260-2326`)

Add your profile to `enqueueHCODependentConfigs()` so it gets reconciled when HCO changes:

```go
// Check if this config uses your profile
if config.Spec.Profile == "your-profile-name" {
    // Only enqueue if user hasn't provided explicit overrides
    if !hasExplicitYourProfileOptions(&config) {
        requests = append(requests, reconcile.Request{
            NamespacedName: types.NamespacedName{
                Name: config.Name,
            },
        })
    }
}
```

**3. Implement drift detection method in the controller**

Add a drift detection method similar to `hcoInputDependenciesChanged()` (line 1562) or `hcoHigherDensityConfigChanged()` (line 1665):

```go
func (r *VirtPlatformConfigReconciler) hcoYourProfileConfigChanged(ctx context.Context, config *advisorv1alpha1.VirtPlatformConfig) (bool, string) {
    if config.Spec.Profile != "your-profile-name" {
        return false, ""
    }

    if hasExplicitYourProfileOptions(config) {
        return false, "" // User override takes precedence
    }

    // Read current HCO value
    currentValue := // ... read from HCO CR using profileutils.GetHCO() ...

    // Read applied value from config.Status.Items
    appliedValue := // ... extract from applied plan items ...

    if currentValue != appliedValue {
        return true, fmt.Sprintf("HCO field changed: %v -> %v", appliedValue, currentValue)
    }

    return false, ""
}
```

**4. Call your drift detection in the reconciliation loop** (~line 400)

Add your drift check after the existing HCO drift checks.

**Reference implementations:**
- `hcoInputDependenciesChanged()` (line 1562): LoadAware profile pattern
- `hcoHigherDensityConfigChanged()` (line 1665): VirtHigherDensity profile pattern

### Validate(configOverrides map[string]string) error
Validates that the provided configuration overrides are valid for this profile. Returns an error if any override is unsupported or invalid.

### GeneratePlanItems(ctx, client, configOverrides) ([]VirtPlatformConfigItem, error)
Generates the ordered list of configuration items to apply. **MUST use PlanItemBuilder** to create items (see "CRITICAL REQUIREMENT" section above).

## Testing Your Profile

### Integration Tests in Profile Subdirectory

Each profile should have integration tests in its own subdirectory using a dedicated test environment:

```go
package myprofile

import (
    "context"
    "path/filepath"
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/envtest"

    advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
)

var (
    integrationTestEnv    *envtest.Environment
    integrationK8sClient  client.Client
    integrationCtx        context.Context
    integrationCancelFunc context.CancelFunc
)

var _ = BeforeSuite(func() {
    integrationCtx, integrationCancelFunc = context.WithCancel(context.Background())

    // Note: CRD paths use "../../../" due to subdirectory structure
    integrationTestEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "..", "config", "crd", "bases"),
        },
    }

    cfg, err := integrationTestEnv.Start()
    Expect(err).NotTo(HaveOccurred())

    integrationK8sClient, err = client.New(cfg, client.Options{})
    Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
    integrationCancelFunc()
    if integrationTestEnv != nil {
        Expect(integrationTestEnv.Stop()).To(Succeed())
    }
})

func TestMyProfileIntegration(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "MyProfile Integration Suite")
}

var _ = Describe("MyProfile Integration", func() {
    It("should generate valid plan items with SSA diffs", func() {
        profile := NewMyProfile()

        items, err := profile.GeneratePlanItems(integrationCtx, integrationK8sClient, nil)
        Expect(err).NotTo(HaveOccurred())
        Expect(items).NotTo(BeEmpty())

        // Verify SSA diff was generated (not manual string)
        Expect(items[0].Diff).NotTo(BeEmpty())
        Expect(items[0].Diff).To(ContainSubstring("---"))  // Unified diff format
        Expect(items[0].Diff).To(ContainSubstring("+++"))

        // Verify TargetRef is set
        Expect(items[0].TargetRef.Name).NotTo(BeEmpty())
        Expect(items[0].TargetRef.APIVersion).NotTo(BeEmpty())
        Expect(items[0].TargetRef.Kind).NotTo(BeEmpty())
    })

    It("should validate configuration overrides", func() {
        profile := NewMyProfile()

        // Test valid configuration
        err := profile.Validate(map[string]string{
            "someOption": "validValue",
        })
        Expect(err).NotTo(HaveOccurred())

        // Test invalid configuration
        err = profile.Validate(map[string]string{
            "invalidOption": "value",
        })
        Expect(err).To(HaveOccurred())
    })
})
```

### Key Testing Principles

1. **Use envtest**: Provides a real API server with validation, defaulting, and webhooks
2. **Test SSA behavior**: Verify that diffs are generated through Server-Side Apply
3. **Test configuration validation**: Ensure invalid options are rejected
4. **Test prerequisites**: Verify that missing CRDs are properly detected
5. **Path adjustments**: Remember to use `"../../../"` for CRD paths due to subdirectory nesting

## Benefits of PlanItemBuilder

### Safety

- **Compile-time safety**: Can't forget to generate diff
- **API validation**: Catches invalid configurations early
- **Consistent behavior**: All profiles use same diff generation logic

### Maintainability

- **Centralized logic**: SSA changes happen in one place
- **Easy upgrades**: API server updates automatically reflected
- **Reduced code**: No manual diff string concatenation

### Correctness

- **Accurate previews**: User sees exactly what will be applied
- **Field ownership**: Respects co-management scenarios
- **Webhook awareness**: Mutations from webhooks are visible

## Common Pitfalls to Avoid

### ❌ Don't manually construct VirtPlatformConfigItem

```go
// WRONG - Bypasses SSA validation
return []advisorv1alpha1.VirtPlatformConfigItem{{
    Name: "my-item",
    Diff: "manual diff string",
    // ...
}}, nil
```

### ❌ Don't call plan.GenerateSSADiff directly

```go
// WRONG - Use builder instead
diff, err := plan.GenerateSSADiff(ctx, c, desired, "field-manager")
item := advisorv1alpha1.VirtPlatformConfigItem{
    Diff: diff,
    // ...
}
```

### ✅ Always use PlanItemBuilder

```go
// CORRECT - Enforces SSA usage
item, err := profileutils.NewPlanItemBuilder(ctx, c, "virt-advisor-operator").
    ForResource(desired, "my-item").
    Build()
```

## Summary

### Directory Structure Best Practices

1. **One profile per subdirectory** under `internal/profiles/` - Enables CODEOWNERS protection
2. **Package name matches directory name** - Clear organization (e.g., `package loadaware`)
3. **Integration tests in profile subdirectory** - Keep tests with implementation
4. **Use profileutils/ for shared code** - Avoid circular dependencies
5. **Register in parent's init()** - Central registration in `internal/profiles/profiles.go`

### PlanItemBuilder Requirements

1. **ALWAYS use `profileutils.NewPlanItemBuilder()`** to create VirtPlatformConfigItems
2. **NEVER manually construct diffs** with string concatenation
3. **Let the builder handle SSA dry-run** for accurate, validated diffs
4. **Specify managed fields** for proper drift detection
5. **Test with envtest** to verify SSA behavior

### Profile Implementation Checklist

- [ ] Create subdirectory under `internal/profiles/`
- [ ] Implement Profile interface in `profile.go`
- [ ] Use `profileutils.NewPlanItemBuilder()` for plan items
- [ ] Add shared GVKs/constants to `profileutils/constants.go`
- [ ] Create integration tests in same subdirectory with `"../../../"` paths
- [ ] Register profile in parent's `init()` function
- [ ] Add profile name to CRD enum validation
- [ ] Add CODEOWNERS entry for your profile subdirectory

Following these guidelines ensures your profile:
- Generates accurate, API-server-validated diffs leveraging CEL rules, defaulting, and webhooks
- Integrates cleanly with the profiles package architecture
- Can be protected with team-specific CODEOWNERS files
- Maintains proper separation of concerns without circular dependencies
