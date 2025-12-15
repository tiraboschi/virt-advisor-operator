# Testing virt-advisor-operator on a Local Kind Cluster

This guide explains how to test the virt-advisor-operator on a local Kind (Kubernetes in Docker/Podman) cluster.

## Prerequisites

- **Go** v1.24.6 or later
- **kubectl** v1.11.3 or later
- **Docker** 17.03+ or **Podman** 3.0+
- **Kind** (will be auto-installed by `make dev-setup`)

## Quick Start (One Command)

The fastest way to get started is using the all-in-one setup command:

```bash
# Creates Kind cluster + installs CRDs + sets up mock resources
make dev-setup
```

This command:
1. Creates a Kind cluster named `virt-advisor-dev`
2. Installs the VirtPlatformConfig CRD
3. Installs mock CRDs for KubeDescheduler and MachineConfig
4. Creates baseline resources (a sample KubeDescheduler and MachineConfig)

## Step-by-Step Testing Workflow

### 1. Start the Operator Locally

In one terminal, run the operator in development mode:

```bash
make run
```

This runs the operator outside the cluster but connected to your Kind cluster. You'll see logs as the operator starts up and reconciles resources.

### 2. Create a VirtPlatformConfig (DryRun Mode)

In another terminal, create a VirtPlatformConfig to test the load-aware-rebalancing profile:

```bash
kubectl apply -f config/samples/loadaware_sample.yaml
```

This creates a VirtPlatformConfig with `action: DryRun`, which means:
- The operator calculates the changes
- Generates diffs showing what would be applied
- **Does NOT apply the changes** (waits for approval)

### 3. Watch the Progress

Watch as the operator processes the plan:

```bash
kubectl get virtplatformconfig load-aware-rebalancing -w
```

You should see the phase transition:
- `Pending` → `Drafting` → `ReviewRequired`

Press `Ctrl+C` to stop watching.

### 4. Review the Generated Diffs

View the detailed status with diffs:

```bash
kubectl get virtplatformconfig load-aware-rebalancing -o yaml
```

Look for the `status.items` section. You should see two items:

**Item 1: KubeDescheduler Configuration**
- Shows changes to the descheduler settings
- Default interval: 60 seconds
- Profile: `KubeVirtRelieveAndMigrate` (or appropriate fallback)
- Impact: Low (just configuration update)

**Item 2: MachineConfig for PSI Metrics**
- Creates a new MachineConfig with `psi=1` kernel argument
- Impact: High (would require node reboot in real OpenShift)

Example output structure:
```yaml
status:
  phase: ReviewRequired
  items:
  - name: configure-descheduler
    state: Pending
    targetRef:
      kind: KubeDescheduler
      name: cluster
    impactSeverity: Low
    diff: |
      --- cluster (KubeDescheduler)
      +++ cluster (KubeDescheduler)
      @@ ...
       spec:
      +  deschedulingIntervalSeconds: 60
      +  profiles:
      +  - KubeVirtRelieveAndMigrate

  - name: enable-psi-metrics
    state: Pending
    targetRef:
      kind: MachineConfig
      name: 99-worker-psi-karg
    impactSeverity: High (Reboot Required)
    diff: |
      + apiVersion: machineconfiguration.openshift.io/v1
      + kind: MachineConfig
      + spec:
      +   kernelArguments:
      +   - psi=1
```

### 5. Approve and Apply the Changes

After reviewing the diffs, approve the plan by changing `action` to `Apply`:

```bash
kubectl patch virtplatformconfig load-aware-rebalancing \
  --type='json' -p='[{"op": "replace", "path": "/spec/action", "value":"Apply"}]'
```

Watch the execution:
```bash
kubectl get virtplatformconfig load-aware-rebalancing -w
```

You'll see the phase change:
- `ReviewRequired` → `InProgress` → `Completed`

### 6. Verify the Applied Changes

Check that the resources were updated:

```bash
# View the updated KubeDescheduler
kubectl get kubedescheduler cluster -o yaml

# View the created MachineConfig
kubectl get machineconfig 99-worker-psi-karg -o yaml
```

You should see:
- `deschedulingIntervalSeconds: 60` in the KubeDescheduler
- `psi=1` in the MachineConfig's kernel arguments
- `devEnableEvictionsInBackground: true` (always enabled)

## Testing Advanced Scenarios

### Test Custom Configuration Options

Create a VirtPlatformConfig with custom options:

```bash
kubectl apply -f config/samples/loadaware_typed_sample.yaml
```

This example shows:
- Custom descheduling interval (120 seconds instead of default 60)
- Type-safe configuration with validation
- All available options documented

### Test the DryRun → Apply Workflow

1. Create with `action: DryRun`
2. Review the diff in `status.items`
3. Patch to `action: Apply`
4. Verify execution

### Test Profile Prerequisites

The operator checks if required CRDs exist before generating a plan.

**To test missing CRD detection:**

1. Delete the KubeDescheduler CRD:
   ```bash
   kubectl delete crd kubedeschedulers.operator.openshift.io
   ```

