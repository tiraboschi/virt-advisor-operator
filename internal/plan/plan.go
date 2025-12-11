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

	hcov1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
)

// ComputeCurrentStateHash computes a hash of the current state of resources
// referenced in the configuration plan items by fetching them from the cluster.
// Only hashes the fields specified in ManagedFields to avoid false-positive drift
// detection when unmanaged fields change.
func ComputeCurrentStateHash(ctx context.Context, c client.Client, items []hcov1alpha1.ConfigurationPlanItem) (string, error) {
	// Build a map to store the managed field values of all target resources
	resourceStates := make(map[string]interface{})

	for _, item := range items {
		// Parse the GVK from the item's target ref
		gv, err := schema.ParseGroupVersion(item.TargetRef.APIVersion)
		if err != nil {
			return "", fmt.Errorf("failed to parse APIVersion %s: %w", item.TargetRef.APIVersion, err)
		}

		gvk := schema.GroupVersionKind{
			Group:   gv.Group,
			Version: gv.Version,
			Kind:    item.TargetRef.Kind,
		}

		// Fetch the resource using unstructured
		obj, err := GetUnstructured(ctx, c, gvk, item.TargetRef.Name, item.TargetRef.Namespace)
		if err != nil {
			if errors.IsNotFound(err) {
				// Resource doesn't exist yet - this is expected for new resources being created.
				// Use a sentinel value in the hash so we can detect if someone creates it
				// externally between plan generation and execution.
				key := fmt.Sprintf("%s/%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind, item.TargetRef.Name)
				if item.TargetRef.Namespace != "" {
					key = fmt.Sprintf("%s/%s/%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind, item.TargetRef.Namespace, item.TargetRef.Name)
				}
				resourceStates[key] = map[string]interface{}{"__state": "NOT_FOUND"}
				continue
			}
			// Other errors (permission denied, API server down, etc.) should fail
			return "", fmt.Errorf("failed to get resource %s/%s: %w", item.TargetRef.Kind, item.TargetRef.Name, err)
		}

		// Extract only the managed fields for hashing
		managedState := make(map[string]interface{})
		if len(item.ManagedFields) > 0 {
			// Only hash the specific fields we manage
			for _, fieldPath := range item.ManagedFields {
				// Parse field path (e.g., "spec.profiles" -> ["spec", "profiles"])
				fields := parseFieldPath(fieldPath)
				value, found, err := unstructured.NestedFieldNoCopy(obj.Object, fields...)
				if err != nil {
					return "", fmt.Errorf("failed to extract field %s from %s/%s: %w", fieldPath, item.TargetRef.Kind, item.TargetRef.Name, err)
				}
				if found {
					managedState[fieldPath] = value
				}
			}
		} else {
			// Fallback: if no managed fields specified, hash entire spec (old behavior)
			spec, found, err := unstructured.NestedMap(obj.Object, "spec")
			if err != nil {
				return "", fmt.Errorf("failed to extract spec from %s/%s: %w", item.TargetRef.Kind, item.TargetRef.Name, err)
			}
			if found {
				managedState["spec"] = spec
			}
		}

		// Use a stable key for consistent ordering
		key := fmt.Sprintf("%s/%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind, item.TargetRef.Name)
		if item.TargetRef.Namespace != "" {
			key = fmt.Sprintf("%s/%s/%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind, item.TargetRef.Namespace, item.TargetRef.Name)
		}
		resourceStates[key] = managedState
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
func DetectDrift(ctx context.Context, c client.Client, items []hcov1alpha1.ConfigurationPlanItem, storedHash string) (bool, error) {
	currentHash, err := ComputeCurrentStateHash(ctx, c, items)
	if err != nil {
		return false, err
	}

	// Drift detected if hashes don't match
	return currentHash != storedHash, nil
}
