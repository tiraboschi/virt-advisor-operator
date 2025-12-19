# VirtPlatformConfig Operator Workflow Guide

This guide walks through a complete real-world workflow of using the VirtPlatformConfig operator to enable load-aware VM rebalancing on an OpenShift cluster. All YAML snippets are from actual cluster output.

---

## Table of Contents

1. [Profile Discovery](#1-profile-discovery)
2. [Initial Status - Ignored Phase](#2-initial-status---ignored-phase)
3. [Initiating DryRun](#3-initiating-dryrun)
4. [Prerequisite Checks](#4-prerequisite-checks)
5. [Installing Prerequisites](#5-installing-prerequisites)
6. [Reviewing the Generated Plan](#6-reviewing-the-generated-plan)
7. [Applying the Configuration](#7-applying-the-configuration)
8. [Monitoring Progress During Long Operations](#8-monitoring-progress-during-long-operations)
9. [Completion](#9-completion)
10. [Drift Detection](#10-drift-detection)
11. [Acknowledging and Resolving Drift](#11-acknowledging-and-resolving-drift)

---

## 1. Profile Discovery

When the operator starts, it automatically advertises available profiles as `VirtPlatformConfig` resources in **Ignore** action (safe by default).

```bash
$ oc get VirtPlatformConfig
NAME                     ACTION   IMPACT   PHASE     AGE
load-aware-rebalancing   Ignore   High     Ignored   44s
virt-higher-density      Ignore   High     Ignored   44s
```

**Key Points:**
- All profiles are **automatically created** by the operator
- Default action is **Ignore** (no changes to cluster)
- **IMPACT** column shows the potential impact level (Low/Medium/High)
- **PHASE** column shows current lifecycle state

**Available Profiles:**
- `load-aware-rebalancing`: Enables intelligent VM scheduling based on node load metrics
- `virt-higher-density`: Optimizes cluster for higher VM density per node

---

## 2. Initial Status - Ignored Phase

Let's inspect the `load-aware-rebalancing` profile in its initial state:

```bash
$ oc get virtplatformconfigs load-aware-rebalancing -o yaml
```

```yaml
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  annotations:
    advisor.kubevirt.io/auto-created: "true"
    advisor.kubevirt.io/description: Enables load-aware VM rebalancing and PSI metrics
      for intelligent scheduling
    advisor.kubevirt.io/impact-summary: Medium - May require node reboots if PSI metrics
      are enabled
  creationTimestamp: "2025-12-18T21:13:17Z"
  generation: 1
  labels:
    advisor.kubevirt.io/category: scheduling
  name: load-aware-rebalancing
  resourceVersion: "69234"
  uid: 893b77f0-692d-4886-ba74-8b0e14bb577a
spec:
  action: Ignore
  bypassOptimisticLock: false
  failurePolicy: Abort
  profile: load-aware-rebalancing
status:
  conditions:
  - lastTransitionTime: "2025-12-18T21:13:17Z"
    message: Configuration is ignored - no management actions will be performed. Resources
      remain in their current state and are now under manual control. Change action
      to 'DryRun' or 'Apply' to resume operator management.
    reason: Ignored
    status: "True"
    type: Ignored
  - lastTransitionTime: "2025-12-18T21:13:17Z"
    message: Not applicable - configuration is ignored
    reason: Ignored
    status: "False"
    type: Drafting
  - lastTransitionTime: "2025-12-18T21:13:17Z"
    message: Not applicable - configuration is ignored
    reason: Ignored
    status: "False"
    type: InProgress
  - lastTransitionTime: "2025-12-18T21:13:17Z"
    message: Not applicable - configuration is ignored
    reason: Ignored
    status: "False"
    type: ReviewRequired
  - lastTransitionTime: "2025-12-18T21:13:17Z"
    message: Not applicable - configuration is ignored
    reason: Ignored
    status: "False"
    type: Completed
  - lastTransitionTime: "2025-12-18T21:13:17Z"
    message: Not applicable - configuration is ignored
    reason: Ignored
    status: "False"
    type: Drifted
  - lastTransitionTime: "2025-12-18T21:13:17Z"
    message: Not applicable - configuration is ignored
    reason: Ignored
    status: "False"
    type: PrerequisiteFailed
  - lastTransitionTime: "2025-12-18T21:13:17Z"
    message: Not applicable - configuration is ignored
    reason: Ignored
    status: "False"
    type: Failed
  impactSeverity: High
  observedGeneration: 1
  phase: Ignored
```

**Key Fields to Watch:**
- `spec.action: Ignore` - No management actions performed
- `status.phase: Ignored` - Current lifecycle phase
- `status.impactSeverity: High` - Potential impact if applied
- `status.conditions` - Kubernetes standard conditions showing current state

**In Ignored Phase:**
- Operator performs **no management actions**
- Resources remain in their current state
- Cluster admin has manual control

---

## 3. Initiating DryRun

To see what changes the profile would make **without applying them**, change action to `DryRun`:

```bash
oc patch virtplatformconfig load-aware-rebalancing \
  --type='json' \
  -p='[{"op": "replace", "path": "/spec/action", "value":"DryRun"}]'
```

**What Happens:**
1. `generation` increments (1 → 2) due to spec change
2. Operator detects spec change via generation mismatch
3. Transitions from `Ignored` → `Pending` → `Drafting`
4. Checks prerequisites (required CRDs)
5. Generates plan if prerequisites are met

**Expected Flow:**
```
Ignored → Pending → Drafting → ReviewRequired (if prerequisites OK)
                  ↓
          PrerequisiteFailed (if CRDs missing)
```

---

## 4. Prerequisite Checks

After setting action to `DryRun`, the operator validates that required CRDs are installed:

```bash
$ oc get virtplatformconfigs load-aware-rebalancing -o yaml
```

```yaml
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  annotations:
    advisor.kubevirt.io/auto-created: "true"
    advisor.kubevirt.io/description: Enables load-aware VM rebalancing and PSI metrics
      for intelligent scheduling
    advisor.kubevirt.io/impact-summary: Medium - May require node reboots if PSI metrics
      are enabled
  creationTimestamp: "2025-12-18T21:13:17Z"
  generation: 2
  labels:
    advisor.kubevirt.io/category: scheduling
  name: load-aware-rebalancing
  resourceVersion: "69923"
  uid: 893b77f0-692d-4886-ba74-8b0e14bb577a
spec:
  action: DryRun
  bypassOptimisticLock: false
  failurePolicy: Abort
  profile: load-aware-rebalancing
status:
  conditions:
  - lastTransitionTime: "2025-12-18T21:14:20Z"
    message: Configuration is being managed
    reason: NotIgnored
    status: "False"
    type: Ignored
  - lastTransitionTime: "2025-12-18T21:14:20Z"
    message: |
      Missing required dependencies:
        - KubeDescheduler (operator.openshift.io.v1): Please install the Descheduler Operator via OLM
    reason: PrerequisitesFailed
    status: "False"
    type: Drafting
  - lastTransitionTime: "2025-12-18T21:14:20Z"
    message: |
      Missing required dependencies:
        - KubeDescheduler (operator.openshift.io.v1): Please install the Descheduler Operator via OLM
    reason: PrerequisitesFailed
    status: "False"
    type: InProgress
  - lastTransitionTime: "2025-12-18T21:14:20Z"
    message: |
      Missing required dependencies:
        - KubeDescheduler (operator.openshift.io.v1): Please install the Descheduler Operator via OLM
    reason: PrerequisitesFailed
    status: "False"
    type: ReviewRequired
  - lastTransitionTime: "2025-12-18T21:14:20Z"
    message: |
      Missing required dependencies:
        - KubeDescheduler (operator.openshift.io.v1): Please install the Descheduler Operator via OLM
    reason: PrerequisitesFailed
    status: "False"
    type: Completed
  - lastTransitionTime: "2025-12-18T21:14:20Z"
    message: |
      Missing required dependencies:
        - KubeDescheduler (operator.openshift.io.v1): Please install the Descheduler Operator via OLM
    reason: PrerequisitesFailed
    status: "False"
    type: Drifted
  - lastTransitionTime: "2025-12-18T21:14:20Z"
    message: |
      Missing required dependencies:
        - KubeDescheduler (operator.openshift.io.v1): Please install the Descheduler Operator via OLM
    reason: PrerequisiteFailed
    status: "True"
    type: PrerequisiteFailed
  - lastTransitionTime: "2025-12-18T21:14:20Z"
    message: |
      Missing required dependencies:
        - KubeDescheduler (operator.openshift.io.v1): Please install the Descheduler Operator via OLM
    reason: PrerequisitesFailed
    status: "False"
    type: Failed
  - lastTransitionTime: "2025-12-18T21:14:20Z"
    message: |
      Missing required dependencies:
        - KubeDescheduler (operator.openshift.io.v1): Please install the Descheduler Operator via OLM
    reason: PrerequisitesFailed
    status: "False"
    type: Pending
  impactSeverity: High
  observedGeneration: 2
  phase: PrerequisiteFailed
```

**Key Points:**
- `status.phase: PrerequisiteFailed` - Prerequisites not met
- Condition `PrerequisiteFailed` is `True` with detailed error message
- Error message tells you **exactly what's missing** and **how to install it**
- `observedGeneration: 2` matches `generation: 2` - operator has processed the spec change

**What's Missing:**
- `KubeDescheduler` CRD (from Descheduler Operator)

**Auto-Retry Behavior:**
The operator will automatically retry every **2 minutes** to check if prerequisites become available. When the CRD is installed, it will automatically proceed to plan generation.

---

## 5. Installing Prerequisites

Install the Descheduler Operator via OperatorHub or OLM:

```bash
# Example: Install via OLM (method varies by cluster setup)
# This is typically done through the OpenShift Console OperatorHub
# or via creating a Subscription resource
```

**What Happens After Installation:**
1. Descheduler Operator installs the `KubeDescheduler` CRD
2. VirtPlatformConfig operator detects CRD availability (via CRD Sentinel or periodic recheck)
3. Automatic transition: `PrerequisiteFailed` → `Pending` → `Drafting` → `ReviewRequired`
4. Plan is generated with diffs showing proposed changes

---

## 6. Reviewing the Generated Plan

Once prerequisites are met, the operator generates a plan and transitions to `ReviewRequired` phase:

```bash
$ oc get virtplatformconfigs load-aware-rebalancing -o yaml
```

```yaml
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  annotations:
    advisor.kubevirt.io/auto-created: "true"
    advisor.kubevirt.io/description: Enables load-aware VM rebalancing and PSI metrics
      for intelligent scheduling
    advisor.kubevirt.io/impact-summary: Medium - May require node reboots if PSI metrics
      are enabled
  creationTimestamp: "2025-12-18T21:13:17Z"
  generation: 2
  labels:
    advisor.kubevirt.io/category: scheduling
  name: load-aware-rebalancing
  resourceVersion: "73137"
  uid: 893b77f0-692d-4886-ba74-8b0e14bb577a
spec:
  action: DryRun
  bypassOptimisticLock: false
  failurePolicy: Abort
  profile: load-aware-rebalancing
status:
  conditions:
  - lastTransitionTime: "2025-12-18T21:14:20Z"
    message: Configuration is being managed
    reason: NotIgnored
    status: "False"
    type: Ignored
  - lastTransitionTime: "2025-12-18T21:19:21Z"
    message: Plan generation completed
    reason: NotDrafting
    status: "False"
    type: Drafting
  - lastTransitionTime: "2025-12-18T21:19:20Z"
    message: Plan is not being applied
    reason: NotInProgress
    status: "False"
    type: InProgress
  - lastTransitionTime: "2025-12-18T21:19:21Z"
    message: Plan ready for review. Change action to 'Apply' to execute.
    reason: ReviewRequired
    status: "True"
    type: ReviewRequired
  - lastTransitionTime: "2025-12-18T21:19:21Z"
    message: Plan not yet applied
    reason: NotCompleted
    status: "False"
    type: Completed
  - lastTransitionTime: "2025-12-18T21:19:21Z"
    message: Configuration is in sync with desired state
    reason: NoDrift
    status: "False"
    type: Drifted
  - lastTransitionTime: "2025-12-18T21:19:21Z"
    message: Plan ready for review. Change action to 'Apply' to execute.
    reason: ReviewRequired
    status: "False"
    type: PrerequisiteFailed
  - lastTransitionTime: "2025-12-18T21:19:21Z"
    message: Plan ready for review. Change action to 'Apply' to execute.
    reason: ReviewRequired
    status: "False"
    type: Failed
  - lastTransitionTime: "2025-12-18T21:19:20Z"
    message: Prerequisites now available
    reason: Pending
    status: "True"
    type: Pending
  impactSeverity: High
  items:
  - desiredState:
      apiVersion: machineconfiguration.openshift.io/v1
      kind: MachineConfig
      metadata:
        labels:
          machineconfiguration.openshift.io/role: worker
        name: 99-worker-psi-karg
      spec:
        kernelArguments:
        - psi=1
    diff: |
      --- /dev/null
      +++ 99-worker-psi-karg (MachineConfig)
      @@ -0,0 +1,10 @@
      +apiVersion: machineconfiguration.openshift.io/v1
      +kind: MachineConfig
      +metadata:
      +  labels:
      +    machineconfiguration.openshift.io/role: worker
      +  name: 99-worker-psi-karg
      +spec:
      +  kernelArguments:
      +  - psi=1
    impactSeverity: High
    managedFields:
    - spec.kernelArguments
    message: MachineConfig '99-worker-psi-karg' will be configured to enable PSI metrics
    name: enable-psi-metrics
    state: Pending
    targetRef:
      apiVersion: machineconfiguration.openshift.io/v1
      kind: MachineConfig
      name: 99-worker-psi-karg
  - desiredState:
      apiVersion: operator.openshift.io/v1
      kind: KubeDescheduler
      metadata:
        name: cluster
        namespace: openshift-kube-descheduler-operator
      spec:
        deschedulingIntervalSeconds: 60
        evictionLimits:
          node: 2
          total: 5
        mode: Automatic
        profileCustomizations:
          devDeviationThresholds: AsymmetricLow
          devEnableEvictionsInBackground: true
          devEnableSoftTainter: true
        profiles:
        - KubeVirtRelieveAndMigrate
    diff: |
      --- /dev/null
      +++ cluster (KubeDescheduler)
      @@ -0,0 +1,20 @@
      +apiVersion: operator.openshift.io/v1
      +kind: KubeDescheduler
      +metadata:
      +  name: cluster
      +  namespace: openshift-kube-descheduler-operator
      +spec:
      +  deschedulingIntervalSeconds: 60
      +  evictionLimits:
      +    node: 2
      +    total: 5
      +  logLevel: Normal
      +  mode: Automatic
      +  operatorLogLevel: Normal
      +  profileCustomizations:
      +    devDeviationThresholds: AsymmetricLow
      +    devEnableEvictionsInBackground: true
      +    devEnableSoftTainter: true
      +  profiles:
      +  - KubeVirtRelieveAndMigrate
    impactSeverity: Medium
    managedFields:
    - spec.deschedulingIntervalSeconds
    - spec.mode
    - spec.profiles
    - spec.profileCustomizations
    - spec.evictionLimits
    message: KubeDescheduler 'cluster' will be configured with profile 'KubeVirtRelieveAndMigrate'
    name: enable-load-aware-descheduling
    state: Pending
    targetRef:
      apiVersion: operator.openshift.io/v1
      kind: KubeDescheduler
      name: cluster
      namespace: openshift-kube-descheduler-operator
  observedGeneration: 6
  operatorVersion: v0.1.31
  phase: ReviewRequired
  sourceSnapshotHash: 8046b8bc3127298de872e1ee226b6464d73a52449bc71d34910bc97914434037
```

**Key Points:**
- `status.phase: ReviewRequired` - Plan ready for review
- `status.items[]` - Array of configuration items to be applied
- Each item includes:
  - `diff` - Unified diff showing **exactly what will change**
  - `impactSeverity` - Impact level for this specific item (Low/Medium/High)
  - `managedFields` - Which fields the operator will manage
  - `desiredState` - Complete target configuration
  - `state: Pending` - Not yet applied

**Plan Items:**

**Item 1: `enable-psi-metrics`**
- **Type:** MachineConfig
- **Impact:** High (requires node reboots)
- **Purpose:** Enables PSI (Pressure Stall Information) kernel metrics on worker nodes
- **Changes:** Adds `psi=1` kernel argument
- **Side Effects:** Worker nodes will reboot one-by-one to apply new kernel arguments

**Item 2: `enable-load-aware-descheduling`**
- **Type:** KubeDescheduler
- **Impact:** Medium
- **Purpose:** Configures descheduler with load-aware VM migration profile
- **Changes:**
  - Enables `KubeVirtRelieveAndMigrate` profile
  - Sets eviction limits (5 total, 2 per node) derived from HCO migration limits
  - Configures descheduling interval (60 seconds)
  - Enables advanced features (soft tainting, background evictions, asymmetric low thresholds)

**Optimistic Locking:**
- `status.sourceSnapshotHash` - Hash of current cluster state
- Before applying, operator verifies cluster state hasn't changed since plan generation
- Prevents applying outdated plans (TOCTOU protection)

---

## 7. Applying the Configuration

After reviewing the plan, change action to `Apply` to execute it:

```bash
oc patch virtplatformconfig load-aware-rebalancing \
  --type='json' \
  -p='[{"op": "replace", "path": "/spec/action", "value":"Apply"}]'
```

**What Happens:**
1. `generation` increments (2 → 3)
2. Operator verifies optimistic lock (cluster state unchanged since plan generation)
3. Transitions: `ReviewRequired` → `InProgress`
4. Applies each plan item sequentially
5. Monitors health/convergence of each item

**Failure Policy:**
- `spec.failurePolicy: Abort` - Stop on first failure (default)
- Alternative: `Continue` - Attempt all items even if some fail

---

## 8. Monitoring Progress During Long Operations

MachineConfig changes trigger node reboots. Monitor progress:

```bash
$ oc get virtplatformconfigs load-aware-rebalancing -o yaml
```

```yaml
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  annotations:
    advisor.kubevirt.io/auto-created: "true"
    advisor.kubevirt.io/description: Enables load-aware VM rebalancing and PSI metrics
      for intelligent scheduling
    advisor.kubevirt.io/impact-summary: Medium - May require node reboots if PSI metrics
      are enabled
  creationTimestamp: "2025-12-18T21:13:17Z"
  generation: 3
  labels:
    advisor.kubevirt.io/category: scheduling
  name: load-aware-rebalancing
  resourceVersion: "79989"
  uid: 893b77f0-692d-4886-ba74-8b0e14bb577a
spec:
  action: Apply
  bypassOptimisticLock: false
  failurePolicy: Abort
  profile: load-aware-rebalancing
status:
  conditions:
  - lastTransitionTime: "2025-12-18T21:14:20Z"
    message: Configuration is being managed
    reason: NotIgnored
    status: "False"
    type: Ignored
  - lastTransitionTime: "2025-12-18T21:19:21Z"
    message: Plan generation completed
    reason: NotDrafting
    status: "False"
    type: Drafting
  - lastTransitionTime: "2025-12-18T21:23:34Z"
    message: Applying plan configuration
    reason: InProgress
    status: "True"
    type: InProgress
  - lastTransitionTime: "2025-12-18T21:23:34Z"
    message: Applying plan configuration
    reason: InProgress
    status: "False"
    type: ReviewRequired
  - lastTransitionTime: "2025-12-18T21:23:34Z"
    message: Applying plan configuration
    reason: InProgress
    status: "False"
    type: Completed
  - lastTransitionTime: "2025-12-18T21:23:34Z"
    message: Applying plan configuration
    reason: InProgress
    status: "False"
    type: Drifted
  - lastTransitionTime: "2025-12-18T21:23:34Z"
    message: Applying plan configuration
    reason: InProgress
    status: "False"
    type: PrerequisiteFailed
  - lastTransitionTime: "2025-12-18T21:23:34Z"
    message: Applying plan configuration
    reason: InProgress
    status: "False"
    type: Failed
  - lastTransitionTime: "2025-12-18T21:23:24Z"
    message: Configuration successfully applied and in sync
    reason: NotCompleted
    status: "False"
    type: Pending
  impactSeverity: High
  items:
  - desiredState:
      apiVersion: machineconfiguration.openshift.io/v1
      kind: MachineConfig
      metadata:
        labels:
          machineconfiguration.openshift.io/role: worker
        name: 99-worker-psi-karg
      spec:
        kernelArguments:
        - psi=1
    diff: |
      --- 99-worker-psi-karg (MachineConfig)
      +++ 99-worker-psi-karg (MachineConfig)
      (no changes)
    impactSeverity: High
    lastTransitionTime: "2025-12-18T21:23:24Z"
    managedFields:
    - spec.kernelArguments
    message: 'MachineConfigPool ''worker'' is updating (Updated: 1/3, Ready: 1/3)'
    name: enable-psi-metrics
    state: InProgress
    targetRef:
      apiVersion: machineconfiguration.openshift.io/v1
      kind: MachineConfig
      name: 99-worker-psi-karg
  - desiredState:
      apiVersion: operator.openshift.io/v1
      kind: KubeDescheduler
      metadata:
        name: cluster
        namespace: openshift-kube-descheduler-operator
      spec:
        deschedulingIntervalSeconds: 60
        evictionLimits:
          node: 2
          total: 5
        mode: Automatic
        profileCustomizations:
          devDeviationThresholds: AsymmetricLow
          devEnableEvictionsInBackground: true
          devEnableSoftTainter: true
        profiles:
        - AffinityAndTaints
        - KubeVirtRelieveAndMigrate
    diff: |
      --- cluster (KubeDescheduler)
      +++ cluster (KubeDescheduler)
      (no changes)
    impactSeverity: Low
    lastTransitionTime: "2025-12-18T21:23:24Z"
    managedFields:
    - spec.deschedulingIntervalSeconds
    - spec.mode
    - spec.profiles
    - spec.profileCustomizations
    - spec.evictionLimits
    message: Applied successfully
    name: enable-load-aware-descheduling
    state: Completed
    targetRef:
      apiVersion: operator.openshift.io/v1
      kind: KubeDescheduler
      name: cluster
      namespace: openshift-kube-descheduler-operator
  observedGeneration: 3
  operatorVersion: v0.1.31
  phase: InProgress
  sourceSnapshotHash: 2cd3af7d3f280c5c3fe075a1c8ae6e4e7c0fc6053d4cd28ca5ccc77337006eb9
```

**Progress Indicators:**
- `status.phase: InProgress` - Actively applying configuration
- `condition InProgress: True` - Confirming in-progress state

**Per-Item Status:**

**Item 1: `enable-psi-metrics`**
- `state: InProgress` - MachineConfig applied, waiting for convergence
- `message: 'MachineConfigPool 'worker' is updating (Updated: 1/3, Ready: 1/3)'`
  - Shows **real-time progress**: 1 out of 3 worker nodes updated
- `diff: (no changes)` - Configuration already applied to MachineConfig resource
- `lastTransitionTime` - When item entered InProgress state

**Item 2: `enable-load-aware-descheduling`**
- `state: Completed` - KubeDescheduler configuration applied successfully
- `message: Applied successfully` - No health monitoring needed for this resource
- `diff: (no changes)` - Configuration matches desired state

**What's Happening:**
- MachineConfigPool is rolling out kernel argument changes to worker nodes
- Nodes reboot **one at a time** (rolling update)
- Operator polls MachineConfigPool status to track progress
- VirtPlatformConfig stays in `InProgress` phase until all nodes are updated

**Monitoring Node Rollout:**
```bash
# Watch MachineConfigPool status
oc get mcp worker -w

# Watch node status
oc get nodes -w
```

---

## 9. Completion

After all nodes have been updated and all items are healthy:

```bash
$ oc get virtplatformconfigs load-aware-rebalancing -o yaml
```

```yaml
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  annotations:
    advisor.kubevirt.io/auto-created: "true"
    advisor.kubevirt.io/description: Enables load-aware VM rebalancing and PSI metrics
      for intelligent scheduling
    advisor.kubevirt.io/impact-summary: Medium - May require node reboots if PSI metrics
      are enabled
  creationTimestamp: "2025-12-18T21:13:17Z"
  generation: 3
  labels:
    advisor.kubevirt.io/category: scheduling
  name: load-aware-rebalancing
  resourceVersion: "90032"
  uid: 893b77f0-692d-4886-ba74-8b0e14bb577a
spec:
  action: Apply
  bypassOptimisticLock: false
  failurePolicy: Abort
  profile: load-aware-rebalancing
status:
  conditions:
  - lastTransitionTime: "2025-12-18T21:14:20Z"
    message: Configuration is being managed
    reason: NotIgnored
    status: "False"
    type: Ignored
  - lastTransitionTime: "2025-12-18T21:19:21Z"
    message: Plan generation completed
    reason: NotDrafting
    status: "False"
    type: Drafting
  - lastTransitionTime: "2025-12-18T21:38:06Z"
    message: Plan application completed
    reason: NotInProgress
    status: "False"
    type: InProgress
  - lastTransitionTime: "2025-12-18T21:38:06Z"
    message: Configuration successfully applied and in sync
    reason: NotCompleted
    status: "False"
    type: ReviewRequired
  - lastTransitionTime: "2025-12-18T21:38:06Z"
    message: Configuration successfully applied and in sync
    reason: Completed
    status: "True"
    type: Completed
  - lastTransitionTime: "2025-12-18T21:38:06Z"
    message: Configuration is in sync with desired state
    reason: NoDrift
    status: "False"
    type: Drifted
  - lastTransitionTime: "2025-12-18T21:38:06Z"
    message: All prerequisites met
    reason: Completed
    status: "False"
    type: PrerequisiteFailed
  - lastTransitionTime: "2025-12-18T21:38:06Z"
    message: No failures occurred
    reason: Completed
    status: "False"
    type: Failed
  - lastTransitionTime: "2025-12-18T21:23:24Z"
    message: Configuration successfully applied and in sync
    reason: NotCompleted
    status: "False"
    type: Pending
  impactSeverity: High
  items:
  - desiredState:
      apiVersion: machineconfiguration.openshift.io/v1
      kind: MachineConfig
      metadata:
        labels:
          machineconfiguration.openshift.io/role: worker
        name: 99-worker-psi-karg
      spec:
        kernelArguments:
        - psi=1
    diff: |
      --- 99-worker-psi-karg (MachineConfig)
      +++ 99-worker-psi-karg (MachineConfig)
      (no changes)
    impactSeverity: High
    lastTransitionTime: "2025-12-18T21:38:05Z"
    managedFields:
    - spec.kernelArguments
    message: MachineConfigPool 'worker' is ready (all 3 nodes updated)
    name: enable-psi-metrics
    state: Completed
    targetRef:
      apiVersion: machineconfiguration.openshift.io/v1
      kind: MachineConfig
      name: 99-worker-psi-karg
  - desiredState:
      apiVersion: operator.openshift.io/v1
      kind: KubeDescheduler
      metadata:
        name: cluster
        namespace: openshift-kube-descheduler-operator
      spec:
        deschedulingIntervalSeconds: 60
        evictionLimits:
          node: 2
          total: 5
        mode: Automatic
        profileCustomizations:
          devDeviationThresholds: AsymmetricLow
          devEnableEvictionsInBackground: true
          devEnableSoftTainter: true
        profiles:
        - AffinityAndTaints
        - KubeVirtRelieveAndMigrate
    diff: |
      --- cluster (KubeDescheduler)
      +++ cluster (KubeDescheduler)
      (no changes)
    impactSeverity: Low
    lastTransitionTime: "2025-12-18T21:23:24Z"
    managedFields:
    - spec.deschedulingIntervalSeconds
    - spec.mode
    - spec.profiles
    - spec.profileCustomizations
    - spec.evictionLimits
    message: Applied successfully
    name: enable-load-aware-descheduling
    state: Completed
    targetRef:
      apiVersion: operator.openshift.io/v1
      kind: KubeDescheduler
      name: cluster
      namespace: openshift-kube-descheduler-operator
  observedGeneration: 3
  operatorVersion: v0.1.31
  phase: Completed
  sourceSnapshotHash: 2cd3af7d3f280c5c3fe075a1c8ae6e4e7c0fc6053d4cd28ca5ccc77337006eb9
```

**Success Indicators:**
- `status.phase: Completed` - All items applied successfully
- `condition Completed: True` - Success condition set
- `condition Drifted: False` - Configuration in sync with desired state
- All items: `state: Completed`

**Item Status:**
- **Item 1:** `message: MachineConfigPool 'worker' is ready (all 3 nodes updated)`
  - All 3 worker nodes rebooted and converged
  - PSI metrics now enabled on all workers
- **Item 2:** `message: Applied successfully`
  - Descheduler configured with load-aware profile

**Drift Detection:**
- Operator continues monitoring in `Completed` phase
- Periodically checks if managed resources drift from desired state
- `sourceSnapshotHash` updated to reflect current applied state

**Timeline:**
- Started: `21:23:24Z` (first item applied)
- Completed: `21:38:05Z` (last item converged)
- **Total Duration:** ~15 minutes (3 nodes × ~5 minutes per node reboot)

---

## 10. Drift Detection

The operator continuously monitors managed resources for drift. Drift can be triggered by:
1. **External dependency changes** (e.g., HCO migration limits changed)
2. **Manual modifications** to managed resources (e.g., someone edits KubeDescheduler CR)

In this example, someone changed the HyperConverged CR's migration limits, which triggers drift detection:

```bash
$ oc get virtplatformconfigs load-aware-rebalancing -o yaml
```

```yaml
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  annotations:
    advisor.kubevirt.io/auto-created: "true"
    advisor.kubevirt.io/description: Enables load-aware VM rebalancing and PSI metrics
      for intelligent scheduling
    advisor.kubevirt.io/impact-summary: Medium - May require node reboots if PSI metrics
      are enabled
  creationTimestamp: "2025-12-18T21:13:17Z"
  generation: 3
  labels:
    advisor.kubevirt.io/category: scheduling
  name: load-aware-rebalancing
  resourceVersion: "90879"
  uid: 893b77f0-692d-4886-ba74-8b0e14bb577a
spec:
  action: Apply
  bypassOptimisticLock: false
  failurePolicy: Abort
  profile: load-aware-rebalancing
status:
  conditions:
  - lastTransitionTime: "2025-12-18T21:14:20Z"
    message: Configuration is being managed
    reason: NotIgnored
    status: "False"
    type: Ignored
  - lastTransitionTime: "2025-12-18T21:39:21Z"
    message: Plan generation completed
    reason: NotDrafting
    status: "False"
    type: Drafting
  - lastTransitionTime: "2025-12-18T21:39:21Z"
    message: Plan is not being applied
    reason: NotInProgress
    status: "False"
    type: InProgress
  - lastTransitionTime: "2025-12-18T21:39:21Z"
    message: Plan ready for review. Change action to 'Apply' to execute.
    reason: ReviewRequired
    status: "True"
    type: ReviewRequired
  - lastTransitionTime: "2025-12-18T21:39:21Z"
    message: Plan not yet applied
    reason: NotCompleted
    status: "False"
    type: Completed
  - lastTransitionTime: "2025-12-18T21:39:21Z"
    message: Configuration is in sync with desired state
    reason: NoDrift
    status: "False"
    type: Drifted
  - lastTransitionTime: "2025-12-18T21:39:21Z"
    message: Plan ready for review. Change action to 'Apply' to execute.
    reason: ReviewRequired
    status: "False"
    type: PrerequisiteFailed
  - lastTransitionTime: "2025-12-18T21:39:21Z"
    message: Plan ready for review. Change action to 'Apply' to execute.
    reason: ReviewRequired
    status: "False"
    type: Failed
  - lastTransitionTime: "2025-12-18T21:23:24Z"
    message: Configuration successfully applied and in sync
    reason: NotCompleted
    status: "False"
    type: Pending
  - lastTransitionTime: "2025-12-18T21:39:21Z"
    message: 'Input dependency changed: HCO parallelMigrationsPerCluster changed (scaled
      5 -> 10)'
    reason: InputChanged
    status: "True"
    type: InputDependencyDrift
  impactSeverity: Low
  items:
  - desiredState:
      apiVersion: machineconfiguration.openshift.io/v1
      kind: MachineConfig
      metadata:
        labels:
          machineconfiguration.openshift.io/role: worker
        name: 99-worker-psi-karg
      spec:
        kernelArguments:
        - psi=1
    diff: |
      --- 99-worker-psi-karg (MachineConfig)
      +++ 99-worker-psi-karg (MachineConfig)
      (no changes)
    impactSeverity: Low
    managedFields:
    - spec.kernelArguments
    message: PSI metrics already effective in MachineConfigPool 'worker'
    name: enable-psi-metrics
    state: Pending
    targetRef:
      apiVersion: machineconfiguration.openshift.io/v1
      kind: MachineConfig
      name: 99-worker-psi-karg
  - desiredState:
      apiVersion: operator.openshift.io/v1
      kind: KubeDescheduler
      metadata:
        name: cluster
        namespace: openshift-kube-descheduler-operator
      spec:
        deschedulingIntervalSeconds: 60
        evictionLimits:
          node: 4
          total: 10
        mode: Automatic
        profileCustomizations:
          devDeviationThresholds: AsymmetricLow
          devEnableEvictionsInBackground: true
          devEnableSoftTainter: true
        profiles:
        - AffinityAndTaints
        - KubeVirtRelieveAndMigrate
    diff: |
      --- cluster (KubeDescheduler)
      +++ cluster (KubeDescheduler)
      @@ -4,10 +4,10 @@
         name: cluster
         namespace: openshift-kube-descheduler-operator
       spec:
      -  deschedulingIntervalSeconds: 180
      +  deschedulingIntervalSeconds: 60
         evictionLimits:
      -    node: 2
      -    total: 5
      +    node: 4
      +    total: 10
         logLevel: Normal
         managementState: Managed
         mode: Automatic
      @@ -28,7 +28,7 @@
         profileCustomizations:
           devDeviationThresholds: AsymmetricLow
           devEnableEvictionsInBackground: true
      -    devEnableSoftTainter: false
      +    devEnableSoftTainter: true
         profiles:
         - AffinityAndTaints
         - KubeVirtRelieveAndMigrate
    impactSeverity: Low
    managedFields:
    - spec.deschedulingIntervalSeconds
    - spec.mode
    - spec.profiles
    - spec.profileCustomizations
    - spec.evictionLimits
    message: KubeDescheduler 'cluster' will be configured with profile 'KubeVirtRelieveAndMigrate'
    name: enable-load-aware-descheduling
    state: Pending
    targetRef:
      apiVersion: operator.openshift.io/v1
      kind: KubeDescheduler
      name: cluster
      namespace: openshift-kube-descheduler-operator
  observedGeneration: 6
  operatorVersion: v0.1.31
  phase: ReviewRequired
  sourceSnapshotHash: abb75beb40eda416c3977d298c46d57bd9cdbc9eda245f2f4a91107973c8c092
```

**What Happened:**
1. External change detected: HCO `parallelMigrationsPerCluster` changed (value increased)
2. Since `load-aware-rebalancing` profile derives eviction limits from HCO, this affects the plan
3. **Automatic transition:** `Completed` → `Drafting` (plan regeneration) → `ReviewRequired`
4. **Key safety feature:** Despite `action: Apply`, operator **DOES NOT auto-apply**
5. Special condition set: `InputDependencyDrift: True`

**Drift Indicators:**
- `status.phase: ReviewRequired` - Waiting for user review despite action=Apply
- `condition InputDependencyDrift: True` - **Forced review marker**
  - `message: 'Input dependency changed: HCO parallelMigrationsPerCluster changed (scaled 5 -> 10)'`
  - Explains **exactly what changed** and **how**
- `observedGeneration: 6` (was 3) - Plan regenerated multiple times

**Why Forced Review?**
External dependency changes could have unintended consequences. The operator requires **explicit user acknowledgment** before applying drift-triggered changes to prevent:
- Accidental application of externally-triggered configuration changes
- Configuration thrashing between multiple controllers
- Unexpected cluster reconfigurations

**Diff Shows:**

**Item 1: `enable-psi-metrics`**
- No changes (PSI already enabled)

**Item 2: `enable-load-aware-descheduling`**
- **Eviction limits changed:** 5→10 total, 2→4 per node (derived from new HCO limits)
- **Descheduling interval changed:** 180→60 seconds (someone manually edited this)
- **Soft tainter changed:** false→true (drift from manual edit)

**Multiple Drift Sources:**
1. **Input dependency drift:** HCO limits changed (5→10)
2. **Manual drift:** Someone manually edited KubeDescheduler CR (changed interval to 180s, disabled soft tainter)

---

## 11. Acknowledging and Resolving Drift

To acknowledge the drift and allow the operator to correct it, you must **change the spec** to bump `generation`:

```bash
# Option 1: Toggle action (DryRun -> Apply)
oc patch virtplatformconfig load-aware-rebalancing \
  --type='json' \
  -p='[{"op": "replace", "path": "/spec/action", "value":"DryRun"}]'

oc patch virtplatformconfig load-aware-rebalancing \
  --type='json' \
  -p='[{"op": "replace", "path": "/spec/action", "value":"Apply"}]'

# Option 2: Modify any spec field (e.g., add/update options)
```

After acknowledgment:

```bash
$ oc get virtplatformconfigs load-aware-rebalancing -o yaml
```

```yaml
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  annotations:
    advisor.kubevirt.io/auto-created: "true"
    advisor.kubevirt.io/description: Enables load-aware VM rebalancing and PSI metrics
      for intelligent scheduling
    advisor.kubevirt.io/impact-summary: Medium - May require node reboots if PSI metrics
      are enabled
  creationTimestamp: "2025-12-18T21:13:17Z"
  generation: 7
  labels:
    advisor.kubevirt.io/category: scheduling
  name: load-aware-rebalancing
  resourceVersion: "96194"
  uid: 893b77f0-692d-4886-ba74-8b0e14bb577a
spec:
  action: Apply
  bypassOptimisticLock: false
  failurePolicy: Abort
  profile: load-aware-rebalancing
status:
  conditions:
  - lastTransitionTime: "2025-12-18T21:14:20Z"
    message: Configuration is being managed
    reason: NotIgnored
    status: "False"
    type: Ignored
  - lastTransitionTime: "2025-12-18T21:43:37Z"
    message: Plan generation completed
    reason: NotDrafting
    status: "False"
    type: Drafting
  - lastTransitionTime: "2025-12-18T21:45:20Z"
    message: Plan application completed
    reason: NotInProgress
    status: "False"
    type: InProgress
  - lastTransitionTime: "2025-12-18T21:45:20Z"
    message: Configuration successfully applied and in sync
    reason: NotCompleted
    status: "False"
    type: ReviewRequired
  - lastTransitionTime: "2025-12-18T21:45:20Z"
    message: Configuration successfully applied and in sync
    reason: Completed
    status: "True"
    type: Completed
  - lastTransitionTime: "2025-12-18T21:45:20Z"
    message: Configuration is in sync with desired state
    reason: NoDrift
    status: "False"
    type: Drifted
  - lastTransitionTime: "2025-12-18T21:45:20Z"
    message: All prerequisites met
    reason: Completed
    status: "False"
    type: PrerequisiteFailed
  - lastTransitionTime: "2025-12-18T21:45:20Z"
    message: No failures occurred
    reason: Completed
    status: "False"
    type: Failed
  - lastTransitionTime: "2025-12-18T21:45:20Z"
    message: Configuration successfully applied and in sync
    reason: NotCompleted
    status: "False"
    type: Pending
  - lastTransitionTime: "2025-12-18T21:42:42Z"
    message: User acknowledged input dependency change
    reason: Acknowledged
    status: "False"
    type: InputDependencyDrift
  impactSeverity: Low
  items:
  - desiredState:
      apiVersion: machineconfiguration.openshift.io/v1
      kind: MachineConfig
      metadata:
        labels:
          machineconfiguration.openshift.io/role: worker
        name: 99-worker-psi-karg
      spec:
        kernelArguments:
        - psi=1
    diff: |
      --- 99-worker-psi-karg (MachineConfig)
      +++ 99-worker-psi-karg (MachineConfig)
      (no changes)
    impactSeverity: Low
    lastTransitionTime: "2025-12-18T21:45:20Z"
    managedFields:
    - spec.kernelArguments
    message: MachineConfigPool 'worker' is ready (all 3 nodes updated)
    name: enable-psi-metrics
    state: Completed
    targetRef:
      apiVersion: machineconfiguration.openshift.io/v1
      kind: MachineConfig
      name: 99-worker-psi-karg
  - desiredState:
      apiVersion: operator.openshift.io/v1
      kind: KubeDescheduler
      metadata:
        name: cluster
        namespace: openshift-kube-descheduler-operator
      spec:
        deschedulingIntervalSeconds: 60
        evictionLimits:
          node: 4
          total: 10
        mode: Automatic
        profileCustomizations:
          devDeviationThresholds: AsymmetricLow
          devEnableEvictionsInBackground: true
          devEnableSoftTainter: true
        profiles:
        - AffinityAndTaints
        - KubeVirtRelieveAndMigrate
    diff: |
      --- cluster (KubeDescheduler)
      +++ cluster (KubeDescheduler)
      (no changes)
    impactSeverity: Low
    lastTransitionTime: "2025-12-18T21:45:20Z"
    managedFields:
    - spec.deschedulingIntervalSeconds
    - spec.mode
    - spec.profiles
    - spec.profileCustomizations
    - spec.evictionLimits
    message: Applied successfully
    name: enable-load-aware-descheduling
    state: Completed
    targetRef:
      apiVersion: operator.openshift.io/v1
      kind: KubeDescheduler
      name: cluster
      namespace: openshift-kube-descheduler-operator
  observedGeneration: 7
  operatorVersion: v0.1.31
  phase: Completed
  sourceSnapshotHash: 4f0dbe9837eacbf99ad4aeab7189d8b5552f64287164112e311507178f3fdcbc
```

**Resolution Flow:**
1. User toggled action (DryRun → Apply), bumping `generation` (6 → 7)
2. Operator detected `generation` change = user acknowledgment
3. `InputDependencyDrift` condition cleared: `reason: Acknowledged`
4. Transition: `ReviewRequired` → `InProgress` → `Completed`
5. Drift corrected:
   - Eviction limits updated to reflect new HCO values (10 total, 4 per node)
   - Manual edits reverted (descheduling interval 180→60, soft tainter enabled)
6. Configuration back in sync: `diff: (no changes)`

**Final State:**
- `status.phase: Completed` - Back to healthy state
- `condition InputDependencyDrift: False` - Drift acknowledged and resolved
- `observedGeneration: 7` - Operator processed acknowledgment
- All diffs show `(no changes)` - Cluster matches desired state

---

## Summary

This workflow demonstrated:

1. **Profile Discovery** - Automatic advertising of available profiles
2. **Safe Defaults** - Profiles start in `Ignore` action
3. **DryRun** - Preview changes before applying
4. **Prerequisite Validation** - Automatic detection of missing dependencies with actionable error messages
5. **Auto-Retry** - Automatic plan generation when prerequisites become available
6. **Diff Preview** - Detailed diffs showing exactly what will change
7. **Optimistic Locking** - Prevention of stale plan application (TOCTOU protection)
8. **Progress Monitoring** - Real-time status for long-running operations (node reboots)
9. **Drift Detection** - Continuous monitoring for configuration drift
10. **Input Dependency Drift** - Special handling for external dependency changes
11. **Forced Review** - Safety mechanism preventing auto-application of drift-triggered changes
12. **User Acknowledgment** - Explicit generation bump required to proceed after drift

**Key Safety Features:**
- **No auto-apply on drift** - External changes require user review
- **Optimistic locking** - Prevents applying outdated plans
- **Detailed diffs** - Know exactly what will change before applying
- **Failure policies** - Control blast radius (Abort vs Continue)
- **Health monitoring** - Wait for convergence before marking complete

**Next Steps:**
- Explore profile customization via `spec.options`
- Try the `virt-higher-density` profile
- Review operator logs for detailed reconciliation traces
- Monitor load-aware descheduling in action via descheduler logs

---

## Appendix: VirtHigherDensity Profile Example

The `virt-higher-density` profile optimizes the cluster for higher VM density by enabling swap and configuring HyperConverged (HCO) memory overcommit settings. Here's a real example showing the generated plan:

```bash
$ oc get virtplatformconfigs virt-higher-density -o yaml
```

```yaml
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  annotations:
    advisor.kubevirt.io/auto-created: "true"
    advisor.kubevirt.io/description: Enables higher VM density through swap and other
      optimizations
    advisor.kubevirt.io/impact-summary: Enables swap for kubelet to allow memory overcommit
      and higher VM density. Requires nodes with swap partition labeled CNV_SWAP.
  creationTimestamp: "2025-12-19T15:05:10Z"
  generation: 4
  labels:
    advisor.kubevirt.io/category: density
  name: virt-higher-density
  resourceVersion: "115519"
  uid: 14f1838b-7df4-47a3-8bf9-85b41b4731f5
spec:
  action: DryRun
  profile: virt-higher-density
status:
  conditions:
  - lastTransitionTime: "2025-12-19T17:01:21Z"
    message: Plan ready for review. Change action to 'Apply' to execute.
    reason: ReviewRequired
    status: "True"
    type: ReviewRequired
  - lastTransitionTime: "2025-12-19T17:01:21Z"
    message: Configuration is in sync with desired state
    reason: NoDrift
    status: "False"
    type: Drifted
  impactSeverity: High
  phase: ReviewRequired
  items:
  - name: enable-kubelet-swap
    state: Pending
    impactSeverity: High
    message: MachineConfig '90-worker-kubelet-swap' will be configured to enable kubelet swap
    targetRef:
      apiVersion: machineconfiguration.openshift.io/v1
      kind: MachineConfig
      name: 90-worker-kubelet-swap
    managedFields:
    - spec.config
    desiredState:
      apiVersion: machineconfiguration.openshift.io/v1
      kind: MachineConfig
      metadata:
        labels:
          machineconfiguration.openshift.io/role: worker
        name: 90-worker-kubelet-swap
      spec:
        config:
          ignition:
            version: 3.2.0
          storage:
            files:
            - contents:
                source: data:text/plain;charset=utf-8;base64,IyEvdXNyL2Jpbi9lbnYgYmFzaApzZXQgLWV1byBwaXBlZmFpbAoKUEFSVExBQkVMPSJDTlZfU1dBUCIKREVWSUNFPSIvZGV2L2Rpc2svYnktcGFydGxhYmVsLyR7UEFSVExBQkVMfSIKCmlmIFtbICEgLWUgIiRERVZJQ0UiIF1dOyB0aGVuCiAgZWNobyAiU3dhcCBwYXJ0aXRpb24gd2l0aCBQQVJUTEFCRUw9JHtQQVJUTEFCRUx9IG5vdCBmb3VuZCIgPiYyCiAgZXhpdCAxCmZpCgppZiBzd2Fwb24gLS1zaG93PU5BTUUgfCBncmVwIC1xICIkREVWSUNFIjsgdGhlbgogIGVjaG8gIlN3YXAgYWxyZWFkeSBlbmFibGVkIG9uICRERVZJQ0UiID4mMgogIGV4aXQgMQpmaQoKZWNobyAiRW5hYmxpbmcgc3dhcCBvbiAkREVWSUNFIiA+JjIKc3dhcG9uICIkREVWSUNFIgp0b3VjaCAvdmFyL3RtcC9zd2FwZmlsZQ==
              mode: 493
              overwrite: true
              path: /etc/find-swap-partition
            - contents:
                source: data:text/plain;charset=utf-8;base64,YXBpVmVyc2lvbjoga3ViZWxldC5jb25maWcuazhzLmlvL3YxYmV0YTEKa2luZDogS3ViZWxldENvbmZpZ3VyYXRpb24KbWVtb3J5U3dhcDoKICBzd2FwQmVoYXZpb3I6IExpbWl0ZWRTd2FwCg==
              mode: 420
              overwrite: true
              path: /etc/openshift/kubelet.conf.d/90-swap.conf
          systemd:
            units:
            - contents: |
                [Unit]
                Description=Provision and enable swap
                ...
              enabled: true
              name: swap-provision.service
    diff: |
      --- /dev/null
      +++ 90-worker-kubelet-swap (MachineConfig)
      @@ -0,0 +1,53 @@
      +apiVersion: machineconfiguration.openshift.io/v1
      +kind: MachineConfig
      ...

  - name: configure-higher-density
    state: Pending
    impactSeverity: Medium
    message: HyperConverged 'kubevirt-hyperconverged' will be configured with memoryOvercommitPercentage=150 and KSM enabled
    targetRef:
      apiVersion: hco.kubevirt.io/v1beta1
      kind: HyperConverged
      name: kubevirt-hyperconverged
      namespace: openshift-cnv
    managedFields:
    - spec.higherWorkloadDensity.memoryOvercommitPercentage
    - spec.ksmConfiguration
    desiredState:
      apiVersion: hco.kubevirt.io/v1beta1
      kind: HyperConverged
      metadata:
        name: kubevirt-hyperconverged
        namespace: openshift-cnv
      spec:
        higherWorkloadDensity:
          memoryOvercommitPercentage: 150
        ksmConfiguration:
          nodeLabelSelector: {}
    diff: |
      --- kubevirt-hyperconverged (HyperConverged)
      +++ kubevirt-hyperconverged (HyperConverged)
      @@ -34,8 +34,10 @@
           enableMultiArchBootImageImport: false
           persistentReservation: false
         higherWorkloadDensity:
      -    memoryOvercommitPercentage: 100
      +    memoryOvercommitPercentage: 150
         infra: {}
      +  ksmConfiguration:
      +    nodeLabelSelector: {}
         liveMigrationConfig:
           allowAutoConverge: false

  observedGeneration: 4
  operatorVersion: v0.1.31
  sourceSnapshotHash: 3bfa730d47b28869339b5450c6134d213df022650d29d3f12dfa834be832fc2f
```

**Key Differences from LoadAware Profile:**

**Item 1: Kubelet Swap Configuration**
- Creates MachineConfig to enable kubelet swap with `LimitedSwap` behavior
- Provisions swap from partition labeled `CNV_SWAP`
- Restricts swap to system.slice to protect system processes
- Requires node reboots (High impact)

**Item 2: HyperConverged Configuration**
- Configures `memoryOvercommitPercentage: 150` (50% overcommit)
- Enables KSM (Kernel Samepage Merging) on all nodes via empty selector
- Managed fields track HCO drift for automatic convergence

**HCO Integration:**
Like `load-aware-rebalancing`, this profile reads from and writes to the HyperConverged CR. If HCO memory settings are changed externally, drift detection triggers automatic plan regeneration with forced user review.
