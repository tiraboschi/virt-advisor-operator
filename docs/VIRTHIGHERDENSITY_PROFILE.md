# VirtHigherDensity Profile

This document describes the `virt-higher-density` profile implementation, which enables higher VM density through kubelet swap configuration and memory overcommitment.

## Overview

The `virt-higher-density` profile configures OpenShift worker nodes to enable swap for kubelet, allowing for higher VM density through memory overcommitment. This is accomplished by:

1. **Deploying MachineConfig** with swap provisioning and kubelet configuration
2. **Setting up systemd units** for automatic swap activation
3. **Restricting swap usage** for system slice to prevent system processes from using swap

## Architecture

### Swap Configuration

The profile deploys a comprehensive swap configuration through MachineConfig:

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

Located in `internal/profiles/virthigherdensity_profile.go`, the profile:

1. **Validates Configuration**: Checks prerequisites (MachineConfig CRD)
2. **Generates Plan Items**: Creates MachineConfig for swap enablement
3. **Generates Diffs**: Shows unified diffs of proposed changes
4. **Tracks Rollout**: Monitors MachineConfigPool for node updates and reboots

## Workflow

### 1. Prepare Nodes (tbd)

Before enabling the profile, ensure your worker nodes have a swap partition (tbd).

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
EOF

# View the generated plan with diffs
kubectl get virtplatformconfig virt-higher-density -o yaml
```

**What happens:**
- Phase: `Pending` → `Drafting` → `ReviewRequired`
- Profile generates MachineConfig plan item
- Diff shows the complete ignition configuration
- No actual changes applied

**Example Output:**
```yaml
status:
  phase: ReviewRequired
  items:
  - name: enable-kubelet-swap
    targetRef:
      apiVersion: machineconfiguration.openshift.io/v1
      kind: MachineConfig
      name: 90-worker-kubelet-swap
    impactSeverity: High - Node reboot required for swap configuration
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

Review the diff carefully. The MachineConfig includes:
- Swap partition detection script at `/etc/find-swap-partition`
- Kubelet configuration at `/etc/openshift/kubelet.conf.d/90-swap.conf`
- Systemd units for swap provisioning and cgroup configuration

### 4. Approve & Apply

```bash
# Change action from DryRun to Apply
kubectl patch virtplatformconfig virt-higher-density \
  --type='json' -p='[{"op": "replace", "path": "/spec/action", "value":"Apply"}]'
```

**What happens:**
- Phase: `ReviewRequired` → `InProgress` → `Completed`
- MachineConfig created/updated
- MachineConfigPool detects new configuration
- Nodes are drained, updated, and rebooted **one at a time**
- Status tracks rollout progress

**Rollout Progress Tracking:**

The operator monitors the MachineConfigPool and provides real-time progress updates:

```bash
# Watch the VirtPlatformConfig status
kubectl get virtplatformconfig virt-higher-density -w

# Example progression:
# - "Applying configuration..." (MachineConfig created)
# - "Waiting for MachineConfigPool 'worker' to stabilize (Updated: 1/10, Ready: 9/10)"
# - "Waiting for MachineConfigPool 'worker' to stabilize (Updated: 2/10, Ready: 8/10)"
# - ... continues for each node ...
# - "MachineConfigPool 'worker' is ready (all 10 nodes updated)" (Completed)
```

**Important:** Rollout can take **hours or even days** on large clusters with many VMs that need to be migrated before node drain. The operator will:
- Stay in `phase: InProgress` throughout the rollout
- Update the item message with current progress
- Only transition to `phase: Completed` when all nodes are updated and ready

### 5. Verify

```bash
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

## Configuration Options

The profile supports the following option:

```yaml
spec:
  profile: virt-higher-density
  options:
    virtHigherDensity:
      # Enable swap on worker nodes (default: true)
      enableSwap: true
```

**To disable swap (remove MachineConfig):**
```yaml
spec:
  profile: virt-higher-density
  action: Apply
  options:
    virtHigherDensity:
      enableSwap: false
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

### Impact Severity: High

- **Node Reboots Required**: Each worker node will be rebooted when the MachineConfig is applied
- **VM Migrations**: All VMs on each node will be live-migrated before the node is drained
- **Rollout Duration**: Can take hours or days on large clusters

### Prerequisites

1. **Swap Partition**: Worker nodes must have a partition labeled `CNV_SWAP`
2. **MachineConfig CRD**: Available on OpenShift (not on vanilla Kubernetes)
3. **Sufficient Resources**: Cluster must have enough capacity to migrate VMs during node updates

### Best Practices

1. **Test in Dev First**: Test the profile in a development environment before production
2. **Review Diffs**: Always review the diff in DryRun mode before applying
3. **Monitor Rollout**: Watch the MachineConfigPool status during rollout
4. **Plan Maintenance Window**: Schedule the rollout during a maintenance window
5. **Check Swap Usage**: Monitor swap usage after rollout to ensure it's working as expected

## Rollout Workflow

The MachineConfig rollout follows this process:

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

To disable swap and rollback:

```yaml
# Set enableSwap to false
spec:
  profile: virt-higher-density
  action: Apply
  options:
    virtHigherDensity:
      enableSwap: false
```

Or delete the VirtPlatformConfig entirely (but MachineConfig will remain - you'll need to delete it manually).

## Testing

### Local Development (Kind)

**Note:** This profile requires OpenShift's MachineConfig CRD and cannot be fully tested on Kind. However, you can test the plan generation:

```bash
# Setup Kind with mock CRDs
make dev-setup

# Create VirtPlatformConfig
kubectl apply -f - <<EOF
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: virt-higher-density
spec:
  profile: virt-higher-density
  action: DryRun
EOF

# View generated plan (will fail prerequisite check without MachineConfig CRD)
kubectl get virtplatformconfig virt-higher-density -o yaml
```

### OpenShift Testing

For full testing, deploy to an OpenShift cluster and follow the workflow above.

## Implementation Details

### Files

- `internal/profiles/virthigherdensity_profile.go`: Profile implementation
- `internal/plan/wait.go`: MachineConfigWaitStrategy for rollout tracking
- `api/v1alpha1/virtplatformconfig_types.go`: VirtHigherDensityConfig type definition

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

## References

- [Kubernetes Swap Support](https://kubernetes.io/docs/concepts/architecture/nodes/#swap-memory)
- [OpenShift MachineConfig](https://docs.openshift.com/container-platform/latest/post_installation_configuration/machine-configuration-tasks.html)
- [Kubelet Configuration](https://kubernetes.io/docs/reference/config-api/kubelet-config.v1beta1/)
