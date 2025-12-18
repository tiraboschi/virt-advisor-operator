/*
Copyright 2025 The KubeVirt Authors.

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

package profiles

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevirt/virt-advisor-operator/internal/plan"
)

// MachineConfigPoolGVK is the GroupVersionKind for MachineConfigPool
var MachineConfigPoolGVK = schema.GroupVersionKind{
	Group:   "machineconfiguration.openshift.io",
	Version: "v1",
	Kind:    "MachineConfigPool",
}

// ParseKernelArg splits a kernel argument into key and value.
// Examples:
//   - "psi=1" -> ("psi", "1")
//   - "debug" -> ("debug", "")
func ParseKernelArg(arg string) (key, value string) {
	parts := strings.SplitN(arg, "=", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

// CheckKernelArg checks if a kernel argument exists in the list.
// Implements "last occurrence wins" logic - if the same key appears multiple times
// with different values (e.g., psi=0, psi=1), only the last one counts.
//
// Examples:
//   - CheckKernelArg([]string{"psi=1"}, "psi=1") -> true
//   - CheckKernelArg([]string{"psi=0", "psi=1"}, "psi=1") -> true (last wins)
//   - CheckKernelArg([]string{"psi=1", "psi=0"}, "psi=1") -> false (last is psi=0)
//   - CheckKernelArg([]string{"debug"}, "debug") -> true (exact match)
func CheckKernelArg(kernelArgs []string, arg string) bool {
	targetKey, targetValue := ParseKernelArg(arg)

	// Find last occurrence of this key
	lastValue := ""
	found := false
	for _, karg := range kernelArgs {
		k, v := ParseKernelArg(karg)
		if k == targetKey {
			lastValue = v
			found = true
		}
	}

	if !found {
		return false
	}

	// Last occurrence must match the desired value
	return lastValue == targetValue
}

// AddKernelArg adds a kernel argument if not already present (exact match).
// If the arg already exists exactly, returns the slice unchanged.
// If the arg has a different value (e.g., psi=0 vs psi=1), appends the new one
// (last occurrence wins on merge).
func AddKernelArg(existing []string, arg string) []string {
	// Check if exact match already exists
	for _, karg := range existing {
		if karg == arg {
			return existing // Already present
		}
	}

	// Add the new argument
	return append(existing, arg)
}

// RemoveKernelArg removes all occurrences of a kernel argument.
// Supports both exact match ("psi=1") and prefix match ("psi=" removes psi=0, psi=1, etc.)
func RemoveKernelArg(existing []string, arg string) []string {
	var result []string
	isPrefix := strings.HasSuffix(arg, "=")

	for _, karg := range existing {
		shouldRemove := false
		if isPrefix {
			// Prefix match: remove if karg starts with arg
			shouldRemove = strings.HasPrefix(karg, arg)
		} else {
			// Exact match
			shouldRemove = (karg == arg)
		}

		if !shouldRemove {
			result = append(result, karg)
		}
	}

	return result
}

// GetMachineConfigPoolForRole fetches a MachineConfigPool by role name.
// Role is typically "worker", "master", etc.
func GetMachineConfigPoolForRole(ctx context.Context, c client.Client, role string) (*unstructured.Unstructured, error) {
	pool, err := plan.GetUnstructured(ctx, c, MachineConfigPoolGVK, role, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get MachineConfigPool %s: %w", role, err)
	}
	return pool, nil
}

// GetRenderedMachineConfigName extracts the rendered config name from a pool.
// Returns the value at status.configuration.name which points to the merged/rendered MachineConfig.
func GetRenderedMachineConfigName(pool *unstructured.Unstructured) (string, error) {
	// Path: status.configuration.name
	name, found, err := unstructured.NestedString(pool.Object, "status", "configuration", "name")
	if err != nil {
		return "", fmt.Errorf("failed to extract rendered config name: %w", err)
	}
	if !found || name == "" {
		return "", fmt.Errorf("pool %s has no rendered configuration", pool.GetName())
	}
	return name, nil
}

// GetRenderedMachineConfig fetches the actual rendered/merged MachineConfig for a pool.
// This is the merged configuration that will be applied to nodes.
func GetRenderedMachineConfig(ctx context.Context, c client.Client, pool *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	renderedName, err := GetRenderedMachineConfigName(pool)
	if err != nil {
		return nil, err
	}

	// MachineConfig GVK is already defined in loadaware_profile.go
	// We'll use it from the plan package
	mcGVK := schema.GroupVersionKind{
		Group:   "machineconfiguration.openshift.io",
		Version: "v1",
		Kind:    "MachineConfig",
	}

	renderedMC, err := plan.GetUnstructured(ctx, c, mcGVK, renderedName, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get rendered MachineConfig %s: %w", renderedName, err)
	}

	return renderedMC, nil
}

// GetEffectiveKernelArgs returns the effective kernel arguments from a rendered MachineConfig.
// This is what will actually be applied to nodes after MCO merges all MachineConfigs.
func GetEffectiveKernelArgs(renderedMC *unstructured.Unstructured) ([]string, error) {
	// Path: spec.kernelArguments
	args, found, err := unstructured.NestedStringSlice(renderedMC.Object, "spec", "kernelArguments")
	if err != nil {
		return nil, fmt.Errorf("failed to extract kernel arguments: %w", err)
	}
	if !found {
		// No kernel args is valid - return empty slice
		return []string{}, nil
	}
	return args, nil
}

// ValidateKernelArgInPool validates that a kernel arg is (or will be) effective in a pool.
// This performs effect-based validation by checking the rendered/merged config.
//
// Returns:
//   - isPresent: true if arg is in the current rendered config
//   - willBePresent: (future: could simulate merge; for now same as isPresent)
//   - err: any error fetching pool/rendered config
//
// Usage:
//
//	isPresent, willBePresent, err := ValidateKernelArgInPool(ctx, c, "worker", "psi=1")
//	if err != nil { return err }
//	if isPresent {
//	    // Already effective - no reboot needed
//	} else if willBePresent {
//	    // Will be effective after our change
//	}
func ValidateKernelArgInPool(ctx context.Context, c client.Client, poolName, arg string) (isPresent bool, willBePresent bool, err error) {
	// 1. Fetch the pool
	pool, err := GetMachineConfigPoolForRole(ctx, c, poolName)
	if err != nil {
		return false, false, err
	}

	// 2. Get rendered config
	renderedMC, err := GetRenderedMachineConfig(ctx, c, pool)
	if err != nil {
		return false, false, err
	}

	// 3. Get effective kernel args from rendered config
	effectiveArgs, err := GetEffectiveKernelArgs(renderedMC)
	if err != nil {
		return false, false, err
	}

	// 4. Check if arg is present (last-occurrence logic)
	isPresent = CheckKernelArg(effectiveArgs, arg)

	// willBePresent is the same as isPresent for read-only validation
	// In a more advanced version, this could simulate the merge with our desired config
	return isPresent, isPresent, nil
}
