# VirtHigherDensity Profile

This document describes the `virt-higher-density` profile implementation, which enables higher VM density through swap configuration, kernel samepage merging (KSM), and memory overcommitment.

## Overview

The `virt-higher-density` profile optimizes OpenShift Virtualization clusters for higher VM density by configuring three complementary mechanisms:

1. **HyperConverged (HCO) Configuration**: Enables KSM and memory overcommitment via the HyperConverged CR
2. **Swap Configuration**: Deploys MachineConfig with kubelet swap support for safe memory overcommitment
3. **Drift Detection**: Automatically detects and converges external HCO configuration changes

This profile is designed for workloads where maximizing VM density is more important than individual VM performance.

## Architecture

### HyperConverged (HCO) Integration

The profile manages the HyperConverged CR to configure virtualization-specific density features:

**Kernel Samepage Merging (KSM):**
- Enables memory deduplication across VMs
- Configurable node selector to target specific nodes
- Default: enabled on all nodes
- Path: `spec.configuration.ksmConfiguration.nodeLabelSelector`

**Memory Overcommitment:**
- Allows VMs to see more memory than their pod's memory request
- Ratio of 150 means VMs see 50% more memory than requested
- Default: 150% (50% overcommit)
- Valid range: 100-300%
- Path: `spec.higherWorkloadDensity.memoryOvercommitPercentage`

**Drift Detection:**
- Monitors HCO CR for external changes
- Automatically regenerates plan when HCO is modified
- Requires user review before re-applying drift-triggered changes
- User overrides take precedence over HCO-derived defaults

### Swap Configuration

The profile deploys comprehensive swap configuration through MachineConfig:

**Swap Partition Detection:**
- Looks for a partition labeled `CNV_SWAP` (using `/dev/disk/by-partlabel/CNV_SWAP`)
- Validates partition existence before enabling swap
- Creates a sentinel file (`/var/tmp/swapfile`) to track activation

**Kubelet Configuration:**
- Sets `swapBehavior: LimitedSwap` in kubelet config
- Allows pods to use swap based on their QoS class
- System slice is restricted from using swap

**Systemd Units:**
1. `swap-provision.service`: Finds and enables swap partition
2. `cgroup-system-slice-config.service`: Restricts swap for system.slice

### Profile Implementation

Located in `internal/profiles/higherdensity/`, the profile:

1. **Validates Configuration**: Checks prerequisites (MachineConfig and HyperConverged CRDs)
2. **Generates Plan Items**: Creates HCO and MachineConfig items
3. **Generates Diffs**: Shows unified diffs of proposed changes using Server-Side Apply
4. **Tracks Rollout**: Monitors both HCO updates and MachineConfigPool for node updates
5. **Detects Drift**: Monitors HCO CR for external modifications

## Configuration Options

The profile supports comprehensive configuration through the `virtHigherDensity` options:

```yaml
spec:
  profile: virt-higher-density
  action: DryRun

  options:
    virtHigherDensity:
      # Enable swap on worker nodes (default: true)
      # Deploys MachineConfig with swap provisioning and kubelet configuration
      # Requires CNV_SWAP partition on worker nodes
      enableSwap: true

      # KSM (Kernel Samepage Merging) configuration
      # When omitted (default), KSM is enabled on all nodes
      # To opt-out of KSM, set enabled: false
      ksmConfiguration:
        # enabled: true  # Optional, defaults to true when ksmConfiguration is present
        # nodeLabelSelector specifies which nodes should have KSM enabled
        # Empty selector {} means all nodes (default)
        nodeLabelSelector:
          matchLabels:
            ksm-enabled: "true"
          # Or use matchExpressions for more complex selectors:
          # matchExpressions:
          #   - key: node-role.kubernetes.io/worker
          #     operator: Exists

      # Memory overcommit ratio (default: 150)
      # This value is written to HyperConverged.spec.higherWorkloadDensity.memoryOvercommitPercentage
      # A value of 150 means VMs will see 50% more memory than their pod's memory request
      # Values > 120 require enableSwap=true (enforced by CEL validation)
      # Valid range: 100-300
      memoryToRequestRatio: 150
```

### Example Configurations

