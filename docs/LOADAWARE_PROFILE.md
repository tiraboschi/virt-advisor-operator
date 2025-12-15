# LoadAwareRebalancing Profile

This document describes the `load-aware-rebalancing` profile implementation, which demonstrates the full VEP (Virtualization Enhancement Proposal) workflow with real server-side apply and diff generation.

## Overview

The `load-aware-rebalancing` profile configures a Kubernetes cluster to enable intelligent, load-aware pod rebalancing for KubeVirt workloads. This is accomplished by:

1. **Configuring KubeDescheduler** with KubeVirt-aware profiles (KubeVirtRelieveAndMigrate/DevKubeVirtRelieveAndMigrate/LongLifecycle)
2. **Enabling PSI Metrics** via MachineConfig kernel arguments
3. **Setting profileCustomizations** for optimal load balancing (when supported)

## Profile Selection Logic

The profile automatically selects the best available descheduler profile based on the cluster's OpenShift version by examining the KubeDescheduler CRD schema:

| OCP Version | Profile Used | ProfileCustomizations |
|-------------|-------------|----------------------|
| **5.32+** | `KubeVirtRelieveAndMigrate` (GA) | ✅ Enabled |
| **5.14-5.21** | `DevKubeVirtRelieveAndMigrate` (dev preview) | ✅ Enabled |
| **5.10** | `LongLifecycle` (fallback) | ❌ Not applicable |

**ProfileCustomizations (when supported):**
- `devEnableEvictionsInBackground`: true
- `devEnableSoftTainter`: true
- `devDeviationThresholds`: AsymmetricLow (default, configurable)

**Profile Preservation:**
The profile intelligently preserves existing configuration:
- ✅ **Preserves**: `AffinityAndTaints`, `SoftTopologyAndDuplicates` if they exist
- ❌ **Removes**: Other profiles like `LifecycleAndUtilization`, `TopologyAndDuplicates`
- ➕ **Adds**: Selected KubeVirt profile (KubeVirtRelieveAndMigrate/DevKubeVirtRelieveAndMigrate/LongLifecycle)

## Architecture

### Mock Resources

Since Kind doesn't include the OpenShift descheduler operator or Machine Config Operator, we've created simplified mock CRDs that simulate these resources:

- **`KubeDescheduler`** (operator.openshift.io/v1): Simplified descheduler configuration
- **`MachineConfig`** (machineconfiguration.openshift.io/v1): Simplified machine configuration

We provide mock CRDs for multiple OCP versions (v5.10, v5.14, v5.21, v5.32) to test the profile selection fallback logic.

### Profile Implementation

Located in `internal/profiles/loadaware_profile.go`, the profile:

1. **Validates Configuration**: Checks that config overrides are supported
2. **Generates Plan Items**: Creates two configuration items:
   - Enable LoadAware in KubeDescheduler
   - Add PSI kernel argument to MachineConfig
3. **Generates Diffs**: Shows unified diffs of proposed changes
4. **Applies Changes**: Uses Kubernetes server-side apply

### Server-Side Apply

Located in `internal/plan/executor.go` and `apply.go`:

- Uses `client.Patch()` with `client.Apply` strategy
- Field manager: `virt-advisor-operator`
- Supports dry-run for preview
- Handles both creation and updates

## Workflow

### 1. Preview (DryRun)

```bash
# Apply the VirtPlatformConfig with action=DryRun
kubectl apply -f config/samples/loadaware_sample.yaml

# View the generated plan with diffs
kubectl get virtplatformconfig load-aware-rebalancing -o yaml
```

**What happens:**
- Phase: `Pending` → `Drafting` → `ReviewRequired`
- Profile generates plan items
- Diffs show proposed changes
- Snapshot hash computed for optimistic locking
- No actual changes applied

**Example Output:**
```yaml
status:
  phase: ReviewRequired
  items:
  - name: enable-load-aware-descheduling
    targetRef:
      apiVersion: operator.openshift.io/v1
      kind: KubeDescheduler
      name: cluster
      namespace: openshift-kube-descheduler-operator
    impactSeverity: Low - Updates existing descheduler configuration
    diff: |
      --- cluster (KubeDescheduler)
      +++ cluster (KubeDescheduler)
      @@ -1,1 +1,1 @@
       spec:
         deschedulingIntervalSeconds: 60
         profileCustomizations:
           devDeviationThresholds: AsymmetricLow
           devEnableEvictionsInBackground: true
           devEnableSoftTainter: true
         profiles:
        - AffinityAndTaints
        - SoftTopologyAndDuplicates
      -  - LifecycleAndUtilization
      +  - KubeVirtRelieveAndMigrate
    state: Pending
    managedFields:
    - spec.deschedulingIntervalSeconds
    - spec.profiles
    - spec.profileCustomizations
    message: KubeDescheduler 'cluster' will be configured with profile 'KubeVirtRelieveAndMigrate'
```

