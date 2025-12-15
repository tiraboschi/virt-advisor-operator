# Deploying virt-advisor-operator on OpenShift

This guide explains how to deploy and use the virt-advisor-operator on a real OpenShift cluster.

## Overview

On OpenShift, the virt-advisor-operator works with real cluster operators like:
- **Descheduler Operator** - For load-aware scheduling
- **Machine Config Operator** - For node-level tuning (comes pre-installed)
- **OpenShift Virtualization** - For KubeVirt workloads

Unlike testing on Kind (which uses mock CRDs), on OpenShift you'll work with actual operators that affect real workloads.

## Prerequisites

### Required Access
- Cluster admin privileges or appropriate RBAC permissions
- Access to an image registry (quay.io, docker.io, or internal OpenShift registry)
- `oc` CLI tool installed and logged into your cluster

### Optional Prerequisites (Depends on Profile)

For the **load-aware-rebalancing** profile:
- **Descheduler Operator** installed via OperatorHub
- **OpenShift Virtualization** (CNV) installed for KubeVirt workloads

## Installation Steps

### Step 1: Build and Push the Operator Image

Build the operator image and push it to your registry:

```bash
# Set your image registry and tag
export IMG=quay.io/your-username/virt-advisor-operator:v0.1.0

# Build and push the image
make docker-build docker-push IMG=${IMG}
```

**Using the OpenShift Internal Registry:**

```bash
# Login to the OpenShift registry
oc registry login

# Get the registry URL
REGISTRY=$(oc get route default-route -n openshift-image-registry -o jsonpath='{.spec.host}')

# Build and push
export IMG=${REGISTRY}/virt-advisor-operator/virt-advisor-operator:v0.1.0
make docker-build docker-push IMG=${IMG}
```

### Step 2: Install the CRDs

Install the VirtPlatformConfig CRD into the cluster:

```bash
make install
```

This creates the `virtplatformconfigs.advisor.kubevirt.io` CRD cluster-wide.

Verify:
```bash
oc get crd virtplatformconfigs.advisor.kubevirt.io
```

### Step 3: Deploy the Operator

Deploy the operator to the cluster:

```bash
make deploy IMG=${IMG}
```

This creates:
- Namespace: `virt-advisor-operator-system`
- ServiceAccount with appropriate RBAC
- Deployment running the operator pod
- ClusterRole and ClusterRoleBinding for cross-namespace operations

**RBAC Permissions Granted:**
The operator's ClusterRole includes permissions to:
- **VirtPlatformConfigs** (advisor.kubevirt.io): Full access to manage the CRD
- **KubeDeschedulers** (operator.openshift.io): get, list, watch, patch
- **MachineConfigs** (machineconfiguration.openshift.io): get, list, watch, create, delete, patch
- **MachineConfigPools** (machineconfiguration.openshift.io): get, list, watch (read-only for monitoring)
- **CustomResourceDefinitions** (apiextensions.k8s.io): get, list, watch (for prerequisite checking)

Verify the deployment:
```bash
# Check the namespace was created
oc get namespace virt-advisor-operator-system

# Check the operator pod is running
oc get pods -n virt-advisor-operator-system

# Check operator logs
oc logs -n virt-advisor-operator-system -l control-plane=controller-manager -f
```

You should see logs like:
```
INFO    Starting manager
INFO    Starting server
INFO    Starting workers        {"controller": "virtplatformconfig"}
```

### Step 4: Update an Existing Deployment (If Applicable)

If you previously deployed the operator and are updating to a newer version with updated RBAC permissions:

```bash
# Re-apply the updated manifests
make deploy IMG=${IMG}
```

This will update:
- ClusterRole with new permissions (KubeDescheduler, MachineConfig access)
- Deployment with the new image
- Other resources as needed

**Force Operator Pod Restart:**
If the deployment doesn't trigger a pod restart, manually restart the operator:

```bash
oc rollout restart deployment/virt-advisor-operator-controller-manager \
  -n virt-advisor-operator-system
```

Verify the new pod is running:
```bash
oc get pods -n virt-advisor-operator-system -w
```

## Installing Required Operators (for load-aware-rebalancing)

### Install the Descheduler Operator

The load-aware-rebalancing profile requires the Descheduler Operator.

