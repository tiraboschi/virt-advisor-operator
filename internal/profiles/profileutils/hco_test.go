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
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLabelSelectorToMap(t *testing.T) {
	tests := []struct {
		name     string
		selector *metav1.LabelSelector
		want     map[string]interface{}
	}{
		{
			name:     "nil selector",
			selector: nil,
			want:     map[string]interface{}{},
		},
		{
			name:     "empty selector",
			selector: &metav1.LabelSelector{},
			want:     map[string]interface{}{},
		},
		{
			name: "matchLabels only",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"ksm-enabled": "true",
					"node-role":   "worker",
				},
			},
			want: map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"ksm-enabled": "true",
					"node-role":   "worker",
				},
			},
		},
		{
			name: "matchExpressions only",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "node-role.kubernetes.io/worker",
						Operator: metav1.LabelSelectorOpExists,
					},
					{
						Key:      "node.kubernetes.io/instance-type",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"m5.xlarge", "m5.2xlarge"},
					},
				},
			},
			want: map[string]interface{}{
				"matchExpressions": []interface{}{
					map[string]interface{}{
						"key":      "node-role.kubernetes.io/worker",
						"operator": string(metav1.LabelSelectorOpExists),
					},
					map[string]interface{}{
						"key":      "node.kubernetes.io/instance-type",
						"operator": string(metav1.LabelSelectorOpIn),
						"values":   []interface{}{"m5.xlarge", "m5.2xlarge"},
					},
				},
			},
		},
		{
			name: "both matchLabels and matchExpressions",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"ksm-enabled": "true",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "node-role.kubernetes.io/worker",
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			},
			want: map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"ksm-enabled": "true",
				},
				"matchExpressions": []interface{}{
					map[string]interface{}{
						"key":      "node-role.kubernetes.io/worker",
						"operator": string(metav1.LabelSelectorOpExists),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LabelSelectorToMap(tt.selector)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LabelSelectorToMap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHCOConstants(t *testing.T) {
	// Verify HCO constants are correctly defined
	if HyperConvergedNamespace != "openshift-cnv" {
		t.Errorf("HyperConvergedNamespace = %q, want %q", HyperConvergedNamespace, "openshift-cnv")
	}

	if HyperConvergedName != "kubevirt-hyperconverged" {
		t.Errorf("HyperConvergedName = %q, want %q", HyperConvergedName, "kubevirt-hyperconverged")
	}

	// Verify GVK
	if HyperConvergedGVK.Group != "hco.kubevirt.io" {
		t.Errorf("HyperConvergedGVK.Group = %q, want %q", HyperConvergedGVK.Group, "hco.kubevirt.io")
	}
	if HyperConvergedGVK.Version != "v1beta1" {
		t.Errorf("HyperConvergedGVK.Version = %q, want %q", HyperConvergedGVK.Version, "v1beta1")
	}
	if HyperConvergedGVK.Kind != "HyperConverged" {
		t.Errorf("HyperConvergedGVK.Kind = %q, want %q", HyperConvergedGVK.Kind, "HyperConverged")
	}
}
