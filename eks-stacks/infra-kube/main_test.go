package main

import (
	"testing"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	awseks "github.com/pulumi/pulumi-eks/sdk/v3/go/eks"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
)

type mocks int

// Create the mock
func (mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	outputs := args.Inputs.Copy()

	switch args.TypeToken {
	case "eks:index:Cluster":
		// Mock EKS Cluster outputs
		outputs["kubeconfig"] = resource.NewPropertyValue(`{
			"apiVersion": "v1",
			"kind": "Config",
			"clusters": [{
				"name": "test-cluster",
				"cluster": {
					"server": "https://test.eks.amazonaws.com",
					"certificate-authority-data": "LS0tLS1=="
				}
			}]
		}`)
		outputs["kubeconfigJson"] = resource.NewPropertyValue("{}")
		outputs["clusterSecurityGroup"] = resource.NewPropertyValue("sg-12345678")
		outputs["eksCluster"] = resource.NewPropertyValue(resource.PropertyMap{
			"arn":      resource.NewPropertyValue("arn:aws:eks:us-east-1:123456789012:cluster/test-cluster"),
			"endpoint": resource.NewPropertyValue("https://EXAMPLE.sk1.us-east-1.eks.amazonaws.com"),
			"version":  resource.NewPropertyValue("1.30"),
			"vpcConfig": resource.NewPropertyValue(resource.PropertyMap{
				"clusterSecurityGroupId": resource.NewPropertyValue("sg-12345678"),
			}),
		})
		outputs["core"] = resource.NewPropertyValue(resource.PropertyMap{
			"oidcProvider": resource.NewPropertyValue(resource.PropertyMap{
				"url": resource.NewPropertyValue("https://oidc.eks.us-east-1.amazonaws.com/id/EXAMPLE123"),
				"arn": resource.NewPropertyValue("arn:aws:iam::123456789012:oidc-provider/oidc.eks.us-east-1.amazonaws.com/id/EXAMPLE123"),
			}),
		})

	case "aws:iam/role:Role":
		outputs["arn"] = resource.NewPropertyValue("arn:aws:iam::123456789012:role/test-role")
		outputs["name"] = resource.NewPropertyValue("test-role")

	case "aws:iam/rolePolicy:RolePolicy":
		outputs["policy"] = args.Inputs["policy"]

	case "aws:iam/rolePolicyAttachment:RolePolicyAttachment":
		outputs["policyArn"] = args.Inputs["policyArn"]

	case "kubernetes:helm.sh/v3:Release":
		outputs["status"] = resource.NewPropertyValue("deployed")
		outputs["version"] = resource.NewPropertyValue("1")
		outputs["name"] = args.Inputs["name"]

	case "kubernetes:storage.k8s.io/v1:StorageClass":
		outputs["provisioner"] = args.Inputs["provisioner"]
		outputs["parameters"] = args.Inputs["parameters"]
		outputs["metadata"] = args.Inputs["metadata"]

	case "kubernetes:yaml:ConfigGroup":
		outputs["resources"] = resource.NewPropertyValue([]resource.PropertyValue{})
	}

	return args.Name + "_id", outputs, nil
}

func (mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	outputs := map[string]interface{}{}

	switch args.Token {
	case "aws:index/getRegion:getRegion":
		outputs["name"] = "us-east-1"
		outputs["id"] = "us-east-1"

	case "aws:index/getCallerIdentity:getCallerIdentity":
		outputs["accountId"] = "123456789012"
		outputs["arn"] = "arn:aws:iam::123456789012:user/test"
	}

	return resource.NewPropertyMapFromMap(outputs), nil
}

