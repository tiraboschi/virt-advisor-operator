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

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
)

var _ = Describe("VirtPlatformConfig Dynamic Watch Functions", func() {
	Context("isManagedResource", func() {
		var (
			testCtx    context.Context
			reconciler *VirtPlatformConfigReconciler
		)

		BeforeEach(func() {
			testCtx = context.Background()
			reconciler = &VirtPlatformConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		It("should identify managed ConfigMap resource", func() {
			By("creating a VirtPlatformConfig that manages a ConfigMap")
			config := &advisorv1alpha1.VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "example-profile",
				},
				Spec: advisorv1alpha1.VirtPlatformConfigSpec{
					Profile: "example-profile",
				},
			}
			Expect(k8sClient.Create(testCtx, config)).To(Succeed())

			By("updating the status to include managed items")
			config.Status = advisorv1alpha1.VirtPlatformConfigStatus{
				Items: []advisorv1alpha1.VirtPlatformConfigItem{
					{
						Name: "config-item",
						TargetRef: advisorv1alpha1.ObjectReference{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       "test-config",
							Namespace:  "default",
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(testCtx, config)).To(Succeed())

			By("checking if the managed ConfigMap is identified")
			managedObj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-config",
						"namespace": "default",
					},
				},
			}

			result := reconciler.isManagedResource(managedObj)
			Expect(result).To(BeTrue())

			By("checking that a different ConfigMap is not identified as managed")
			unmanagedObj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "different-config",
						"namespace": "default",
					},
				},
			}

			result = reconciler.isManagedResource(unmanagedObj)
			Expect(result).To(BeFalse())

			By("cleanup")
			Expect(k8sClient.Delete(testCtx, config)).To(Succeed())
		})

		It("should identify managed cluster-scoped resource", func() {
			By("creating a VirtPlatformConfig that manages a cluster-scoped resource")
			config := &advisorv1alpha1.VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "load-aware-rebalancing",
				},
				Spec: advisorv1alpha1.VirtPlatformConfigSpec{
					Profile: "load-aware-rebalancing",
				},
			}
			Expect(k8sClient.Create(testCtx, config)).To(Succeed())

			By("updating the status to include managed items")
			config.Status = advisorv1alpha1.VirtPlatformConfigStatus{
				Items: []advisorv1alpha1.VirtPlatformConfigItem{
					{
						Name: "descheduler-item",
						TargetRef: advisorv1alpha1.ObjectReference{
							APIVersion: "operator.openshift.io/v1",
							Kind:       "KubeDescheduler",
							Name:       "cluster",
							Namespace:  "",
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(testCtx, config)).To(Succeed())

			By("checking if the cluster-scoped resource is identified")
			managedObj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "operator.openshift.io/v1",
					"kind":       "KubeDescheduler",
					"metadata": map[string]interface{}{
						"name": "cluster",
					},
				},
			}

			result := reconciler.isManagedResource(managedObj)
			Expect(result).To(BeTrue())

			By("cleanup")
			Expect(k8sClient.Delete(testCtx, config)).To(Succeed())
		})

		It("should reject resource with wrong namespace", func() {
			By("creating a VirtPlatformConfig")
			config := &advisorv1alpha1.VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "virt-higher-density",
				},
				Spec: advisorv1alpha1.VirtPlatformConfigSpec{
					Profile: "virt-higher-density",
				},
			}
			Expect(k8sClient.Create(testCtx, config)).To(Succeed())

			By("updating the status to include managed items")
			config.Status = advisorv1alpha1.VirtPlatformConfigStatus{
				Items: []advisorv1alpha1.VirtPlatformConfigItem{
					{
						Name: "config-item",
						TargetRef: advisorv1alpha1.ObjectReference{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       "test-config",
							Namespace:  "default",
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(testCtx, config)).To(Succeed())

			By("checking resource in wrong namespace")
			wrongNamespaceObj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-config",
						"namespace": "other-namespace",
					},
				},
			}

			result := reconciler.isManagedResource(wrongNamespaceObj)
			Expect(result).To(BeFalse())

			By("cleanup")
			Expect(k8sClient.Delete(testCtx, config)).To(Succeed())
		})

		It("should reject resource with wrong kind", func() {
			By("creating a VirtPlatformConfig")
			config := &advisorv1alpha1.VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "example-profile",
				},
				Spec: advisorv1alpha1.VirtPlatformConfigSpec{
					Profile: "example-profile",
				},
			}
			Expect(k8sClient.Create(testCtx, config)).To(Succeed())

			By("updating the status to include managed items")
			config.Status = advisorv1alpha1.VirtPlatformConfigStatus{
				Items: []advisorv1alpha1.VirtPlatformConfigItem{
					{
						Name: "config-item",
						TargetRef: advisorv1alpha1.ObjectReference{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       "test-config",
							Namespace:  "default",
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(testCtx, config)).To(Succeed())

			By("checking resource with wrong kind")
			wrongKindObj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"metadata": map[string]interface{}{
						"name":      "test-config",
						"namespace": "default",
					},
				},
			}

			result := reconciler.isManagedResource(wrongKindObj)
			Expect(result).To(BeFalse())

			By("cleanup")
			Expect(k8sClient.Delete(testCtx, config)).To(Succeed())
		})

		It("should return false for non-unstructured object", func() {
			By("checking non-unstructured object")
			// Create a non-unstructured object (this would never happen in practice)
			type nonUnstructured struct {
				client.Object
			}

			obj := &nonUnstructured{}
			result := reconciler.isManagedResource(obj)
			Expect(result).To(BeFalse())
		})
	})

	Context("managedResourcePredicate", func() {
		var (
			testCtx    context.Context
			reconciler *VirtPlatformConfigReconciler
		)

		BeforeEach(func() {
			testCtx = context.Background()
			reconciler = &VirtPlatformConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		It("should filter events for managed and unmanaged resources", func() {
			By("creating a VirtPlatformConfig")
			config := &advisorv1alpha1.VirtPlatformConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "load-aware-rebalancing",
				},
				Spec: advisorv1alpha1.VirtPlatformConfigSpec{
					Profile: "load-aware-rebalancing",
				},
			}
			Expect(k8sClient.Create(testCtx, config)).To(Succeed())

			By("updating the status to include managed items")
			config.Status = advisorv1alpha1.VirtPlatformConfigStatus{
				Items: []advisorv1alpha1.VirtPlatformConfigItem{
					{
						Name: "config-item",
						TargetRef: advisorv1alpha1.ObjectReference{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       "managed-config",
							Namespace:  "default",
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(testCtx, config)).To(Succeed())

			predicate := reconciler.managedResourcePredicate()

			managedObj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "managed-config",
						"namespace": "default",
					},
				},
			}

			unmanagedObj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "unmanaged-config",
						"namespace": "default",
					},
				},
			}

			By("testing CreateFunc")
			Expect(predicate.Create(event.CreateEvent{Object: managedObj})).To(BeTrue())
			Expect(predicate.Create(event.CreateEvent{Object: unmanagedObj})).To(BeFalse())

			By("testing UpdateFunc")
			Expect(predicate.Update(event.UpdateEvent{ObjectNew: managedObj})).To(BeTrue())
			Expect(predicate.Update(event.UpdateEvent{ObjectNew: unmanagedObj})).To(BeFalse())

			By("testing DeleteFunc")
			Expect(predicate.Delete(event.DeleteEvent{Object: managedObj})).To(BeTrue())
			Expect(predicate.Delete(event.DeleteEvent{Object: unmanagedObj})).To(BeFalse())

			By("testing GenericFunc")
			Expect(predicate.Generic(event.GenericEvent{Object: managedObj})).To(BeTrue())
			Expect(predicate.Generic(event.GenericEvent{Object: unmanagedObj})).To(BeFalse())

			By("cleanup")
			Expect(k8sClient.Delete(testCtx, config)).To(Succeed())
		})
	})

	Context("enqueueManagedResourceOwners", func() {
		It("should return a non-nil EventHandler", func() {
			reconciler := &VirtPlatformConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			handler := reconciler.enqueueManagedResourceOwners()
			Expect(handler).ToNot(BeNil())
		})
	})

	Context("hcoMigrationConfigPredicate", func() {
		var (
			reconciler *VirtPlatformConfigReconciler
		)

		BeforeEach(func() {
			reconciler = &VirtPlatformConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		It("should trigger on HCO creation", func() {
			predicate := reconciler.hcoMigrationConfigPredicate()

			hco := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "hco.kubevirt.io/v1beta1",
					"kind":       "HyperConverged",
					"metadata": map[string]interface{}{
						"name":      "kubevirt-hyperconverged",
						"namespace": "openshift-cnv",
					},
					"spec": map[string]interface{}{
						"liveMigrationConfig": map[string]interface{}{
							"parallelMigrationsPerCluster":      int64(20),
							"parallelOutboundMigrationsPerNode": int64(10),
						},
					},
				},
			}

			result := predicate.Create(event.CreateEvent{Object: hco})
			Expect(result).To(BeTrue(), "should trigger on HCO creation")
		})

		It("should trigger when spec.liveMigrationConfig changes", func() {
			predicate := reconciler.hcoMigrationConfigPredicate()

			oldHCO := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "hco.kubevirt.io/v1beta1",
					"kind":       "HyperConverged",
					"metadata": map[string]interface{}{
						"name":      "kubevirt-hyperconverged",
						"namespace": "openshift-cnv",
					},
					"spec": map[string]interface{}{
						"liveMigrationConfig": map[string]interface{}{
							"parallelMigrationsPerCluster":      int64(20),
							"parallelOutboundMigrationsPerNode": int64(10),
						},
					},
				},
			}

			newHCO := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "hco.kubevirt.io/v1beta1",
					"kind":       "HyperConverged",
					"metadata": map[string]interface{}{
						"name":      "kubevirt-hyperconverged",
						"namespace": "openshift-cnv",
					},
					"spec": map[string]interface{}{
						"liveMigrationConfig": map[string]interface{}{
							"parallelMigrationsPerCluster":      int64(30), // Changed!
							"parallelOutboundMigrationsPerNode": int64(10),
						},
					},
				},
			}

			result := predicate.Update(event.UpdateEvent{
				ObjectOld: oldHCO,
				ObjectNew: newHCO,
			})
			Expect(result).To(BeTrue(), "should trigger when liveMigrationConfig changes")
		})

		It("should not trigger when spec.liveMigrationConfig unchanged", func() {
			predicate := reconciler.hcoMigrationConfigPredicate()

			oldHCO := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "hco.kubevirt.io/v1beta1",
					"kind":       "HyperConverged",
					"metadata": map[string]interface{}{
						"name":            "kubevirt-hyperconverged",
						"namespace":       "openshift-cnv",
						"resourceVersion": "1",
					},
					"spec": map[string]interface{}{
						"liveMigrationConfig": map[string]interface{}{
							"parallelMigrationsPerCluster":      int64(20),
							"parallelOutboundMigrationsPerNode": int64(10),
						},
					},
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{
								"type":   "Available",
								"status": "True",
							},
						},
					},
				},
			}

			newHCO := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "hco.kubevirt.io/v1beta1",
					"kind":       "HyperConverged",
					"metadata": map[string]interface{}{
						"name":            "kubevirt-hyperconverged",
						"namespace":       "openshift-cnv",
						"resourceVersion": "2", // Only metadata/status changed
					},
					"spec": map[string]interface{}{
						"liveMigrationConfig": map[string]interface{}{
							"parallelMigrationsPerCluster":      int64(20), // Same
							"parallelOutboundMigrationsPerNode": int64(10), // Same
						},
					},
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{
								"type":   "Available",
								"status": "False", // Status changed
							},
						},
					},
				},
			}

			result := predicate.Update(event.UpdateEvent{
				ObjectOld: oldHCO,
				ObjectNew: newHCO,
			})
			Expect(result).To(BeFalse(), "should not trigger when only status changes")
		})

		It("should not trigger on HCO deletion", func() {
			predicate := reconciler.hcoMigrationConfigPredicate()

			hco := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "hco.kubevirt.io/v1beta1",
					"kind":       "HyperConverged",
					"metadata": map[string]interface{}{
						"name":      "kubevirt-hyperconverged",
						"namespace": "openshift-cnv",
					},
				},
			}

			result := predicate.Delete(event.DeleteEvent{Object: hco})
			Expect(result).To(BeFalse(), "should not trigger on HCO deletion")
		})
	})
})
