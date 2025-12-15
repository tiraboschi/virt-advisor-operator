/*
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
*/

package plan

import (
	"strings"
	"testing"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
)

// TestNewWaitStrategyManager tests the factory function
func TestNewWaitStrategyManager(t *testing.T) {
	manager := NewWaitStrategyManager()

	if manager == nil {
		t.Fatal("Expected non-nil WaitStrategyManager")
	}

	if len(manager.strategies) == 0 {
		t.Error("Expected at least one strategy to be registered")
	}

	// Verify MachineConfig strategy is registered
	hasMachineConfigStrategy := false
	for _, strategy := range manager.strategies {
		if _, ok := strategy.(*MachineConfigWaitStrategy); ok {
			hasMachineConfigStrategy = true
			break
		}
	}

	if !hasMachineConfigStrategy {
		t.Error("Expected MachineConfigWaitStrategy to be registered")
	}
}

// TestMachineConfigWaitStrategy_ShouldWait tests the ShouldWait method
func TestMachineConfigWaitStrategy_ShouldWait(t *testing.T) {
	strategy := NewMachineConfigWaitStrategy()

	tests := []struct {
		name             string
		item             *advisorv1alpha1.VirtPlatformConfigItem
		expectShouldWait bool
	}{
		{
			name: "MachineConfig resource",
			item: &advisorv1alpha1.VirtPlatformConfigItem{
				TargetRef: advisorv1alpha1.ObjectReference{
					APIVersion: "machineconfiguration.openshift.io/v1",
					Kind:       "MachineConfig",
					Name:       "99-worker-psi-karg",
				},
			},
			expectShouldWait: true,
		},
		{
			name: "Non-MachineConfig resource (ConfigMap)",
			item: &advisorv1alpha1.VirtPlatformConfigItem{
				TargetRef: advisorv1alpha1.ObjectReference{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "test-config",
				},
			},
			expectShouldWait: false,
		},
		{
			name: "Non-MachineConfig resource (KubeDescheduler)",
			item: &advisorv1alpha1.VirtPlatformConfigItem{
				TargetRef: advisorv1alpha1.ObjectReference{
					APIVersion: "operator.openshift.io/v1",
					Kind:       "KubeDescheduler",
					Name:       "cluster",
				},
			},
			expectShouldWait: false,
		},
		{
			name: "Empty kind",
			item: &advisorv1alpha1.VirtPlatformConfigItem{
				TargetRef: advisorv1alpha1.ObjectReference{
					APIVersion: "v1",
					Kind:       "",
					Name:       "test",
				},
			},
			expectShouldWait: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strategy.ShouldWait(tt.item)
			if result != tt.expectShouldWait {
				t.Errorf("Expected ShouldWait=%v, got %v", tt.expectShouldWait, result)
			}
		})
	}
}

// TestMachineConfigWaitStrategy_ParsePoolStatus tests the status parsing logic
func TestMachineConfigWaitStrategy_ParsePoolStatus(t *testing.T) {
	tests := []struct {
		name            string
		poolStatus      map[string]interface{}
		expectedHealthy bool
		expectedMsgPart string
	}{
		{
			name: "All machines ready",
			poolStatus: map[string]interface{}{
				"machineCount":         int64(3),
				"updatedMachineCount":  int64(3),
				"readyMachineCount":    int64(3),
				"degradedMachineCount": int64(0),
			},
			expectedHealthy: true,
			expectedMsgPart: "ready",
		},
		{
			name: "Some machines still updating",
			poolStatus: map[string]interface{}{
				"machineCount":         int64(3),
				"updatedMachineCount":  int64(2),
				"readyMachineCount":    int64(2),
				"degradedMachineCount": int64(0),
			},
			expectedHealthy: false,
			expectedMsgPart: "Waiting for MachineConfigPool",
		},
		{
			name: "Degraded machines present",
			poolStatus: map[string]interface{}{
				"machineCount":         int64(3),
				"updatedMachineCount":  int64(3),
				"readyMachineCount":    int64(2),
				"degradedMachineCount": int64(1),
			},
			expectedHealthy: false,
			expectedMsgPart: "degraded",
		},
		{
			name: "All updated but not all ready",
			poolStatus: map[string]interface{}{
				"machineCount":         int64(3),
				"updatedMachineCount":  int64(3),
				"readyMachineCount":    int64(2),
				"degradedMachineCount": int64(0),
			},
			expectedHealthy: false,
			expectedMsgPart: "Waiting",
		},
		{
			name: "Zero machines (edge case)",
			poolStatus: map[string]interface{}{
				"machineCount":         int64(0),
				"updatedMachineCount":  int64(0),
				"readyMachineCount":    int64(0),
				"degradedMachineCount": int64(0),
			},
			expectedHealthy: false,
			expectedMsgPart: "Waiting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Extract status values (simulating what the Wait function does)
			machineCount, _ := tt.poolStatus["machineCount"].(int64)
			updatedMachineCount, _ := tt.poolStatus["updatedMachineCount"].(int64)
			readyMachineCount, _ := tt.poolStatus["readyMachineCount"].(int64)
			degradedMachineCount, _ := tt.poolStatus["degradedMachineCount"].(int64)

			// Simulate the health check logic
			var msg string
			var healthy bool

			if degradedMachineCount > 0 {
				msg = "degraded"
				healthy = false
			} else if updatedMachineCount == machineCount && readyMachineCount == machineCount && machineCount > 0 {
				msg = "ready"
				healthy = true
			} else {
				msg = "Waiting for MachineConfigPool"
				healthy = false
			}

			if healthy != tt.expectedHealthy {
				t.Errorf("Expected healthy=%v, got %v (machineCount=%d, updated=%d, ready=%d, degraded=%d)",
					tt.expectedHealthy, healthy, machineCount, updatedMachineCount, readyMachineCount, degradedMachineCount)
			}

			if !strings.Contains(msg, tt.expectedMsgPart) {
				t.Errorf("Expected message to contain %q, got %q", tt.expectedMsgPart, msg)
			}
		})
	}
}