**1. Maximum Density with All Features:**
```yaml
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: virt-higher-density
spec:
  profile: virt-higher-density
  action: Apply

  options:
    virtHigherDensity:
      enableSwap: true
      ksmConfiguration:
        nodeLabelSelector: {}  # Empty = all nodes
      memoryToRequestRatio: 200  # High overcommit (requires swap)
```

**2. Higher Density Without KSM:**
```yaml
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: virt-higher-density
spec:
  profile: virt-higher-density
  action: Apply

  options:
    virtHigherDensity:
      enableSwap: true
      ksmConfiguration:
        enabled: false  # Explicitly disable KSM
      memoryToRequestRatio: 120  # Safe maximum without swap
```

**3. KSM on Specific Nodes:**
```yaml
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: virt-higher-density
spec:
  profile: virt-higher-density
  action: Apply

  options:
    virtHigherDensity:
      enableSwap: true
      ksmConfiguration:
        nodeLabelSelector:
          matchLabels:
            ksm-enabled: "true"  # Only nodes with this label
      memoryToRequestRatio: 150
```

**4. Minimal Configuration (All Defaults):**
```yaml
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: virt-higher-density
spec:
  profile: virt-higher-density
  action: DryRun
  # No options specified - uses defaults:
  # - enableSwap: true
  # - ksmConfiguration: enabled on all nodes
  # - memoryToRequestRatio: 150
```

## CEL Validation Rules

The API enforces the following validation rules at admission time using Kubernetes Common Expression Language (CEL):

### Rule: High Memory Overcommit Requires Swap

**Constraint:** `memoryToRequestRatio > 120` requires `enableSwap=true`

**Rationale:** Memory overcommit ratios above 120% can cause significant memory pressure. Swap is required to handle memory pressure safely without OOMKills.

**Examples:**

✅ **Valid:**
```yaml
options:
  virtHigherDensity:
    enableSwap: true
    memoryToRequestRatio: 200  # OK: swap is enabled
```

✅ **Valid:**
```yaml
options:
  virtHigherDensity:
    enableSwap: false
    memoryToRequestRatio: 120  # OK: 120% without swap is safe
```

❌ **Invalid:**
```yaml
options:
  virtHigherDensity:
    enableSwap: false
    memoryToRequestRatio: 150  # ERROR: >120% requires swap
```

**Error Message:**
```
admission webhook denied the request:
VirtPlatformConfig.advisor.kubevirt.io "virt-higher-density" is invalid:
spec.options.virtHigherDensity: Invalid value: "object": memoryToRequestRatio > 120 requires enableSwap=true
```

## Drift Detection and Auto-Convergence

The VirtHigherDensity profile includes comprehensive drift detection for HCO configuration changes, ensuring the cluster configuration stays synchronized with the declared desired state.

### How Drift Detection Works

**1. Initial Configuration:**
- Profile writes KSM and memory overcommit config to HCO CR
- Stores applied configuration in `status.items[].desiredState`
- Marks configuration as `Completed`

**2. Drift Detection:**
- Controller watches HCO CR for changes using dynamic event handling
- Compares current HCO values vs. applied values in status
- Detects changes to:
  - `spec.configuration.ksmConfiguration` (KSM enabled/disabled or node selector changes)
  - `spec.higherWorkloadDensity.memoryOvercommitPercentage` (memory ratio changes)

**3. Drift Handling:**
- Sets `InputDependencyDrift` condition to `True` with reason message
- Transitions phase to `Drafting` → `ReviewRequired`
- Regenerates plan with new HCO values
- **Does NOT auto-apply** (requires user review)

**4. User Review Required:**
- Even if `action: Apply`, drift-triggered changes require explicit user acknowledgment
- User must review new plan and toggle spec to acknowledge
- Prevents unintended changes from external HCO modifications

**5. Auto-Resolution:**
- If HCO changes back to original values, drift auto-resolves
- `InputDependencyDrift` condition cleared
- Plan regenerated with original values
- Can proceed with normal flow

### User Override Priority

**Key Principle:** User-provided explicit configuration overrides always take precedence over HCO-derived defaults.

**Behavior:**
- If user specifies `memoryToRequestRatio` in spec.options, drift detection is skipped for memory overcommit
- If user specifies `ksmConfiguration` in spec.options, drift detection is skipped for KSM settings
- This allows users to "pin" their configuration and ignore external HCO changes

