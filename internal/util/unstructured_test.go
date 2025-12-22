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

package util

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGetNestedInt64(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"count": int64(42),
			},
		},
	}

	value, found, err := GetNestedInt64(obj, "spec", "count")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !found {
		t.Error("Expected value to be found")
	}
	if value != 42 {
		t.Errorf("Expected 42, got %d", value)
	}

	// Test not found
	_, found, err = GetNestedInt64(obj, "spec", "missing")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if found {
		t.Error("Expected value not to be found")
	}
}

func TestGetNestedString(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test-object",
			},
		},
	}

	value, found, err := GetNestedString(obj, "metadata", "name")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !found {
		t.Error("Expected value to be found")
	}
	if value != "test-object" {
		t.Errorf("Expected 'test-object', got %s", value)
	}
}

func TestGetNestedBool(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"enabled": true,
			},
		},
	}

	value, found, err := GetNestedBool(obj, "spec", "enabled")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !found {
		t.Error("Expected value to be found")
	}
	if !value {
		t.Error("Expected true, got false")
	}
}

func TestGetNestedMap(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"config": map[string]interface{}{
					"key": "value",
				},
			},
		},
	}

	value, found, err := GetNestedMap(obj, "spec", "config")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !found {
		t.Error("Expected value to be found")
	}
	if value["key"] != "value" {
		t.Errorf("Expected 'value', got %v", value["key"])
	}
}

func TestGetNestedInt64OrDefault(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"count": int64(42),
			},
		},
	}

	// Test found value
	value := GetNestedInt64OrDefault(obj, 99, "spec", "count")
	if value != 42 {
		t.Errorf("Expected 42, got %d", value)
	}

	// Test default value
	value = GetNestedInt64OrDefault(obj, 99, "spec", "missing")
	if value != 99 {
		t.Errorf("Expected 99, got %d", value)
	}
}

func TestGetNestedStringOrDefault(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test-object",
			},
		},
	}

	// Test found value
	value := GetNestedStringOrDefault(obj, "default", "metadata", "name")
	if value != "test-object" {
		t.Errorf("Expected 'test-object', got %s", value)
	}

	// Test default value
	value = GetNestedStringOrDefault(obj, "default", "metadata", "missing")
	if value != "default" {
		t.Errorf("Expected 'default', got %s", value)
	}
}

func TestGetNestedBoolOrDefault(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"enabled": true,
			},
		},
	}

	// Test found value
	value := GetNestedBoolOrDefault(obj, false, "spec", "enabled")
	if !value {
		t.Error("Expected true, got false")
	}

	// Test default value
	value = GetNestedBoolOrDefault(obj, false, "spec", "missing")
	if value {
		t.Error("Expected false, got true")
	}
}
