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

package profileutils

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// HyperConvergedNamespace is the namespace where HCO CR is located
	HyperConvergedNamespace = "openshift-cnv"
	// HyperConvergedName is the name of the HCO CR
	HyperConvergedName = "kubevirt-hyperconverged"
)

// HyperConvergedGVK is the GroupVersionKind for the HyperConverged resource
var HyperConvergedGVK = schema.GroupVersionKind{
	Group:   "hco.kubevirt.io",
	Version: "v1beta1",
	Kind:    "HyperConverged",
}

// GetHCO fetches the HyperConverged CR from the cluster.
// Returns the CR as an unstructured object, or an error if not found.
func GetHCO(ctx context.Context, c client.Client) (*unstructured.Unstructured, error) {
	hco := &unstructured.Unstructured{}
	hco.SetGroupVersionKind(HyperConvergedGVK)

	key := types.NamespacedName{
		Name:      HyperConvergedName,
		Namespace: HyperConvergedNamespace,
	}

	if err := c.Get(ctx, key, hco); err != nil {
		return nil, err
	}

	return hco, nil
}

// LabelSelectorToMap converts a Kubernetes LabelSelector to a map suitable for unstructured objects.
// Handles both matchLabels and matchExpressions.
func LabelSelectorToMap(selector *metav1.LabelSelector) map[string]interface{} {
	if selector == nil {
		return map[string]interface{}{}
	}

	result := make(map[string]interface{})

	// Convert matchLabels
	if len(selector.MatchLabels) > 0 {
		matchLabels := make(map[string]interface{})
		for k, v := range selector.MatchLabels {
			matchLabels[k] = v
		}
		result["matchLabels"] = matchLabels
	}

	// Convert matchExpressions
	if len(selector.MatchExpressions) > 0 {
		matchExpressions := make([]interface{}, len(selector.MatchExpressions))
		for i, expr := range selector.MatchExpressions {
			e := map[string]interface{}{
				"key":      expr.Key,
				"operator": string(expr.Operator),
			}
			if len(expr.Values) > 0 {
				values := make([]interface{}, len(expr.Values))
				for j, v := range expr.Values {
					values[j] = v
				}
				e["values"] = values
			}
			matchExpressions[i] = e
		}
		result["matchExpressions"] = matchExpressions
	}

	return result
}
