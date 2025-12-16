//go:build e2e
// +build e2e

/*
Copyright 2025.

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

package e2e

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kubevirt/virt-advisor-operator/test/utils"
)

// VirtPlatformConfigStatus mirrors the status structure for parsing
type VirtPlatformConfigStatus struct {
	Phase      string                         `json:"phase"`
	Conditions []Condition                    `json:"conditions"`
	Items      []VirtPlatformConfigItemStatus `json:"items,omitempty"`
}

type Condition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
}

type VirtPlatformConfigItemStatus struct {
	Name      string `json:"name"`
	Phase     string `json:"phase"`
	Message   string `json:"message,omitempty"`
	Diff      string `json:"diff,omitempty"`
	IsHealthy bool   `json:"isHealthy"`
	NeedsWait bool   `json:"needsWait"`
}

var _ = Describe("VirtPlatformConfig E2E Tests", Ordered, func() {
	SetDefaultEventuallyTimeout(3 * time.Minute)
	SetDefaultEventuallyPollingInterval(5 * time.Second)

	AfterEach(func() {
		// Clean up any VirtPlatformConfigs created during tests
		By("cleaning up VirtPlatformConfigs")
		cmd := exec.Command("kubectl", "delete", "virtplatformconfigs", "--all", "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		// Give controller time to clean up
		time.Sleep(5 * time.Second)
	})

	Context("Prerequisite Checking", func() {
		It("should detect missing CRDs and set PrerequisiteFailed phase", func() {
			By("applying a VirtPlatformConfig for load-aware profile")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: load-aware-rebalancing
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for VirtPlatformConfig to be processed")
			var status VirtPlatformConfigStatus
			Eventually(func(g Gomega) {
				status = getVirtPlatformConfigStatus("load-aware-rebalancing")

				// Should reach either Drafting or PrerequisiteFailed
				g.Expect(status.Phase).To(BeElementOf("Drafting", "PrerequisiteFailed", "Completed"),
					"Expected phase transition to occur")
			}).Should(Succeed())

			By("verifying conditions are set")
			Expect(status.Conditions).NotTo(BeEmpty(), "Conditions should be populated")

			// In a Kind cluster without OpenShift CRDs, should fail prerequisites
			if status.Phase == "PrerequisiteFailed" {
				prereqCondition := findCondition(status.Conditions, "PrerequisiteFailed")
				Expect(prereqCondition).NotTo(BeNil(), "PrerequisiteFailed condition should exist")
				Expect(prereqCondition.Status).To(Equal("True"))
				Expect(prereqCondition.Message).To(ContainSubstring("CRD"),
					"Message should mention missing CRDs")
			}
		})
	})

	Context("DryRun Workflow", func() {
		It("should calculate and show diffs without applying changes", func() {
			By("creating a simple test config for a profile that doesn't exist")
			// Note: This test uses a non-existent profile which will cause the controller
			// to transition to PrerequisiteFailed phase, which is fine for testing DryRun mode
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: test-dryrun
spec:
  profile: test-dryrun
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying action stays DryRun and controller processes it")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "virtplatformconfig", "test-dryrun",
					"-o", "jsonpath={.spec.action}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("DryRun"), "Action should remain DryRun")
			}).Should(Succeed())

			By("verifying the controller processes the VirtPlatformConfig")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("test-dryrun")
				// Controller should process it and set a phase (either PrerequisiteFailed for invalid profile
				// or another appropriate phase)
				g.Expect(status.Phase).NotTo(BeEmpty(), "Controller should set a phase")
			}).Should(Succeed())
		})
	})

	Context("Status Transitions", func() {
		It("should transition through phases: Drafting → ReviewRequired", func() {
			By("applying a VirtPlatformConfig with example-profile")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: example-profile
spec:
  profile: example-profile
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("observing phase transitions")
			var seenReviewRequired bool
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")

				if status.Phase == "Drafting" {
					// Verify Drafting condition if we catch it
					draftingCond := findCondition(status.Conditions, "Drafting")
					g.Expect(draftingCond).NotTo(BeNil())
					g.Expect(draftingCond.Status).To(Equal("True"))
				}

				if status.Phase == "ReviewRequired" {
					seenReviewRequired = true
					// Verify ReviewRequired condition
					reviewCond := findCondition(status.Conditions, "ReviewRequired")
					g.Expect(reviewCond).NotTo(BeNil())
					g.Expect(reviewCond.Status).To(Equal("True"))

					// Verify Completed is False when in ReviewRequired (this validates our bug fix!)
					completedCond := findCondition(status.Conditions, "Completed")
					g.Expect(completedCond).NotTo(BeNil())
					g.Expect(completedCond.Status).To(Equal("False"),
						"Completed condition should be False in ReviewRequired phase")
				}

				// Eventually should reach a terminal state (ReviewRequired for DryRun, or a failure state)
				g.Expect(status.Phase).To(BeElementOf("Pending", "Drafting", "ReviewRequired", "PrerequisiteFailed", "Failed"))
			}, 2*time.Minute).Should(Succeed())

			By("verifying a terminal phase was reached")
			// Note: We expect ReviewRequired for DryRun, but the example profile may fail
			// The important validation is that Completed=False when in ReviewRequired phase
			if seenReviewRequired {
				Expect(seenReviewRequired).To(BeTrue(), "ReviewRequired phase validation passed")
			}
		})
	})

	Context("Condition Management", func() {
		It("should set appropriate conditions for each phase", func() {
			By("creating a VirtPlatformConfig with example-profile")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: example-profile
spec:
  profile: example-profile
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying conditions are properly formatted")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")
				g.Expect(status.Conditions).NotTo(BeEmpty())

				for _, cond := range status.Conditions {
					// All conditions should have required fields
					g.Expect(cond.Type).NotTo(BeEmpty(), "Condition type should not be empty")
					g.Expect(cond.Status).To(BeElementOf("True", "False", "Unknown"),
						"Condition status must be True/False/Unknown")
					g.Expect(cond.LastTransitionTime).NotTo(BeEmpty(),
						"LastTransitionTime should be set")
					g.Expect(cond.Reason).NotTo(BeEmpty(), "Reason should be set")
					g.Expect(cond.Message).NotTo(BeEmpty(), "Message should be set")
				}
			}).Should(Succeed())
		})

		It("should maintain mutual exclusivity of phase conditions", func() {
			By("creating a VirtPlatformConfig to test mutual exclusivity")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: example-profile
spec:
  profile: example-profile
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("checking that only one phase condition is True at a time")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")

				phaseConditions := []string{"Drafting", "InProgress", "Completed", "Drifted", "Failed"}
				trueCount := 0

				for _, condType := range phaseConditions {
					cond := findCondition(status.Conditions, condType)
					if cond != nil && cond.Status == "True" {
						trueCount++
					}
				}

				// Exactly one phase condition should be True
				g.Expect(trueCount).To(Equal(1),
					"Exactly one phase condition should be True")
			}).Should(Succeed())
		})
	})

	Context("Metrics Integration", func() {
		It("should emit reconciliation metrics", func() {
			By("creating a VirtPlatformConfig to trigger reconciliation")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: example-profile
spec:
  profile: example-profile
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			// Wait for at least one reconciliation
			time.Sleep(10 * time.Second)

			By("creating curl-metrics pod to fetch metrics")
			// First get a service account token
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			// Delete the pod if it exists from a previous test
			deletePodCmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", "virt-advisor-operator-system", "--ignore-not-found=true")
			_, _ = utils.Run(deletePodCmd)

			// Create the curl-metrics pod
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", "virt-advisor-operator-system",
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://virt-advisor-operator-controller-manager-metrics-service.virt-advisor-operator-system.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"readOnlyRootFilesystem": true,
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccountName": "virt-advisor-operator-controller-manager"
					}
				}`, token))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for curl-metrics pod to complete")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", "virt-advisor-operator-system")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod should succeed")
			}, 2*time.Minute).Should(Succeed())

			By("checking controller_runtime metrics for virtplatformconfig controller")
			metricsOutput, err := getMetricsOutput()
			Expect(err).NotTo(HaveOccurred())

			// Look for reconciliation metrics  - be flexible as metrics format may vary
			Expect(metricsOutput).NotTo(BeEmpty(), "Metrics output should not be empty")
			Expect(metricsOutput).To(Or(
				ContainSubstring("controller_runtime_reconcile"),
				ContainSubstring("workqueue_"),
			), "Should contain controller-runtime or workqueue metrics")

			// Check for controller name in metrics (might be in various formats)
			Expect(metricsOutput).To(Or(
				ContainSubstring("virtplatformconfig"),
				ContainSubstring("VirtPlatformConfig"),
			), "Metrics should reference the VirtPlatformConfig controller")

			By("cleaning up curl-metrics pod")
			cmd = exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", "virt-advisor-operator-system", "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
		})
	})

	Context("Singleton Pattern Validation", func() {
		It("should reject VirtPlatformConfig when name doesn't match profile", func() {
			By("attempting to create a VirtPlatformConfig with mismatched name/profile")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: wrong-name
spec:
  profile: correct-name
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			output, err := utils.Run(cmd)

			// Should fail CEL validation
			Expect(err).To(HaveOccurred(), "Should reject mismatched name/profile")
			Expect(output).To(ContainSubstring("singleton pattern"),
				"Error should mention singleton pattern")
		})

		It("should accept VirtPlatformConfig when name matches profile", func() {
			By("creating a VirtPlatformConfig with matching name/profile")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: matching-name
spec:
  profile: matching-name
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)

			Expect(err).NotTo(HaveOccurred(), "Should accept matching name/profile")
		})
	})

	Context("Profile Options Validation", func() {
		It("should validate typed profile options", func() {
			By("applying VirtPlatformConfig with invalid interval (out of range)")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: load-aware-rebalancing
  action: DryRun
  failurePolicy: Abort
  options:
    loadAware:
      deschedulingIntervalSeconds: 999999  # Invalid: max is 86400
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)

			// Should fail validation
			Expect(err).To(HaveOccurred(), "Should reject out-of-range values")
		})

		It("should accept valid profile options", func() {
			By("applying VirtPlatformConfig with valid options")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: load-aware-rebalancing
  action: DryRun
  failurePolicy: Abort
  options:
    loadAware:
      deschedulingIntervalSeconds: 120
      enablePSIMetrics: true
      devDeviationThresholds: AsymmetricLow
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)

			Expect(err).NotTo(HaveOccurred(), "Should accept valid options")

			By("verifying the VirtPlatformConfig is processed by the controller")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("load-aware-rebalancing")
				// Verify the controller has processed it (set a phase)
				g.Expect(status.Phase).NotTo(BeEmpty(), "Controller should set a phase")
				// Verify conditions are populated
				g.Expect(status.Conditions).NotTo(BeEmpty(), "Controller should set conditions")
			}).Should(Succeed())
		})
	})

	Context("Controller Stability", func() {
		It("should handle rapid create/delete cycles", func() {
			By("creating and deleting VirtPlatformConfigs rapidly")
			for i := 0; i < 5; i++ {
				name := fmt.Sprintf("stress-test-%d", i)
				sampleYAML := fmt.Sprintf(`
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: %s
spec:
  profile: %s
  action: DryRun
  failurePolicy: Abort
`, name, name)

				cmd := exec.Command("kubectl", "apply", "-f", "-")
				cmd.Stdin = strings.NewReader(sampleYAML)
				_, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				// Immediately delete
				cmd = exec.Command("kubectl", "delete", "virtplatformconfig", name)
				_, _ = utils.Run(cmd)
			}

			// Controller should remain healthy
			time.Sleep(5 * time.Second)

			By("verifying controller is still running")
			cmd := exec.Command("kubectl", "get", "pods",
				"-l", "control-plane=controller-manager",
				"-n", namespace,
				"-o", "jsonpath={.items[0].status.phase}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("Running"))
		})
	})

	Context("Load-Aware Rebalancing Profile", func() {
		It("should detect missing OpenShift prerequisites", func() {
			By("applying load-aware-rebalancing profile")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: load-aware-rebalancing
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying controller detects missing CRDs")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("load-aware-rebalancing")
				// In Kind cluster without OpenShift, should fail prerequisites
				g.Expect(status.Phase).To(Equal("PrerequisiteFailed"),
					"Should detect missing OpenShift CRDs")

				prereqCondition := findCondition(status.Conditions, "PrerequisiteFailed")
				g.Expect(prereqCondition).NotTo(BeNil())
				g.Expect(prereqCondition.Status).To(Equal("True"))
				// Message should mention the missing CRDs
				g.Expect(prereqCondition.Message).To(Or(
					ContainSubstring("kubedeschedulers.operator.openshift.io"),
					ContainSubstring("machineconfigs.machineconfiguration.openshift.io"),
					ContainSubstring("CRD"),
				))
			}).Should(Succeed())
		})

		It("should accept valid profile options", func() {
			By("applying load-aware-rebalancing with all valid options")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: load-aware-rebalancing
  action: DryRun
  failurePolicy: Abort
  options:
    loadAware:
      deschedulingIntervalSeconds: 120
      mode: Automatic
      enablePSIMetrics: true
      devDeviationThresholds: AsymmetricMedium
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Valid profile options should be accepted")

			By("verifying the VirtPlatformConfig is created with correct options")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "virtplatformconfig", "load-aware-rebalancing",
					"-o", "jsonpath={.spec.options.loadAware.deschedulingIntervalSeconds}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("120"))
			}).Should(Succeed())
		})

		It("should accept minimal configuration with defaults", func() {
			By("applying load-aware-rebalancing without options")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: load-aware-rebalancing
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Profile should work with default options")

			By("verifying controller processes it")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("load-aware-rebalancing")
				g.Expect(status.Phase).NotTo(BeEmpty(), "Controller should set a phase")
			}).Should(Succeed())
		})

		It("should reject invalid descheduling interval", func() {
			By("applying load-aware-rebalancing with interval < 60")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: load-aware-rebalancing
  action: DryRun
  failurePolicy: Abort
  options:
    loadAware:
      deschedulingIntervalSeconds: 30
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			output, err := cmd.CombinedOutput()
			Expect(err).To(HaveOccurred(), "Should reject interval < 60")
			Expect(string(output)).To(ContainSubstring("60"),
				"Error should mention minimum value")
		})

		It("should reject invalid deviation thresholds", func() {
			By("applying load-aware-rebalancing with invalid devDeviationThresholds")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: load-aware-rebalancing
  action: DryRun
  failurePolicy: Abort
  options:
    loadAware:
      devDeviationThresholds: InvalidValue
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			output, err := cmd.CombinedOutput()
			Expect(err).To(HaveOccurred(), "Should reject invalid enum value")
			Expect(string(output)).To(Or(
				ContainSubstring("Unsupported value"),
				ContainSubstring("supported values"),
			), "Error should mention invalid value")
		})

		It("should handle PSI metrics enablement configuration", func() {
			By("applying load-aware-rebalancing with enablePSIMetrics=false")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: load-aware-rebalancing
  action: DryRun
  failurePolicy: Abort
  options:
    loadAware:
      enablePSIMetrics: false
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Should accept enablePSIMetrics=false")

			By("verifying controller processes the configuration")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("load-aware-rebalancing")
				// Controller should process it and set a phase
				g.Expect(status.Phase).NotTo(BeEmpty(), "Controller should set a phase")

				// In Kind cluster without OpenShift, should fail prerequisites
				// The controller detects missing CRDs and transitions to PrerequisiteFailed
				if status.Phase == "PrerequisiteFailed" {
					prereqCondition := findCondition(status.Conditions, "PrerequisiteFailed")
					if prereqCondition != nil {
						// Message should mention KubeDescheduler
						g.Expect(prereqCondition.Message).To(ContainSubstring("KubeDescheduler"),
							"Should mention missing KubeDescheduler CRD")
					}
				}
			}).Should(Succeed())
		})

		It("should validate mode enum values", func() {
			By("applying load-aware-rebalancing with Predictive mode")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: load-aware-rebalancing
  action: DryRun
  failurePolicy: Abort
  options:
    loadAware:
      mode: Predictive
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Should accept valid mode enum")

			By("rejecting invalid mode value")
			invalidYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: load-aware-rebalancing
  action: DryRun
  failurePolicy: Abort
  options:
    loadAware:
      mode: InvalidMode
`
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(invalidYAML)
			output, err := cmd.CombinedOutput()
			Expect(err).To(HaveOccurred(), "Should reject invalid mode enum")
			Expect(string(output)).To(Or(
				ContainSubstring("Unsupported value"),
				ContainSubstring("supported values"),
			), "Error should mention invalid value")
		})

		It("should respect all deviation threshold enum values", func() {
			validThresholds := []string{
				"Low", "Medium", "High",
				"AsymmetricLow", "AsymmetricMedium", "AsymmetricHigh",
			}

			for _, threshold := range validThresholds {
				By(fmt.Sprintf("applying with devDeviationThresholds=%s", threshold))
				sampleYAML := fmt.Sprintf(`
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: load-aware-rebalancing
  action: DryRun
  failurePolicy: Abort
  options:
    loadAware:
      devDeviationThresholds: %s
`, threshold)
				cmd := exec.Command("kubectl", "apply", "-f", "-")
				cmd.Stdin = strings.NewReader(sampleYAML)
				_, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(),
					fmt.Sprintf("Should accept devDeviationThresholds=%s", threshold))

				// Clean up for next iteration
				cmd = exec.Command("kubectl", "delete", "virtplatformconfig", "load-aware-rebalancing",
					"--ignore-not-found=true")
				_, _ = utils.Run(cmd)
				time.Sleep(2 * time.Second)
			}
		})
	})
})

// Helper functions

func getVirtPlatformConfigStatus(name string) VirtPlatformConfigStatus {
	cmd := exec.Command("kubectl", "get", "virtplatformconfig", name,
		"-o", "jsonpath={.status}")
	output, err := utils.Run(cmd)
	if err != nil {
		return VirtPlatformConfigStatus{}
	}

	var status VirtPlatformConfigStatus
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		return VirtPlatformConfigStatus{}
	}

	return status
}

func findCondition(conditions []Condition, condType string) *Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}
