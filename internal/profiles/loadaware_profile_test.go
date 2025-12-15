package profiles

import (
	"testing"
)

func TestLoadAwareProfile_GetName(t *testing.T) {
	profile := NewLoadAwareRebalancingProfile()

	if got := profile.GetName(); got != ProfileNameLoadAware {
		t.Errorf("GetName() = %q, want %q", got, ProfileNameLoadAware)
	}

	if got := profile.GetName(); got != "load-aware-rebalancing" {
		t.Errorf("GetName() = %q, want %q", got, "load-aware-rebalancing")
	}
}

func TestLoadAwareProfile_Validate(t *testing.T) {
	profile := NewLoadAwareRebalancingProfile()

	tests := []struct {
		name            string
		configOverrides map[string]string
		wantErr         bool
		errContains     string
	}{
		// Valid cases
		{
			name:            "empty overrides",
			configOverrides: map[string]string{},
			wantErr:         false,
		},
		{
			name: "deschedulingIntervalSeconds only",
			configOverrides: map[string]string{
				"deschedulingIntervalSeconds": "3600",
			},
			wantErr: false,
		},
		{
			name: "enablePSIMetrics only",
			configOverrides: map[string]string{
				"enablePSIMetrics": "true",
			},
			wantErr: false,
		},
		{
			name: "both valid keys",
			configOverrides: map[string]string{
				"deschedulingIntervalSeconds": "7200",
				"enablePSIMetrics":            "false",
			},
			wantErr: false,
		},
		{
			name: "valid with numeric string for interval",
			configOverrides: map[string]string{
				"deschedulingIntervalSeconds": "900",
			},
			wantErr: false,
		},
		{
			name: "valid with psi enabled string variations",
			configOverrides: map[string]string{
				"enablePSIMetrics": "True",
			},
			wantErr: false,
		},
		{
			name: "valid with devDeviationThresholds",
			configOverrides: map[string]string{
				"devDeviationThresholds": "High",
			},
			wantErr: false,
		},
		{
			name: "valid with all supported keys",
			configOverrides: map[string]string{
				"deschedulingIntervalSeconds": "3600",
				"enablePSIMetrics":            "true",
				"devDeviationThresholds":      "Medium",
			},
			wantErr: false,
		},

		// Invalid cases
		{
			name: "unknown key",
			configOverrides: map[string]string{
				"unknownKey": "value",
			},
			wantErr:     true,
			errContains: "unsupported config override",
		},
		{
			name: "invalid key with valid key",
			configOverrides: map[string]string{
				"deschedulingIntervalSeconds": "1800",
				"invalidKey":                  "value",
			},
			wantErr:     true,
			errContains: "unsupported config override",
		},
		{
			name: "typo in key name",
			configOverrides: map[string]string{
				"deschedulingInterval": "1800", // Missing 'Seconds'
			},
			wantErr:     true,
			errContains: "unsupported config override",
		},
		{
			name: "case sensitive key check",
			configOverrides: map[string]string{
				"DeschedulingIntervalSeconds": "1800", // Wrong case
			},
			wantErr:     true,
			errContains: "unsupported config override",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := profile.Validate(tt.configOverrides)

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil && tt.errContains != "" {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("Validate() error = %v, should contain %q", err, tt.errContains)
				}
			}
		})
	}
}

