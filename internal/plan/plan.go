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

package plan

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
)

// buildResourceKey creates a stable key for a resource
func buildResourceKey(gvk schema.GroupVersionKind, name, namespace string) string {
	if namespace != "" {
		return fmt.Sprintf("%s/%s/%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind, namespace, name)
	}
	return fmt.Sprintf("%s/%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind, name)
}

// extractManagedState extracts managed fields from an object for hashing
func extractManagedState(obj *unstructured.Unstructured, item *advisorv1alpha1.VirtPlatformConfigItem) (map[string]interface{}, error) {
	managedState := make(map[string]interface{})

	if len(item.ManagedFields) > 0 {
		// Only hash the specific fields we manage
		for _, fieldPath := range item.ManagedFields {
			fields := parseFieldPath(fieldPath)
			value, found, err := unstructured.NestedFieldNoCopy(obj.Object, fields...)
			if err != nil {
				return nil, fmt.Errorf("failed to extract field %s from %s/%s: %w", fieldPath, item.TargetRef.Kind, item.TargetRef.Name, err)
			}
			if found {
				managedState[fieldPath] = value
			}
		}
	} else {
		// Fallback: if no managed fields specified, hash entire spec
		spec, found, err := unstructured.NestedMap(obj.Object, "spec")
		if err != nil {
			return nil, fmt.Errorf("failed to extract spec from %s/%s: %w", item.TargetRef.Kind, item.TargetRef.Name, err)
		}
		if found {
			managedState["spec"] = spec
		}
	}

	return managedState, nil
}

// fetchResourceForHashing fetches a resource and returns its managed state or a sentinel value if not found
func fetchResourceForHashing(ctx context.Context, c client.Client, item *advisorv1alpha1.VirtPlatformConfigItem, gvk schema.GroupVersionKind) (string, interface{}, error) {
	key := buildResourceKey(gvk, item.TargetRef.Name, item.TargetRef.Namespace)

	obj, err := GetUnstructured(ctx, c, gvk, item.TargetRef.Name, item.TargetRef.Namespace)
	if err != nil {
		if errors.IsNotFound(err) {
			// Resource doesn't exist yet - use sentinel value
			return key, map[string]interface{}{"__state": "NOT_FOUND"}, nil
		}
		return "", nil, fmt.Errorf("failed to get resource %s/%s: %w", item.TargetRef.Kind, item.TargetRef.Name, err)
	}

	managedState, err := extractManagedState(obj, item)
	if err != nil {
		return "", nil, err
	}

	return key, managedState, nil
}

// ComputeCurrentStateHash computes a hash of the current state of resources
// referenced in the configuration plan items by fetching them from the cluster.
// Only hashes the fields specified in ManagedFields to avoid false-positive drift
// detection when unmanaged fields change.
func ComputeCurrentStateHash(ctx context.Context, c client.Client, items []advisorv1alpha1.VirtPlatformConfigItem) (string, error) {
	resourceStates := make(map[string]interface{})

	for _, item := range items {
		gv, err := schema.ParseGroupVersion(item.TargetRef.APIVersion)
		if err != nil {
			return "", fmt.Errorf("failed to parse APIVersion %s: %w", item.TargetRef.APIVersion, err)
		}

		gvk := schema.GroupVersionKind{
			Group:   gv.Group,
			Version: gv.Version,
			Kind:    item.TargetRef.Kind,
		}

		key, state, err := fetchResourceForHashing(ctx, c, &item, gvk)
		if err != nil {
			return "", err
		}

		resourceStates[key] = state
	}

	// Marshal to JSON for consistent hashing
	data, err := json.Marshal(resourceStates)
	if err != nil {
		return "", fmt.Errorf("failed to marshal resource states for hashing: %w", err)
	}

	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash), nil
}

// parseFieldPath converts a dot-separated field path to a slice of field names
// e.g., "spec.profiles" -> ["spec", "profiles"]
func parseFieldPath(path string) []string {
	if path == "" {
		return nil
	}
	fields := []string{}
	for _, field := range splitPath(path) {
		if field != "" {
			fields = append(fields, field)
		}
	}
	return fields
}

// splitPath splits a field path by dots
func splitPath(path string) []string {
	result := []string{}
	current := ""
	for _, ch := range path {
		if ch == '.' {
			result = append(result, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

// DetectDrift compares the current state of resources against the stored snapshot hash.
// Returns true if drift is detected, false if resources match the snapshot.
func DetectDrift(ctx context.Context, c client.Client, items []advisorv1alpha1.VirtPlatformConfigItem, storedHash string) (bool, error) {
	currentHash, err := ComputeCurrentStateHash(ctx, c, items)
	if err != nil {
		return false, err
	}

	// Drift detected if hashes don't match
	return currentHash != storedHash, nil
}