**Example:**
```yaml
spec:
  options:
    virtHigherDensity:
      memoryToRequestRatio: 180  # Explicit override - drift detection skipped
```

Even if HCO's `memoryOvercommitPercentage` is changed externally, this config will stay at 180 without triggering drift.

### Drift Detection Workflow

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Initial State: Completed                                │
│    HCO memoryOvercommitPercentage: 150                     │
│    Status applied config: 150                              │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. External Change                                          │
│    User modifies HCO CR: memoryOvercommitPercentage: 200   │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Drift Detected                                           │
│    Controller reconcile detects mismatch (200 vs 150)      │
│    Sets InputDependencyDrift condition                     │
│    Phase: Completed → Drafting                             │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Plan Regeneration                                        │
│    Generate new plan with current HCO values (200)         │
│    Phase: Drafting → ReviewRequired                        │
│    BLOCKS auto-apply (even if action=Apply)                │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 5. User Review Required                                     │
│    User reviews new plan showing 150 → 200 change          │
│    User acknowledges by toggling action or updating spec   │
│    Phase: ReviewRequired → InProgress → Completed          │
└─────────────────────────────────────────────────────────────┘
```

### Example: Drift Detection in Action

**Initial Setup:**
```bash
# Apply configuration with 150% memory overcommit
kubectl apply -f - <<EOF
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: virt-higher-density
spec:
  profile: virt-higher-density
  action: Apply
  options:
    virtHigherDensity:
      memoryToRequestRatio: 150
EOF

# Configuration applied successfully
# Status: phase: Completed
```

**External HCO Change:**
```bash
# Someone modifies HCO directly
kubectl patch hyperconverged kubevirt-hyperconverged -n openshift-cnv \
  --type='json' -p='[{"op": "replace", "path": "/spec/higherWorkloadDensity/memoryOvercommitPercentage", "value":200}]'
```

**Drift Detected:**
```bash
# Controller detects drift on next reconcile
kubectl get virtplatformconfig virt-higher-density -o yaml
```

```yaml
status:
  phase: ReviewRequired  # Was Completed, now requires review
  conditions:
  - type: InputDependencyDrift
    status: "True"
    reason: InputChanged
    message: "Input dependency changed: HCO memoryOvercommitPercentage changed (150 -> 200)"
  items:
  - name: configure-higher-density
    diff: |
      --- kubevirt-hyperconverged (HyperConverged)
      +++ kubevirt-hyperconverged (HyperConverged)
      @@ spec.higherWorkloadDensity @@
      -  memoryOvercommitPercentage: 150
      +  memoryOvercommitPercentage: 200
    state: Pending
    message: "Plan regenerated due to HCO configuration drift"
```

**User Acknowledgment:**
```bash
# User reviews change and acknowledges by toggling action
kubectl patch virtplatformconfig virt-higher-density \
  --type='json' -p='[{"op": "replace", "path": "/spec/action", "value":"DryRun"}]'

kubectl patch virtplatformconfig virt-higher-density \
  --type='json' -p='[{"op": "replace", "path": "/spec/action", "value":"Apply"}]'

# Now proceeds to InProgress → Completed
# InputDependencyDrift condition cleared
```

## Workflow

### 1. Prepare Nodes (for Swap)

Before enabling swap, ensure your worker nodes have a swap partition labeled `CNV_SWAP`:

```bash
# On each worker node:
# 1. Create partition (example using parted)
parted /dev/sda mkpart primary linux-swap 100GB 116GB
parted /dev/sda name 3 CNV_SWAP

# 2. Format as swap
mkswap /dev/sda3

# 3. Verify label
ls -l /dev/disk/by-partlabel/CNV_SWAP
```

**Note:** Swap partition setup is manual and must be done before applying the profile.

### 2. Preview (DryRun)

```bash
# Apply the VirtPlatformConfig with action=DryRun
kubectl apply -f - <<EOF
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: virt-higher-density
spec:
  profile: virt-higher-density
  action: DryRun
  options:
    virtHigherDensity:
      enableSwap: true
      ksmConfiguration:
        nodeLabelSelector: {}
      memoryToRequestRatio: 150
EOF

