package plan

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hcov1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
)

var (
	// KubeDeschedulerGVK is the GroupVersionKind for KubeDescheduler
	KubeDeschedulerGVK = schema.GroupVersionKind{
		Group:   "operator.openshift.io",
		Version: "v1",
		Kind:    "KubeDescheduler",
	}

	// MachineConfigGVK is the GroupVersionKind for MachineConfig
	MachineConfigGVK = schema.GroupVersionKind{
		Group:   "machineconfiguration.openshift.io",
		Version: "v1",
		Kind:    "MachineConfig",
	}
)

// ExecuteItem executes a single configuration plan item by applying the changes
// using unstructured objects to avoid requiring third-party CRDs in the scheme
func ExecuteItem(ctx context.Context, c client.Client, item *hcov1alpha1.ConfigurationPlanItem) error {
	// Handle specific known types
	switch item.TargetRef.Kind {
	case "KubeDescheduler":
		return executeKubeDescheduler(ctx, c, item)
	case "MachineConfig":
		return executeMachineConfig(ctx, c, item)
	default:
		return fmt.Errorf("unsupported resource kind: %s", item.TargetRef.Kind)
	}
}

// executeKubeDescheduler applies the KubeDescheduler configuration using unstructured
func executeKubeDescheduler(ctx context.Context, c client.Client, item *hcov1alpha1.ConfigurationPlanItem) error {
	// Build the desired KubeDescheduler object as unstructured
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(KubeDeschedulerGVK)
	obj.SetName(item.TargetRef.Name)

	// Set the spec
	spec := map[string]interface{}{
		"deschedulingIntervalSeconds": int64(1800), // 30 minutes
		"profiles":                    []interface{}{"LoadAware"},
	}

	if err := unstructured.SetNestedMap(obj.Object, spec, "spec"); err != nil {
		return fmt.Errorf("failed to set spec: %w", err)
	}

	// Use server-side apply
	opts := DefaultApplyOptions()
	return ApplyUnstructured(ctx, c, obj, opts)
}

// executeMachineConfig applies the MachineConfig configuration using unstructured
func executeMachineConfig(ctx context.Context, c client.Client, item *hcov1alpha1.ConfigurationPlanItem) error {
	// Build the desired MachineConfig object as unstructured
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(MachineConfigGVK)
	obj.SetName(item.TargetRef.Name)

	// Set labels
	obj.SetLabels(map[string]string{
		"machineconfiguration.openshift.io/role": "worker",
	})

	// Set the spec with kernel arguments
	spec := map[string]interface{}{
		"kernelArguments": []interface{}{"psi=1"},
	}

	if err := unstructured.SetNestedMap(obj.Object, spec, "spec"); err != nil {
		return fmt.Errorf("failed to set spec: %w", err)
	}

	// Use server-side apply
	opts := DefaultApplyOptions()
	return ApplyUnstructured(ctx, c, obj, opts)
}

// createObjectFromRef is no longer needed, kept for backwards compatibility
func createObjectFromRef(ref hcov1alpha1.ObjectReference) (client.Object, error) {
	gvk := schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind)
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName(ref.Name)
	if ref.Namespace != "" {
		obj.SetNamespace(ref.Namespace)
	}
	return obj, nil
}
