package plan

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	hcov1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
)

// WaitStrategy defines how to wait for a resource to become healthy after applying changes.
// Different resource types require different wait strategies (e.g., MachineConfig waits for
// MachineConfigPool, Deployment waits for replicas ready, etc.)
type WaitStrategy interface {
	// ShouldWait returns true if this strategy applies to the given item
	ShouldWait(item *hcov1alpha1.ConfigurationPlanItem) bool

	// Wait polls the resource and related workloads until healthy or timeout
	// Returns:
	// - progressMessage: Human-readable status for item.message field
	// - isHealthy: true if resource is healthy and ready
	// - err: error if something went wrong (not timeout)
	Wait(ctx context.Context, c client.Client, item *hcov1alpha1.ConfigurationPlanItem) (progressMessage string, isHealthy bool, err error)
}

// WaitStrategyManager coordinates multiple wait strategies
type WaitStrategyManager struct {
	strategies []WaitStrategy
}

// NewWaitStrategyManager creates a manager with default strategies
func NewWaitStrategyManager() *WaitStrategyManager {
	return &WaitStrategyManager{
		strategies: []WaitStrategy{
			NewMachineConfigWaitStrategy(),
			// Future strategies:
			// NewDeploymentWaitStrategy(),
			// NewDaemonSetWaitStrategy(),
		},
	}
}

// CheckHealthy performs a single health check for a resource without blocking.
// This is designed to be called on each reconciliation loop, allowing the controller
// to update status and requeue until the resource becomes healthy.
//
// Returns:
//   - progressMessage: Human-readable status for item.message field
//   - isHealthy: true if resource is healthy and ready
//   - needsWait: true if this resource requires health monitoring
//   - err: error if something went wrong during health check
func (m *WaitStrategyManager) CheckHealthy(ctx context.Context, c client.Client, item *hcov1alpha1.ConfigurationPlanItem) (progressMessage string, isHealthy bool, needsWait bool, err error) {
	// Find applicable strategy
	var strategy WaitStrategy
	for _, s := range m.strategies {
		if s.ShouldWait(item) {
			strategy = s
			break
		}
	}

	// No strategy applies - resource doesn't need waiting
	if strategy == nil {
		return "Applied successfully", true, false, nil
	}

	// Perform single health check
	logger := log.FromContext(ctx)
	msg, healthy, err := strategy.Wait(ctx, c, item)
	if err != nil {
		return "", false, true, fmt.Errorf("failed to check resource health: %w", err)
	}

	logger.V(1).Info("Health check status", "item", item.Name, "healthy", healthy, "message", msg)

	return msg, healthy, true, nil
}

// MachineConfigWaitStrategy waits for MachineConfigPool to become ready after applying MachineConfig
type MachineConfigWaitStrategy struct {
	machineConfigPoolGVK schema.GroupVersionKind
}

// NewMachineConfigWaitStrategy creates a strategy for MachineConfig resources
func NewMachineConfigWaitStrategy() *MachineConfigWaitStrategy {
	return &MachineConfigWaitStrategy{
		machineConfigPoolGVK: schema.GroupVersionKind{
			Group:   "machineconfiguration.openshift.io",
			Version: "v1",
			Kind:    "MachineConfigPool",
		},
	}
}

// ShouldWait returns true for MachineConfig resources
func (s *MachineConfigWaitStrategy) ShouldWait(item *hcov1alpha1.ConfigurationPlanItem) bool {
	return item.TargetRef.Kind == "MachineConfig"
}

// Wait polls the MachineConfigPool status until all nodes are updated and ready
func (s *MachineConfigWaitStrategy) Wait(ctx context.Context, c client.Client, item *hcov1alpha1.ConfigurationPlanItem) (string, bool, error) {
	// Determine which MachineConfigPool to watch
	// MachineConfigs have labels like "machineconfiguration.openshift.io/role: worker"
	// which map to MachineConfigPool names
	poolName := s.getMachineConfigPoolName(ctx, c, item)

	// Fetch the MachineConfigPool
	mcp, err := GetUnstructured(ctx, c, s.machineConfigPoolGVK, poolName, "")
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Sprintf("MachineConfigPool '%s' not found", poolName), false, nil
		}
		return "", false, fmt.Errorf("failed to get MachineConfigPool: %w", err)
	}

	// Extract status fields
	status, found, err := unstructured.NestedMap(mcp.Object, "status")
	if err != nil || !found {
		return "MachineConfigPool status not available", false, nil
	}

	// Parse status fields
	machineCount, _, _ := unstructured.NestedInt64(status, "machineCount")
	updatedMachineCount, _, _ := unstructured.NestedInt64(status, "updatedMachineCount")
	readyMachineCount, _, _ := unstructured.NestedInt64(status, "readyMachineCount")
	degradedMachineCount, _, _ := unstructured.NestedInt64(status, "degradedMachineCount")

	// Check if degraded
	if degradedMachineCount > 0 {
		return fmt.Sprintf("MachineConfigPool '%s' is degraded (%d degraded machines)", poolName, degradedMachineCount), false, nil
	}

	// Check if all machines are updated and ready
	if updatedMachineCount == machineCount && readyMachineCount == machineCount && machineCount > 0 {
		return fmt.Sprintf("MachineConfigPool '%s' is ready (all %d nodes updated)", poolName, machineCount), true, nil
	}

	// Still updating
	return fmt.Sprintf("Waiting for MachineConfigPool '%s' to stabilize (Updated: %d/%d, Ready: %d/%d)",
		poolName, updatedMachineCount, machineCount, readyMachineCount, machineCount), false, nil
}

// getMachineConfigPoolName extracts the pool name from MachineConfig labels
// Typically from label "machineconfiguration.openshift.io/role: worker" -> "worker"
func (s *MachineConfigWaitStrategy) getMachineConfigPoolName(ctx context.Context, c client.Client, item *hcov1alpha1.ConfigurationPlanItem) string {
	// Construct the MachineConfig GVK
	mcGVK := schema.GroupVersionKind{
		Group:   "machineconfiguration.openshift.io",
		Version: "v1",
		Kind:    "MachineConfig",
	}

	// Fetch the MachineConfig to inspect its labels
	mc, err := GetUnstructured(ctx, c, mcGVK, item.TargetRef.Name, item.TargetRef.Namespace)
	if err != nil {
		// If we can't fetch it, default to "worker"
		log.FromContext(ctx).V(1).Info("Could not fetch MachineConfig, defaulting to worker pool", "error", err)
		return "worker"
	}

	// Extract the role label
	labels := mc.GetLabels()
	if labels == nil {
		return "worker"
	}

	// The label is "machineconfiguration.openshift.io/role"
	role, ok := labels["machineconfiguration.openshift.io/role"]
	if !ok || role == "" {
		return "worker"
	}

	return role
}
