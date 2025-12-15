package controller

import (
	"context"
	"os"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// CRDSentinelReconciler watches CustomResourceDefinitions and restarts
// the operator pod when a previously missing CRD is discovered.
//
// This enables dynamic registration of watches for soft dependencies.
// When a user installs a missing operator (e.g., Descheduler Operator),
// the sentinel detects the new CRD and restarts the pod, allowing the
// operator to register watches for the new resource type.
type CRDSentinelReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// MissingCRDs tracks CRDs that were not found at startup
	// Map key: CRD name (e.g., "kubedeschedulers.operator.openshift.io")
	MissingCRDs map[string]bool
}

// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

// Reconcile watches for CRD creation events
func (r *CRDSentinelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Check if this CRD was previously marked as missing
	if r.MissingCRDs == nil {
		r.MissingCRDs = make(map[string]bool)
	}

	crdName := req.Name

	// Only act if this was a CRD we were waiting for
	if !r.MissingCRDs[crdName] {
		return ctrl.Result{}, nil
	}

	// Verify the CRD actually exists now (not a deletion event)
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := r.Get(ctx, req.NamespacedName, crd); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// CRD was deleted, ignore
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get CRD", "crd", crdName)
		return ctrl.Result{}, err
	}

	// CRD now exists! Restart the operator to enable watches
	logger.Info("Soft dependency CRD discovered. Restarting operator to enable watches.",
		"crd", crdName,
		"group", crd.Spec.Group,
		"kind", crd.Spec.Names.Kind)

	// Clean exit - Kubernetes will restart the pod
	os.Exit(0)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CRDSentinelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiextensionsv1.CustomResourceDefinition{}).
		Named("crdsentinel").
		Complete(r)
}

// MarkCRDMissing records that a CRD is missing at startup
func (r *CRDSentinelReconciler) MarkCRDMissing(crdName string) {
	if r.MissingCRDs == nil {
		r.MissingCRDs = make(map[string]bool)
	}
	r.MissingCRDs[crdName] = true
}
