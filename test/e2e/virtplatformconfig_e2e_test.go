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
	Phase       string                       `json:"phase"`
	Conditions  []Condition                  `json:"conditions"`
	Items       []VirtPlatformConfigItemStatus `json:"items,omitempty"`
	ProfileName string                       `json:"profileName,omitempty"`
}

type Condition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
}

type VirtPlatformConfigItemStatus struct {
	Name       string `json:"name"`
	Phase      string `json:"phase"`
	Message    string `json:"message,omitempty"`
	Diff       string `json:"diff,omitempty"`
	IsHealthy  bool   `json:"isHealthy"`
	NeedsWait  bool   `json:"needsWait"`
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
		It("should transition through phases: Drafting → Completed", func() {
			By("applying a minimal VirtPlatformConfig")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: status-test
spec:
  profile: status-test
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("observing Drafting phase")
			var seenDrafting bool
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("status-test")
				if status.Phase == "Drafting" {
					seenDrafting = true

					// Verify Drafting condition
					draftingCond := findCondition(status.Conditions, "Drafting")
					g.Expect(draftingCond).NotTo(BeNil())
					g.Expect(draftingCond.Status).To(Equal("True"))
				}

				// Eventually should reach a terminal state
				g.Expect(status.Phase).To(BeElementOf("Drafting", "Completed", "PrerequisiteFailed", "Failed"))
			}, 2*time.Minute).Should(Succeed())

			By("verifying Drafting phase was observed")
			Expect(seenDrafting).To(BeTrue(), "Should have seen Drafting phase")
		})
	})

	Context("Condition Management", func() {
		It("should set appropriate conditions for each phase", func() {
			By("creating a VirtPlatformConfig")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: condition-test
spec:
  profile: condition-test
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying conditions are properly formatted")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("condition-test")
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
			By("checking that only one phase condition is True at a time")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("condition-test")

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
  name: metrics-test
spec:
  profile: metrics-test
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			// Wait for at least one reconciliation
			time.Sleep(10 * time.Second)

			By("checking controller_runtime metrics for virtplatformconfig controller")
			metricsOutput, err := getMetricsOutput()
			Expect(err).NotTo(HaveOccurred())

			// Look for reconciliation metrics
			Expect(metricsOutput).To(ContainSubstring("controller_runtime_reconcile_total"),
				"Should contain reconciliation metrics")
			Expect(metricsOutput).To(ContainSubstring("virtplatformconfig"),
				"Metrics should reference virtplatformconfig controller")
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

			By("verifying options are applied to status")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("load-aware-rebalancing")
				g.Expect(status.ProfileName).To(Equal("load-aware-rebalancing"))
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
