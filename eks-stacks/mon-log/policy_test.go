package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// PolicyValidation represents a security policy validation for monitoring
type PolicyValidation struct {
	Name             string
	Description      string
	EnforcementLevel string // "mandatory" or "advisory"
	Validate         func(resources []ResourceInfo) []PolicyViolation
}

// ResourceInfo represents resource information for policy validation
type ResourceInfo struct {
	Type       string
	Name       string
	Properties map[string]interface{}
}

// PolicyViolation represents a policy validation failure
type PolicyViolation struct {
	ResourceName string
	PolicyName   string
	Message      string
}

// MonitoringSecurityPolicies returns all security policies for monitoring stack
func MonitoringSecurityPolicies() []PolicyValidation {
	return []PolicyValidation{
		{
			Name:             "helm-release-version-pinned",
			Description:      "Ensures Helm releases have pinned versions for reproducibility",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "kubernetes:helm.sh/v3:Release" {
						version, hasVersion := r.Properties["version"]
						if !hasVersion || version == "" || version == "latest" {
							violations = append(violations, PolicyViolation{
								ResourceName: r.Name,
								PolicyName:   "helm-release-version-pinned",
								Message:      "Helm release must have explicit version (not 'latest')",
							})
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "monitoring-namespace-required",
			Description:      "Ensures monitoring resources are deployed in 'monitoring' namespace",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "kubernetes:helm.sh/v3:Release" {
						if namespace, ok := r.Properties["namespace"].(string); ok {
							if namespace != "monitoring" {
								violations = append(violations, PolicyViolation{
									ResourceName: r.Name,
									PolicyName:   "monitoring-namespace-required",
									Message:      "Monitoring Helm releases must be deployed in 'monitoring' namespace",
								})
							}
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "persistent-storage-required",
			Description:      "Ensures critical monitoring components have persistent storage",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				criticalComponents := []string{"prometheus", "loki", "grafana"}

				for _, r := range resources {
					if r.Type == "kubernetes:helm.sh/v3:Release" {
						name := strings.ToLower(r.Name)

						for _, component := range criticalComponents {
							if strings.Contains(name, component) {
								// Check if persistence is configured in values
								if values, ok := r.Properties["values"].(map[string]interface{}); ok {
									hasStorage := false

									// Check different possible storage configurations
									if _, hasPrometheusStorage := values["prometheus"].(map[string]interface{})["prometheusSpec"].(map[string]interface{})["storageSpec"]; hasPrometheusStorage {
										hasStorage = true
									}
									if persistence, hasPersistence := values["persistence"].(map[string]interface{}); hasPersistence {
										if enabled, ok := persistence["enabled"].(bool); ok && enabled {
											hasStorage = true
										}
									}
									if singleBinary, hasSB := values["singleBinary"].(map[string]interface{}); hasSB {
										if persistence, hasPersistence := singleBinary["persistence"].(map[string]interface{}); hasPersistence {
											if enabled, ok := persistence["enabled"].(bool); ok && enabled {
												hasStorage = true
											}
										}
									}

									if !hasStorage {
										violations = append(violations, PolicyViolation{
											ResourceName: r.Name,
											PolicyName:   "persistent-storage-required",
											Message:      fmt.Sprintf("Critical component '%s' must have persistent storage configured", component),
										})
									}
								}
							}
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "storage-encryption-required",
			Description:      "Ensures storage classes for monitoring use encryption",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "kubernetes:helm.sh/v3:Release" {
						if values, ok := r.Properties["values"].(map[string]interface{}); ok {
							storageClasses := extractStorageClasses(values)

							for _, sc := range storageClasses {
								if !strings.Contains(sc, "enc") && !strings.Contains(sc, "encrypted") {
									violations = append(violations, PolicyViolation{
										ResourceName: r.Name,
										PolicyName:   "storage-encryption-required",
										Message:      fmt.Sprintf("Storage class '%s' should use encryption (contain 'enc' or 'encrypted')", sc),
									})
								}
							}
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "resource-limits-required",
			Description:      "Ensures monitoring components have resource limits defined",
			EnforcementLevel: "advisory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "kubernetes:helm.sh/v3:Release" {
						if values, ok := r.Properties["values"].(map[string]interface{}); ok {
							hasLimits := checkResourceLimits(values)
							if !hasLimits {
								violations = append(violations, PolicyViolation{
									ResourceName: r.Name,
									PolicyName:   "resource-limits-required",
									Message:      "Monitoring component should have resource limits defined",
								})
							}
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "prometheus-retention-configured",
			Description:      "Ensures Prometheus has explicit retention policy",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "kubernetes:helm.sh/v3:Release" && strings.Contains(strings.ToLower(r.Name), "prometheus") {
						if values, ok := r.Properties["values"].(map[string]interface{}); ok {
							hasRetention := false

							if prometheus, ok := values["prometheus"].(map[string]interface{}); ok {
								if spec, ok := prometheus["prometheusSpec"].(map[string]interface{}); ok {
									if retention, ok := spec["retention"]; ok && retention != "" {
										hasRetention = true
									}
								}
							}

							if !hasRetention {
								violations = append(violations, PolicyViolation{
									ResourceName: r.Name,
									PolicyName:   "prometheus-retention-configured",
									Message:      "Prometheus must have explicit retention policy configured",
								})
							}
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "loki-retention-configured",
			Description:      "Ensures Loki has retention policy configured",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "kubernetes:helm.sh/v3:Release" && strings.Contains(strings.ToLower(r.Name), "loki") {
						if values, ok := r.Properties["values"].(map[string]interface{}); ok {
							hasRetention := false

							if loki, ok := values["loki"].(map[string]interface{}); ok {
								if limitsConfig, ok := loki["limits_config"].(map[string]interface{}); ok {
									if retention, ok := limitsConfig["retention_period"]; ok && retention != "" {
										hasRetention = true
									}
								}
							}

							if !hasRetention {
								violations = append(violations, PolicyViolation{
									ResourceName: r.Name,
									PolicyName:   "loki-retention-configured",
									Message:      "Loki should have retention policy configured",
								})
							}
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "grafana-auth-configured",
			Description:      "Ensures Grafana has authentication configured",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "kubernetes:helm.sh/v3:Release" {
						if values, ok := r.Properties["values"].(map[string]interface{}); ok {
							if grafana, ok := values["grafana"].(map[string]interface{}); ok {
								hasAuth := false

								// Check for admin password
								if adminPassword, ok := grafana["adminPassword"]; ok && adminPassword != "" {
									hasAuth = true
								}

								// Check if anonymous auth is disabled
								if authConfig, ok := grafana["grafana.ini"].(map[string]interface{}); ok {
									if auth, ok := authConfig["auth.anonymous"].(map[string]interface{}); ok {
										if enabled, ok := auth["enabled"].(bool); ok && enabled {
											hasAuth = false // Anonymous auth should be disabled
										}
									}
								}

								if !hasAuth {
									violations = append(violations, PolicyViolation{
										ResourceName: r.Name,
										PolicyName:   "grafana-auth-configured",
										Message:      "Grafana must have authentication configured (admin password required)",
									})
								}
							}
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "helm-timeout-configured",
			Description:      "Ensures Helm releases have appropriate timeout values",
			EnforcementLevel: "advisory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "kubernetes:helm.sh/v3:Release" {
						timeout, hasTimeout := r.Properties["timeout"]
						if !hasTimeout {
							violations = append(violations, PolicyViolation{
								ResourceName: r.Name,
								PolicyName:   "helm-timeout-configured",
								Message:      "Helm release should have explicit timeout configured",
							})
						} else if timeoutInt, ok := timeout.(int); ok {
							if timeoutInt < 300 { // Less than 5 minutes
								violations = append(violations, PolicyViolation{
									ResourceName: r.Name,
									PolicyName:   "helm-timeout-configured",
									Message:      "Helm release timeout should be at least 300 seconds for monitoring components",
								})
							}
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "monitoring-labels-present",
			Description:      "Ensures monitoring resources have proper labels",
			EnforcementLevel: "advisory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				requiredLabels := []string{"app", "component", "managed-by"}

				for _, r := range resources {
					if r.Type == "kubernetes:core/v1:Namespace" && r.Name == "monitoring" {
						if metadata, ok := r.Properties["metadata"].(map[string]interface{}); ok {
							if labels, ok := metadata["labels"].(map[string]interface{}); ok {
								for _, reqLabel := range requiredLabels {
									if _, hasLabel := labels[reqLabel]; !hasLabel {
										violations = append(violations, PolicyViolation{
											ResourceName: r.Name,
											PolicyName:   "monitoring-labels-present",
											Message:      fmt.Sprintf("Monitoring namespace should have label '%s'", reqLabel),
										})
									}
								}
							}
						}
					}
				}
				return violations
			},
		},
	}
}

// Helper functions
func extractStorageClasses(values map[string]interface{}) []string {
	var storageClasses []string

	// Check Prometheus storage class
	if prometheus, ok := values["prometheus"].(map[string]interface{}); ok {
		if spec, ok := prometheus["prometheusSpec"].(map[string]interface{}); ok {
			if storageSpec, ok := spec["storageSpec"].(map[string]interface{}); ok {
				if vct, ok := storageSpec["volumeClaimTemplate"].(map[string]interface{}); ok {
					if spec, ok := vct["spec"].(map[string]interface{}); ok {
						if sc, ok := spec["storageClassName"].(string); ok {
							storageClasses = append(storageClasses, sc)
						}
					}
				}
			}
		}
	}

	// Check Grafana storage class
	if grafana, ok := values["grafana"].(map[string]interface{}); ok {
		if persistence, ok := grafana["persistence"].(map[string]interface{}); ok {
			if sc, ok := persistence["storageClassName"].(string); ok {
				storageClasses = append(storageClasses, sc)
			}
		}
	}

	// Check Loki storage class
	if singleBinary, ok := values["singleBinary"].(map[string]interface{}); ok {
		if persistence, ok := singleBinary["persistence"].(map[string]interface{}); ok {
			if sc, ok := persistence["storageClass"].(string); ok {
				storageClasses = append(storageClasses, sc)
			}
		}
	}

	return storageClasses
}

func checkResourceLimits(values map[string]interface{}) bool {
	// Check for resources configuration in any component
	if resources, ok := values["resources"].(map[string]interface{}); ok {
		if limits, ok := resources["limits"].(map[string]interface{}); ok {
			return len(limits) > 0
		}
	}

	// Check Prometheus resources
	if prometheus, ok := values["prometheus"].(map[string]interface{}); ok {
		if spec, ok := prometheus["prometheusSpec"].(map[string]interface{}); ok {
			if resources, ok := spec["resources"].(map[string]interface{}); ok {
				if limits, ok := resources["limits"].(map[string]interface{}); ok {
					return len(limits) > 0
				}
			}
		}
	}

	return false
}

// Tests for policy framework
func TestMonitoringPolicyFramework(t *testing.T) {
	policies := MonitoringSecurityPolicies()

	assert.NotEmpty(t, policies, "Should have monitoring security policies defined")
	assert.GreaterOrEqual(t, len(policies), 10, "Should have at least 10 policies")

	for _, policy := range policies {
		assert.NotEmpty(t, policy.Name, "Policy should have a name")
		assert.NotEmpty(t, policy.Description, "Policy should have a description")
		assert.NotEmpty(t, policy.EnforcementLevel, "Policy should have enforcement level")
		assert.NotNil(t, policy.Validate, "Policy should have validation function")

		assert.Contains(t, []string{"mandatory", "advisory"}, policy.EnforcementLevel,
			"Enforcement level should be 'mandatory' or 'advisory'")
	}
}

// Test helm version pinning policy
func TestHelmVersionPinnedPolicy(t *testing.T) {
	policies := MonitoringSecurityPolicies()
	var versionPolicy PolicyValidation

	for _, p := range policies {
		if p.Name == "helm-release-version-pinned" {
			versionPolicy = p
			break
		}
	}

	// Test compliant resource
	compliantResources := []ResourceInfo{
		{
			Type: "kubernetes:helm.sh/v3:Release",
			Name: "prometheus",
			Properties: map[string]interface{}{
				"version": "58.2.2",
			},
		},
	}

	violations := versionPolicy.Validate(compliantResources)
	assert.Empty(t, violations, "Helm release with pinned version should have no violations")

	// Test non-compliant resource
	nonCompliantResources := []ResourceInfo{
		{
			Type: "kubernetes:helm.sh/v3:Release",
			Name: "prometheus",
			Properties: map[string]interface{}{
				"version": "latest",
			},
		},
	}

	violations = versionPolicy.Validate(nonCompliantResources)
	assert.NotEmpty(t, violations, "Helm release with 'latest' should have violations")
}

// Test namespace policy
func TestMonitoringNamespacePolicy(t *testing.T) {
	policies := MonitoringSecurityPolicies()
	var nsPolicy PolicyValidation

	for _, p := range policies {
		if p.Name == "monitoring-namespace-required" {
			nsPolicy = p
			break
		}
	}

	// Test compliant resource
	compliantResources := []ResourceInfo{
		{
			Type: "kubernetes:helm.sh/v3:Release",
			Name: "prometheus",
			Properties: map[string]interface{}{
				"namespace": "monitoring",
			},
		},
	}

	violations := nsPolicy.Validate(compliantResources)
	assert.Empty(t, violations, "Helm release in 'monitoring' namespace should have no violations")

	// Test non-compliant resource
	nonCompliantResources := []ResourceInfo{
		{
			Type: "kubernetes:helm.sh/v3:Release",
			Name: "prometheus",
			Properties: map[string]interface{}{
				"namespace": "default",
			},
		},
	}

	violations = nsPolicy.Validate(nonCompliantResources)
	assert.NotEmpty(t, violations, "Helm release in wrong namespace should have violations")
}

// Test storage encryption policy
func TestStorageEncryptionPolicy(t *testing.T) {
	policies := MonitoringSecurityPolicies()
	var encPolicy PolicyValidation

	for _, p := range policies {
		if p.Name == "storage-encryption-required" {
			encPolicy = p
			break
		}
	}

	// Test compliant resource
	compliantResources := []ResourceInfo{
		{
			Type: "kubernetes:helm.sh/v3:Release",
			Name: "prometheus",
			Properties: map[string]interface{}{
				"values": map[string]interface{}{
					"prometheus": map[string]interface{}{
						"prometheusSpec": map[string]interface{}{
							"storageSpec": map[string]interface{}{
								"volumeClaimTemplate": map[string]interface{}{
									"spec": map[string]interface{}{
										"storageClassName": "gp3-enc",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	violations := encPolicy.Validate(compliantResources)
	assert.Empty(t, violations, "Encrypted storage class should have no violations")
}

// Test policy enforcement levels
func TestPolicyEnforcementLevels(t *testing.T) {
	policies := MonitoringSecurityPolicies()

	mandatoryCount := 0
	advisoryCount := 0

	for _, policy := range policies {
		switch policy.EnforcementLevel {
		case "mandatory":
			mandatoryCount++
		case "advisory":
			advisoryCount++
		}
	}

	assert.Greater(t, mandatoryCount, 0, "Should have mandatory policies")
	assert.GreaterOrEqual(t, mandatoryCount, 6, "Should have at least 6 mandatory policies")

	t.Logf("Monitoring Policy Summary:")
	t.Logf("  Mandatory: %d", mandatoryCount)
	t.Logf("  Advisory: %d", advisoryCount)
	t.Logf("  Total: %d", len(policies))
}

// Test compliance report generation
func TestMonitoringComplianceReport(t *testing.T) {
	policies := MonitoringSecurityPolicies()

	// Simulate monitoring resources
	testResources := []ResourceInfo{
		{
			Type: "kubernetes:helm.sh/v3:Release",
			Name: "kube-prometheus-stack",
			Properties: map[string]interface{}{
				"namespace": "monitoring",
				"version":   "58.2.2",
				"timeout":   300,
				"values": map[string]interface{}{
					"prometheus": map[string]interface{}{
						"prometheusSpec": map[string]interface{}{
							"retention": "7d",
						},
					},
					"grafana": map[string]interface{}{
						"adminPassword": "secure-password",
					},
				},
			},
		},
	}

	report := make(map[string]interface{})
	report["timestamp"] = "2024-12-16T12:00:00Z"
	report["totalPolicies"] = len(policies)

	mandatoryPassed := 0
	mandatoryFailed := 0
	advisoryPassed := 0
	advisoryFailed := 0

	for _, policy := range policies {
		violations := policy.Validate(testResources)

		if policy.EnforcementLevel == "mandatory" {
			if len(violations) == 0 {
				mandatoryPassed++
			} else {
				mandatoryFailed++
			}
		} else {
			if len(violations) == 0 {
				advisoryPassed++
			} else {
				advisoryFailed++
			}
		}
	}

	report["mandatory"] = map[string]int{
		"passed": mandatoryPassed,
		"failed": mandatoryFailed,
	}
	report["advisory"] = map[string]int{
		"passed": advisoryPassed,
		"failed": advisoryFailed,
	}

	reportJSON, err := json.MarshalIndent(report, "", "  ")
	assert.NoError(t, err)

	t.Logf("Monitoring Compliance Report:\n%s", string(reportJSON))

	// Calculate compliance rate
	if mandatoryPassed+mandatoryFailed > 0 {
		complianceRate := float64(mandatoryPassed) / float64(mandatoryPassed+mandatoryFailed) * 100
		t.Logf("Mandatory Policy Compliance: %.1f%%", complianceRate)
	}
}