# View the generated plan with diffs
kubectl get virtplatformconfig virt-higher-density -o yaml
```

**What happens:**
- Phase: `Pending` → `Drafting` → `ReviewRequired`
- Profile generates two plan items:
  1. HyperConverged item: KSM and memory overcommit configuration
  2. MachineConfig item: Swap enablement
- Diffs show the complete proposed changes
- No actual changes applied

**Example Output:**
```yaml
status:
  phase: ReviewRequired
  items:
  - name: configure-higher-density
    targetRef:
      apiVersion: hco.kubevirt.io/v1beta1
      kind: HyperConverged
      name: kubevirt-hyperconverged
      namespace: openshift-cnv
    impactSeverity: Medium
    diff: |
      --- kubevirt-hyperconverged (HyperConverged)
      +++ kubevirt-hyperconverged (HyperConverged)
      @@ spec.configuration @@
      +  ksmConfiguration:
      +    nodeLabelSelector: {}
      @@ spec.higherWorkloadDensity @@
      +  memoryOvercommitPercentage: 150
    state: Pending
    managedFields:
    - spec.configuration.ksmConfiguration
    - spec.higherWorkloadDensity.memoryOvercommitPercentage
    message: Configure HCO with KSM enabled (all nodes), memoryOvercommitPercentage=150

  - name: enable-kubelet-swap
    targetRef:
      apiVersion: machineconfiguration.openshift.io/v1
      kind: MachineConfig
      name: 90-worker-kubelet-swap
    impactSeverity: High
    diff: |
      --- 90-worker-kubelet-swap (MachineConfig)
      +++ 90-worker-kubelet-swap (MachineConfig)
      (shows complete MachineConfig with ignition config)
    state: Pending
    managedFields:
    - spec.config
    message: MachineConfig '90-worker-kubelet-swap' will be configured to enable kubelet swap
```

### 3. Review

Review the diffs carefully:

**HyperConverged Item:**
- Verify KSM configuration (enabled/disabled, node selector)
- Verify memory overcommit percentage
- Impact: Medium (no node restarts, VMs may see more memory)

**MachineConfig Item:**
- Verify swap partition detection script
- Verify kubelet configuration
- Verify systemd units
- Impact: High (node reboot required)

### 4. Approve & Apply

```bash
# Change action from DryRun to Apply
kubectl patch virtplatformconfig virt-higher-density \
  --type='json' -p='[{"op": "replace", "path": "/spec/action", "value":"Apply"}]'
```

**What happens:**
- Phase: `ReviewRequired` → `InProgress` → `Completed`
- HyperConverged CR updated (immediate, no reboot)
- MachineConfig created/updated
- MachineConfigPool detects new configuration
- Nodes are drained, updated, and rebooted **one at a time**
- Status tracks rollout progress

**Rollout Progress Tracking:**

```bash
# Watch the VirtPlatformConfig status
kubectl get virtplatformconfig virt-higher-density -w

# Example progression:
# - "Applying HCO configuration..." (HyperConverged updated)
# - "HCO configuration applied successfully"
# - "Applying MachineConfig..." (MachineConfig created)
# - "Waiting for MachineConfigPool 'worker' to stabilize (Updated: 1/10, Ready: 9/10)"
# - "Waiting for MachineConfigPool 'worker' to stabilize (Updated: 2/10, Ready: 8/10)"
# - ... continues for each node ...
# - "MachineConfigPool 'worker' is ready (all 10 nodes updated)" (Completed)
```

**Important:** Rollout can take **hours or even days** on large clusters with many VMs. The operator will:
- Stay in `phase: InProgress` throughout the rollout
- Update item messages with current progress
- Only transition to `phase: Completed` when all changes are applied

### 5. Verify

```bash
# Check HyperConverged configuration
kubectl get hyperconverged kubevirt-hyperconverged -n openshift-cnv -o yaml

# Verify KSM configuration
# Should see:
#   spec:
#     configuration:
#       ksmConfiguration:
#         nodeLabelSelector: {}

# Verify memory overcommit
# Should see:
#   spec:
#     higherWorkloadDensity:
#       memoryOvercommitPercentage: 150

# Check MachineConfig
kubectl get machineconfig 90-worker-kubelet-swap -o yaml

# Check MachineConfigPool status
kubectl get machineconfigpool worker -o yaml

