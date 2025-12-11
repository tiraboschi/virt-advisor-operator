# Profile Development Guide

## Overview

This guide explains how to create new configuration profiles for the virt-advisor-operator. Profiles define reusable configuration capabilities that can be applied to the cluster with proper governance controls.

## CRITICAL REQUIREMENT: Always Use PlanItemBuilder

**You MUST use `PlanItemBuilder` when creating `ConfigurationPlanItem` objects.** This ensures:

1. ✅ **API Server Validation**: Diffs reflect CEL validation rules
2. ✅ **Defaulting**: Shows actual defaults the API server will apply
3. ✅ **Webhook Processing**: Captures mutations from admission webhooks
4. ✅ **Accurate Diffs**: What you see is exactly what will be applied

### ❌ WRONG - Manual Diff Construction

```go
// DO NOT DO THIS - Bypasses API server validation!
func (p *MyProfile) GeneratePlanItems(ctx context.Context, c client.Client, overrides map[string]string) ([]hcov1alpha1.ConfigurationPlanItem, error) {
    diff := fmt.Sprintf(`--- my-resource
+++ my-resource
@@ -1,1 +1,1 @@
 spec:
-  oldValue: 1
+  newValue: 2
`)

    return []hcov1alpha1.ConfigurationPlanItem{{
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
func (p *MyProfile) GeneratePlanItems(ctx context.Context, c client.Client, overrides map[string]string) ([]hcov1alpha1.ConfigurationPlanItem, error) {
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

    return []hcov1alpha1.ConfigurationPlanItem{item}, nil
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
5. **Returns** complete `ConfigurationPlanItem`

## Complete Example: LoadAwareRebalancing Profile

```go
package profiles

import (
    "context"
    "fmt"

    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "sigs.k8s.io/controller-runtime/pkg/client"

    hcov1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
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

func (p *LoadAwareRebalancingProfile) GeneratePlanItems(ctx context.Context, c client.Client, configOverrides map[string]string) ([]hcov1alpha1.ConfigurationPlanItem, error) {
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

    return []hcov1alpha1.ConfigurationPlanItem{item}, nil
}

func init() {
    if err := DefaultRegistry.Register(NewLoadAwareRebalancingProfile()); err != nil {
        panic(fmt.Sprintf("failed to register profile: %v", err))
    }
}
```

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

### ❌ Don't manually construct ConfigurationPlanItem

```go
// WRONG - Bypasses SSA validation
return []hcov1alpha1.ConfigurationPlanItem{{
    Name: "my-item",
    Diff: "manual diff string",
    // ...
}}, nil
```

### ❌ Don't call plan.GenerateSSADiff directly

```go
// WRONG - Use builder instead
diff, err := plan.GenerateSSADiff(ctx, c, desired, "field-manager")
item := hcov1alpha1.ConfigurationPlanItem{
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

1. **ALWAYS use `PlanItemBuilder`** to create ConfigurationPlanItems
2. **NEVER manually construct diffs** with string concatenation
3. **Let the builder handle SSA dry-run** for accurate, validated diffs
4. **Specify managed fields** for proper drift detection
5. **Test with envtest** to verify SSA behavior

Following these guidelines ensures your profile generates accurate, API-server-validated diffs that leverage CEL rules, defaulting, and webhooks as required by the VEP.
