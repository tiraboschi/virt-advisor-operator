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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TestConvertToUnstructured tests the ConvertToUnstructured function
func TestConvertToUnstructured(t *testing.T) {
	tests := []struct {
		name          string
		obj           runtime.Object
		expectError   bool
		expectedKind  string
		expectedGroup string
	}{
		{
			name: "Convert ConfigMap",
			obj: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config",
					Namespace: "default",
				},
				Data: map[string]string{
					"key": "value",
				},
			},
			expectError:   false,
			expectedKind:  "ConfigMap",
			expectedGroup: "",
		},
		{
			name: "Convert Secret",
			obj: &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				StringData: map[string]string{
					"password": "secret",
				},
			},
			expectError:   false,
			expectedKind:  "Secret",
			expectedGroup: "",
		},
		{
			name: "Convert Pod",
			obj: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Pod",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
						},
					},
				},
			},
			expectError:   false,
			expectedKind:  "Pod",
			expectedGroup: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unstructured, err := ConvertToUnstructured(tt.obj)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if unstructured == nil {
				t.Fatal("Expected non-nil unstructured object")
			}

			// Verify GVK is preserved
			gvk := unstructured.GroupVersionKind()
			if gvk.Kind != tt.expectedKind {
				t.Errorf("Expected kind %s, got %s", tt.expectedKind, gvk.Kind)
			}

			if gvk.Group != tt.expectedGroup {
				t.Errorf("Expected group %q, got %q", tt.expectedGroup, gvk.Group)
			}

			// Verify basic fields are accessible
			name := unstructured.GetName()
			if name == "" {
				t.Error("Expected name to be set")
			}
		})
	}
}

// TestConvertToUnstructured_PreservesData tests that data is preserved during conversion
func TestConvertToUnstructured_PreservesData(t *testing.T) {
	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: "default",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Data: map[string]string{
			"config.yaml": "key: value",
			"settings":    "debug: true",
		},
	}

	unstructured, err := ConvertToUnstructured(configMap)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify labels are preserved
	labels := unstructured.GetLabels()
	if labels["app"] != "test" {
		t.Errorf("Expected label app=test, got %v", labels)
	}

	// Verify data is accessible
	data, found := unstructuredNestedStringMap(unstructured.Object, "data")
	if !found {
		t.Error("Data field not found")
	}
	if data["config.yaml"] != "key: value" {
		t.Errorf("Data not preserved correctly: %v", data)
	}
}

// Helper function to extract nested string map from unstructured
func unstructuredNestedStringMap(obj map[string]interface{}, field string) (map[string]string, bool) {
	val, found := obj[field]
	if !found {
		return nil, false
	}

	mapVal, ok := val.(map[string]interface{})
	if !ok {
		return nil, false
	}

	result := make(map[string]string)
	for k, v := range mapVal {
		strVal, ok := v.(string)
		if ok {
			result[k] = strVal
		}
	}

	return result, true
}