# On a worker node (after update), verify swap is enabled
oc debug node/<node-name>
chroot /host
swapon --show
cat /etc/openshift/kubelet.conf.d/90-swap.conf
```

Expected output:
```
NAME                               TYPE SIZE USED PRIO
/dev/disk/by-partlabel/CNV_SWAP   partition  16G   0B   -2
```

## Wait Timeout

For large clusters, you can optionally set a `waitTimeout` to fail the rollout if nodes don't become ready within a specific timeframe:

```yaml
spec:
  profile: virt-higher-density
  action: Apply
  waitTimeout: 24h  # Fail if rollout takes longer than 24 hours
```

**Recommendation:** For production clusters, **do not set waitTimeout** (or set it very high) to allow the operator to wait indefinitely for the rollout to complete. Node updates can take unpredictable amounts of time depending on:
- Number of VMs that need to be live-migrated
- Cluster load and resource availability
- Network conditions during VM migration

## Impact & Considerations

### Impact Severity

**HyperConverged Changes (Medium):**
- KSM enablement: Immediate effect, no restarts required
- Memory overcommit: VMs may see more memory on next start/migration
- Existing running VMs: Unaffected until restart or migration

**MachineConfig Changes (High):**
- **Node Reboots Required**: Each worker node will be rebooted when the MachineConfig is applied
- **VM Migrations**: All VMs on each node will be live-migrated before the node is drained
- **Rollout Duration**: Can take hours or days on large clusters

### Prerequisites

1. **Swap Partition** (if enableSwap=true): Worker nodes must have a partition labeled `CNV_SWAP`
2. **MachineConfig CRD**: Available on OpenShift (not on vanilla Kubernetes)
3. **HyperConverged CRD**: OpenShift Virtualization must be installed
4. **Sufficient Resources**: Cluster must have enough capacity to migrate VMs during node updates

### Best Practices

1. **Start with DryRun**: Always review the plan before applying
2. **Test in Dev First**: Test the profile in a development environment before production
3. **Monitor Drift**: Watch for InputDependencyDrift conditions on active configurations
4. **Plan Maintenance Window**: Schedule MachineConfig rollouts during a maintenance window
5. **Check Resource Usage**: Monitor KSM effectiveness and swap usage after rollout
6. **Use User Overrides**: Pin critical settings with explicit overrides to prevent drift
7. **Review External HCO Changes**: If drift is detected, review why HCO was changed externally

## Rollout Workflow

The complete rollout follows this process:

**Phase 1: HyperConverged Update (Immediate)**
1. Operator applies KSM configuration to HCO CR
2. Operator applies memory overcommit percentage to HCO CR
3. HCO operator reconciles and updates KubeVirt config
4. New VMs start with new memory overcommit settings

**Phase 2: MachineConfig Rollout (Gradual)**
1. **MachineConfig Created**: Operator applies the MachineConfig
2. **Pool Detects Change**: MachineConfigPool detects new configuration
3. **Node Selection**: MCO selects first node for update
4. **VM Migration**: All VMs on the node are live-migrated to other nodes
5. **Node Drain**: Node is cordoned and drained
6. **Configuration Update**: MCO applies new configuration
7. **Node Reboot**: Node is rebooted with new config
8. **Node Ready**: Node becomes ready and rejoins cluster
9. **Repeat**: Process repeats for each remaining node (one at a time by default)

## Troubleshooting

### Drift Detection Issues

**Drift continuously detected:**
```bash
# Check if someone/something is modifying HCO externally
kubectl get hyperconverged kubevirt-hyperconverged -n openshift-cnv -o yaml

# Check managed fields to see who last modified
kubectl get hyperconverged kubevirt-hyperconverged -n openshift-cnv -o jsonpath='{.metadata.managedFields}' | jq

# To stop drift detection, add explicit override
kubectl patch virtplatformconfig virt-higher-density --type='merge' -p '
spec:
  options:
    virtHigherDensity:
      memoryToRequestRatio: 150  # Pin to specific value
'
```

**InputDependencyDrift condition stuck:**
```bash
# Check current HCO values vs applied values
kubectl get virtplatformconfig virt-higher-density -o jsonpath='{.status.items[?(@.targetRef.kind=="HyperConverged")].desiredState}' | jq

# Compare with actual HCO
kubectl get hyperconverged kubevirt-hyperconverged -n openshift-cnv -o jsonpath='{.spec}' | jq