**Via OpenShift Console:**
1. Navigate to **Operators → OperatorHub**
2. Search for "Cluster Descheduler Operator"
3. Click **Install**
4. Keep the defaults (auto-update, openshift-kube-descheduler-operator namespace)
5. Click **Install**

**Via CLI:**

```bash
# Create the operator namespace
cat <<EOF | oc apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: openshift-kube-descheduler-operator
EOF

# Create the OperatorGroup
cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: descheduler-operator
  namespace: openshift-kube-descheduler-operator
spec:
  targetNamespaces:
  - openshift-kube-descheduler-operator
EOF

# Create the Subscription
cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: cluster-kube-descheduler-operator
  namespace: openshift-kube-descheduler-operator
spec:
  channel: stable
  name: cluster-kube-descheduler-operator
  source: redhat-operators
  sourceNamespace: openshift-marketplace
EOF
```

Wait for the operator to install:
```bash
oc get csv -n openshift-kube-descheduler-operator
```

Create the KubeDescheduler instance:
```bash
cat <<EOF | oc apply -f -
apiVersion: operator.openshift.io/v1
kind: KubeDescheduler
metadata:
  name: cluster
  namespace: openshift-kube-descheduler-operator
spec:
  mode: Automatic
  deschedulingIntervalSeconds: 3600
  profiles:
  - AffinityAndTaints
  - SoftTopologyAndDuplicates
  - LifecycleAndUtilization
EOF
```

Verify:
```bash
oc get kubedescheduler cluster -n openshift-kube-descheduler-operator
```

## Using the Operator

### Test Prerequisites Check (Recommended First Step)

Before creating a full configuration, verify prerequisites are met:

```bash
cat <<EOF | oc apply -f -
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: load-aware-rebalancing
  action: DryRun
EOF
```

Check the status:
```bash
oc get virtplatformconfig load-aware-rebalancing -o yaml
```

If prerequisites are missing, you'll see:
```yaml
status:
  phase: PrerequisiteFailed
  message: |
    Missing required dependencies:
    - KubeDescheduler (operator.openshift.io.v1): Please install the Descheduler Operator via OLM
```

### Create a VirtPlatformConfig (DryRun First)

**IMPORTANT:** Always start with `action: DryRun` to preview changes before applying them.

```bash
cat <<EOF | oc apply -f -
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: load-aware-rebalancing
  action: DryRun
  failurePolicy: Abort
  options:
    loadAware:
      deschedulingIntervalSeconds: 120  # 2 minutes
      enablePSIMetrics: true
      devDeviationThresholds: AsymmetricLow
EOF
```

### Review the Generated Plan

Watch the plan progress:
```bash
oc get virtplatformconfig load-aware-rebalancing -w
```

Wait for `phase: ReviewRequired`, then review the diff:
```bash
oc get virtplatformconfig load-aware-rebalancing -o yaml
```

Pay special attention to the `status.items` section:

**Item 1: KubeDescheduler Changes**
- Impact: Low (configuration update)
- Review the profile changes and interval settings
- No node disruption

**Item 2: MachineConfig for PSI Metrics**
- Impact: **HIGH** - Requires node reboot
- Creates kernel argument `psi=1`
- All nodes in the targeted MachineConfigPool will reboot sequentially
- **WARNING:** This will cause workload disruption during rollout

### Understanding MachineConfig Impact

**What happens when you apply a MachineConfig?**

1. The Machine Config Operator (MCO) updates the MachineConfigPool (MCP)
2. Nodes are cordoned one by one
3. Each node:
   - Drains workloads (respecting PodDisruptionBudgets)
   - Reboots with new kernel arguments
   - Comes back up
   - Waits for the node to stabilize
4. Proceeds to the next node

**Expected timeline:**
- Per node: ~5-15 minutes (depends on drain time + boot time)
- Total cluster: Nodes × per-node time (sequential rollout)
- 3-node cluster: ~15-45 minutes
- 10-node cluster: ~50-150 minutes

**During the rollout:**
- Workloads with replicas will migrate to other nodes
- Stateful workloads may experience downtime if not replicated
- Monitor with: `oc get mcp worker -w`

### Apply the Plan (After Reviewing)

