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

package profiles

import (
	"encoding/json"
	"fmt"
	"strconv"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
	"github.com/kubevirt/virt-advisor-operator/internal/profiles/higherdensity"
	"github.com/kubevirt/virt-advisor-operator/internal/profiles/loadaware"
)

// OptionsToMap converts typed ProfileOptions to map[string]string for profiles that still use map-based config.
// This is a temporary compatibility layer - profiles should eventually accept typed configs directly.
func OptionsToMap(profileName string, options *advisorv1alpha1.ProfileOptions) map[string]string {
	if options == nil {
		return map[string]string{}
	}

	switch profileName {
	case loadaware.ProfileNameLoadAware:
		if options.LoadAware == nil {
			return map[string]string{}
		}
		return loadAwareToMap(options.LoadAware)
	case higherdensity.ProfileNameVirtHigherDensity:
		if options.VirtHigherDensity == nil {
			return map[string]string{}
		}
		return virtHigherDensityToMap(options.VirtHigherDensity)
	default:
		return map[string]string{}
	}
}

// loadAwareToMap converts LoadAwareConfig to map format.
func loadAwareToMap(config *advisorv1alpha1.LoadAwareConfig) map[string]string {
	m := make(map[string]string)

	if config.DeschedulingIntervalSeconds != nil {
		m["deschedulingIntervalSeconds"] = strconv.FormatInt(int64(*config.DeschedulingIntervalSeconds), 10)
	}

	if config.EnablePSIMetrics != nil {
		m["enablePSIMetrics"] = strconv.FormatBool(*config.EnablePSIMetrics)
	}

	if config.DevDeviationThresholds != nil {
		m["devDeviationThresholds"] = *config.DevDeviationThresholds
	}

	if config.EvictionLimitTotal != nil {
		m["evictionLimitTotal"] = strconv.FormatInt(int64(*config.EvictionLimitTotal), 10)
	}

	if config.EvictionLimitNode != nil {
		m["evictionLimitNode"] = strconv.FormatInt(int64(*config.EvictionLimitNode), 10)
	}

	return m
}

// virtHigherDensityToMap converts VirtHigherDensityConfig to map format.
func virtHigherDensityToMap(config *advisorv1alpha1.VirtHigherDensityConfig) map[string]string {
	m := make(map[string]string)

	if config.EnableSwap != nil {
		m["enableSwap"] = strconv.FormatBool(*config.EnableSwap)
	}

	if config.KSMConfiguration != nil {
		// Check if KSM is explicitly enabled (default is true)
		enabled := true
		if config.KSMConfiguration.Enabled != nil {
			enabled = *config.KSMConfiguration.Enabled
		}
		m["ksmEnabled"] = strconv.FormatBool(enabled)

		// Serialize the node selector if KSM is enabled
		if enabled && config.KSMConfiguration.NodeLabelSelector != nil {
			if selectorJSON, err := json.Marshal(config.KSMConfiguration.NodeLabelSelector); err == nil {
				m["ksmNodeLabelSelector"] = string(selectorJSON)
			}
		}
	} else {
		// Default behavior when ksmConfiguration is omitted: enabled with all nodes
		m["ksmEnabled"] = "true"
	}

	if config.MemoryToRequestRatio != nil {
		m["memoryToRequestRatio"] = strconv.FormatInt(int64(*config.MemoryToRequestRatio), 10)
	}

	return m
}

// ValidateOptions ensures the ProfileOptions matches the selected profile.
// This is a runtime check in addition to CEL validation.
func ValidateOptions(profileName string, options *advisorv1alpha1.ProfileOptions) error {
	if options == nil {
		return nil // Optional field
	}

	// Check that only the field matching the profile is set
	switch profileName {
	case loadaware.ProfileNameLoadAware:
		// LoadAware can be nil (use defaults) or set (use custom values)
		// No validation needed - CEL already ensures other fields aren't set
		return nil
	case higherdensity.ProfileNameVirtHigherDensity:
		// VirtHigherDensity can be nil (use defaults) or set (use custom values)
		// No validation needed - CEL already ensures other fields aren't set
		return nil
	default:
		return fmt.Errorf("unknown profile: %s", profileName)
	}
}