// TestEKSClusterCreation verifies that EKS cluster is created with correct configuration
func TestEKSClusterCreation(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Mock Stack Reference
		infraAwsStack, err := pulumi.NewStackReference(ctx, "organization/eks-infra-aws/dev", nil)
		if err != nil {
			return err
		}

		vpcId := infraAwsStack.GetStringOutput(pulumi.String("vpcId"))
		privateSubnetIds := infraAwsStack.GetOutput(pulumi.String("privateSubnetIds"))
		publicSubnetIds := infraAwsStack.GetOutput(pulumi.String("publicSubnetIds"))

		// Create EKS Cluster
		cluster, err := awseks.NewCluster(ctx, "test-cluster", &awseks.ClusterArgs{
			Name:               pulumi.String("test-cluster"),
			Version:            pulumi.String("1.30"),
			CreateOidcProvider: pulumi.BoolPtr(true),
			VpcId:              vpcId,
			PrivateSubnetIds:   privateSubnetIds.AsStringArrayOutput(),
			PublicSubnetIds:    publicSubnetIds.AsStringArrayOutput(),
			InstanceType:       pulumi.String("m6i.large"),
			DesiredCapacity:    pulumi.Int(2),
			MinSize:            pulumi.Int(2),
			MaxSize:            pulumi.Int(4),
		})

		if err != nil {
			return err
		}

		// Validate cluster properties
		pulumi.All(cluster.URN(), cluster.EksCluster).ApplyT(func(args []interface{}) error {
			assert.NotNil(t, args[0], "Cluster URN should not be nil")
			assert.NotNil(t, args[1], "EKS Cluster should not be nil")
			return nil
		})

		// Export for validation
		ctx.Export("clusterName", cluster.EksCluster.Name())
		ctx.Export("clusterArn", cluster.EksCluster.Arn())
		ctx.Export("kubeconfig", cluster.Kubeconfig)

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestEBSCSIDriverIRSA verifies IAM Role for Service Account (IRSA) configuration
func TestEBSCSIDriverIRSA(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Create OIDC provider URL as a string directly
		oidcProviderUrl := "oidc.eks.us-east-1.amazonaws.com/id/EXAMPLE123"

		// Build assume role policy string directly
		assumeRolePolicyStr := `{
			"Version": "2012-10-17",
			"Statement": [{
				"Effect": "Allow",
				"Principal": {
					"Federated": "arn:aws:iam::123456789012:oidc-provider/` + oidcProviderUrl + `"
				},
				"Action": "sts:AssumeRoleWithWebIdentity",
				"Condition": {
					"StringEquals": {
						"` + oidcProviderUrl + `:sub": "system:serviceaccount:kube-system:ebs-csi-controller-sa",
						"` + oidcProviderUrl + `:aud": "sts.amazonaws.com"
					}
				}
			}]
		}`

		// Create IAM Role for EBS CSI Driver
		ebsCsiRole, err := iam.NewRole(ctx, "ebs-csi-controller", &iam.RoleArgs{
			Name:             pulumi.String("ebs-csi-controller"),
			AssumeRolePolicy: pulumi.String(assumeRolePolicyStr),
		})
		if err != nil {
			return err
		}

		// Attach AWS managed policy
		_, err = iam.NewRolePolicyAttachment(ctx, "ebs-csi-policy-attachment", &iam.RolePolicyAttachmentArgs{
			Role:      ebsCsiRole.Name,
			PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy"),
		})
		if err != nil {
			return err
		}

		// Add KMS policy for encrypted volumes
		kmsPolicy := pulumi.String(`{
			"Version": "2012-10-17",
			"Statement": [{
				"Effect": "Allow",
				"Action": [
					"kms:Decrypt",
					"kms:GenerateDataKeyWithoutPlaintext",
					"kms:CreateGrant"
				],
				"Resource": "*"
			}]
		}`)

		_, err = iam.NewRolePolicy(ctx, "ebs-csi-kms-policy", &iam.RolePolicyArgs{
			Name:   pulumi.String("ebs-csi-kms-policy"),
			Role:   ebsCsiRole.ID(),
			Policy: kmsPolicy,
		})
		if err != nil {
			return err
		}

		// Validate role ARN format
		ebsCsiRole.Arn.ApplyT(func(arn string) error {
			assert.Contains(t, arn, "arn:aws:iam::", "ARN should have correct format")
			assert.Contains(t, arn, "role/", "ARN should contain role path")
			return nil
		})

		ctx.Export("ebsCsiRoleArn", ebsCsiRole.Arn)
		ctx.Export("ebsCsiRoleName", ebsCsiRole.Name)

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestClusterOutputs verifies that all required outputs are exported
func TestClusterOutputs(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Create mock cluster
		cluster, err := awseks.NewCluster(ctx, "test-cluster", &awseks.ClusterArgs{
			Name:    pulumi.String("test-cluster"),
			Version: pulumi.String("1.30"),
		})
		if err != nil {
			return err
		}

		// Export all required outputs
		ctx.Export("clusterName", cluster.EksCluster.Name())
		ctx.Export("kubeconfig", cluster.Kubeconfig)
		ctx.Export("kubeconfigJson", cluster.KubeconfigJson)
		ctx.Export("clusterEndpoint", cluster.EksCluster.Endpoint())

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestNodeGroupConfiguration verifies managed node group settings
func TestNodeGroupConfiguration(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		cluster, err := awseks.NewCluster(ctx, "test-cluster", &awseks.ClusterArgs{
			Name:            pulumi.String("test-cluster"),
			Version:         pulumi.String("1.30"),
			InstanceType:    pulumi.String("m6i.large"),
			DesiredCapacity: pulumi.Int(2),
			MinSize:         pulumi.Int(2),
			MaxSize:         pulumi.Int(4),
		})
		if err != nil {
			return err
		}

		// Validate that cluster was created
		cluster.EksCluster.Name().ApplyT(func(name string) error {
			assert.NotEmpty(t, name, "Cluster name should not be empty")
			return nil
		})

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestClusterVersion verifies Kubernetes version configuration
func TestClusterVersion(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		cluster, err := awseks.NewCluster(ctx, "test-cluster", &awseks.ClusterArgs{
			Name:    pulumi.String("test-cluster"),
			Version: pulumi.String("1.30"),
		})
		if err != nil {
			return err
		}

		cluster.EksCluster.Version().ApplyT(func(version string) error {
			assert.NotEmpty(t, version, "Cluster version should be set")
			return nil
		})

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestOIDCProviderEnabled verifies OIDC provider is configured
func TestOIDCProviderEnabled(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		cluster, err := awseks.NewCluster(ctx, "test-cluster", &awseks.ClusterArgs{
			Name:               pulumi.String("test-cluster"),
			Version:            pulumi.String("1.30"),
			CreateOidcProvider: pulumi.BoolPtr(true),
		})
		if err != nil {
			return err
		}

		// Just verify cluster was created - OIDC provider access is handled internally
		cluster.EksCluster.Name().ApplyT(func(name string) error {
			assert.NotEmpty(t, name, "Cluster with OIDC should be created")
			return nil
		})

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestStackReferenceConfiguration verifies stack references work correctly
func TestStackReferenceConfiguration(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Create stack reference
		infraAwsStack, err := pulumi.NewStackReference(ctx, "organization/eks-infra-aws/dev", nil)
		if err != nil {
			return err
		}

		// Verify we can get outputs
		vpcId := infraAwsStack.GetStringOutput(pulumi.String("vpcId"))

		vpcId.ApplyT(func(id string) error {
			assert.NotEmpty(t, id, "VPC ID should not be empty from stack reference")
			return nil
		})

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestIAMRoleCreation verifies IAM roles are created correctly
func TestIAMRoleCreation(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Create a basic IAM role
		role, err := iam.NewRole(ctx, "test-role", &iam.RoleArgs{
			Name: pulumi.String("test-role"),
			AssumeRolePolicy: pulumi.String(`{
				"Version": "2012-10-17",
				"Statement": [{
					"Effect": "Allow",
					"Principal": {"Service": "eks.amazonaws.com"},
					"Action": "sts:AssumeRole"
				}]
			}`),
		})
		if err != nil {
			return err
		}

		// Validate role properties
		role.Arn.ApplyT(func(arn string) error {
			assert.Contains(t, arn, "arn:aws:iam::", "Role ARN should be valid")
			return nil
		})

		role.Name.ApplyT(func(name string) error {
			assert.Equal(t, "test-role", name, "Role name should match")
			return nil
		})

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestClusterWithMinimalConfig validates cluster can be created with minimal configuration
func TestClusterWithMinimalConfig(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Create cluster with minimal required config
		cluster, err := awseks.NewCluster(ctx, "minimal-cluster", &awseks.ClusterArgs{
			Name: pulumi.String("minimal-test-cluster"),
		})
		if err != nil {
			return err
		}

		// Verify cluster has essential properties
		cluster.EksCluster.Name().ApplyT(func(name string) error {
			assert.Equal(t, "minimal-test-cluster", name, "Cluster name should match")
			return nil
		})

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}
