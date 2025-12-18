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

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CRDChecker provides utilities for checking CRD existence
type CRDChecker struct {
	client client.Client
}

// NewCRDChecker creates a new CRD checker
func NewCRDChecker(c client.Client) *CRDChecker {
	return &CRDChecker{client: c}
}

// CRDExists checks if a CRD exists in the cluster
func (cc *CRDChecker) CRDExists(ctx context.Context, crdName string) (bool, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	err := cc.client.Get(ctx, client.ObjectKey{Name: crdName}, crd)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check CRD existence: %w", err)
	}
	return true, nil
}

// CheckRequiredCRDs checks if all required CRDs exist
func (cc *CRDChecker) CheckRequiredCRDs(ctx context.Context, crdNames []string) (missing []string, err error) {
	for _, crdName := range crdNames {
		exists, err := cc.CRDExists(ctx, crdName)
		if err != nil {
			return nil, err
		}
		if !exists {
			missing = append(missing, crdName)
		}
	}
	return missing, nil
}

// GetUnstructured fetches a resource as an unstructured object
func GetUnstructured(ctx context.Context, c client.Client, gvk schema.GroupVersionKind, name, namespace string) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	key := client.ObjectKey{Name: name}
	if namespace != "" {
		key.Namespace = namespace
	}

	err := c.Get(ctx, key, obj)
	if err != nil {
		return nil, err
	}

	return obj, nil
}

// ApplyUnstructured applies an unstructured object using server-side apply
func ApplyUnstructured(ctx context.Context, c client.Client, obj *unstructured.Unstructured, opts *ApplyOptions) error {
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

// CreateUnstructured creates a new unstructured object with the specified GVK
func CreateUnstructured(gvk schema.GroupVersionKind, name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName(name)
	if namespace != "" {
		obj.SetNamespace(namespace)
	}
	return obj
}
