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
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// GetNestedInt64 retrieves a nested int64 value from an unstructured object.
// Returns the value, whether it was found, and any error encountered.
func GetNestedInt64(obj *unstructured.Unstructured, fields ...string) (int64, bool, error) {
	value, found, err := unstructured.NestedInt64(obj.Object, fields...)
	if err != nil {
		return 0, false, fmt.Errorf("failed to get nested int64 at %v: %w", fields, err)
	}
	return value, found, nil
}

// GetNestedInt32 retrieves a nested int64 value and converts it to int32.
// Returns the value, whether it was found, and any error encountered.
// Returns an error if the value cannot be represented as int32.
func GetNestedInt32(obj *unstructured.Unstructured, fields ...string) (int32, bool, error) {
	value, found, err := unstructured.NestedInt64(obj.Object, fields...)
	if err != nil {
		return 0, false, fmt.Errorf("failed to get nested int32 at %v: %w", fields, err)
	}
	if !found {
		return 0, false, nil
	}
	// Check for overflow
	if value > int64(int32(^uint32(0)>>1)) || value < int64(-int32(^uint32(0)>>1)-1) {
		return 0, false, fmt.Errorf("int64 value %d at %v cannot be represented as int32", value, fields)
	}
	return int32(value), true, nil
}

// GetNestedString retrieves a nested string value from an unstructured object.
// Returns the value, whether it was found, and any error encountered.
func GetNestedString(obj *unstructured.Unstructured, fields ...string) (string, bool, error) {
	value, found, err := unstructured.NestedString(obj.Object, fields...)
	if err != nil {
		return "", false, fmt.Errorf("failed to get nested string at %v: %w", fields, err)
	}
	return value, found, nil
}

// GetNestedBool retrieves a nested bool value from an unstructured object.
// Returns the value, whether it was found, and any error encountered.
func GetNestedBool(obj *unstructured.Unstructured, fields ...string) (bool, bool, error) {
	value, found, err := unstructured.NestedBool(obj.Object, fields...)
	if err != nil {
		return false, false, fmt.Errorf("failed to get nested bool at %v: %w", fields, err)
	}
	return value, found, nil
}

// GetNestedMap retrieves a nested map from an unstructured object.
// Returns the map, whether it was found, and any error encountered.
func GetNestedMap(obj *unstructured.Unstructured, fields ...string) (map[string]interface{}, bool, error) {
	value, found, err := unstructured.NestedMap(obj.Object, fields...)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get nested map at %v: %w", fields, err)
	}
	return value, found, nil
}

// GetNestedSlice retrieves a nested slice from an unstructured object.
// Returns the slice, whether it was found, and any error encountered.
func GetNestedSlice(obj *unstructured.Unstructured, fields ...string) ([]interface{}, bool, error) {
	value, found, err := unstructured.NestedSlice(obj.Object, fields...)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get nested slice at %v: %w", fields, err)
	}
	return value, found, nil
}

// GetNestedInt64OrDefault retrieves a nested int64 value with a default fallback.
// Returns the value if found, otherwise returns the default value.
// Logs error if encountered but does not return it.
func GetNestedInt64OrDefault(obj *unstructured.Unstructured, defaultValue int64, fields ...string) int64 {
	value, found, err := unstructured.NestedInt64(obj.Object, fields...)
	if err != nil || !found {
		return defaultValue
	}
	return value
}

// GetNestedStringOrDefault retrieves a nested string value with a default fallback.
// Returns the value if found, otherwise returns the default value.
// Logs error if encountered but does not return it.
func GetNestedStringOrDefault(obj *unstructured.Unstructured, defaultValue string, fields ...string) string {
	value, found, err := unstructured.NestedString(obj.Object, fields...)
	if err != nil || !found {
		return defaultValue
	}
	return value
}

// GetNestedBoolOrDefault retrieves a nested bool value with a default fallback.
// Returns the value if found, otherwise returns the default value.
// Logs error if encountered but does not return it.
func GetNestedBoolOrDefault(obj *unstructured.Unstructured, defaultValue bool, fields ...string) bool {
	value, found, err := unstructured.NestedBool(obj.Object, fields...)
	if err != nil || !found {
		return defaultValue
	}
	return value
}
