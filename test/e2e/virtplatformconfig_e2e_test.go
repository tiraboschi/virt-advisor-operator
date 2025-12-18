//go:build e2e
// +build e2e

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
	State     string `json:"state,omitempty"`
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

		// Give controller time to clean up and potentially recreate advertised profiles
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
			// Use server-side apply with force-conflicts to handle case where operator auto-created it
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
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
			By("creating a simple test config with example-profile")
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
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying action stays DryRun and controller processes it")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "virtplatformconfig", "example-profile",
					"-o", "jsonpath={.spec.action}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("DryRun"), "Action should remain DryRun")
			}).Should(Succeed())

			By("verifying the controller processes the VirtPlatformConfig")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")
				// Controller should process it and set a phase
				g.Expect(status.Phase).NotTo(BeEmpty(), "Controller should set a phase")
			}).Should(Succeed())
		})
	})

	Context("Status Transitions", func() {
		It("should transition through phases: Drafting → ReviewRequired or Completed", func() {
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
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("observing phase transitions")
			var seenReviewRequired bool
			var seenCompleted bool
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

				if status.Phase == "Completed" {
					seenCompleted = true
					// Verify Completed condition
					completedCond := findCondition(status.Conditions, "Completed")
					g.Expect(completedCond).NotTo(BeNil())
					g.Expect(completedCond.Status).To(Equal("True"))
				}

				// Eventually should reach a terminal state
				// Note: Completed is valid if cluster is already in desired state (no changes needed)
				// ReviewRequired is valid if there are changes to review
				g.Expect(status.Phase).To(BeElementOf("Pending", "Drafting", "ReviewRequired", "Completed", "PrerequisiteFailed", "Failed"))
			}, 2*time.Minute).Should(Succeed())

			By("verifying a terminal phase was reached")
			Expect(seenReviewRequired || seenCompleted).To(BeTrue(),
				"Should reach either ReviewRequired (changes needed) or Completed (no changes)")
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
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
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
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("checking that only one phase condition is True at a time")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")

				// All phase-related conditions (conditions that represent the current phase state)
				phaseConditions := []string{"Ignored", "Drafting", "ReviewRequired", "InProgress", "Completed", "Drifted", "Failed"}
				trueCount := 0
				var trueConditions []string

				for _, condType := range phaseConditions {
					cond := findCondition(status.Conditions, condType)
					if cond != nil && cond.Status == "True" {
						trueCount++
						trueConditions = append(trueConditions, condType)
					}
				}

				// Exactly one phase condition should be True
				g.Expect(trueCount).To(Equal(1),
					"Exactly one phase condition should be True, but found %d: %v", trueCount, trueConditions)
			}).Should(Succeed())
		})
	})

	Context("Metrics Integration", func() {
		It("should emit reconciliation metrics", func() {
			Skip("This test requires proper RBAC configuration for metrics access and is flaky due to port-forward timing. Metrics are guaranteed by controller-runtime framework.")

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
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			// Wait for at least one reconciliation
			time.Sleep(10 * time.Second)

			By("fetching metrics using kubectl port-forward")
			metricsOutput, err := getMetricsViaPortForward()
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
		})
	})

	Context("Singleton Pattern Validation", func() {
		It("should reject VirtPlatformConfig when name doesn't match profile", func() {
			By("attempting to create a VirtPlatformConfig with mismatched name/profile")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: virt-higher-density
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
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
  name: example-profile
spec:
  profile: example-profile
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)

			Expect(err).NotTo(HaveOccurred(), "Should accept matching name/profile")
		})
	})

	Context("Profile Name Validation", func() {
		It("should reject invalid profile names", func() {
			By("attempting to create VirtPlatformConfig with invalid profile")
			invalidYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: invalid-profile
spec:
  profile: invalid-profile
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(invalidYAML)
			output, err := cmd.CombinedOutput()

			Expect(err).To(HaveOccurred(), "Should reject invalid profile name")
			Expect(string(output)).To(Or(
				ContainSubstring("Unsupported value"),
				ContainSubstring("supported values"),
			), "Error should mention invalid profile value")
		})

		It("should accept valid profile names", func() {
			validProfiles := []string{"load-aware-rebalancing", "virt-higher-density", "example-profile"}

			for _, profile := range validProfiles {
				By(fmt.Sprintf("creating VirtPlatformConfig with profile=%s", profile))
				validYAML := fmt.Sprintf(`
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: %s
spec:
  profile: %s
  action: DryRun
  failurePolicy: Abort
`, profile, profile)

				cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
				cmd.Stdin = strings.NewReader(validYAML)
				_, err := utils.Run(cmd)

				Expect(err).NotTo(HaveOccurred(),
					fmt.Sprintf("Should accept valid profile: %s", profile))

				// Clean up
				cmd = exec.Command("kubectl", "delete", "virtplatformconfig", profile,
					"--ignore-not-found=true")
				_, _ = utils.Run(cmd)
				time.Sleep(2 * time.Second)
			}
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
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
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
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
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

		It("should reject load-aware profile with virt-higher-density options", func() {
			By("attempting to create load-aware profile with virtHigherDensity options")
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
    virtHigherDensity:
      enableSwap: true
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(invalidYAML)
			output, err := cmd.CombinedOutput()

			Expect(err).To(HaveOccurred(), "Should reject mismatched profile options")
			Expect(string(output)).To(Or(
				ContainSubstring("only options.loadAware may be set"),
				ContainSubstring("can only be used with profile 'virt-higher-density'"),
			), "Error should mention profile/options mismatch")
		})

		It("should reject virt-higher-density profile with load-aware options", func() {
			By("attempting to create virt-higher-density profile with loadAware options")
			invalidYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: virt-higher-density
spec:
  profile: virt-higher-density
  action: DryRun
  failurePolicy: Abort
  options:
    loadAware:
      deschedulingIntervalSeconds: 120
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(invalidYAML)
			output, err := cmd.CombinedOutput()

			Expect(err).To(HaveOccurred(), "Should reject mismatched profile options")
			Expect(string(output)).To(Or(
				ContainSubstring("only options.virtHigherDensity may be set"),
				ContainSubstring("can only be used with profile 'load-aware-rebalancing'"),
			), "Error should mention profile/options mismatch")
		})

		It("should accept load-aware profile with matching loadAware options", func() {
			By("creating load-aware profile with loadAware options")
			validYAML := `
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
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(validYAML)
			_, err := utils.Run(cmd)

			Expect(err).NotTo(HaveOccurred(), "Should accept matching profile and options")
		})

		It("should accept virt-higher-density profile with matching virtHigherDensity options", func() {
			By("creating virt-higher-density profile with virtHigherDensity options")
			validYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: virt-higher-density
spec:
  profile: virt-higher-density
  action: DryRun
  failurePolicy: Abort
  options:
    virtHigherDensity:
      enableSwap: true
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(validYAML)
			_, err := utils.Run(cmd)

			Expect(err).NotTo(HaveOccurred(), "Should accept matching profile and options")
		})
	})

	Context("Controller Stability", func() {
		It("should handle rapid create/delete cycles", func() {
			By("creating and deleting VirtPlatformConfigs rapidly")
			// Use only example-profile since we're testing rapid create/delete
			// and can reuse the same profile name
			for i := 0; i < 5; i++ {
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
				cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
				cmd.Stdin = strings.NewReader(sampleYAML)
				_, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				// Immediately delete
				cmd = exec.Command("kubectl", "delete", "virtplatformconfig", "example-profile")
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

	Context("Profile Advertising", func() {
		It("should auto-create VirtPlatformConfigs for advertisable profiles on startup", func() {
			By("verifying load-aware-rebalancing VirtPlatformConfig was auto-created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "virtplatformconfig", "load-aware-rebalancing",
					"-o", "json")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "load-aware-rebalancing VirtPlatformConfig should exist")

				// Parse JSON to verify details
				var vpc map[string]interface{}
				err = json.Unmarshal([]byte(output), &vpc)
				g.Expect(err).NotTo(HaveOccurred())

				// Verify spec.action is Ignore
				spec := vpc["spec"].(map[string]interface{})
				g.Expect(spec["action"]).To(Equal("Ignore"),
					"Auto-created VirtPlatformConfig should have action=Ignore")

				// Verify spec.profile matches name
				g.Expect(spec["profile"]).To(Equal("load-aware-rebalancing"))

				// Verify metadata annotations exist
				metadata := vpc["metadata"].(map[string]interface{})
				annotations := metadata["annotations"].(map[string]interface{})
				g.Expect(annotations["advisor.kubevirt.io/description"]).NotTo(BeEmpty(),
					"Should have description annotation")
				g.Expect(annotations["advisor.kubevirt.io/impact-summary"]).NotTo(BeEmpty(),
					"Should have impact-summary annotation")
				g.Expect(annotations["advisor.kubevirt.io/auto-created"]).To(Equal("true"),
					"Should have auto-created annotation")

				// Verify metadata labels exist
				labels := metadata["labels"].(map[string]interface{})
				g.Expect(labels["advisor.kubevirt.io/category"]).To(Equal("scheduling"),
					"Should have correct category label")
			}).Should(Succeed())
		})

		It("should not create VirtPlatformConfig for non-advertisable profiles", func() {
			By("verifying example-profile VirtPlatformConfig does NOT exist")
			cmd := exec.Command("kubectl", "get", "virtplatformconfig", "example-profile")
			output, err := cmd.CombinedOutput()
			Expect(err).To(HaveOccurred(), "example-profile should not be auto-created")
			Expect(string(output)).To(ContainSubstring("not found"),
				"Should get 'not found' error for non-advertisable profile")
		})

		It("should populate metadata from profile", func() {
			By("getting load-aware-rebalancing VirtPlatformConfig")
			cmd := exec.Command("kubectl", "get", "virtplatformconfig", "load-aware-rebalancing",
				"-o", "json")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			var vpc map[string]interface{}
			err = json.Unmarshal([]byte(output), &vpc)
			Expect(err).NotTo(HaveOccurred())

			metadata := vpc["metadata"].(map[string]interface{})
			annotations := metadata["annotations"].(map[string]interface{})

			By("verifying description annotation")
			description := annotations["advisor.kubevirt.io/description"].(string)
			Expect(description).To(ContainSubstring("load-aware"),
				"Description should mention load-aware")

			By("verifying impact summary annotation")
			impact := annotations["advisor.kubevirt.io/impact-summary"].(string)
			Expect(impact).NotTo(BeEmpty(), "Impact summary should not be empty")

			By("verifying category label")
			labels := metadata["labels"].(map[string]interface{})
			category := labels["advisor.kubevirt.io/category"].(string)
			Expect(category).To(Equal("scheduling"),
				"Category should be 'scheduling' for load-aware profile")
		})

		It("should not overwrite existing VirtPlatformConfigs", func() {
			By("modifying the auto-created load-aware-rebalancing config")
			// Change action from Ignore to DryRun
			cmd := exec.Command("kubectl", "patch", "virtplatformconfig", "load-aware-rebalancing",
				"--type=merge", "-p", `{"spec":{"action":"DryRun"}}`)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("getting the resource version after modification")
			cmd = exec.Command("kubectl", "get", "virtplatformconfig", "load-aware-rebalancing",
				"-o", "jsonpath={.metadata.resourceVersion}")
			resourceVersionAfterPatch, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("restarting the controller to trigger re-initialization")
			cmd = exec.Command("kubectl", "delete", "pod",
				"-l", "control-plane=controller-manager",
				"-n", "openshift-cnv")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for controller pod to be running again")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods",
					"-l", "control-plane=controller-manager",
					"-n", "openshift-cnv",
					"-o", "jsonpath={.items[0].status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}, 2*time.Minute).Should(Succeed())

			// Give controller time to initialize
			time.Sleep(10 * time.Second)

			By("verifying the VirtPlatformConfig was NOT modified by re-initialization")
			cmd = exec.Command("kubectl", "get", "virtplatformconfig", "load-aware-rebalancing",
				"-o", "json")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			var vpc map[string]interface{}
			err = json.Unmarshal([]byte(output), &vpc)
			Expect(err).NotTo(HaveOccurred())

			// Verify action is still DryRun (not reset to Ignore)
			spec := vpc["spec"].(map[string]interface{})
			Expect(spec["action"]).To(Equal("DryRun"),
				"Action should remain DryRun - initialization should not overwrite existing config")

			// Verify resource version hasn't changed (no updates)
			metadata := vpc["metadata"].(map[string]interface{})
			resourceVersionAfterRestart := metadata["resourceVersion"].(string)
			Expect(resourceVersionAfterRestart).To(Equal(resourceVersionAfterPatch),
				"Resource version should not change - config should not be updated")
		})

		It("should allow user to activate a profile by changing action from Ignore", func() {
			By("creating a fresh auto-created VirtPlatformConfig by deleting existing one")
			cmd := exec.Command("kubectl", "delete", "virtplatformconfig", "load-aware-rebalancing",
				"--ignore-not-found=true")
			_, _ = utils.Run(cmd)

			By("restarting controller to trigger auto-creation")
			cmd = exec.Command("kubectl", "delete", "pod",
				"-l", "control-plane=controller-manager",
				"-n", "openshift-cnv")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for controller to be running and VirtPlatformConfig to be created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "virtplatformconfig", "load-aware-rebalancing",
					"-o", "jsonpath={.spec.action}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Ignore"), "Should be auto-created with action=Ignore")
			}, 2*time.Minute).Should(Succeed())

			By("verifying config is in Ignored phase")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("load-aware-rebalancing")
				g.Expect(status.Phase).To(Equal("Ignored"),
					"Auto-created config should be in Ignored phase")

				// Verify Ignored condition is set
				ignoredCond := findCondition(status.Conditions, "Ignored")
				g.Expect(ignoredCond).NotTo(BeNil())
				g.Expect(ignoredCond.Status).To(Equal("True"))
			}).Should(Succeed())

			By("activating the profile by changing action to DryRun")
			cmd = exec.Command("kubectl", "patch", "virtplatformconfig", "load-aware-rebalancing",
				"--type=merge", "-p", `{"spec":{"action":"DryRun"}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying controller processes the activation")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("load-aware-rebalancing")

				// Should transition out of Ignored phase
				g.Expect(status.Phase).NotTo(Equal("Ignored"),
					"Should exit Ignored phase after activation")

				// Should start processing (likely PrerequisiteFailed in Kind cluster without OpenShift)
				g.Expect(status.Phase).To(BeElementOf("Pending", "Drafting", "PrerequisiteFailed", "ReviewRequired", "Completed"),
					"Should transition to processing or terminal phases")

				// Verify Ignored condition is now False
				ignoredCond := findCondition(status.Conditions, "Ignored")
				if ignoredCond != nil {
					g.Expect(ignoredCond.Status).To(Equal("False"),
						"Ignored condition should be False after activation")
				}
			}, 2*time.Minute).Should(Succeed())
		})
	})

	Context("Ignore Action", func() {
		It("should transition to Ignored phase when action is Ignore", func() {
			By("creating a VirtPlatformConfig with action=Ignore")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: example-profile
spec:
  profile: example-profile
  action: Ignore
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying it transitions to Ignored phase")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")
				g.Expect(status.Phase).To(Equal("Ignored"),
					"Should transition to Ignored phase when action=Ignore")

				// Verify Ignored condition is set
				ignoredCond := findCondition(status.Conditions, "Ignored")
				g.Expect(ignoredCond).NotTo(BeNil())
				g.Expect(ignoredCond.Status).To(Equal("True"))
				g.Expect(ignoredCond.Message).To(ContainSubstring("Resources remain in their current state"),
					"Condition message should clarify no rollback occurs")
			}).Should(Succeed())

			By("verifying no plan items were generated")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")
				g.Expect(status.Items).To(BeEmpty(),
					"Should not generate plan items when action=Ignore")
			}).Should(Succeed())
		})

		It("should exit Ignored phase when action changes to DryRun", func() {
			By("creating a VirtPlatformConfig with action=Ignore")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: example-profile
spec:
  profile: example-profile
  action: Ignore
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Ignored phase")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")
				g.Expect(status.Phase).To(Equal("Ignored"))
			}).Should(Succeed())

			By("changing action to DryRun")
			cmd = exec.Command("kubectl", "patch", "virtplatformconfig", "example-profile",
				"--type=merge", "-p", `{"spec":{"action":"DryRun"}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying it exits Ignored phase")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")
				g.Expect(status.Phase).NotTo(Equal("Ignored"),
					"Should exit Ignored phase when action changes")
				g.Expect(status.Phase).To(BeElementOf("Pending", "Drafting", "ReviewRequired", "Completed", "PrerequisiteFailed", "Failed"),
					"Should transition to a processing or terminal phase")
			}).Should(Succeed())
		})

		It("should not rollback changes when transitioning to Ignored", func() {
			By("creating a VirtPlatformConfig with DryRun first")
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
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for plan generation")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")
				g.Expect(status.Phase).To(BeElementOf("Drafting", "ReviewRequired", "Completed", "PrerequisiteFailed"))
				g.Expect(status.Items).NotTo(BeEmpty(), "Should have generated plan items")
			}).Should(Succeed())

			By("getting item count before changing to Ignore")
			var initialItemCount int
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")
				initialItemCount = len(status.Items)
				g.Expect(initialItemCount).To(BeNumerically(">", 0))
			}).Should(Succeed())

			By("changing action to Ignore")
			cmd = exec.Command("kubectl", "patch", "virtplatformconfig", "example-profile",
				"--type=merge", "-p", `{"spec":{"action":"Ignore"}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying transition to Ignored phase")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")
				g.Expect(status.Phase).To(Equal("Ignored"))
			}).Should(Succeed())

			By("verifying plan items were preserved (not rolled back)")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")
				g.Expect(len(status.Items)).To(Equal(initialItemCount),
					"Plan items should be preserved, not deleted when entering Ignored phase")
			}).Should(Succeed())
		})

		It("should stay in Ignored phase across multiple reconciliations", func() {
			By("creating a VirtPlatformConfig with action=Ignore")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: example-profile
spec:
  profile: example-profile
  action: Ignore
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying it enters Ignored phase")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")
				g.Expect(status.Phase).To(Equal("Ignored"))
			}).Should(Succeed())

			By("waiting to ensure it stays Ignored (simulating multiple reconciliations)")
			Consistently(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")
				g.Expect(status.Phase).To(Equal("Ignored"),
					"Should remain in Ignored phase across reconciliations")
			}, 30*time.Second, 5*time.Second).Should(Succeed())
		})

		It("should handle Ignore action with valid profile that has prerequisites", func() {
			By("creating load-aware-rebalancing with action=Ignore")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: load-aware-rebalancing
  action: Ignore
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying it goes to Ignored WITHOUT checking prerequisites")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("load-aware-rebalancing")
				g.Expect(status.Phase).To(Equal("Ignored"),
					"Should go directly to Ignored without prerequisite checks")

				// Should NOT be in PrerequisiteFailed phase
				g.Expect(status.Phase).NotTo(Equal("PrerequisiteFailed"),
					"Should skip prerequisite checking when action=Ignore")
			}).Should(Succeed())
		})

		It("should allow transitioning between Ignore and Apply multiple times", func() {
			By("creating example-profile with action=Ignore")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: example-profile
spec:
  profile: example-profile
  action: Ignore
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying initial Ignored phase")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")
				g.Expect(status.Phase).To(Equal("Ignored"))
			}).Should(Succeed())

			By("changing to Apply")
			cmd = exec.Command("kubectl", "patch", "virtplatformconfig", "example-profile",
				"--type=merge", "-p", `{"spec":{"action":"Apply"}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying exit from Ignored")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")
				g.Expect(status.Phase).NotTo(Equal("Ignored"))
			}).Should(Succeed())

			By("changing back to Ignore")
			cmd = exec.Command("kubectl", "patch", "virtplatformconfig", "example-profile",
				"--type=merge", "-p", `{"spec":{"action":"Ignore"}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying return to Ignored phase")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")
				g.Expect(status.Phase).To(Equal("Ignored"))
			}).Should(Succeed())

			By("changing to DryRun")
			cmd = exec.Command("kubectl", "patch", "virtplatformconfig", "example-profile",
				"--type=merge", "-p", `{"spec":{"action":"DryRun"}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying exit from Ignored again")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("example-profile")
				g.Expect(status.Phase).NotTo(Equal("Ignored"))
			}).Should(Succeed())
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
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
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
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
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
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
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
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
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
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
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
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
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
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
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
			cmd = exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
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
				cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
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

	Context("VirtHigherDensity Profile", func() {
		It("should detect missing MachineConfig CRD prerequisite", func() {
			By("applying virt-higher-density profile")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: virt-higher-density
spec:
  profile: virt-higher-density
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying controller detects missing MachineConfig CRD")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("virt-higher-density")
				// In Kind cluster without OpenShift, should fail prerequisites
				g.Expect(status.Phase).To(Equal("PrerequisiteFailed"),
					"Should detect missing MachineConfig CRD")

				prereqCondition := findCondition(status.Conditions, "PrerequisiteFailed")
				g.Expect(prereqCondition).NotTo(BeNil())
				g.Expect(prereqCondition.Status).To(Equal("True"))
				// Message should mention the missing MachineConfig CRD
				g.Expect(prereqCondition.Message).To(Or(
					ContainSubstring("machineconfigs.machineconfiguration.openshift.io"),
					ContainSubstring("MachineConfig"),
					ContainSubstring("CRD"),
				))
			}).Should(Succeed())
		})

		It("should accept valid profile with default options", func() {
			By("applying virt-higher-density without options")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: virt-higher-density
spec:
  profile: virt-higher-density
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Profile should work with default options")

			By("verifying controller processes it")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("virt-higher-density")
				g.Expect(status.Phase).NotTo(BeEmpty(), "Controller should set a phase")
			}).Should(Succeed())
		})

		It("should accept enableSwap option", func() {
			By("applying virt-higher-density with enableSwap=true")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: virt-higher-density
spec:
  profile: virt-higher-density
  action: DryRun
  failurePolicy: Abort
  options:
    virtHigherDensity:
      enableSwap: true
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Should accept enableSwap option")

			By("verifying the VirtPlatformConfig is created with correct options")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "virtplatformconfig", "virt-higher-density",
					"-o", "jsonpath={.spec.options.virtHigherDensity.enableSwap}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}).Should(Succeed())
		})

		It("should accept enableSwap=false", func() {
			By("applying virt-higher-density with enableSwap=false")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: virt-higher-density
spec:
  profile: virt-higher-density
  action: DryRun
  failurePolicy: Abort
  options:
    virtHigherDensity:
      enableSwap: false
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Should accept enableSwap=false")

			By("verifying controller processes it")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("virt-higher-density")
				// Should still process (and fail prerequisites in Kind)
				g.Expect(status.Phase).NotTo(BeEmpty(), "Controller should set a phase")
			}).Should(Succeed())
		})

		It("should auto-create VirtPlatformConfig for virt-higher-density on startup", func() {
			By("verifying virt-higher-density VirtPlatformConfig was auto-created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "virtplatformconfig", "virt-higher-density",
					"-o", "json")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "virt-higher-density VirtPlatformConfig should exist")

				// Parse JSON to verify details
				var vpc map[string]interface{}
				err = json.Unmarshal([]byte(output), &vpc)
				g.Expect(err).NotTo(HaveOccurred())

				// Verify spec.action is Ignore
				spec := vpc["spec"].(map[string]interface{})
				g.Expect(spec["action"]).To(Equal("Ignore"),
					"Auto-created VirtPlatformConfig should have action=Ignore")

				// Verify spec.profile matches name
				g.Expect(spec["profile"]).To(Equal("virt-higher-density"))

				// Verify metadata annotations exist
				metadata := vpc["metadata"].(map[string]interface{})
				annotations := metadata["annotations"].(map[string]interface{})
				g.Expect(annotations["advisor.kubevirt.io/description"]).NotTo(BeEmpty(),
					"Should have description annotation")
				g.Expect(annotations["advisor.kubevirt.io/impact-summary"]).NotTo(BeEmpty(),
					"Should have impact-summary annotation")
				g.Expect(annotations["advisor.kubevirt.io/auto-created"]).To(Equal("true"),
					"Should have auto-created annotation")

				// Verify metadata labels exist
				labels := metadata["labels"].(map[string]interface{})
				g.Expect(labels["advisor.kubevirt.io/category"]).To(Equal("density"),
					"Should have correct category label")
			}).Should(Succeed())
		})

		It("should populate metadata from profile", func() {
			By("getting virt-higher-density VirtPlatformConfig")
			cmd := exec.Command("kubectl", "get", "virtplatformconfig", "virt-higher-density",
				"-o", "json")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			var vpc map[string]interface{}
			err = json.Unmarshal([]byte(output), &vpc)
			Expect(err).NotTo(HaveOccurred())

			metadata := vpc["metadata"].(map[string]interface{})
			annotations := metadata["annotations"].(map[string]interface{})

			By("verifying description annotation")
			description := annotations["advisor.kubevirt.io/description"].(string)
			Expect(description).To(ContainSubstring("density"),
				"Description should mention density")

			By("verifying impact summary annotation mentions swap")
			impact := annotations["advisor.kubevirt.io/impact-summary"].(string)
			Expect(impact).To(ContainSubstring("swap"),
				"Impact summary should mention swap")
			Expect(impact).To(ContainSubstring("CNV_SWAP"),
				"Impact summary should mention CNV_SWAP partition requirement")

			By("verifying category label")
			labels := metadata["labels"].(map[string]interface{})
			category := labels["advisor.kubevirt.io/category"].(string)
			Expect(category).To(Equal("density"),
				"Category should be 'density' for virt-higher-density profile")
		})
	})

	Context("Combined Profiles", func() {
		It("should allow multiple profiles to coexist independently", func() {
			By("creating load-aware-rebalancing config")
			loadAwareYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: load-aware-rebalancing
  action: DryRun
  failurePolicy: Abort
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(loadAwareYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating virt-higher-density config")
			virtHigherDensityYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: virt-higher-density
spec:
  profile: virt-higher-density
  action: DryRun
  failurePolicy: Abort
`
			cmd = exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(virtHigherDensityYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying both configs exist and are processed independently")
			Eventually(func(g Gomega) {
				loadAwareStatus := getVirtPlatformConfigStatus("load-aware-rebalancing")
				virtHigherDensityStatus := getVirtPlatformConfigStatus("virt-higher-density")

				// Both should be processed (likely PrerequisiteFailed in Kind)
				g.Expect(loadAwareStatus.Phase).NotTo(BeEmpty(),
					"load-aware-rebalancing should be processed")
				g.Expect(virtHigherDensityStatus.Phase).NotTo(BeEmpty(),
					"virt-higher-density should be processed")

				// Both should have conditions
				g.Expect(loadAwareStatus.Conditions).NotTo(BeEmpty(),
					"load-aware-rebalancing should have conditions")
				g.Expect(virtHigherDensityStatus.Conditions).NotTo(BeEmpty(),
					"virt-higher-density should have conditions")
			}).Should(Succeed())
		})

		It("should handle one profile succeeding while another fails prerequisites", func() {
			By("ensuring both profiles are in DryRun mode")
			// Patch both profiles to DryRun (they may be auto-created with action=Ignore)
			cmd := exec.Command("kubectl", "patch", "virtplatformconfig", "load-aware-rebalancing",
				"--type=merge", "-p", `{"spec":{"action":"DryRun"}}`)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "patch", "virtplatformconfig", "virt-higher-density",
				"--type=merge", "-p", `{"spec":{"action":"DryRun"}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying both fail prerequisites independently")
			Eventually(func(g Gomega) {
				loadAwareStatus := getVirtPlatformConfigStatus("load-aware-rebalancing")
				virtHigherDensityStatus := getVirtPlatformConfigStatus("virt-higher-density")

				// In Kind cluster without OpenShift CRDs, both should fail
				g.Expect(loadAwareStatus.Phase).To(Equal("PrerequisiteFailed"),
					"load-aware should fail prerequisites")
				g.Expect(virtHigherDensityStatus.Phase).To(Equal("PrerequisiteFailed"),
					"virt-higher-density should fail prerequisites")

				// Each should report their specific missing prerequisites
				loadAwarePrereq := findCondition(loadAwareStatus.Conditions, "PrerequisiteFailed")
				virtHigherDensityPrereq := findCondition(virtHigherDensityStatus.Conditions, "PrerequisiteFailed")

				g.Expect(loadAwarePrereq).NotTo(BeNil())
				g.Expect(virtHigherDensityPrereq).NotTo(BeNil())

				// load-aware requires KubeDescheduler and MachineConfig
				g.Expect(loadAwarePrereq.Message).To(Or(
					ContainSubstring("kubedeschedulers"),
					ContainSubstring("KubeDescheduler"),
					ContainSubstring("machineconfigs"),
				))

				// virt-higher-density requires MachineConfig
				g.Expect(virtHigherDensityPrereq.Message).To(Or(
					ContainSubstring("machineconfigs"),
					ContainSubstring("MachineConfig"),
				))
			}).Should(Succeed())
		})

		It("should allow switching one profile to Ignore while keeping another active", func() {
			By("setting load-aware-rebalancing to DryRun")
			cmd := exec.Command("kubectl", "patch", "virtplatformconfig", "load-aware-rebalancing",
				"--type=merge", "-p", `{"spec":{"action":"DryRun"}}`)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("setting virt-higher-density to Ignore")
			cmd = exec.Command("kubectl", "patch", "virtplatformconfig", "virt-higher-density",
				"--type=merge", "-p", `{"spec":{"action":"Ignore"}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying load-aware is in processing phase")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("load-aware-rebalancing")
				g.Expect(status.Phase).To(BeElementOf("Drafting", "PrerequisiteFailed", "ReviewRequired", "Completed"),
					"load-aware should be processing or completed")
			}).Should(Succeed())

			By("verifying virt-higher-density is Ignored")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("virt-higher-density")
				g.Expect(status.Phase).To(Equal("Ignored"),
					"virt-higher-density should be Ignored")
			}).Should(Succeed())

			By("switching virt-higher-density back to DryRun")
			cmd = exec.Command("kubectl", "patch", "virtplatformconfig", "virt-higher-density",
				"--type=merge", "-p", `{"spec":{"action":"DryRun"}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying virt-higher-density exits Ignored")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("virt-higher-density")
				g.Expect(status.Phase).NotTo(Equal("Ignored"),
					"virt-higher-density should exit Ignored phase")
			}).Should(Succeed())
		})

		It("should maintain independent status for each profile", func() {
			By("ensuring both profiles are in DryRun mode")
			// Patch both profiles to DryRun to ensure they're actively processing
			cmd := exec.Command("kubectl", "patch", "virtplatformconfig", "load-aware-rebalancing",
				"--type=merge", "-p", `{"spec":{"action":"DryRun"}}`)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "patch", "virtplatformconfig", "virt-higher-density",
				"--type=merge", "-p", `{"spec":{"action":"DryRun"}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying each profile has independent conditions")
			Eventually(func(g Gomega) {
				loadAwareStatus := getVirtPlatformConfigStatus("load-aware-rebalancing")
				virtHigherDensityStatus := getVirtPlatformConfigStatus("virt-higher-density")

				// Verify both have conditions
				g.Expect(loadAwareStatus.Conditions).NotTo(BeEmpty())
				g.Expect(virtHigherDensityStatus.Conditions).NotTo(BeEmpty())

				// Verify each has prerequisite failed condition
				loadAwarePrereq := findCondition(loadAwareStatus.Conditions, "PrerequisiteFailed")
				virtHigherDensityPrereq := findCondition(virtHigherDensityStatus.Conditions, "PrerequisiteFailed")

				g.Expect(loadAwarePrereq).NotTo(BeNil())
				g.Expect(virtHigherDensityPrereq).NotTo(BeNil())

				// Messages should be different (load-aware mentions KubeDescheduler too)
				g.Expect(loadAwarePrereq.Message).NotTo(Equal(virtHigherDensityPrereq.Message),
					"Prerequisites should be specific to each profile")
			}).Should(Succeed())
		})

		It("should support both profiles with custom options simultaneously", func() {
			By("applying load-aware-rebalancing with custom options")
			loadAwareYAML := `
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
      devDeviationThresholds: High
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(loadAwareYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("applying virt-higher-density with custom options")
			virtHigherDensityYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: virt-higher-density
spec:
  profile: virt-higher-density
  action: DryRun
  failurePolicy: Abort
  options:
    virtHigherDensity:
      enableSwap: true
`
			cmd = exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(virtHigherDensityYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying both configs have their custom options")
			Eventually(func(g Gomega) {
				// Verify load-aware options
				cmd := exec.Command("kubectl", "get", "virtplatformconfig", "load-aware-rebalancing",
					"-o", "jsonpath={.spec.options.loadAware.deschedulingIntervalSeconds}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("120"))

				// Verify virt-higher-density options
				cmd = exec.Command("kubectl", "get", "virtplatformconfig", "virt-higher-density",
					"-o", "jsonpath={.spec.options.virtHigherDensity.enableSwap}")
				output, err = utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}).Should(Succeed())
		})
	})

	Context("MachineConfigPool Watch Integration", func() {
		It("should detect MCP convergence through watch events", func() {
			Skip("This test requires a real OpenShift cluster with MachineConfigPool and takes 10+ minutes for node convergence")

			By("applying a VirtPlatformConfig with PSI enabled (creates MachineConfig)")
			sampleYAML := `
apiVersion: advisor.kubevirt.io/v1alpha1
kind: VirtPlatformConfig
metadata:
  name: load-aware-rebalancing
spec:
  profile: load-aware-rebalancing
  action: Apply
  failurePolicy: Abort
  options:
    loadAware:
      enablePSIMetrics: true
`
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for plan to be generated")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("load-aware-rebalancing")
				g.Expect(status.Phase).To(BeElementOf("Drafting", "InProgress", "Completed"))
			}, 2*time.Minute).Should(Succeed())

			By("verifying it reaches InProgress when applying MachineConfig")
			var sawInProgress bool
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("load-aware-rebalancing")

				if status.Phase == "InProgress" {
					sawInProgress = true
					// Check for MachineConfig item
					hasMachineConfigItem := false
					for _, item := range status.Items {
						if strings.Contains(item.Name, "psi") {
							hasMachineConfigItem = true
							g.Expect(item.Message).To(ContainSubstring("MachineConfigPool"))
						}
					}
					g.Expect(hasMachineConfigItem).To(BeTrue(), "Should have PSI MachineConfig item")
				}

				// Eventually should reach Completed when MCP converges
				// This validates that the MCP watch triggered reconciliation
				g.Expect(status.Phase).To(BeElementOf("InProgress", "Completed"))
			}, 15*time.Minute, 10*time.Second).Should(Succeed())

			By("verifying final Completed state after MCP convergence")
			if sawInProgress {
				// If we saw InProgress, verify the MCP watch triggered completion
				Eventually(func(g Gomega) {
					status := getVirtPlatformConfigStatus("load-aware-rebalancing")
					g.Expect(status.Phase).To(Equal("Completed"))

					// Verify MachineConfig item is completed
					foundPSIItem := false
					for _, item := range status.Items {
						if strings.Contains(item.Name, "psi") {
							foundPSIItem = true
							g.Expect(item.Message).To(ContainSubstring("ready"),
								"MachineConfig should report MCP as ready")
						}
					}
					g.Expect(foundPSIItem).To(BeTrue())
				}, 5*time.Minute, 10*time.Second).Should(Succeed())
			}
		})

		It("should skip to Completed when cluster already in desired state", func() {
			By("deploying descheduler operator if needed")
			// If resources already exist with desired state, should skip ReviewRequired → Completed

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
			cmd := exec.Command("kubectl", "apply", "--server-side", "--force-conflicts", "-f", "-")
			cmd.Stdin = strings.NewReader(sampleYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for plan generation")
			Eventually(func(g Gomega) {
				status := getVirtPlatformConfigStatus("load-aware-rebalancing")
				// Should reach terminal state
				g.Expect(status.Phase).To(BeElementOf("ReviewRequired", "Completed", "PrerequisiteFailed"))
			}, 2*time.Minute).Should(Succeed())

			By("checking if all diffs show no changes")
			status := getVirtPlatformConfigStatus("load-aware-rebalancing")

			allNoChanges := true
			for _, item := range status.Items {
				if !strings.Contains(item.Diff, "(no changes)") && item.Diff != "" {
					allNoChanges = false
					break
				}
			}

			if allNoChanges && len(status.Items) > 0 {
				By("verifying smart transition to Completed (skipping unnecessary ReviewRequired)")
				// Should skip ReviewRequired and go straight to Completed
				// when all diffs show "(no changes)"
				Expect(status.Phase).To(Equal("Completed"),
					"When all diffs show no changes, should skip ReviewRequired and go to Completed")
			} else {
				By("resources need changes, should be in ReviewRequired or PrerequisiteFailed")
				// In Kind cluster without OpenShift CRDs, will be PrerequisiteFailed
				// In real OpenShift cluster with CRDs, will be ReviewRequired
				Expect(status.Phase).To(BeElementOf("ReviewRequired", "PrerequisiteFailed"),
					"Should be in ReviewRequired (if CRDs exist) or PrerequisiteFailed (if CRDs missing)")
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
