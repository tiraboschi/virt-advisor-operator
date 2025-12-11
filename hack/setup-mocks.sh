#!/usr/bin/env bash

# Setup script for installing mock CRDs and baseline resources
# This script prepares a Kind cluster for testing the virt-advisor-operator

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "===== Setting up Mock Resources for virt-advisor-operator ====="

# Install mock CRDs
echo ""
echo "Installing mock CRDs..."
kubectl apply -f "$PROJECT_ROOT/config/crd/mocks/descheduler_crd.yaml"
kubectl apply -f "$PROJECT_ROOT/config/crd/mocks/machineconfig_crd.yaml"

echo "Waiting for CRDs to be established..."
kubectl wait --for condition=established --timeout=60s \
  crd/kubedeschedulers.operator.openshift.io \
  crd/machineconfigs.machineconfiguration.openshift.io

# Install baseline resources
echo ""
echo "Installing baseline resources..."
kubectl apply -f "$PROJECT_ROOT/config/samples/mock_baseline_resources.yaml"

echo ""
echo "Verifying installation..."
kubectl get crd | grep -E "(kubedeschedulers|machineconfigs)"
kubectl get kubedescheduler cluster -o yaml | head -20
kubectl get machineconfig 50-worker-psi-metrics -o yaml | head -20

echo ""
echo "===== Mock environment setup complete! ====="
echo ""
echo "You can now:"
echo "  1. Run the operator: make run"
echo "  2. Create a ConfigurationPlan: kubectl apply -f config/samples/loadaware_sample.yaml"
echo "  3. Watch the progress: kubectl get configurationplan -w"
