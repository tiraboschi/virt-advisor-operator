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

package controller

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"

	advisorv1alpha1 "github.com/kubevirt/virt-advisor-operator/api/v1alpha1"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	ctx       context.Context
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	var err error
	err = advisorv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Register core v1 scheme for Namespace operations
	err = corev1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Register apiextensions scheme for CRD operations
	err = apiextensionsv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By("creating openshift-cnv namespace for HyperConverged CR")
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "openshift-cnv",
		},
	}
	err = k8sClient.Create(ctx, ns)
	Expect(err).NotTo(HaveOccurred())

	By("loading HyperConverged CRD from file")
	hcoCRD := loadHyperConvergedCRDFromFile()
	err = k8sClient.Create(ctx, hcoCRD)
	Expect(err).NotTo(HaveOccurred())

	By("loading MachineConfig CRD from file")
	mcCRD := loadMachineConfigCRDFromFile()
	err = k8sClient.Create(ctx, mcCRD)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
//
// This function streamlines the process by finding the required binaries, similar to
// setting the 'KUBEBUILDER_ASSETS' environment variable. To ensure the binaries are
// properly set up, run 'make setup-envtest' beforehand.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}

// loadHyperConvergedCRDFromFile loads the HyperConverged CRD from the mocks directory
func loadHyperConvergedCRDFromFile() *apiextensionsv1.CustomResourceDefinition {
	crdPath := filepath.Join("..", "..", "config", "crd", "mocks", "hyperconverged_crd.yaml")
	crdBytes, err := os.ReadFile(crdPath)
	Expect(err).NotTo(HaveOccurred(), "Failed to read HyperConverged CRD file")

	var crd apiextensionsv1.CustomResourceDefinition
	err = yaml.Unmarshal(crdBytes, &crd)
	Expect(err).NotTo(HaveOccurred(), "Failed to unmarshal HyperConverged CRD")

	return &crd
}

// loadMachineConfigCRDFromFile loads the MachineConfig CRD from the mocks directory
func loadMachineConfigCRDFromFile() *apiextensionsv1.CustomResourceDefinition {
	crdPath := filepath.Join("..", "..", "config", "crd", "mocks", "machineconfig_crd.yaml")
	crdBytes, err := os.ReadFile(crdPath)
	Expect(err).NotTo(HaveOccurred(), "Failed to read MachineConfig CRD file")

	var crd apiextensionsv1.CustomResourceDefinition
	err = yaml.Unmarshal(crdBytes, &crd)
	Expect(err).NotTo(HaveOccurred(), "Failed to unmarshal MachineConfig CRD")

	return &crd
}