Once you've reviewed the diffs and understand the impact:

```bash
# Approve and apply
oc patch virtplatformconfig load-aware-rebalancing \
  --type='json' \
  -p='[{"op": "replace", "path": "/spec/action", "value":"Apply"}]'
```

Watch the execution:
```bash
oc get virtplatformconfig load-aware-rebalancing -w
```

You'll see:
- `phase: InProgress`
- Item 1 (Descheduler) completes quickly
- Item 2 (MachineConfig) stays in `InProgress` for a long time
- Status message updates with MCP progress: "Waiting for MCP 'worker' to stabilize (Updated: 2/10 nodes)"

### Monitor MachineConfigPool Rollout

In another terminal, monitor the MCP status:

```bash
# Watch the worker MCP
oc get mcp worker -w

# Or more detailed
watch 'oc get mcp worker -o json | jq -r ".status | {updated: .updatedMachineCount, ready: .readyMachineCount, total: .machineCount, degraded: .degradedMachineCount}"'
```

Wait for:
```
UPDATED   READY   TOTAL
10        10      10
```

### Verify the Changes

Once `phase: Completed`, verify the configuration:

```bash
# Check KubeDescheduler
oc get kubedescheduler cluster -n openshift-kube-descheduler-operator -o yaml

# Verify profile is set
oc get kubedescheduler cluster -o jsonpath='{.spec.profiles}'

# Check MachineConfig
oc get machineconfig 99-worker-psi-karg -o yaml

# Verify PSI kernel arg is present
oc get machineconfig 99-worker-psi-karg -o jsonpath='{.spec.kernelArguments}'

# Verify nodes have the kernel arg (after reboot)
oc debug node/worker-0 -- chroot /host cat /proc/cmdline | grep psi
```

## Production Considerations

### 1. Test in Non-Production First

Always test on a development or staging cluster first:
- Verify the diffs match expectations
- Time the MachineConfig rollout to estimate production impact
- Test rollback procedures

### 2. Maintenance Windows

For production deployments:
- Schedule MachineConfig changes during maintenance windows
- Communicate expected downtime to users
- Have rollback procedures ready

### 3. Gradual Rollout

For large clusters, consider rolling out to worker pools gradually:

```yaml
# Option 1: Disable PSI metrics initially
spec:
  options:
    loadAware:
      enablePSIMetrics: false  # Apply descheduler only
```

Apply and verify descheduler works, then:

```yaml
# Option 2: Enable PSI in a second phase
spec:
  options:
    loadAware:
      enablePSIMetrics: true
```

### 4. Monitor for Drift

After applying a plan:
- Periodically check `oc get virtplatformconfig -o yaml`
- Watch for drift detection (future feature)
- Set up alerts on the VirtPlatformConfig status

### 5. GitOps Integration

For GitOps workflows (ArgoCD, Flux):

```yaml
# Store in Git
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: load-aware-rebalancing
  action: Apply  # Auto-apply in GitOps
  options:
    loadAware:
      deschedulingIntervalSeconds: 120
```

**Important:** With `action: Apply`, changes apply automatically. Use DryRun for manual approval workflows.

## Rollback Procedures

### Deleting a VirtPlatformConfig

Deleting the VirtPlatformConfig does **NOT** revert the changes:

```bash
oc delete virtplatformconfig load-aware-rebalancing
```

This removes the governance CR but leaves the configured resources in place.

### Manual Rollback of Descheduler

```bash
# Revert to original profiles
oc patch kubedescheduler cluster \
  --type=merge \
  -p '{"spec":{"profiles":["AffinityAndTaints","SoftTopologyAndDuplicates","LifecycleAndUtilization"]}}'
```

### Manual Rollback of MachineConfig

**WARNING:** Removing a MachineConfig triggers another reboot cycle.

```bash
# Delete the MachineConfig
oc delete machineconfig 99-worker-psi-karg

# Monitor the rollback
oc get mcp worker -w
```

Nodes will reboot again to remove the kernel argument.

### Creating a Rollback Plan

Create a new VirtPlatformConfig that reverts to defaults:

```yaml
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rollback
spec:
  profile: load-aware-rebalancing
  action: DryRun  # Review first
  options:
    loadAware:
      enablePSIMetrics: false  # Removes MachineConfig
      # Other options to revert
```