# If values match, drift should auto-clear on next reconcile
# Force reconcile by adding annotation
kubectl annotate virtplatformconfig virt-higher-density reconcile="$(date +%s)"
```

### CEL Validation Errors

**Error: memoryToRequestRatio > 120 requires enableSwap=true**
```bash
# Fix by either:
# 1. Enable swap
kubectl patch virtplatformconfig virt-higher-density --type='merge' -p '
spec:
  options:
    virtHigherDensity:
      enableSwap: true
'

# OR 2. Lower memory ratio to 120 or below
kubectl patch virtplatformconfig virt-higher-density --type='merge' -p '
spec:
  options:
    virtHigherDensity:
      memoryToRequestRatio: 120
'
```

### KSM Not Working

```bash
# Check if KSM is enabled in HCO
kubectl get hyperconverged kubevirt-hyperconverged -n openshift-cnv \
  -o jsonpath='{.spec.configuration.ksmConfiguration}' | jq

# Check KSM status on nodes
oc debug node/<node-name>
chroot /host
cat /sys/kernel/mm/ksm/run  # Should be 1 (enabled)
cat /sys/kernel/mm/ksm/pages_sharing  # Shows KSM effectiveness

# Check node labels if using selector
kubectl get nodes --show-labels | grep ksm-enabled
```

### Rollout Stuck

If the rollout appears stuck:

```bash
# Check MachineConfigPool status
kubectl get machineconfigpool worker -o yaml

# Look for:
# - degradedMachineCount: Should be 0
# - conditions: Check for errors or degradation
# - machineCount vs updatedMachineCount: Shows progress

# Check for degraded nodes
kubectl get nodes -l node-role.kubernetes.io/worker -o wide

# Check node that's currently updating
kubectl get nodes -l machineconfiguration.openshift.io/currentConfig!=machineconfiguration.openshift.io/desiredConfig
```

### Swap Not Enabled

If swap is not enabled after update:

```bash
# On the node
oc debug node/<node-name>
chroot /host

# Check if partition exists
ls -l /dev/disk/by-partlabel/CNV_SWAP

# Check systemd unit status
systemctl status swap-provision.service

# Check logs
journalctl -u swap-provision.service
```

### Rollback

**To rollback HCO changes:**
```yaml
# Modify configuration or delete VirtPlatformConfig
kubectl delete virtplatformconfig virt-higher-density

# Manually revert HCO (operator won't auto-revert)
kubectl patch hyperconverged kubevirt-hyperconverged -n openshift-cnv --type='json' -p='
[
  {"op": "remove", "path": "/spec/configuration/ksmConfiguration"},
  {"op": "replace", "path": "/spec/higherWorkloadDensity/memoryOvercommitPercentage", "value": 100}
]
'
```

**To disable swap:**
```yaml
# Set enableSwap to false
spec:
  profile: virt-higher-density
  action: Apply
  options:
    virtHigherDensity:
      enableSwap: false
      # Keep HCO configuration
      memoryToRequestRatio: 120  # Must be ≤120 without swap
```

## Testing

### Unit Tests

```bash
# Run profile unit tests
KUBEBUILDER_ASSETS="$(pwd)/bin/k8s/1.34.1-linux-amd64" \
  go test ./internal/profiles/higherdensity/... -v

# Run HCO utility tests
KUBEBUILDER_ASSETS="$(pwd)/bin/k8s/1.34.1-linux-amd64" \
  go test ./internal/profiles/profileutils/... -v -run TestHCO
```

### Integration Tests

```bash
# Run integration tests (includes HCO configuration tests)
KUBEBUILDER_ASSETS="$(pwd)/bin/k8s/1.34.1-linux-amd64" \
  go test ./internal/profiles/higherdensity/... -v -run Integration
```

### Controller Tests

```bash
# Run drift detection tests
KUBEBUILDER_ASSETS="$(pwd)/bin/k8s/1.34.1-linux-amd64" \
  go test ./internal/controller/... -v -run "HCO higher-density"
```

### OpenShift E2E Testing

For full testing, deploy to an OpenShift cluster with OpenShift Virtualization installed:

```bash
# 1. Deploy the operator
make deploy IMG=your-registry/virt-advisor-operator:latest

# 2. Create test VirtPlatformConfig
kubectl apply -f config/samples/virthigherdensity_sample.yaml