2. Create a new VirtPlatformConfig:
   ```bash
   kubectl apply -f config/samples/loadaware_sample.yaml
   ```

3. Check the status:
   ```bash
   kubectl get virtplatformconfig load-aware-rebalancing -o yaml
   ```

   You should see:
   - `phase: PrerequisiteFailed`
   - Message explaining which CRD is missing

4. Restore the CRD:
   ```bash
   kubectl apply -f config/crd/mocks/descheduler_crd.yaml
   ```

5. The operator should automatically restart (CRD Sentinel) and the plan should progress.

### Test Drift Detection

1. Apply a plan to completion
2. Manually modify the managed resource:
   ```bash
   kubectl patch kubedescheduler cluster --type=merge -p '{"spec":{"deschedulingIntervalSeconds":999}}'
   ```
3. The operator detects the drift and updates the VirtPlatformConfig status
4. Check for drift in the status (future feature)

### Test with PSI Metrics Disabled

Create a plan with PSI metrics disabled:

```yaml
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-no-psi
spec:
  profile: load-aware-rebalancing
  action: DryRun
  options:
    loadAware:
      enablePSIMetrics: false
```

The plan should only include the KubeDescheduler item (no MachineConfig).

## Understanding Mock Resources

The Kind cluster includes mock CRDs that simulate OpenShift resources:

### Mock KubeDescheduler CRD
- Located: `config/crd/mocks/descheduler_crd.yaml`
- Simulates: OpenShift Cluster Descheduler Operator CRD
- Versions: Supports v5.10, v5.14, v5.21, v5.32 profiles
- Purpose: Allows testing descheduler configuration without installing the real operator

### Mock MachineConfig CRD
- Located: `config/crd/mocks/machineconfig_crd.yaml`
- Simulates: OpenShift Machine Config Operator CRD
- Purpose: Allows testing node configuration without requiring OpenShift

### Baseline Resources
- Located: `config/samples/mock_baseline_resources.yaml`
- Creates:
  - A KubeDescheduler named `cluster` with initial config
  - A MachineConfig named `99-worker-psi-karg` (initially empty)

## Checking Operator Logs

The operator runs in the foreground when using `make run`, so you'll see logs directly.

Key log messages to watch for:

```
INFO    Starting manager
INFO    CRD not found at startup, will watch for installation    {"crd": "kubedeschedulers.operator.openshift.io"}
INFO    Starting workers        {"controller": "virtplatformconfig"}
INFO    Reconciling VirtPlatformConfig  {"name": "load-aware-rebalancing"}
INFO    Phase transition        {"from": "Pending", "to": "Drafting"}
INFO    Generated plan items    {"count": 2}
INFO    Phase transition        {"from": "Drafting", "to": "ReviewRequired"}
```

## Cleaning Up

### Delete the VirtPlatformConfig
```bash
kubectl delete virtplatformconfig load-aware-rebalancing
```

### Stop the Operator
In the terminal running `make run`, press `Ctrl+C`.

### Delete the Kind Cluster
```bash
make kind-delete
```

This removes the entire Kind cluster and all resources.

## Troubleshooting

### Cluster Already Exists
If you see "Kind cluster 'virt-advisor-dev' already exists":
```bash
make kind-delete  # Delete the old cluster
make dev-setup    # Create a fresh one
```

### CRDs Not Installing
If CRDs fail to install:
```bash
# Manually verify CRDs
kubectl get crd | grep -E "(virtplatformconfig|kubedescheduler|machineconfig)"

# Reinstall CRDs
make install
./hack/setup-mocks.sh
```

### Operator Not Starting
If the operator fails to start:
1. Check you have the right kubectl context: `kubectl config current-context`
2. Verify the Kind cluster is running: `kind get clusters`
3. Check for port conflicts (operator uses :8081 for health probes)

### Plan Stuck in Pending
If the VirtPlatformConfig stays in `Pending`:
1. Check operator logs for errors
2. Verify CRDs are installed: `kubectl get crd`
3. Check the status for error messages: `kubectl get virtplatformconfig -o yaml`

## Container Tool Selection

The Makefile auto-detects Docker or Podman. To override:

```bash
# Force Docker
make dev-setup CONTAINER_TOOL=docker

# Force Podman
make dev-setup CONTAINER_TOOL=podman
```

Kind works with both Docker and Podman (using `KIND_EXPERIMENTAL_PROVIDER=podman`).

## Running E2E Tests

To run the full end-to-end test suite:

```bash
make test-e2e
```

This:
1. Creates a temporary Kind cluster
2. Builds and loads the operator image
3. Deploys the operator
4. Runs the test suite
5. Cleans up

**Note:** E2E tests take several minutes to complete.

## Next Steps

- Read the [VEP Document](vep.md) for architecture details
- Check the [Load Aware Profile Documentation](LOADAWARE_PROFILE.md)
- Review [Profile Development Guide](profile-development-guide.md) to create your own profiles
- Explore the code in `internal/profiles/` to understand profile implementation