## Troubleshooting

### Operator Pod Not Starting

Check pod status:
```bash
oc get pods -n virt-advisor-operator-system
oc describe pod -n virt-advisor-operator-system -l control-plane=controller-manager
```

Common issues:
- Image pull errors: Verify `IMG` is accessible
- RBAC errors: Verify ClusterRole and ClusterRoleBinding exist
- Resource limits: Check if namespace has quotas

### RBAC Permission Errors

If you see errors like:
```
kubedeschedulers.operator.openshift.io "cluster" is forbidden:
User "system:serviceaccount:virt-advisor-operator-system:virt-advisor-operator-controller-manager"
cannot get resource "kubedeschedulers" in API group "operator.openshift.io"
```

This means the ClusterRole doesn't have the required permissions.

**Solution: Update RBAC**

1. Ensure you're using the latest manifests:
   ```bash
   make manifests
   ```

2. Re-apply the deployment to update the ClusterRole:
   ```bash
   make deploy IMG=${IMG}
   ```

3. Verify the ClusterRole has the correct permissions:
   ```bash
   oc get clusterrole virt-advisor-operator-manager-role -o yaml
   ```

   You should see rules for:
   - `operator.openshift.io` / `kubedeschedulers`
   - `machineconfiguration.openshift.io` / `machineconfigs`
   - `machineconfiguration.openshift.io` / `machineconfigpools`

4. Restart the operator pod to pick up new permissions:
   ```bash
   oc rollout restart deployment/virt-advisor-operator-controller-manager \
     -n virt-advisor-operator-system
   ```

5. Check the VirtPlatformConfig status again:
   ```bash
   oc get virtplatformconfig load-aware-rebalancing -o yaml
   ```

**Automatic Retry on Spec Change:**

As of the latest version, the operator automatically retries Failed plans when you change the spec:

```bash
# Failed plan will automatically retry if you change any spec field
oc patch virtplatformconfig load-aware-rebalancing \
  --type=merge \
  -p '{"spec":{"action":"DryRun"}}'  # Changing action triggers retry

# Or change options
oc patch virtplatformconfig load-aware-rebalancing \
  --type=merge \
  -p '{"spec":{"options":{"loadAware":{"deschedulingIntervalSeconds":120}}}}'
```

The operator tracks `status.observedGeneration` vs `metadata.generation` to detect spec changes. When they differ, it automatically resets the plan to Pending and retries.

**Manual Retry (Alternative):**

If you don't want to change the spec, you can manually trigger a retry:

```bash
# Option 1: Delete and recreate
oc delete virtplatformconfig load-aware-rebalancing
oc apply -f your-config.yaml

# Option 2: Manually reset status (requires status subresource access)
oc patch virtplatformconfig load-aware-rebalancing \
  --type=merge \
  --subresource=status \
  -p '{"status":{"phase":"Pending","observedGeneration":0}}'
```

### Plan Stuck in PrerequisiteFailed

Check which prerequisite is missing:
```bash
oc get virtplatformconfig load-aware-rebalancing -o jsonpath='{.status.message}'
```

Install the missing operator (e.g., Descheduler) and the operator will detect it automatically via CRD Sentinel.

**CRD Sentinel Behavior:**
- If a required CRD is missing at startup, the operator logs it
- When the CRD is installed, the sentinel detects it
- The operator pod automatically restarts to register watches
- The plan is automatically retried every 5 minutes until prerequisites are met
- Alternatively, changing the spec will trigger an immediate retry

**Force Immediate Retry:**
```bash
# Change any spec field to trigger immediate retry
oc patch virtplatformconfig load-aware-rebalancing \
  --type=merge \
  -p '{"spec":{"action":"DryRun"}}'
```

### Plan Stuck in InProgress

Check item status:
```bash
oc get virtplatformconfig load-aware-rebalancing -o jsonpath='{.status.items[*].state}'
```

If MachineConfig item is stuck:
```bash
# Check MCP status
oc get mcp worker

# Look for degraded nodes
oc get nodes

# Check MCO logs
oc logs -n openshift-machine-config-operator -l k8s-app=machine-config-operator
```

### Operator Logs

