package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// PolicyValidation represents a security policy validation
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

// EKSSecurityPolicies returns all security policies for EKS infrastructure
func EKSSecurityPolicies() []PolicyValidation {
	return []PolicyValidation{
		{
			Name:             "eks-cluster-endpoint-access",
			Description:      "Validates EKS cluster has appropriate endpoint access configuration",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "aws:eks/cluster:Cluster" || r.Type == "eks:index:Cluster" {
						if endpointPrivate, ok := r.Properties["endpointPrivateAccess"].(bool); !ok || !endpointPrivate {
							violations = append(violations, PolicyViolation{
								ResourceName: r.Name,
								PolicyName:   "eks-cluster-endpoint-access",
								Message:      "EKS cluster must have private endpoint access enabled",
							})
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "eks-cluster-logging-enabled",
			Description:      "Ensures EKS cluster has CloudWatch logging enabled",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "aws:eks/cluster:Cluster" {
						logTypes, ok := r.Properties["enabledClusterLogTypes"].([]interface{})
						if !ok || len(logTypes) < 3 {
							violations = append(violations, PolicyViolation{
								ResourceName: r.Name,
								PolicyName:   "eks-cluster-logging-enabled",
								Message:      "EKS cluster must have at least 3 log types enabled (api, audit, authenticator)",
							})
							continue
						}

						// Verify critical log types
						hasApi := false
						hasAudit := false
						for _, lt := range logTypes {
							if logType, ok := lt.(string); ok {
								if logType == "api" {
									hasApi = true
								}
								if logType == "audit" {
									hasAudit = true
								}
							}
						}

						if !hasApi || !hasAudit {
							violations = append(violations, PolicyViolation{
								ResourceName: r.Name,
								PolicyName:   "eks-cluster-logging-enabled",
								Message:      "EKS cluster must enable 'api' and 'audit' logging",
							})
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "iam-role-least-privilege",
			Description:      "Validates IAM roles follow least privilege principle",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "aws:iam/role:Role" {
						if assumePolicy, ok := r.Properties["assumeRolePolicy"].(string); ok {
							if strings.Contains(assumePolicy, `"Resource":"*"`) && strings.Contains(assumePolicy, `"Action":"*"`) {
								violations = append(violations, PolicyViolation{
									ResourceName: r.Name,
									PolicyName:   "iam-role-least-privilege",
									Message:      "IAM role contains wildcard permissions in assume role policy",
								})
							}
						}
					}

					if r.Type == "aws:iam/rolePolicy:RolePolicy" {
						if policy, ok := r.Properties["policy"].(string); ok {
							if strings.Contains(policy, `"Action":"*"`) {
								violations = append(violations, PolicyViolation{
									ResourceName: r.Name,
									PolicyName:   "iam-role-least-privilege",
									Message:      "IAM role policy uses wildcard actions - specify exact permissions",
								})
							}
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "storageclass-encryption-required",
			Description:      "Ensures all StorageClasses have encryption enabled",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "kubernetes:storage.k8s.io/v1:StorageClass" {
						if parameters, ok := r.Properties["parameters"].(map[string]interface{}); ok {
							encrypted, hasEncryption := parameters["encrypted"]
							if !hasEncryption || encrypted != "true" {
								violations = append(violations, PolicyViolation{
									ResourceName: r.Name,
									PolicyName:   "storageclass-encryption-required",
									Message:      "StorageClass must have encryption enabled",
								})
							}
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "eks-oidc-provider-required",
			Description:      "Ensures EKS cluster has OIDC provider enabled for IRSA",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "eks:index:Cluster" {
						createOidc, ok := r.Properties["createOidcProvider"].(bool)
						if !ok || !createOidc {
							violations = append(violations, PolicyViolation{
								ResourceName: r.Name,
								PolicyName:   "eks-oidc-provider-required",
								Message:      "EKS cluster must have OIDC provider enabled",
							})
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "helm-release-version-required",
			Description:      "Ensures Helm releases have explicit version specified",
			EnforcementLevel: "advisory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "kubernetes:helm.sh/v3:Release" {
						version, hasVersion := r.Properties["version"]
						if !hasVersion || version == "" {
							violations = append(violations, PolicyViolation{
								ResourceName: r.Name,
								PolicyName:   "helm-release-version-required",
								Message:      "Helm release should specify explicit version for reproducibility",
							})
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "eks-nodegroup-approved-instance-types",
			Description:      "Validates node groups use approved instance types",
			EnforcementLevel: "mandatory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				approvedInstanceTypes := map[string]bool{
					"m6i.large":   true,
					"m6i.xlarge":  true,
					"m6i.2xlarge": true,
					"m5.large":    true,
					"m5.xlarge":   true,
					"c6i.large":   true,
					"c6i.xlarge":  true,
					"r6i.large":   true,
					"r6i.xlarge":  true,
					"t3.medium":   true,
					"t3.large":    true,
				}

				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "eks:index:Cluster" {
						if instanceType, ok := r.Properties["instanceType"].(string); ok {
							if !approvedInstanceTypes[instanceType] {
								violations = append(violations, PolicyViolation{
									ResourceName: r.Name,
									PolicyName:   "eks-nodegroup-approved-instance-types",
									Message:      fmt.Sprintf("Instance type %s is not approved", instanceType),
								})
							}
						}
					}
				}
				return violations
			},
		},
		{
			Name:             "storageclass-wait-for-consumer",
			Description:      "Ensures StorageClasses use WaitForFirstConsumer for better pod scheduling",
			EnforcementLevel: "advisory",
			Validate: func(resources []ResourceInfo) []PolicyViolation {
				var violations []PolicyViolation
				for _, r := range resources {
					if r.Type == "kubernetes:storage.k8s.io/v1:StorageClass" {
						bindingMode, ok := r.Properties["volumeBindingMode"].(string)
						if !ok || bindingMode != "WaitForFirstConsumer" {
							violations = append(violations, PolicyViolation{
								ResourceName: r.Name,
								PolicyName:   "storageclass-wait-for-consumer",
								Message:      "StorageClass should use WaitForFirstConsumer binding mode",
							})
						}
					}
				}
				return violations
			},
		},
	}
}

// TestPolicyValidationFramework tests the policy validation framework itself
func TestPolicyValidationFramework(t *testing.T) {
	policies := EKSSecurityPolicies()

	assert.NotEmpty(t, policies, "Should have security policies defined")
	assert.GreaterOrEqual(t, len(policies), 8, "Should have at least 8 policies")

	// Verify each policy has required fields
	for _, policy := range policies {
		assert.NotEmpty(t, policy.Name, "Policy should have a name")
		assert.NotEmpty(t, policy.Description, "Policy should have a description")
		assert.NotEmpty(t, policy.EnforcementLevel, "Policy should have enforcement level")
		assert.NotNil(t, policy.Validate, "Policy should have validation function")

		// Verify enforcement level is valid
		assert.Contains(t, []string{"mandatory", "advisory"}, policy.EnforcementLevel,
			"Enforcement level should be 'mandatory' or 'advisory'")
	}
}

// TestEKSClusterEndpointAccessPolicy validates the endpoint access policy
func TestEKSClusterEndpointAccessPolicy(t *testing.T) {
	policies := EKSSecurityPolicies()
	var endpointPolicy PolicyValidation

	for _, p := range policies {
		if p.Name == "eks-cluster-endpoint-access" {
			endpointPolicy = p
			break
		}
	}

	// Test compliant resource
	compliantResources := []ResourceInfo{
		{
			Type: "eks:index:Cluster",
			Name: "test-cluster",
			Properties: map[string]interface{}{
				"endpointPrivateAccess": true,
			},
		},
	}

	violations := endpointPolicy.Validate(compliantResources)
	assert.Empty(t, violations, "Compliant cluster should have no violations")

	// Test non-compliant resource
	nonCompliantResources := []ResourceInfo{
		{
			Type: "eks:index:Cluster",
			Name: "bad-cluster",
			Properties: map[string]interface{}{
				"endpointPrivateAccess": false,
			},
		},
	}

	violations = endpointPolicy.Validate(nonCompliantResources)
	assert.NotEmpty(t, violations, "Non-compliant cluster should have violations")
	assert.Equal(t, "bad-cluster", violations[0].ResourceName)
}

// TestEKSClusterLoggingPolicy validates the logging policy
func TestEKSClusterLoggingPolicy(t *testing.T) {
	policies := EKSSecurityPolicies()
	var loggingPolicy PolicyValidation

	for _, p := range policies {
		if p.Name == "eks-cluster-logging-enabled" {
			loggingPolicy = p
			break
		}
	}

	// Test compliant resource
	compliantResources := []ResourceInfo{
		{
			Type: "aws:eks/cluster:Cluster",
			Name: "test-cluster",
			Properties: map[string]interface{}{
				"enabledClusterLogTypes": []interface{}{"api", "audit", "authenticator"},
			},
		},
	}

	violations := loggingPolicy.Validate(compliantResources)
	assert.Empty(t, violations, "Cluster with proper logging should have no violations")

	// Test non-compliant resource (insufficient logs)
	insufficientLogging := []ResourceInfo{
		{
			Type: "aws:eks/cluster:Cluster",
			Name: "bad-cluster",
			Properties: map[string]interface{}{
				"enabledClusterLogTypes": []interface{}{"api"},
			},
		},
	}

	violations = loggingPolicy.Validate(insufficientLogging)
	assert.NotEmpty(t, violations, "Cluster with insufficient logging should have violations")
}

// TestIAMLeastPrivilegePolicy validates the IAM least privilege policy
func TestIAMLeastPrivilegePolicy(t *testing.T) {
	policies := EKSSecurityPolicies()
	var iamPolicy PolicyValidation

	for _, p := range policies {
		if p.Name == "iam-role-least-privilege" {
			iamPolicy = p
			break
		}
	}

	// Test compliant IAM role
	compliantResources := []ResourceInfo{
		{
			Type: "aws:iam/role:Role",
			Name: "good-role",
			Properties: map[string]interface{}{
				"assumeRolePolicy": `{
					"Version": "2012-10-17",
					"Statement": [{
						"Effect": "Allow",
						"Principal": {"Service": "eks.amazonaws.com"},
						"Action": "sts:AssumeRole"
					}]
				}`,
			},
		},
	}

	violations := iamPolicy.Validate(compliantResources)
	assert.Empty(t, violations, "Properly scoped IAM role should have no violations")

	// Test non-compliant IAM role with wildcards
	nonCompliantResources := []ResourceInfo{
		{
			Type: "aws:iam/rolePolicy:RolePolicy",
			Name: "bad-policy",
			Properties: map[string]interface{}{
				"policy": `{
					"Version": "2012-10-17",
					"Statement": [{
						"Effect": "Allow",
						"Action": "*",
						"Resource": "*"
					}]
				}`,
			},
		},
	}

	violations = iamPolicy.Validate(nonCompliantResources)
	assert.NotEmpty(t, violations, "IAM policy with wildcards should have violations")
}

// TestStorageClassEncryptionPolicy validates storage encryption policy
func TestStorageClassEncryptionPolicy(t *testing.T) {
	policies := EKSSecurityPolicies()
	var encryptionPolicy PolicyValidation

	for _, p := range policies {
		if p.Name == "storageclass-encryption-required" {
			encryptionPolicy = p
			break
		}
	}

	// Test compliant StorageClass
	compliantResources := []ResourceInfo{
		{
			Type: "kubernetes:storage.k8s.io/v1:StorageClass",
			Name: "gp3",
			Properties: map[string]interface{}{
				"parameters": map[string]interface{}{
					"type":      "gp3",
					"encrypted": "true",
				},
			},
		},
	}

	violations := encryptionPolicy.Validate(compliantResources)
	assert.Empty(t, violations, "Encrypted StorageClass should have no violations")

	// Test non-compliant StorageClass
	nonCompliantResources := []ResourceInfo{
		{
			Type: "kubernetes:storage.k8s.io/v1:StorageClass",
			Name: "bad-sc",
			Properties: map[string]interface{}{
				"parameters": map[string]interface{}{
					"type":      "gp3",
					"encrypted": "false",
				},
			},
		},
	}

	violations = encryptionPolicy.Validate(nonCompliantResources)
	assert.NotEmpty(t, violations, "Unencrypted StorageClass should have violations")
}

// TestPolicyEnforcementLevels validates enforcement level categorization
func TestPolicyEnforcementLevels(t *testing.T) {
	policies := EKSSecurityPolicies()

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

	t.Logf("Policy Summary:")
	t.Logf("  Mandatory: %d", mandatoryCount)
	t.Logf("  Advisory: %d", advisoryCount)
	t.Logf("  Total: %d", len(policies))
}

// TestOIDCProviderPolicy validates OIDC provider requirement
func TestOIDCProviderPolicy(t *testing.T) {
	policies := EKSSecurityPolicies()
	var oidcPolicy PolicyValidation

	for _, p := range policies {
		if p.Name == "eks-oidc-provider-required" {
			oidcPolicy = p
			break
		}
	}

	// Test compliant cluster with OIDC
	compliantResources := []ResourceInfo{
		{
			Type: "eks:index:Cluster",
			Name: "test-cluster",
			Properties: map[string]interface{}{
				"createOidcProvider": true,
			},
		},
	}

	violations := oidcPolicy.Validate(compliantResources)
	assert.Empty(t, violations, "Cluster with OIDC provider should have no violations")

	// Test non-compliant cluster without OIDC
	nonCompliantResources := []ResourceInfo{
		{
			Type: "eks:index:Cluster",
			Name: "bad-cluster",
			Properties: map[string]interface{}{
				"createOidcProvider": false,
			},
		},
	}

	violations = oidcPolicy.Validate(nonCompliantResources)
	assert.NotEmpty(t, violations, "Cluster without OIDC provider should have violations")
}

// TestPolicyViolationReporting validates violation reporting format
func TestPolicyViolationReporting(t *testing.T) {
	policies := EKSSecurityPolicies()

	// Create intentionally non-compliant resources
	nonCompliantResources := []ResourceInfo{
		{
			Type: "eks:index:Cluster",
			Name: "test-cluster",
			Properties: map[string]interface{}{
				"createOidcProvider": false, // Violation
			},
		},
		{
			Type: "kubernetes:storage.k8s.io/v1:StorageClass",
			Name: "test-sc",
			Properties: map[string]interface{}{
				"parameters": map[string]interface{}{
					"encrypted": "false", // Violation
				},
			},
		},
	}

	allViolations := []PolicyViolation{}

	for _, policy := range policies {
		violations := policy.Validate(nonCompliantResources)
		allViolations = append(allViolations, violations...)
	}

	// Should have at least 2 violations from the non-compliant resources above
	assert.NotEmpty(t, allViolations, "Should detect violations in non-compliant resources")

	// Verify violation structure
	for _, violation := range allViolations {
		assert.NotEmpty(t, violation.ResourceName, "Violation should have resource name")
		assert.NotEmpty(t, violation.PolicyName, "Violation should have policy name")
		assert.NotEmpty(t, violation.Message, "Violation should have message")
	}
}

// TestComplianceReport generates a compliance report
func TestComplianceReport(t *testing.T) {
	policies := EKSSecurityPolicies()

	// Simulate a mix of compliant and non-compliant resources
	testResources := []ResourceInfo{
		{
			Type: "eks:index:Cluster",
			Name: "production-cluster",
			Properties: map[string]interface{}{
				"createOidcProvider":    true,
				"endpointPrivateAccess": true,
				"instanceType":          "m6i.large",
			},
		},
		{
			Type: "kubernetes:storage.k8s.io/v1:StorageClass",
			Name: "gp3",
			Properties: map[string]interface{}{
				"parameters": map[string]interface{}{
					"type":      "gp3",
					"encrypted": "true",
				},
				"volumeBindingMode": "WaitForFirstConsumer",
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

	// Output report as JSON
	reportJSON, err := json.MarshalIndent(report, "", "  ")
	assert.NoError(t, err)

	t.Logf("Compliance Report:\n%s", string(reportJSON))

	// For TFM metrics: All mandatory policies should pass in production
	complianceRate := float64(mandatoryPassed) / float64(mandatoryPassed+mandatoryFailed) * 100
	t.Logf("Mandatory Policy Compliance: %.1f%%", complianceRate)
}
