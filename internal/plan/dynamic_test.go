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
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TestCreateUnstructured tests the CreateUnstructured function
func TestCreateUnstructured(t *testing.T) {
	tests := []struct {
		name      string
		gvk       schema.GroupVersionKind
		resName   string
		namespace string
	}{
		{
			name: "Cluster-scoped resource",
			gvk: schema.GroupVersionKind{
				Group:   "operator.openshift.io",
				Version: "v1",
				Kind:    "KubeDescheduler",
			},
			resName:   "cluster",
			namespace: "",
		},
		{
			name: "Namespaced resource",
			gvk: schema.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "ConfigMap",
			},
			resName:   "test-config",
			namespace: "default",
		},
		{
			name: "Custom resource",
			gvk: schema.GroupVersionKind{
				Group:   "advisor.kubevirt.io",
				Version: "v1alpha1",
				Kind:    "VirtPlatformConfig",
			},
			resName:   "my-config",
			namespace: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := CreateUnstructured(tt.gvk, tt.resName, tt.namespace)

			// Verify GVK is set correctly
			if obj.GetAPIVersion() != tt.gvk.GroupVersion().String() {
				t.Errorf("Expected APIVersion %s, got %s",
					tt.gvk.GroupVersion().String(), obj.GetAPIVersion())
			}

			if obj.GetKind() != tt.gvk.Kind {
				t.Errorf("Expected Kind %s, got %s", tt.gvk.Kind, obj.GetKind())
			}

			// Verify name is set
			if obj.GetName() != tt.resName {
				t.Errorf("Expected name %s, got %s", tt.resName, obj.GetName())
			}

			// Verify namespace is set (or not set for cluster-scoped)
			if obj.GetNamespace() != tt.namespace {
				t.Errorf("Expected namespace %q, got %q", tt.namespace, obj.GetNamespace())
			}
		})
	}
}

// TestDefaultApplyOptions tests the DefaultApplyOptions function
func TestDefaultApplyOptions(t *testing.T) {
	opts := DefaultApplyOptions()

	if opts == nil {
		t.Fatal("Expected non-nil ApplyOptions")
	}

	if opts.FieldManager != "virt-advisor-operator" {
		t.Errorf("Expected FieldManager 'virt-advisor-operator', got %q", opts.FieldManager)
	}

	if !opts.Force {
		t.Error("Expected Force to be true by default")
	}

	if opts.DryRun {
		t.Error("Expected DryRun to be false by default")
	}
}

// TestApplyOptions tests the ApplyOptions struct
func TestApplyOptions(t *testing.T) {
	tests := []struct {
		name         string
		opts         *ApplyOptions
		expectForce  bool
		expectDryRun bool
	}{
		{
			name: "Default options",
			opts: &ApplyOptions{
				FieldManager: "test-manager",
				Force:        true,
				DryRun:       false,
			},
			expectForce:  true,
			expectDryRun: false,
		},
		{
			name: "Dry run enabled",
			opts: &ApplyOptions{
				FieldManager: "test-manager",
				Force:        false,
				DryRun:       true,
			},
			expectForce:  false,
			expectDryRun: true,
		},
		{
			name: "Force and dry run both enabled",
			opts: &ApplyOptions{
				FieldManager: "test-manager",
				Force:        true,
				DryRun:       true,
			},
			expectForce:  true,
			expectDryRun: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.opts.Force != tt.expectForce {
				t.Errorf("Expected Force=%v, got %v", tt.expectForce, tt.opts.Force)
			}

			if tt.opts.DryRun != tt.expectDryRun {
				t.Errorf("Expected DryRun=%v, got %v", tt.expectDryRun, tt.opts.DryRun)
			}

			if tt.opts.FieldManager == "" {
				t.Error("FieldManager should not be empty")
			}
		})
	}
}
