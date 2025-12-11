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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hcov1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
)

var _ = Describe("ConfigurationPlan Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
			// No namespace - ConfigurationPlan is cluster-scoped
		}
		configurationplan := &hcov1alpha1.ConfigurationPlan{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ConfigurationPlan")
			err := k8sClient.Get(ctx, typeNamespacedName, configurationplan)
			if err != nil && errors.IsNotFound(err) {
				resource := &hcov1alpha1.ConfigurationPlan{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
						// No namespace - ConfigurationPlan is cluster-scoped
					},
					Spec: hcov1alpha1.ConfigurationPlanSpec{
						Profile: resourceName, // Must match metadata.name per CEL validation
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("Cleanup the specific resource instance ConfigurationPlan")
			resource := &hcov1alpha1.ConfigurationPlan{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ConfigurationPlanReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When validating singleton pattern", func() {
		ctx := context.Background()

		AfterEach(func() {
			// Clean up any resources that might have been created
			resource := &hcov1alpha1.ConfigurationPlan{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "mismatched-name"}, resource)
			if err == nil {
				_ = k8sClient.Delete(ctx, resource)
			}
		})

		It("should reject ConfigurationPlan when name does not match profile", func() {
			By("attempting to create a ConfigurationPlan with mismatched name and profile")
			resource := &hcov1alpha1.ConfigurationPlan{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mismatched-name",
				},
				Spec: hcov1alpha1.ConfigurationPlanSpec{
					Profile: "different-profile",
				},
			}

			err := k8sClient.Create(ctx, resource)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("To ensure a singleton pattern, the ConfigurationPlan name must exactly match the spec.profile"))
		})

		It("should accept ConfigurationPlan when name matches profile", func() {
			By("creating a ConfigurationPlan with matching name and profile")
			const matchingName = "matching-profile"
			resource := &hcov1alpha1.ConfigurationPlan{
				ObjectMeta: metav1.ObjectMeta{
					Name: matchingName,
				},
				Spec: hcov1alpha1.ConfigurationPlanSpec{
					Profile: matchingName,
				},
			}

			err := k8sClient.Create(ctx, resource)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
	})
})
