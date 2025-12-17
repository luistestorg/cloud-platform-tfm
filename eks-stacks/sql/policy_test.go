package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// PolicyValidation represents a security policy validation for SQL/RDS
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

// SQLSecurityPolicies returns all security policies for SQL stack
func SQLSecurityPolicies() []PolicyValidation {
	return []PolicyValidation{
		{
			Name:             "rds-storage-encryption-required",
			Description:      "Ensures RDS instances have encrypted storage",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "aws:rds/instance:Instance" {
						encrypted, hasEncryption := r.Properties["storageEncrypted"].(bool)
						if !hasEncryption || !encrypted {
							violations = append(violations, PolicyViolation{
								ResourceName: r.Name,
								PolicyName:   "rds-storage-encryption-required",
								Message:      "RDS instance must have encrypted storage (storageEncrypted: true)",
							})
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "rds-backup-retention-required",
			Description:      "Ensures RDS instances have adequate backup retention",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				minRetention := 7 // days

				for _, r := range resources {
					if r.Type == "aws:rds/instance:Instance" {
						retention, hasRetention := r.Properties["backupRetentionPeriod"].(int)
						if !hasRetention || retention < minRetention {
							violations = append(violations, PolicyViolation{
								ResourceName: r.Name,
								PolicyName:   "rds-backup-retention-required",
								Message:      fmt.Sprintf("RDS instance must have backup retention >= %d days", minRetention),
							})
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "rds-multi-az-recommended",
			Description:      "Recommends Multi-AZ deployment for production RDS instances",
			EnforcementLevel: "advisory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "aws:rds/instance:Instance" {
						multiAz, hasMultiAz := r.Properties["multiAz"].(bool)
						if !hasMultiAz || !multiAz {
							violations = append(violations, PolicyViolation{
								ResourceName: r.Name,
								PolicyName:   "rds-multi-az-recommended",
								Message:      "RDS instance should use Multi-AZ deployment for high availability",
							})
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "rds-deletion-protection-recommended",
			Description:      "Recommends deletion protection for production databases",
			EnforcementLevel: "advisory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "aws:rds/instance:Instance" {
						delProtection, hasDelProtection := r.Properties["deletionProtection"].(bool)
						if !hasDelProtection || !delProtection {
							violations = append(violations, PolicyViolation{
								ResourceName: r.Name,
								PolicyName:   "rds-deletion-protection-recommended",
								Message:      "RDS instance should have deletion protection enabled for production",
							})
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "rds-public-access-forbidden",
			Description:      "Ensures RDS instances are not publicly accessible",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "aws:rds/instance:Instance" {
						publicAccess, hasPublicAccess := r.Properties["publiclyAccessible"].(bool)
						if hasPublicAccess && publicAccess {
							violations = append(violations, PolicyViolation{
								ResourceName: r.Name,
								PolicyName:   "rds-public-access-forbidden",
								Message:      "RDS instance must not be publicly accessible (publiclyAccessible: false)",
							})
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "rds-engine-version-current",
			Description:      "Ensures RDS uses a current PostgreSQL version",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				minimumVersion := "15" // PostgreSQL 15+

				for _, r := range resources {
					if r.Type == "aws:rds/instance:Instance" {
						engine, hasEngine := r.Properties["engine"].(string)
						if hasEngine && engine == "postgres" {
							version, hasVersion := r.Properties["engineVersion"].(string)
							if hasVersion {
								// Extract major version
								majorVersion := strings.Split(version, ".")[0]
								if majorVersion < minimumVersion {
									violations = append(violations, PolicyViolation{
										ResourceName: r.Name,
										PolicyName:   "rds-engine-version-current",
										Message:      fmt.Sprintf("PostgreSQL version should be %s or higher (current: %s)", minimumVersion, version),
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
			Name:             "rds-enhanced-monitoring-enabled",
			Description:      "Ensures RDS has enhanced monitoring enabled",
			EnforcementLevel: "advisory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "aws:rds/instance:Instance" {
						logs, hasLogs := r.Properties["enabledCloudwatchLogsExports"].([]interface{})
						if !hasLogs || len(logs) == 0 {
							violations = append(violations, PolicyViolation{
								ResourceName: r.Name,
								PolicyName:   "rds-enhanced-monitoring-enabled",
								Message:      "RDS instance should have CloudWatch logs exports enabled",
							})
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "rds-performance-insights-enabled",
			Description:      "Ensures RDS has Performance Insights enabled",
			EnforcementLevel: "advisory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "aws:rds/instance:Instance" {
						piEnabled, hasPi := r.Properties["performanceInsightsEnabled"].(bool)
						if !hasPi || !piEnabled {
							violations = append(violations, PolicyViolation{
								ResourceName: r.Name,
								PolicyName:   "rds-performance-insights-enabled",
								Message:      "RDS instance should have Performance Insights enabled for monitoring",
							})
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "rds-auto-minor-version-upgrade",
			Description:      "Ensures RDS has automatic minor version upgrades enabled",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "aws:rds/instance:Instance" {
						autoUpgrade, hasAutoUpgrade := r.Properties["autoMinorVersionUpgrade"].(bool)
						if hasAutoUpgrade && !autoUpgrade {
							violations = append(violations, PolicyViolation{
								ResourceName: r.Name,
								PolicyName:   "rds-auto-minor-version-upgrade",
								Message:      "RDS instance should have automatic minor version upgrades enabled",
							})
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "security-group-ingress-restricted",
			Description:      "Ensures RDS security group has restricted ingress (VPC only)",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "aws:ec2/securityGroup:SecurityGroup" {
						if strings.Contains(strings.ToLower(r.Name), "rds") {
							ingress, hasIngress := r.Properties["ingress"].([]interface{})
							if hasIngress {
								for _, rule := range ingress {
									if ruleMap, ok := rule.(map[string]interface{}); ok {
										cidrBlocks, hasCidr := ruleMap["cidrBlocks"].([]interface{})
										if hasCidr {
											for _, cidr := range cidrBlocks {
												cidrStr := fmt.Sprintf("%v", cidr)
												if cidrStr == "0.0.0.0/0" {
													violations = append(violations, PolicyViolation{
														ResourceName: r.Name,
														PolicyName:   "security-group-ingress-restricted",
														Message:      "RDS security group should not allow ingress from 0.0.0.0/0",
													})
												}
											}
										}
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
			Name:             "cloudwatch-alarms-configured",
			Description:      "Ensures CloudWatch alarms are configured for RDS monitoring",
			EnforcementLevel: "advisory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				
				// Check if there are CloudWatch alarms
				hasAlarms := false
				for _, r := range resources {
					if r.Type == "aws:cloudwatch/metricAlarm:MetricAlarm" {
						hasAlarms = true
						break
					}
				}

				if !hasAlarms {
					violations = append(violations, PolicyViolation{
						ResourceName: "stack",
						PolicyName:   "cloudwatch-alarms-configured",
						Message:      "Stack should have CloudWatch alarms configured for RDS monitoring",
					})
				}

				return violations
			},
		},
		{
			Name:             "rds-storage-type-recommended",
			Description:      "Recommends using gp3 storage type for cost and performance",
			EnforcementLevel: "advisory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "aws:rds/instance:Instance" {
						storageType, hasStorageType := r.Properties["storageType"].(string)
						if hasStorageType && storageType != "gp3" {
							violations = append(violations, PolicyViolation{
								ResourceName: r.Name,
								PolicyName:   "rds-storage-type-recommended",
								Message:      "RDS instance should use gp3 storage type for better cost/performance",
							})
						}
					}
				}
				return violations
			},
		},
	}
}

// Tests for policy framework
func TestSQLPolicyFramework(t *testing.T) {
	policies := SQLSecurityPolicies()

	assert.NotEmpty(t, policies, "Should have SQL security policies defined")
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

// Test storage encryption policy
func TestStorageEncryptionPolicy(t *testing.T) {
	policies := SQLSecurityPolicies()
	var encPolicy PolicyValidation

	for _, p := range policies {
		if p.Name == "rds-storage-encryption-required" {
			encPolicy = p
			break
		}
	}

	// Test compliant resource
	compliantResources := []ResourceInfo{
		{
			Type: "aws:rds/instance:Instance",
			Name: "tfmdb",
			Properties: map[string]interface{}{
				"storageEncrypted": true,
			},
		},
	}

	violations := encPolicy.Validate(compliantResources)
	assert.Empty(t, violations, "RDS with encrypted storage should have no violations")

	// Test non-compliant resource
	nonCompliantResources := []ResourceInfo{
		{
			Type: "aws:rds/instance:Instance",
			Name: "tfmdb",
			Properties: map[string]interface{}{
				"storageEncrypted": false,
			},
		},
	}

	violations = encPolicy.Validate(nonCompliantResources)
	assert.NotEmpty(t, violations, "RDS without encrypted storage should have violations")
}

// Test backup retention policy
func TestBackupRetentionPolicy(t *testing.T) {
	policies := SQLSecurityPolicies()
	var retentionPolicy PolicyValidation

	for _, p := range policies {
		if p.Name == "rds-backup-retention-required" {
			retentionPolicy = p
			break
		}
	}

	// Test compliant resource
	compliantResources := []ResourceInfo{
		{
			Type: "aws:rds/instance:Instance",
			Name: "tfmdb",
			Properties: map[string]interface{}{
				"backupRetentionPeriod": 7,
			},
		},
	}

	violations := retentionPolicy.Validate(compliantResources)
	assert.Empty(t, violations, "RDS with adequate backup retention should have no violations")

	// Test non-compliant resource
	nonCompliantResources := []ResourceInfo{
		{
			Type: "aws:rds/instance:Instance",
			Name: "tfmdb",
			Properties: map[string]interface{}{
				"backupRetentionPeriod": 0,
			},
		},
	}

	violations = retentionPolicy.Validate(nonCompliantResources)
	assert.NotEmpty(t, violations, "RDS with insufficient backup retention should have violations")
}

// Test public access policy
func TestPublicAccessPolicy(t *testing.T) {
	policies := SQLSecurityPolicies()
	var publicPolicy PolicyValidation

	for _, p := range policies {
		if p.Name == "rds-public-access-forbidden" {
			publicPolicy = p
			break
		}
	}

	// Test compliant resource
	compliantResources := []ResourceInfo{
		{
			Type: "aws:rds/instance:Instance",
			Name: "tfmdb",
			Properties: map[string]interface{}{
				"publiclyAccessible": false,
			},
		},
	}

	violations := publicPolicy.Validate(compliantResources)
	assert.Empty(t, violations, "Private RDS instance should have no violations")

	// Test non-compliant resource
	nonCompliantResources := []ResourceInfo{
		{
			Type: "aws:rds/instance:Instance",
			Name: "tfmdb",
			Properties: map[string]interface{}{
				"publiclyAccessible": true,
			},
		},
	}

	violations = publicPolicy.Validate(nonCompliantResources)
	assert.NotEmpty(t, violations, "Public RDS instance should have violations")
}

// Test security group policy
func TestSecurityGroupIngressPolicy(t *testing.T) {
	policies := SQLSecurityPolicies()
	var sgPolicy PolicyValidation

	for _, p := range policies {
		if p.Name == "security-group-ingress-restricted" {
			sgPolicy = p
			break
		}
	}

	// Test compliant resource
	compliantResources := []ResourceInfo{
		{
			Type: "aws:ec2/securityGroup:SecurityGroup",
			Name: "tfmdb-rds-sg",
			Properties: map[string]interface{}{
				"ingress": []interface{}{
					map[string]interface{}{
						"cidrBlocks": []interface{}{"10.0.0.0/16"},
					},
				},
			},
		},
	}

	violations := sgPolicy.Validate(compliantResources)
	assert.Empty(t, violations, "Security group with VPC-only ingress should have no violations")
}

// Test policy enforcement levels
func TestPolicyEnforcementLevels(t *testing.T) {
	policies := SQLSecurityPolicies()

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
	assert.GreaterOrEqual(t, mandatoryCount, 5, "Should have at least 5 mandatory policies")

	t.Logf("SQL Policy Summary:")
	t.Logf("  Mandatory: %d", mandatoryCount)
	t.Logf("  Advisory: %d", advisoryCount)
	t.Logf("  Total: %d", len(policies))
}

// Test compliance report generation
func TestSQLComplianceReport(t *testing.T) {
	policies := SQLSecurityPolicies()

	// Simulate RDS resources
	testResources := []ResourceInfo{
		{
			Type: "aws:rds/instance:Instance",
			Name: "tfmdb",
			Properties: map[string]interface{}{
				"storageEncrypted":            true,
				"backupRetentionPeriod":       7,
				"publiclyAccessible":          false,
				"engine":                      "postgres",
				"engineVersion":               "16.3",
				"autoMinorVersionUpgrade":     true,
				"performanceInsightsEnabled":  true,
				"enabledCloudwatchLogsExports": []interface{}{"postgresql"},
				"storageType":                 "gp3",
			},
		},
		{
			Type: "aws:cloudwatch/metricAlarm:MetricAlarm",
			Name: "tfmdb-cpu-alarm",
			Properties: map[string]interface{}{
				"metricName": "CPUUtilization",
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

	t.Logf("SQL Compliance Report:\n%s", string(reportJSON))

	// Calculate compliance rate
	if mandatoryPassed+mandatoryFailed > 0 {
		complianceRate := float64(mandatoryPassed) / float64(mandatoryPassed+mandatoryFailed) * 100
		t.Logf("Mandatory Policy Compliance: %.1f%%", complianceRate)
	}
}
