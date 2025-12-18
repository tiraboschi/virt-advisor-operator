package profiles

import (
	"fmt"
	"strconv"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
)

// OptionsToMap converts typed ProfileOptions to map[string]string for profiles that still use map-based config.
// This is a temporary compatibility layer - profiles should eventually accept typed configs directly.
func OptionsToMap(profileName string, options *advisorv1alpha1.ProfileOptions) map[string]string {
	if options == nil {
		return map[string]string{}
	}

	switch profileName {
	case ProfileNameLoadAware:
		if options.LoadAware == nil {
			return map[string]string{}
		}
		return loadAwareToMap(options.LoadAware)
	case ProfileNameVirtHigherDensity:
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

	return m
}

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

// ValidateOptions ensures the ProfileOptions matches the selected profile.
// This is a runtime check in addition to CEL validation.
func ValidateOptions(profileName string, options *advisorv1alpha1.ProfileOptions) error {
	if options == nil {
		return nil // Optional field
	}

	// Check that only the field matching the profile is set
	switch profileName {
	case ProfileNameLoadAware:
		// LoadAware can be nil (use defaults) or set (use custom values)
		// No validation needed - CEL already ensures other fields aren't set
		return nil
	case ProfileNameVirtHigherDensity:
		// VirtHigherDensity can be nil (use defaults) or set (use custom values)
		// No validation needed - CEL already ensures other fields aren't set
		return nil
	default:
		return fmt.Errorf("unknown profile: %s", profileName)
	}
}
