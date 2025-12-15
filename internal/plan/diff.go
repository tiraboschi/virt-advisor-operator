package plan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// GenerateSSADiff generates a unified diff using Server-Side Apply dry-run.
// This leverages the API server's validation, defaulting, and webhook processing
// to show accurate diffs that reflect what will actually be applied.
//
// Algorithm (per VEP):
// 1. Construct Intent: The desired object (passed as parameter)
// 2. Dry-Run Apply: Send PATCH with DryRun=true and FieldManager set
// 3. Sanitize: Strip noise (timestamps, managedFields, resourceVersion, etc.)
// 4. Compute Diff: Calculate unified diff between sanitized(live) and sanitized(dry-run result)
func GenerateSSADiff(ctx context.Context, c client.Client, desired *unstructured.Unstructured, fieldManager string) (string, error) {
	// Step 1: Get the current/live object (if it exists)
	live := &unstructured.Unstructured{}
	live.SetGroupVersionKind(desired.GroupVersionKind())

	key := client.ObjectKey{
		Name:      desired.GetName(),
		Namespace: desired.GetNamespace(),
	}

	err := c.Get(ctx, key, live)
	isNew := errors.IsNotFound(err)

	if err != nil && !isNew {
		return "", fmt.Errorf("failed to get live object: %w", err)
	}

	// Step 2: Perform Server-Side Apply with DryRun=true
	// This returns what the object WILL look like after apply, with all:
	// - API server defaults applied
	// - CEL validation rules checked
	// - Mutating webhooks applied
	// - Validating webhooks checked
	//
	// IMPORTANT: Use Force=true for dry-run to claim field ownership and show
	// what WOULD happen if we forcefully apply. This prevents conflicts with
	// other field managers (like kubectl, ArgoCD, etc.)
	dryRunResult := desired.DeepCopy()

	opts := &ApplyOptions{
		FieldManager: fieldManager,
		Force:        true, // Force ownership for dry-run to see the full result
		DryRun:       true,
	}

	if err := ApplyUnstructured(ctx, c, dryRunResult, opts); err != nil {
		return "", fmt.Errorf("failed to perform dry-run apply: %w", err)
	}

	// Step 3: Sanitize both objects
	// Remove fields that are noise for diff comparison:
	// - metadata.managedFields (SSA bookkeeping)
	// - metadata.resourceVersion (changes on every update)
	// - metadata.generation (changes on spec updates)
	// - metadata.creationTimestamp (only on live)
	// - metadata.uid (only on live)
	// - status (we only manage spec)
	sanitizedLive := sanitizeForDiff(live)
	sanitizedResult := sanitizeForDiff(dryRunResult)

	// Step 4: Generate unified diff
	if isNew {
		// Object doesn't exist - show full creation
		return generateCreationDiff(sanitizedResult)
	}

	return generateUnifiedDiff(sanitizedLive, sanitizedResult, desired.GetName(), desired.GetKind())
}

// sanitizeForDiff removes fields that cause noise in diffs
func sanitizeForDiff(obj *unstructured.Unstructured) *unstructured.Unstructured {
	sanitized := obj.DeepCopy()

	// Remove metadata noise
	unstructured.RemoveNestedField(sanitized.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(sanitized.Object, "metadata", "resourceVersion")
	unstructured.RemoveNestedField(sanitized.Object, "metadata", "generation")
	unstructured.RemoveNestedField(sanitized.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(sanitized.Object, "metadata", "uid")
	unstructured.RemoveNestedField(sanitized.Object, "metadata", "selfLink")

	// Remove status (we only manage spec)
	unstructured.RemoveNestedField(sanitized.Object, "status")

	return sanitized
}

// generateCreationDiff shows a diff for creating a new object
func generateCreationDiff(obj *unstructured.Unstructured) (string, error) {
	// Convert to YAML for readability
	yamlBytes, err := yaml.Marshal(obj.Object)
	if err != nil {
		return "", fmt.Errorf("failed to marshal object to YAML: %w", err)
	}

	lines := strings.Split(string(yamlBytes), "\n")

	// Format as unified diff (all additions)
	var diff strings.Builder
	diff.WriteString("--- /dev/null\n")
	diff.WriteString(fmt.Sprintf("+++ %s (%s)\n", obj.GetName(), obj.GetKind()))
	diff.WriteString(fmt.Sprintf("@@ -0,0 +1,%d @@\n", len(lines)))

	for _, line := range lines {
		if line != "" {
			diff.WriteString("+" + line + "\n")
		}
	}

	return diff.String(), nil
}

// generateUnifiedDiff generates a unified diff between two objects
func generateUnifiedDiff(live, result *unstructured.Unstructured, name, kind string) (string, error) {
	// Convert both to YAML for human-readable diff
	liveYAML, err := yaml.Marshal(live.Object)
	if err != nil {
		return "", fmt.Errorf("failed to marshal live object to YAML: %w", err)
	}

	resultYAML, err := yaml.Marshal(result.Object)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result object to YAML: %w", err)
	}

	// Check if there are any changes
	if string(liveYAML) == string(resultYAML) {
		return fmt.Sprintf("--- %s (%s)\n+++ %s (%s)\n(no changes)\n", name, kind, name, kind), nil
	}

	// Generate unified diff using difflib
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(liveYAML)),
		B:        difflib.SplitLines(string(resultYAML)),
		FromFile: fmt.Sprintf("%s (%s)", name, kind),
		ToFile:   fmt.Sprintf("%s (%s)", name, kind),
		Context:  3,
	}

	text, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		return "", fmt.Errorf("failed to generate unified diff: %w", err)
	}

	return text, nil
}

// GenerateSSADiffForManagedFields generates a diff showing only the fields
// that are managed by this operator (specified in managedFields list).
// This is useful for focused diffs when co-managing resources with other tools.
func GenerateSSADiffForManagedFields(ctx context.Context, c client.Client, desired *unstructured.Unstructured, fieldManager string, managedFields []string) (string, error) {
	// First get the full SSA diff
	fullDiff, err := GenerateSSADiff(ctx, c, desired, fieldManager)
	if err != nil {
		return "", err
	}

	// TODO: Filter the diff to only show managed fields
	// For now, return the full diff
	// In a production implementation, this would parse the diff and only
	// show lines that relate to the managed fields

	return fullDiff, nil
}

// PrettyPrintJSON returns a pretty-printed JSON representation of an object
// Useful for debugging
func PrettyPrintJSON(obj interface{}) (string, error) {
	bytes, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
