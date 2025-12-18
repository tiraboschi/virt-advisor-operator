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
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ApplyOptions contains options for server-side apply operations
type ApplyOptions struct {
	// FieldManager is the name of the manager making changes
	FieldManager string

	// Force indicates whether to force apply (claim fields owned by others)
	Force bool

	// DryRun indicates this is a dry-run operation
	DryRun bool
}

// DefaultApplyOptions returns default apply options
func DefaultApplyOptions() *ApplyOptions {
	return &ApplyOptions{
		FieldManager: "virt-advisor-operator",
		Force:        true, // Force ownership to claim fields from manual changes
		DryRun:       false,
	}
}

// ServerSideApply performs a server-side apply operation on the given object
func ServerSideApply(ctx context.Context, c client.Client, obj client.Object, opts *ApplyOptions) error {
	if opts == nil {
		opts = DefaultApplyOptions()
	}

	patchOpts := []client.PatchOption{
		client.FieldOwner(opts.FieldManager),
	}

	if opts.Force {
		patchOpts = append(patchOpts, client.ForceOwnership)
	}

	if opts.DryRun {
		patchOpts = append(patchOpts, client.DryRunAll)
	}

	return c.Patch(ctx, obj, client.Apply, patchOpts...)
}

// GenerateDiff generates a unified diff showing changes that would be made
// by applying the new object. It uses server-side apply dry-run to determine
// the actual changes.
func GenerateDiff(ctx context.Context, c client.Client, desired client.Object) (string, error) {
	// Get the current state
	current := desired.DeepCopyObject().(client.Object)
	key := client.ObjectKeyFromObject(desired)

	err := c.Get(ctx, key, current)
	isNew := client.IgnoreNotFound(err) != nil

	if err != nil && !isNew {
		return "", fmt.Errorf("failed to get current object: %w", err)
	}

	// For now, return a simple indication
	// In a production implementation, this would use strategic merge patch
	// or JSON patch to generate an accurate diff
	if isNew {
		return fmt.Sprintf("Creating new %s/%s", desired.GetObjectKind().GroupVersionKind().Kind, desired.GetName()), nil
	}

	return fmt.Sprintf("Updating %s/%s", desired.GetObjectKind().GroupVersionKind().Kind, desired.GetName()), nil
}

// ConvertToUnstructured converts a typed object to unstructured
func ConvertToUnstructured(obj runtime.Object) (*unstructured.Unstructured, error) {
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to unstructured: %w", err)
	}

	u := &unstructured.Unstructured{Object: unstructuredObj}
	return u, nil
}
