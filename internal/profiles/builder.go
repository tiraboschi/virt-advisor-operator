package profiles

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hcov1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
	"github.com/kubevirt/virt-advisor-operator/internal/plan"
)

// PlanItemBuilder provides a safe way to build VirtPlatformConfigItems
// that ALWAYS use Server-Side Apply (SSA) for diff generation.
//
// This enforces the VEP requirement that diffs leverage API server validation,
// defaulting, CEL rules, and webhooks. It prevents profiles from accidentally
// using manual string-based diffs.
//
// Usage:
//
//	builder := profiles.NewPlanItemBuilder(ctx, client, "virt-advisor-operator")
//	item, err := builder.
//	    ForResource(desired, "enable-load-aware-descheduling").
//	    WithManagedFields([]string{"spec.profiles", "spec.deschedulingIntervalSeconds"}).
//	    WithImpactSeverity("Low - Updates existing configuration").
//	    Build()
type PlanItemBuilder struct {
	ctx          context.Context
	client       client.Client
	fieldManager string

	// Required fields
	desired *unstructured.Unstructured
	name    string

	// Optional fields
	managedFields  []string
	impactSeverity string
	message        string
}

// NewPlanItemBuilder creates a new builder for VirtPlatformConfigItems.
// All items built through this builder will use SSA dry-run for diff generation.
func NewPlanItemBuilder(ctx context.Context, c client.Client, fieldManager string) *PlanItemBuilder {
	return &PlanItemBuilder{
		ctx:          ctx,
		client:       c,
		fieldManager: fieldManager,
	}
}

// ForResource sets the desired resource and item name.
// The desired object should be the complete configuration you want to apply.
func (b *PlanItemBuilder) ForResource(desired *unstructured.Unstructured, name string) *PlanItemBuilder {
	b.desired = desired
	b.name = name
	return b
}

// WithManagedFields specifies which fields this item manages.
// Used for drift detection to only monitor fields we control.
// Format: JSON path like "spec.profiles", "spec.kernelArguments"
func (b *PlanItemBuilder) WithManagedFields(fields []string) *PlanItemBuilder {
	b.managedFields = fields
	return b
}

// WithImpactSeverity sets the human-readable impact description.
// Examples: "Low - Updates existing configuration", "High - Node reboot required"
func (b *PlanItemBuilder) WithImpactSeverity(severity string) *PlanItemBuilder {
	b.impactSeverity = severity
	return b
}

// WithMessage sets the initial status message for the item.
func (b *PlanItemBuilder) WithMessage(message string) *PlanItemBuilder {
	b.message = message
	return b
}

// Build constructs the VirtPlatformConfigItem with an SSA-generated diff.
//
// This method:
// 1. Validates required fields are set
// 2. Determines impact severity if not manually set (based on resource existence)
// 3. Generates the diff using SSA dry-run (API server validation)
// 4. Returns a complete VirtPlatformConfigItem
func (b *PlanItemBuilder) Build() (hcov1alpha1.VirtPlatformConfigItem, error) {
	// Validate required fields
	if b.desired == nil {
		return hcov1alpha1.VirtPlatformConfigItem{}, fmt.Errorf("desired resource not set (use ForResource)")
	}
	if b.name == "" {
		return hcov1alpha1.VirtPlatformConfigItem{}, fmt.Errorf("item name not set (use ForResource)")
	}

	gvk := b.desired.GroupVersionKind()

	// Auto-determine impact severity if not set
	if b.impactSeverity == "" {
		_, err := plan.GetUnstructured(b.ctx, b.client, gvk, b.desired.GetName(), b.desired.GetNamespace())
		isNew := errors.IsNotFound(err)

		if isNew {
			b.impactSeverity = fmt.Sprintf("Medium - Creates new %s", gvk.Kind)
		} else if err != nil {
			return hcov1alpha1.VirtPlatformConfigItem{}, fmt.Errorf("failed to check if resource exists: %w", err)
		} else {
			b.impactSeverity = fmt.Sprintf("Low - Updates existing %s", gvk.Kind)
		}
	}

	// Auto-set message if not provided
	if b.message == "" {
		b.message = fmt.Sprintf("%s '%s' will be configured", gvk.Kind, b.desired.GetName())
	}

	// CRITICAL: Generate diff using SSA dry-run
	// This ensures we leverage API server validation, defaulting, CEL, and webhooks
	diff, err := plan.GenerateSSADiff(b.ctx, b.client, b.desired, b.fieldManager)
	if err != nil {
		return hcov1alpha1.VirtPlatformConfigItem{}, fmt.Errorf("failed to generate SSA diff: %w", err)
	}

	return hcov1alpha1.VirtPlatformConfigItem{
		Name: b.name,
		TargetRef: hcov1alpha1.ObjectReference{
			APIVersion: gvk.GroupVersion().String(),
			Kind:       gvk.Kind,
			Name:       b.desired.GetName(),
			Namespace:  b.desired.GetNamespace(),
		},
		ImpactSeverity:     b.impactSeverity,
		Diff:               diff,
		State:              hcov1alpha1.ItemStatePending,
		Message:            b.message,
		ManagedFields:      b.managedFields,
		LastTransitionTime: nil, // Set by controller
	}, nil
}