func TestLoadAwareProfile_Constants(t *testing.T) {
	// Verify profile constants are correctly defined
	if ProfileNameLoadAware != "load-aware-rebalancing" {
		t.Errorf("ProfileNameLoadAware = %q, want %q", ProfileNameLoadAware, "load-aware-rebalancing")
	}

	if defaultDeschedulingInterval != 60 {
		t.Errorf("defaultDeschedulingInterval = %d, want %d", defaultDeschedulingInterval, 60)
	}

	if psiKernelArg != "psi=1" {
		t.Errorf("psiKernelArg = %q, want %q", psiKernelArg, "psi=1")
	}

	if kubeDeschedulerCRD != "kubedeschedulers.operator.openshift.io" {
		t.Errorf("kubeDeschedulerCRD = %q, want %q", kubeDeschedulerCRD, "kubedeschedulers.operator.openshift.io")
	}

	if machineConfigCRD != "machineconfigs.machineconfiguration.openshift.io" {
		t.Errorf("machineConfigCRD = %q, want %q", machineConfigCRD, "machineconfigs.machineconfiguration.openshift.io")
	}

	if kubeDeschedulerNamespace != "openshift-kube-descheduler-operator" {
		t.Errorf("kubeDeschedulerNamespace = %q, want %q", kubeDeschedulerNamespace, "openshift-kube-descheduler-operator")
	}

	if kubeDeschedulerName != "cluster" {
		t.Errorf("kubeDeschedulerName = %q, want %q", kubeDeschedulerName, "cluster")
	}

	// Verify descheduler profile name constants
	if profileKubeVirtRelieveAndMigrate != "KubeVirtRelieveAndMigrate" {
		t.Errorf("profileKubeVirtRelieveAndMigrate = %q, want %q", profileKubeVirtRelieveAndMigrate, "KubeVirtRelieveAndMigrate")
	}

	if profileDevKubeVirtRelieveAndMigrate != "DevKubeVirtRelieveAndMigrate" {
		t.Errorf("profileDevKubeVirtRelieveAndMigrate = %q, want %q", profileDevKubeVirtRelieveAndMigrate, "DevKubeVirtRelieveAndMigrate")
	}

	if profileLongLifecycle != "LongLifecycle" {
		t.Errorf("profileLongLifecycle = %q, want %q", profileLongLifecycle, "LongLifecycle")
	}

	if defaultDeviationThresholds != "AsymmetricLow" {
		t.Errorf("defaultDeviationThresholds = %q, want %q", defaultDeviationThresholds, "AsymmetricLow")
	}
}

func TestLoadAwareProfile_GVKDefinitions(t *testing.T) {
	// Verify GroupVersionKind definitions
	if KubeDeschedulerGVK.Group != "operator.openshift.io" {
		t.Errorf("KubeDeschedulerGVK.Group = %q, want %q", KubeDeschedulerGVK.Group, "operator.openshift.io")
	}
	if KubeDeschedulerGVK.Version != "v1" {
		t.Errorf("KubeDeschedulerGVK.Version = %q, want %q", KubeDeschedulerGVK.Version, "v1")
	}
	if KubeDeschedulerGVK.Kind != "KubeDescheduler" {
		t.Errorf("KubeDeschedulerGVK.Kind = %q, want %q", KubeDeschedulerGVK.Kind, "KubeDescheduler")
	}

	if MachineConfigGVK.Group != "machineconfiguration.openshift.io" {
		t.Errorf("MachineConfigGVK.Group = %q, want %q", MachineConfigGVK.Group, "machineconfiguration.openshift.io")
	}
	if MachineConfigGVK.Version != "v1" {
		t.Errorf("MachineConfigGVK.Version = %q, want %q", MachineConfigGVK.Version, "v1")
	}
	if MachineConfigGVK.Kind != "MachineConfig" {
		t.Errorf("MachineConfigGVK.Kind = %q, want %q", MachineConfigGVK.Kind, "MachineConfig")
	}
}

func TestLoadAwareProfile_Registration(t *testing.T) {
	// Verify the profile is registered in the default registry
	profile, err := DefaultRegistry.Get(ProfileNameLoadAware)
	if err != nil {
		t.Fatalf("DefaultRegistry.Get(%q) error = %v", ProfileNameLoadAware, err)
	}

	if profile == nil {
		t.Fatal("DefaultRegistry.Get() returned nil profile")
	}

	if profile.GetName() != ProfileNameLoadAware {
		t.Errorf("Registered profile name = %q, want %q", profile.GetName(), ProfileNameLoadAware)
	}

	// Verify it's listed in available profiles
	profiles := DefaultRegistry.List()
	found := false
	for _, name := range profiles {
		if name == ProfileNameLoadAware {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Profile %q not found in registry list %v", ProfileNameLoadAware, profiles)
	}
}
