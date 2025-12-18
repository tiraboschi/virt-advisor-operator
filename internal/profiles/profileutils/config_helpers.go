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
	"strconv"
)

// GetIntConfig extracts an integer configuration value from map with a default.
func GetIntConfig(configMap map[string]string, key string, defaultValue int) int {
	if v, ok := configMap[key]; ok {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultValue
}

// GetBoolConfig extracts a boolean configuration value from map with a default.
func GetBoolConfig(configMap map[string]string, key string, defaultValue bool) bool {
	if v, ok := configMap[key]; ok {
		return v != "false"
	}
	return defaultValue
}

// GetStringConfig extracts a string configuration value from map with a default.
func GetStringConfig(configMap map[string]string, key string, defaultValue string) string {
	if v, ok := configMap[key]; ok {
		return v
	}
	return defaultValue
}
