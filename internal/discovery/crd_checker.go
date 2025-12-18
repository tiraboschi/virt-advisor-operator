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

package discovery

import (
	"context"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Prerequisite represents a required CRD for a profile
type Prerequisite struct {
	GVK         schema.GroupVersionKind
	Description string // Human-readable description for error messages
}

// CRDChecker validates that required CRDs exist in the cluster
type CRDChecker struct {
	client client.Client
}

// NewCRDChecker creates a new CRD checker
func NewCRDChecker(c client.Client) *CRDChecker {
	return &CRDChecker{client: c}
}

// CheckPrerequisites validates that all required CRDs exist
// Returns a list of missing prerequisites
func (c *CRDChecker) CheckPrerequisites(ctx context.Context, prerequisites []Prerequisite) ([]Prerequisite, error) {
	var missing []Prerequisite

	for _, prereq := range prerequisites {
		exists, err := c.crdExists(ctx, prereq.GVK)
		if err != nil {
			return nil, fmt.Errorf("failed to check CRD %s: %w", prereq.GVK, err)
		}
		if !exists {
			missing = append(missing, prereq)
		}
	}

	return missing, nil
}

// crdExists checks if a CRD for the given GVK exists
func (c *CRDChecker) crdExists(ctx context.Context, gvk schema.GroupVersionKind) (bool, error) {
	// Construct the CRD name: <plural>.<group>
	// We need to derive plural from the kind (heuristic: lowercase + s)
	// For production, we'd use RESTMapper, but for now use a simple approach
	crdName := GVKToCRDName(gvk)

	return c.CRDExists(ctx, crdName)
}

// CRDExists checks if a CRD with the given name exists
func (c *CRDChecker) CRDExists(ctx context.Context, crdName string) (bool, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	err := c.client.Get(ctx, client.ObjectKey{Name: crdName}, crd)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil // CRD not found
		}
		return false, err // Some other error
	}

	return true, nil
}

// GVKToCRDName builds a CRD name from GVK
// Format: <plural>.<group>
// This is exported for use by the controller when registering dynamic watches
func GVKToCRDName(gvk schema.GroupVersionKind) string {
	// Simple pluralization heuristic
	// This matches common patterns but could be improved with a full inflector
	kind := gvk.Kind

	// Handle common cases
	var plural string
	switch kind {
	case "MachineConfig":
		plural = "machineconfigs"
	case "MachineConfigPool":
		plural = "machineconfigpools"
	case "KubeDescheduler":
		plural = "kubedeschedulers"
	default:
		// Simple heuristic: lowercase and add 's'
		plural = toLower(kind) + "s"
	}

	return plural + "." + gvk.Group
}

func toLower(s string) string {
	if len(s) == 0 {
		return s
	}
	// Simple ASCII lowercase for Kind names
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c = c + ('a' - 'A')
		}
		result[i] = c
	}
	return string(result)
}

// FormatMissingPrerequisites creates a user-friendly error message
func FormatMissingPrerequisites(missing []Prerequisite) string {
	if len(missing) == 0 {
		return ""
	}

	msg := "Missing required dependencies:\n"
	for _, prereq := range missing {
		msg += fmt.Sprintf("  - %s (%s.%s): %s\n",
			prereq.GVK.Kind,
			prereq.GVK.Group,
			prereq.GVK.Version,
			prereq.Description)
	}
	return msg
}
