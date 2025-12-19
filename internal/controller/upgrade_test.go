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

package controller

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
)

// Unit tests for upgrade detection plan comparison logic
// These specs are picked up by the main TestControllers suite in suite_test.go

var _ = Describe("Plan Item Comparison", func() {
	Describe("planItemsDiffer", func() {
		It("should detect no difference when items are identical", func() {
			items1 := []advisorv1alpha1.VirtPlatformConfigItem{
				{
					Name: "test-item",
					TargetRef: advisorv1alpha1.ObjectReference{
						APIVersion: "v1",
						Kind:       "ConfigMap",
						Name:       "test-cm",
					},
					ImpactSeverity: advisorv1alpha1.ImpactMedium,
					DesiredState: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test-cm"},"data":{"key":"value"}}`),
					},
					ManagedFields: []string{"data.key"},
				},
			}

			items2 := []advisorv1alpha1.VirtPlatformConfigItem{
				{
					Name: "test-item",
					TargetRef: advisorv1alpha1.ObjectReference{
						APIVersion: "v1",
						Kind:       "ConfigMap",
						Name:       "test-cm",
					},
					ImpactSeverity: advisorv1alpha1.ImpactMedium,
					DesiredState: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test-cm"},"data":{"key":"value"}}`),
					},
					ManagedFields: []string{"data.key"},
				},
			}

			Expect(planItemsDiffer(items1, items2)).To(BeFalse())
		})

		It("should detect difference in number of items", func() {
			items1 := []advisorv1alpha1.VirtPlatformConfigItem{
				{Name: "item1"},
				{Name: "item2"},
			}

			items2 := []advisorv1alpha1.VirtPlatformConfigItem{
				{Name: "item1"},
			}

			Expect(planItemsDiffer(items1, items2)).To(BeTrue())
		})

		It("should detect new item added", func() {
			items1 := []advisorv1alpha1.VirtPlatformConfigItem{
				{Name: "item1"},
			}

			items2 := []advisorv1alpha1.VirtPlatformConfigItem{
				{Name: "item1"},
				{Name: "item2"},
			}

			Expect(planItemsDiffer(items1, items2)).To(BeTrue())
		})

		It("should detect difference in desired state", func() {
			items1 := []advisorv1alpha1.VirtPlatformConfigItem{
				{
					Name: "test-item",
					DesiredState: &runtime.RawExtension{
						Raw: []byte(`{"data":{"key":"value1"}}`),
					},
				},
			}

			items2 := []advisorv1alpha1.VirtPlatformConfigItem{
				{
					Name: "test-item",
					DesiredState: &runtime.RawExtension{
						Raw: []byte(`{"data":{"key":"value2"}}`),
					},
				},
			}

			Expect(planItemsDiffer(items1, items2)).To(BeTrue())
		})

		It("should ignore execution state differences (State, Message, LastTransitionTime)", func() {
			items1 := []advisorv1alpha1.VirtPlatformConfigItem{
				{
					Name:    "test-item",
					State:   advisorv1alpha1.ItemStateCompleted,
					Message: "Completed successfully",
					DesiredState: &runtime.RawExtension{
						Raw: []byte(`{"data":{"key":"value"}}`),
					},
				},
			}

			items2 := []advisorv1alpha1.VirtPlatformConfigItem{
				{
					Name:    "test-item",
					State:   advisorv1alpha1.ItemStatePending,
					Message: "Pending execution",
					DesiredState: &runtime.RawExtension{
						Raw: []byte(`{"data":{"key":"value"}}`),
					},
				},
			}

			// Should be identical because only execution state differs
			Expect(planItemsDiffer(items1, items2)).To(BeFalse())
		})
	})

	Describe("itemConfigDiffers", func() {
		It("should detect no difference when configs are identical", func() {
			item1 := &advisorv1alpha1.VirtPlatformConfigItem{
				Name: "test",
				TargetRef: advisorv1alpha1.ObjectReference{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "test-cm",
				},
				ImpactSeverity: advisorv1alpha1.ImpactHigh,
				DesiredState: &runtime.RawExtension{
					Raw: []byte(`{"data":{"key":"value"}}`),
				},
				ManagedFields: []string{"data.key"},
			}

			item2 := &advisorv1alpha1.VirtPlatformConfigItem{
				Name: "test",
				TargetRef: advisorv1alpha1.ObjectReference{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "test-cm",
				},
				ImpactSeverity: advisorv1alpha1.ImpactHigh,
				DesiredState: &runtime.RawExtension{
					Raw: []byte(`{"data":{"key":"value"}}`),
				},
				ManagedFields: []string{"data.key"},
			}

			Expect(itemConfigDiffers(item1, item2)).To(BeFalse())
		})

		It("should detect difference in target reference", func() {
			item1 := &advisorv1alpha1.VirtPlatformConfigItem{
				TargetRef: advisorv1alpha1.ObjectReference{
					Kind: "ConfigMap",
					Name: "cm1",
				},
			}

			item2 := &advisorv1alpha1.VirtPlatformConfigItem{
				TargetRef: advisorv1alpha1.ObjectReference{
					Kind: "ConfigMap",
					Name: "cm2",
				},
			}

			Expect(itemConfigDiffers(item1, item2)).To(BeTrue())
		})

		It("should detect difference in impact severity", func() {
			item1 := &advisorv1alpha1.VirtPlatformConfigItem{
				ImpactSeverity: advisorv1alpha1.ImpactLow,
			}

			item2 := &advisorv1alpha1.VirtPlatformConfigItem{
				ImpactSeverity: advisorv1alpha1.ImpactHigh,
			}

			Expect(itemConfigDiffers(item1, item2)).To(BeTrue())
		})

		It("should detect difference in desired state JSON", func() {
			item1 := &advisorv1alpha1.VirtPlatformConfigItem{
				DesiredState: &runtime.RawExtension{
					Raw: []byte(`{"spec":{"replicas":3}}`),
				},
			}

			item2 := &advisorv1alpha1.VirtPlatformConfigItem{
				DesiredState: &runtime.RawExtension{
					Raw: []byte(`{"spec":{"replicas":5}}`),
				},
			}

			Expect(itemConfigDiffers(item1, item2)).To(BeTrue())
		})

		It("should detect difference when one has desired state and other doesn't", func() {
			item1 := &advisorv1alpha1.VirtPlatformConfigItem{
				DesiredState: &runtime.RawExtension{
					Raw: []byte(`{"data":{}}`),
				},
			}

			item2 := &advisorv1alpha1.VirtPlatformConfigItem{
				DesiredState: nil,
			}

			Expect(itemConfigDiffers(item1, item2)).To(BeTrue())
		})

		It("should detect difference in managed fields", func() {
			item1 := &advisorv1alpha1.VirtPlatformConfigItem{
				ManagedFields: []string{"spec.replicas", "spec.template"},
			}

			item2 := &advisorv1alpha1.VirtPlatformConfigItem{
				ManagedFields: []string{"spec.replicas"},
			}

			Expect(itemConfigDiffers(item1, item2)).To(BeTrue())
		})

		It("should handle complex nested JSON in desired state", func() {
			complexJSON1 := map[string]interface{}{
				"spec": map[string]interface{}{
					"profileCustomizations": []map[string]interface{}{
						{
							"name": "LoadAware",
							"args": map[string]interface{}{
								"evictionLimits": map[string]int{
									"total": 5,
									"node":  2,
								},
							},
						},
					},
				},
			}

			complexJSON2 := map[string]interface{}{
				"spec": map[string]interface{}{
					"profileCustomizations": []map[string]interface{}{
						{
							"name": "LoadAware",
							"args": map[string]interface{}{
								"evictionLimits": map[string]int{
									"total": 10, // Changed!
									"node":  2,
								},
							},
						},
					},
				},
			}

			raw1, _ := json.Marshal(complexJSON1)
			raw2, _ := json.Marshal(complexJSON2)

			item1 := &advisorv1alpha1.VirtPlatformConfigItem{
				DesiredState: &runtime.RawExtension{Raw: raw1},
			}

			item2 := &advisorv1alpha1.VirtPlatformConfigItem{
				DesiredState: &runtime.RawExtension{Raw: raw2},
			}

			Expect(itemConfigDiffers(item1, item2)).To(BeTrue())
		})
	})
})