View operator logs:
```bash
oc logs -n virt-advisor-operator-system deployment/virt-advisor-operator-controller-manager -f
```

Enable debug logging (if supported):
```bash
oc set env deployment/virt-advisor-operator-controller-manager -n virt-advisor-operator-system LOG_LEVEL=debug
```

## Uninstalling

### Step 1: Delete All VirtPlatformConfigs

```bash
oc delete virtplatformconfig --all
```

**Note:** This does NOT revert the applied configurations. Manually revert if needed (see Rollback Procedures).

### Step 2: Undeploy the Operator

```bash
make undeploy
```

This removes:
- Operator deployment
- ServiceAccount and RBAC
- Namespace (if no other resources remain)

### Step 3: Uninstall CRDs

```bash
make uninstall
```

**WARNING:** This deletes the CRD and ALL VirtPlatformConfig resources.

### Step 4: Clean Up Managed Resources (Manual)

The operator doesn't automatically clean up resources it created. Remove them manually:

```bash
# Remove MachineConfig (triggers reboot)
oc delete machineconfig 99-worker-psi-karg

# Reset Descheduler to defaults (or delete it)
oc delete kubedescheduler cluster
```

## Security Considerations

### RBAC Permissions

The operator requires broad permissions to manage various resources:

**What the operator can do:**
- Read/Write `VirtPlatformConfig` resources
- Read/Write `KubeDescheduler` resources (if installed)
- Read/Write `MachineConfig` resources
- Read `CustomResourceDefinitions` (for prerequisite checking)
- Read `MachineConfigPools` (for status monitoring)

**Audit Trail:**
- All actions are logged in operator logs
- Managed resources have `advisor.kubevirt.io/managed-by` annotation
- Kubernetes audit logs show operator as the actor

### Network Policies

If using network policies, ensure the operator can:
- Access the Kubernetes API server
- Communicate with webhooks (if configured)

### Pod Security

The operator runs with:
- Non-root user
- No privileged containers
- Limited capabilities

## Monitoring and Observability

### Prometheus Metrics (Future)

The operator will expose metrics:
- `virtplatformconfig_phase{name, profile}` - Current phase
- `virtplatformconfig_drift_detected{name}` - Drift detection
- `virtplatformconfig_apply_duration_seconds{name}` - Apply time

### Events

Watch Kubernetes events:
```bash
oc get events -n virt-advisor-operator-system --watch
```

### Status Conditions

Check detailed status:
```bash
oc get virtplatformconfig load-aware-rebalancing -o jsonpath='{.status.conditions}' | jq
```

## Best Practices

1. **Always DryRun First** - Never skip the review step
2. **Test on Non-Prod** - Validate changes in a safe environment
3. **Use Maintenance Windows** - Schedule disruptive changes appropriately
4. **Monitor MCPs** - Watch node rollouts during MachineConfig changes
5. **Document Changes** - Keep a record of what profiles are applied
6. **Version Control** - Store VirtPlatformConfigs in Git
7. **Review Operator Logs** - Check for warnings or errors
8. **Plan Rollbacks** - Know how to revert before applying

## Advanced: Multi-Cluster Deployments

For managing multiple clusters:

1. Build and push image once
2. Deploy to each cluster with the same image
3. Use cluster-specific configurations via options
4. Consider using ArgoCD ApplicationSets for GitOps

Example ApplicationSet:
```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: virt-advisor-configs
spec:
  generators:
  - list:
      elements:
      - cluster: prod-east
        interval: 300
      - cluster: prod-west
        interval: 300
  template:
    spec:
      source:
        path: configs/{{cluster}}
      destination:
        name: '{{cluster}}'
```

## Additional Resources

- [VEP Document](vep.md) - Architecture and design
- [Load Aware Profile Guide](LOADAWARE_PROFILE.md) - Profile-specific documentation
- [Testing on Kind](TESTING_ON_KIND.md) - Local development testing
- [Profile Development Guide](profile-development-guide.md) - Creating custom profiles

## Getting Help

- Check operator logs: `oc logs -n virt-advisor-operator-system -l control-plane=controller-manager`
- Review status: `oc get virtplatformconfig -o yaml`
- Open issues: https://github.com/kubevirt/virt-advisor-operator/issues
