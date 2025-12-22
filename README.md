# virt-advisor-operator

An operational governance layer for safe configuration of third-party integrations in KubeVirt environments.

## Table of Contents

- [Description](#description)
- [Getting Started](#getting-started)
- [Architecture](#architecture)
- [Available Profiles](#available-profiles)
- [Deployment](#deployment)
- [Contributing](#contributing)

## Description

The virt-advisor-operator implements a declarative "Plan" pattern for managing configuration changes to external operators and cluster components. It provides a preview-and-approve workflow that enables cluster administrators to safely tune third-party components without direct ownership.

### Key Features

- **Preview & Approve Workflow**: Multiple action modes for flexible management
  - `DryRun`: Calculate and display proposed changes before applying
  - `Apply`: Immediately execute the configuration changes
  - `Ignore`: Temporarily pause management without deleting the resource (no drift detection, no metrics, no reconciliation)
- **Profile-Based Configuration**: Select from predefined capability sets (e.g., "LoadAwareRebalancing")
- **Plugin Architecture for Drift Detection**:
  - Each profile declares which resources it manages via `GetManagedResourceTypes()`
  - Controller automatically creates watches for all managed resource types
  - Memory-optimized with predicate filtering to only cache managed resources
  - Adding new profiles automatically registers their watches - no manual configuration needed
- **Drift Detection & Remediation**:
  - Automatically monitors for configuration drift on all managed resources
  - Watches trigger immediate reconciliation when managed resources change
  - Automatically transitions to `Drifted` phase when drift is detected
  - Supports manual remediation workflow (default) or automatic drift correction (aggressive mode)
  - Prevents fighting with other controllers or manual changes
- **Condition Management**: Standard Kubernetes conditions (Drafting, InProgress, Drifted, Completed) for monitoring
- **Optimistic Locking**: Uses snapshot hashing to prevent conflicting changes
- **Granular Control**: Fine-grained failure policies and per-item status tracking
- **Safe Evolution**: Server-side apply ensures accurate diffs and controlled updates

### Use Cases

- Enabling advanced scheduling features (descheduler, load-aware rebalancing)
- Configuring performance tuning profiles
- Managing integration with monitoring and observability tools
- Applying cluster-wide operational policies with preview capabilities

## Getting Started

### Prerequisites
- go version v1.24.6+
- docker version 17.03+ or podman version 3.0+
- kubectl version v1.11.3+
- Access to a Kubernetes v1.11.3+ cluster

### Local Development Setup

For local development and testing, you can use Kind (Kubernetes in Docker/Podman):

```sh
# One-command setup: creates cluster + installs CRDs + sets up mock resources
make dev-setup

# Run the controller locally
make run

# In another terminal, test the LoadAwareRebalancing profile
kubectl apply -f config/samples/loadaware_sample.yaml

# Watch the VirtPlatformConfig progress through phases
kubectl get virtplatformconfig load-aware-rebalancing -w

# View detailed status with diffs
kubectl get virtplatformconfig load-aware-rebalancing -o yaml

# After reviewing the DryRun diff, approve and apply changes
kubectl patch virtplatformconfig load-aware-rebalancing \
  --type='json' -p='[{"op": "replace", "path": "/spec/action", "value":"Apply"}]'

# Verify the changes were applied
kubectl get kubedescheduler cluster -o yaml
kubectl get machineconfig 99-worker-psi-karg -o yaml

# To temporarily pause management (e.g., during troubleshooting)
kubectl patch virtplatformconfig load-aware-rebalancing \
  --type='json' -p='[{"op": "replace", "path": "/spec/action", "value":"Ignore"}]'

# Resume management when ready
kubectl patch virtplatformconfig load-aware-rebalancing \
  --type='json' -p='[{"op": "replace", "path": "/spec/action", "value":"DryRun"}]'

# Clean up
kubectl delete -f config/samples/loadaware_sample.yaml
make kind-delete  # Delete the Kind cluster
```

**For a complete workflow walkthrough**, see [Workflow Example](docs/WORKFLOW_EXAMPLE.md) which provides a real-world example covering:
- Profile discovery and initial setup
- DryRun preview with prerequisite validation
- Plan review with detailed diffs
- Applying configurations and monitoring progress
- Drift detection and resolution workflows
- Complete lifecycle from Ignored → Completed phases

**For a comprehensive testing guide**, see [Testing on Kind](docs/TESTING_ON_KIND.md) which covers:
- Step-by-step testing workflow with detailed explanations
- Testing custom configuration options
- Testing prerequisite checks and CRD sentinel behavior
- Testing drift detection
- Troubleshooting common issues


## Architecture

### Profile-Based Design

The operator uses a **profile-based architecture** where each profile is a self-contained module that manages specific configurations:

```
internal/profiles/
├── profiles.go              # Central registry
├── profileutils/            # Shared utilities (GVKs, helpers, builders)
├── loadaware/              # Load-aware rebalancing profile
├── higherdensity/          # Higher-density profile
└── example/                # Example profile for testing
```

**Key design principles:**
- **One profile per subdirectory** - Enables CODEOWNERS protection for team-specific profiles
- **Automatic drift detection** - Each profile declares managed resources; controller watches them automatically
- **Server-Side Apply** - All diffs are generated through SSA for accuracy and validation
- **Plugin architecture** - Adding new profiles requires no controller changes

👉 **For profile developers**: See [Profile Development Guide](docs/profile-development-guide.md)

## Available Profiles

### 🧪 example-profile
**Purpose**: Simple demonstration profile showing basic functionality
**Use for**: Testing and learning the operator workflow

### ⚖️ load-aware-rebalancing

**Purpose**: Implements load-aware VM rebalancing to prevent hotspots and improve cluster utilization

**What it configures:**
- **KubeDescheduler**: Enables KubeVirt-aware descheduling with intelligent version fallback
- **MachineConfig** (optional): Enables PSI (Pressure Stall Information) kernel metrics

**Configuration options:**
| Option | Default | Description |
|--------|---------|-------------|
| `deschedulingIntervalSeconds` | `60` | How often to run descheduling (60-86400 seconds) |
| `mode` | `Automatic` | When to run: `Automatic` (continuous) or `Predictive` (future: ML-based) |
| `enablePSIMetrics` | `true` | Enable kernel PSI metrics for load awareness |
| `devDeviationThresholds` | `AsymmetricLow` | Balancing sensitivity: `Low`, `Medium`, `High`, `AsymmetricLow/Medium/High` |

**Version compatibility:**
- **OCP 5.32+**: `KubeVirtRelieveAndMigrate` (GA)
- **OCP 5.14-5.21**: `DevKubeVirtRelieveAndMigrate` (dev preview)
- **OCP 5.10**: `LongLifecycle` fallback

**Impact:**
- 🟡 **Medium**: Descheduler configuration changes (no reboot)
- 🔴 **High**: PSI metrics enablement (requires node reboot)

---

### 📦 virt-higher-density

**Purpose**: Enables higher VM density through KSM, memory overcommitment, and kubelet swap

**What it configures:**
- **HyperConverged (HCO)**: Configures virtualization density features
  - **KSM (Kernel Samepage Merging)**: Memory deduplication across VMs
  - **Memory Overcommit**: Allows VMs to see more memory than pod requests
  - **Drift Detection**: Monitors HCO for external changes and auto-converges
- **MachineConfig**: Deploys swap configuration for worker nodes
  - Provisions swap from partition labeled `CNV_SWAP`
  - Configures kubelet with `LimitedSwap` behavior
  - Sets up systemd units for swap management
  - Restricts swap usage for system processes

**Configuration options:**
| Option | Default | Description |
|--------|---------|-------------|
| `enableSwap` | `true` | Enable kubelet swap on worker nodes |
| `ksmConfiguration.nodeLabelSelector` | `{}` (all nodes) | Target specific nodes for KSM. Omit to disable KSM entirely |
| `memoryToRequestRatio` | `150` | Memory overcommit percentage (100-300). Values >120 require `enableSwap=true` |

**HCO Integration & Drift Detection:**
- Profile reads from and writes to HyperConverged CR
- Automatically detects external HCO changes (e.g., admin modifying memory settings)
- Triggers plan regeneration with forced user review
- User overrides take precedence over HCO-derived defaults
- Auto-resolves if external changes are reverted

**Prerequisites:**
- **HyperConverged CRD**: OpenShift Virtualization must be installed
- **MachineConfig CRD**: OpenShift only (not available on vanilla Kubernetes)
- **Swap Partition**: Nodes must have partition labeled `CNV_SWAP` if `enableSwap=true`

**Impact:**
- 🟡 **Medium**: HCO configuration changes (KSM and memory overcommit, no reboot)
- 🔴 **High**: MachineConfig deployment (requires node reboots)

**Example usage:**

**Basic configuration (all defaults):**
```yaml
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: virt-higher-density
spec:
  profile: virt-higher-density
  action: DryRun
  # Uses defaults: swap enabled, KSM on all nodes, 150% memory overcommit
```

**Maximum density with KSM on all nodes:**
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
        nodeLabelSelector: {}  # Empty selector = all nodes
      memoryToRequestRatio: 200  # High overcommit (requires swap)
```

**KSM on specific nodes only:**
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

**Higher density without swap (safe maximum):**
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
      enableSwap: false
      memoryToRequestRatio: 120  # Max without swap (CEL validation)
      # ksmConfiguration omitted = KSM disabled
```

**📖 For complete documentation**, see [VirtHigherDensity Profile Guide](docs/VIRTHIGHERDENSITY_PROFILE.md) covering:
- KSM and memory overcommit configuration
- CEL validation rules
- Drift detection and auto-convergence workflows
- Swap partition setup
- Troubleshooting and rollback procedures

## Deployment

### OpenShift Cluster Deployment

**For production OpenShift deployments**, see the comprehensive [OpenShift Deployment Guide](docs/OPENSHIFT_DEPLOYMENT.md) which covers:
- Prerequisites and required operators
- Building and pushing images
- Installing and deploying the operator
- Safe workflow with DryRun → Review → Apply
- Understanding MachineConfig impact (node reboots)
- Monitoring rollouts and troubleshooting
- Production best practices and rollback procedures

### Generic Kubernetes Cluster Deployment

**Container Tool Support**

The Makefile supports both Docker and Podman. It will auto-detect which tool is available (preferring Podman if both are installed). You can override this by setting the `CONTAINER_TOOL` environment variable:

```sh
# Auto-detect (uses podman if available, otherwise docker)
make docker-build

# Force use of docker
make docker-build CONTAINER_TOOL=docker

# Force use of podman
make docker-build CONTAINER_TOOL=podman
```

**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/virt-advisor-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don't work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/virt-advisor-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/virt-advisor-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/virt-advisor-operator/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v2-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing

### Developing New Profiles

To create a new profile for managing additional cluster configurations:

1. **Read the guide**: See [Profile Development Guide](docs/profile-development-guide.md) for complete instructions
2. **Create subdirectory**: `internal/profiles/myprofile/`
3. **Implement interface**: Profile with `GeneratePlanItems()`, `GetPrerequisites()`, etc.
4. **Add tests**: Integration tests in the same subdirectory
5. **Register**: Add to `internal/profiles/profiles.go` init function
6. **Protect with CODEOWNERS**: Add your team to `.github/CODEOWNERS`

**Key principles:**
- Use `profileutils.NewPlanItemBuilder()` for generating plan items (never manual diffs)
- Each profile is self-contained in its own package
- Server-Side Apply ensures accurate diffs with API validation
- Integration tests use envtest with real Kubernetes API server

### General Contributions

We welcome contributions! Areas where help is needed:
- Additional profiles for KubeVirt/OpenShift integrations
- Enhanced drift detection capabilities
- Documentation improvements
- Test coverage expansion

**Development workflow:**
```sh
# Set up local environment
make dev-setup

# Run tests
make test

# Run e2e tests (requires Kind cluster)
make test-e2e

# Run the operator locally
make run
```

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