### 2. Review

Review the diffs in the status to understand what will change.

### 3. Approve & Apply

```bash
# Change action from DryRun to Apply
kubectl patch virtplatformconfig load-aware-rebalancing \
  --type='json' -p='[{"op": "replace", "path": "/spec/action", "value":"Apply"}]'
```

**What happens:**
- Phase: `ReviewRequired` → `InProgress` → `Completed`
- Each item executed sequentially
- Server-side apply used for each resource
- Status updated as items complete
- Failure policy respected (Abort or Continue)

### 4. Verify

```bash
# Check descheduler configuration
kubectl get kubedescheduler cluster -o yaml

# Check machine config
kubectl get machineconfig 99-worker-psi-karg -o yaml
```

## Configuration Overrides

The profile supports the following overrides:

```yaml
spec:
  profile: load-aware-rebalancing
  configOverrides:
    # Descheduling interval in seconds (default: 60 = 1 minute)
    deschedulingIntervalSeconds: "60"

    # Enable PSI metrics (default: true)
    # Set to "false" to skip MachineConfig changes
    enablePSIMetrics: "true"

    # Deviation threshold for load balancing (default: AsymmetricLow)
    # Only applies to KubeVirtRelieveAndMigrate and DevKubeVirtRelieveAndMigrate
    # Valid values: Low, Medium, High, AsymmetricLow, AsymmetricMedium, AsymmetricHigh
    devDeviationThresholds: "AsymmetricLow"
```

**Note:** The `devDeviationThresholds` setting only applies when using KubeVirtRelieveAndMigrate or DevKubeVirtRelieveAndMigrate profiles. It has no effect when the profile falls back to LongLifecycle (OCP 5.10).

## Testing

### Local Development

```bash
# Complete setup (cluster + CRDs + mocks)
make dev-setup

# Run operator
make run

# Test the profile
kubectl apply -f config/samples/loadaware_sample.yaml
kubectl get virtplatformconfig load-aware-rebalancing -w
```

### Manual Testing

```bash
# Create baseline resources
kubectl apply -f config/samples/mock_baseline_resources.yaml

# Create configuration plan
kubectl apply -f config/samples/loadaware_sample.yaml

# Watch phases
kubectl get virtplatformconfig -w

# Approve and apply
kubectl patch virtplatformconfig load-aware-rebalancing \
  --type='json' -p='[{"op": "replace", "path": "/spec/action", "value":"Apply"}]'

# Verify changes
kubectl get kubedescheduler cluster -n openshift-kube-descheduler-operator -o jsonpath='{.spec.profiles}' | grep KubeVirtRelieveAndMigrate
kubectl get kubedescheduler cluster -n openshift-kube-descheduler-operator -o jsonpath='{.spec.profileCustomizations}'
kubectl get machineconfig 99-worker-psi-karg -o jsonpath='{.spec.kernelArguments}' | grep psi
```

## Implementation Details

### Files

- `internal/profiles/loadaware_profile.go`: Profile implementation
- `internal/plan/executor.go`: Server-side apply execution
- `internal/plan/apply.go`: Apply utilities
- `internal/thirdparty/`: Mock CRD type definitions
- `config/crd/mocks/`: Mock CRD definitions
- `config/samples/mock_baseline_resources.yaml`: Baseline test data
- `config/samples/loadaware_sample.yaml`: Example VirtPlatformConfig

### State Machine

```
Pending → Drafting → ReviewRequired → InProgress → Completed
                   ↘                 ↗
                     (user changes action)
```

### Error Handling

- **Validation errors**: Move to `PrerequisiteFailed`
- **Generation errors**: Move to `Failed`
- **Apply errors**:
  - `FailurePolicy=Abort`: Move to `Failed`
  - `FailurePolicy=Continue`: Mark item as failed, continue
- **Partial completion**: End in `CompletedWithErrors`

## Future Enhancements

1. **Real Diff Generation**: Use strategic merge patch or JSON patch for accurate diffs
2. **Drift Detection**: Implement periodic drift checking in `Completed` phase
3. **Rollback**: Add rollback capability for failed applies
4. **Webhooks**: Add validation webhooks for VirtPlatformConfig
5. **More Profiles**: Implement additional profiles from the VEP

## References

- [VEP Document](https://github.com/tiraboschi/kubevirt_enhancements/blob/eacb5aa36721a2d6cb72d9be3162de85f288a1ef/veps/sig-compute/XX-3rd-party-integration/vep.md)
- [Kubernetes Server-Side Apply](https://kubernetes.io/docs/reference/using-api/server-side-apply/)
- [OpenShift Descheduler Operator](https://docs.openshift.com/container-platform/latest/nodes/scheduling/nodes-descheduler.html)
