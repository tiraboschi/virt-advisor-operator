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

// PtrEqual compares two pointers for equality.
// Returns true if both are nil or both point to equal values.
// Returns false if only one is nil or values are not equal.
func PtrEqual[T comparable](a, b *T) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// Legacy type-specific functions for backward compatibility
// These can be removed once all callers are migrated to PtrEqual

// Int32PtrEqual compares two int32 pointers for equality.
// Deprecated: Use PtrEqual[int32] instead.
func Int32PtrEqual(a, b *int32) bool {
	return PtrEqual(a, b)
}

// BoolPtrEqual compares two bool pointers for equality.
// Deprecated: Use PtrEqual[bool] instead.
func BoolPtrEqual(a, b *bool) bool {
	return PtrEqual(a, b)
}

// StringPtrEqual compares two string pointers for equality.
// Deprecated: Use PtrEqual[string] instead.
func StringPtrEqual(a, b *string) bool {
	return PtrEqual(a, b)
}
