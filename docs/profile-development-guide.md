# Profile Development Guide

## Overview

This guide explains how to create new configuration profiles for the virt-advisor-operator. Profiles define reusable configuration capabilities that can be applied to the cluster with proper governance controls.

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
    item, err := NewPlanItemBuilder(ctx, c, "virt-advisor-operator").
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
package profiles

import (
    "context"
    "fmt"

    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "sigs.k8s.io/controller-runtime/pkg/client"

    advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
    "github.com/kubevirt/virt-advisor-operator/internal/plan"
)

var KubeDeschedulerGVK = schema.GroupVersionKind{
    Group:   "operator.openshift.io",
    Version: "v1",
    Kind:    "KubeDescheduler",
}

type LoadAwareRebalancingProfile struct {
    name string
}

func NewLoadAwareRebalancingProfile() *LoadAwareRebalancingProfile {
    return &LoadAwareRebalancingProfile{
        name: "load-aware-rebalancing",
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

    // Build desired configuration
    desired := plan.CreateUnstructured(KubeDeschedulerGVK, "cluster", "openshift-kube-descheduler-operator")

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
    item, err := NewPlanItemBuilder(ctx, c, "virt-advisor-operator").
        ForResource(desired, "enable-load-aware-descheduling").
        WithManagedFields(managedFields).
        WithMessage(fmt.Sprintf("KubeDescheduler 'cluster' will be configured with profile '%s'", profile)).
        Build()

    if err != nil {
        return nil, err
    }

    return []advisorv1alpha1.VirtPlatformConfigItem{item}, nil
}

func init() {
    if err := DefaultRegistry.Register(NewLoadAwareRebalancingProfile()); err != nil {
        panic(fmt.Sprintf("failed to register profile: %v", err))
    }
}
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

### Validate(configOverrides map[string]string) error
Validates that the provided configuration overrides are valid for this profile. Returns an error if any override is unsupported or invalid.

### GeneratePlanItems(ctx, client, configOverrides) ([]VirtPlatformConfigItem, error)
Generates the ordered list of configuration items to apply. **MUST use PlanItemBuilder** to create items (see "CRITICAL REQUIREMENT" section above).

## Testing Your Profile

### Unit Tests

```go
func TestMyProfile(t *testing.T) {
    // Set up envtest with real API server
    testEnv := &envtest.Environment{
        CRDDirectoryPaths: []string{"path/to/crds"},
    }

    cfg, err := testEnv.Start()
    require.NoError(t, err)
    defer testEnv.Stop()

    c, err := client.New(cfg, client.Options{})
    require.NoError(t, err)

    // Create profile
    profile := NewMyProfile()

    // Generate plan items
    items, err := profile.GeneratePlanItems(context.Background(), c, nil)
    require.NoError(t, err)

    // Verify SSA diff was generated (not manual string)
    require.NotEmpty(t, items[0].Diff)
    require.Contains(t, items[0].Diff, "---")  // Unified diff format
    require.Contains(t, items[0].Diff, "+++")
}
```

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
item, err := NewPlanItemBuilder(ctx, c, "virt-advisor-operator").
    ForResource(desired, "my-item").
    Build()
```

## Summary

1. **ALWAYS use `PlanItemBuilder`** to create VirtPlatformConfigItems
2. **NEVER manually construct diffs** with string concatenation
3. **Let the builder handle SSA dry-run** for accurate, validated diffs
4. **Specify managed fields** for proper drift detection
5. **Test with envtest** to verify SSA behavior

Following these guidelines ensures your profile generates accurate, API-server-validated diffs that leverage CEL rules, defaulting, and webhooks as required by the VEP.
