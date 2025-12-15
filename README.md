# virt-advisor-operator

An operational governance layer for safe configuration of third-party integrations in KubeVirt environments.

## Description

The virt-advisor-operator implements a declarative "Plan" pattern for managing configuration changes to external operators and cluster components. It provides a preview-and-approve workflow that enables cluster administrators to safely tune third-party components without direct ownership.

### Key Features

- **Preview & Approve Workflow**: DryRun mode calculates and displays proposed changes before applying them
- **Profile-Based Configuration**: Select from predefined capability sets (e.g., "LoadAwareRebalancing")
- **Drift Detection**: Monitors for configuration drift from the intended state
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

# Clean up
kubectl delete -f config/samples/loadaware_sample.yaml
make kind-delete  # Delete the Kind cluster
```

## Available Profiles

### example-profile
A simple demonstration profile showing basic functionality.

### load-aware-rebalancing
Implements the VEP's load-aware rebalancing capability by configuring:
1. **KubeDescheduler**: Enables KubeVirt-aware descheduling profiles with intelligent fallback
2. **MachineConfig**: Enables PSI (Pressure Stall Information) metrics for load awareness

**Profile Selection Logic:**
- **OCP 5.32+**: Uses `KubeVirtRelieveAndMigrate` (GA) with profileCustomizations
- **OCP 5.14-5.21**: Falls back to `DevKubeVirtRelieveAndMigrate` (dev preview)
- **OCP 5.10**: Falls back to `LongLifecycle` (no profileCustomizations)

**Profile Preservation:**
- Preserves `AffinityAndTaints` and `SoftTopologyAndDuplicates` if they exist
- Removes other conflicting profiles

**Supported Config Overrides:**
- `deschedulingIntervalSeconds`: How often to run descheduling (default: 1800 = 30 minutes)
- `enablePSIMetrics`: Whether to enable PSI kernel metrics (default: true)
- `devDeviationThresholds`: Deviation threshold for load balancing (default: AsymmetricLow)
  - Valid values: Low, Medium, High, AsymmetricLow, AsymmetricMedium, AsymmetricHigh
  - Only applies to KubeVirtRelieveAndMigrate and DevKubeVirtRelieveAndMigrate

**Impact:**
- Medium: Creates or updates descheduler configuration
- High: Enables PSI metrics (requires node reboot in real OpenShift clusters)

### To Deploy on the cluster

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
// TODO(user): Add detailed information on how you would like others to contribute to this project

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