# 3. Watch reconciliation
kubectl get virtplatformconfig virt-higher-density -w

# 4. Test drift detection
kubectl patch hyperconverged kubevirt-hyperconverged -n openshift-cnv \
  --type='json' -p='[{"op": "replace", "path": "/spec/higherWorkloadDensity/memoryOvercommitPercentage", "value":200}]'

# Watch for InputDependencyDrift condition
kubectl get virtplatformconfig virt-higher-density -o yaml | grep -A5 InputDependencyDrift
```

## Implementation Details

### Files

**Core Profile:**
- `internal/profiles/higherdensity/profile.go`: Profile implementation with HCO and swap generation
- `internal/profiles/higherdensity/profile_test.go`: Unit tests
- `internal/profiles/higherdensity/profile_integration_test.go`: Integration tests with mock HCO CRD

**Shared Utilities:**
- `internal/profiles/profileutils/hco.go`: Shared HCO utilities (GVK, helpers, builders)
- `internal/profiles/profileutils/hco_test.go`: Tests for shared utilities

**Controller:**
- `internal/controller/virtplatformconfig_controller.go`: Drift detection logic, HCO watch setup
- `internal/controller/virtplatformconfig_controller_test.go`: Controller tests for drift detection

**API Types:**
- `api/v1alpha1/virtplatformconfig_types.go`: VirtHigherDensityConfig with nested KSMConfiguration

**Plan Execution:**
- `internal/plan/wait.go`: MachineConfigWaitStrategy for rollout tracking

**Samples:**
- `config/samples/virthigherdensity_sample.yaml`: Example configurations

**Mocks:**
- `config/crd/mocks/hyperconverged_crd.yaml`: Mock HCO CRD for testing
- `config/crd/mocks/machineconfig_crd.yaml`: Mock MachineConfig CRD for testing

### Swap Scripts

**Swap Detection Script** (base64 encoded in MachineConfig):
```bash
#!/usr/bin/env bash
set -euo pipefail

PARTLABEL="CNV_SWAP"
DEVICE="/dev/disk/by-partlabel/${PARTLABEL}"

if [[ ! -e "$DEVICE" ]]; then
  echo "Swap partition with PARTLABEL=${PARTLABEL} not found" >&2
  exit 1
fi

if swapon --show=NAME | grep -q "$DEVICE"; then
  echo "Swap already enabled on $DEVICE" >&2
  exit 1
fi

echo "Enabling swap on $DEVICE" >&2
swapon "$DEVICE"
touch /var/tmp/swapfile
```

**Kubelet Config** (base64 encoded in MachineConfig):
```yaml
apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
memorySwap:
  swapBehavior: LimitedSwap
```

### HCO Configuration Paths

The profile manages these HyperConverged CR spec paths:

**KSM Configuration:**
```yaml
spec:
  configuration:
    ksmConfiguration:
      nodeLabelSelector:
        matchLabels: {}
        matchExpressions: []
```

**Memory Overcommit:**
```yaml
spec:
  higherWorkloadDensity:
    memoryOvercommitPercentage: 150
```

### Managed Fields

The profile tracks these managed fields for drift detection:

- `spec.configuration.ksmConfiguration` (if KSM enabled)
- `spec.higherWorkloadDensity.memoryOvercommitPercentage` (always)

Changes to these fields by external actors trigger drift detection and plan regeneration.

## References

- [KubeVirt Memory Overcommit](https://kubevirt.io/user-guide/operations/memory_overcommit/)
- [OpenShift Virtualization Higher Workload Density](https://docs.openshift.com/container-platform/latest/virt/virtual_machines/advanced_vm_management/virt-configuring-higher-vm-workload-density.html)
- [Kubernetes Swap Support](https://kubernetes.io/docs/concepts/architecture/nodes/#swap-memory)
- [Kernel Samepage Merging (KSM)](https://www.kernel.org/doc/html/latest/admin-guide/mm/ksm.html)
- [OpenShift MachineConfig](https://docs.openshift.com/container-platform/latest/post_installation_configuration/machine-configuration-tasks.html)
- [Kubelet Configuration](https://kubernetes.io/docs/reference/config-api/kubelet-config.v1beta1/)
- [HyperConverged Operator](https://github.com/kubevirt/hyperconverged-cluster-operator)