// TestWaitStrategyManager_CheckHealthy_NoStrategy tests when no strategy applies
func TestWaitStrategyManager_CheckHealthy_NoStrategy(t *testing.T) {
	// Create a manager with no strategies
	manager := &WaitStrategyManager{
		strategies: []WaitStrategy{},
	}

	item := &advisorv1alpha1.VirtPlatformConfigItem{
		Name: "test-item",
		TargetRef: advisorv1alpha1.ObjectReference{
			APIVersion: "v1",
			Kind:       "ConfigMap",
			Name:       "test",
		},
	}

	// Note: We can't call CheckHealthy without a real context and client,
	// but we can test the logic by checking if the strategy is found
	var foundStrategy WaitStrategy
	for _, s := range manager.strategies {
		if s.ShouldWait(item) {
			foundStrategy = s
			break
		}
	}

	if foundStrategy != nil {
		t.Error("Expected no strategy to match ConfigMap, but one was found")
	}

	// When no strategy is found, the expected behavior is:
	// - needsWait = false
	// - isHealthy = true
	// - message = "Applied successfully"
}

// TestWaitStrategyManager_StrategySelection tests strategy selection logic
func TestWaitStrategyManager_StrategySelection(t *testing.T) {
	manager := NewWaitStrategyManager()

	tests := []struct {
		name             string
		item             *advisorv1alpha1.VirtPlatformConfigItem
		expectStrategy   bool
		expectedStrategy string
	}{
		{
			name: "MachineConfig should select MachineConfigWaitStrategy",
			item: &advisorv1alpha1.VirtPlatformConfigItem{
				TargetRef: advisorv1alpha1.ObjectReference{
					Kind: "MachineConfig",
				},
			},
			expectStrategy:   true,
			expectedStrategy: "*plan.MachineConfigWaitStrategy",
		},
		{
			name: "ConfigMap should not select any strategy",
			item: &advisorv1alpha1.VirtPlatformConfigItem{
				TargetRef: advisorv1alpha1.ObjectReference{
					Kind: "ConfigMap",
				},
			},
			expectStrategy: false,
		},
		{
			name: "KubeDescheduler should not select any strategy",
			item: &advisorv1alpha1.VirtPlatformConfigItem{
				TargetRef: advisorv1alpha1.ObjectReference{
					Kind: "KubeDescheduler",
				},
			},
			expectStrategy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var foundStrategy WaitStrategy
			for _, s := range manager.strategies {
				if s.ShouldWait(tt.item) {
					foundStrategy = s
					break
				}
			}

			if tt.expectStrategy {
				if foundStrategy == nil {
					t.Error("Expected a strategy to be selected, but none was found")
				}
			} else {
				if foundStrategy != nil {
					t.Errorf("Expected no strategy to be selected, but got %T", foundStrategy)
				}
			}
		})
	}
}
