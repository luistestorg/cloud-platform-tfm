package main

import (
	"fmt"

	awsiam "github.com/pulumi/pulumi-aws-iam/sdk/go/aws-iam"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/iam"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	"unir-tfm.com/shared"
)

const (
	awsGatewayAPIControllerChartVers = "v1.1.0"
)

type ApiConfig struct {
	ProjectName                string
	Environment                string
	Domain                     string
	Route53ZoneID              string
	EnableAwsGatewayController bool
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Load configuration
		cfg := config.New(ctx, "")
		awsCfg := config.New(ctx, "aws")

		// Get stack reference to infra-kube
		infraKubeStackRef := cfg.Require("infraKubeStackRef")
		infraKubeStack, err := pulumi.NewStackReference(ctx, infraKubeStackRef, nil)
		if err != nil {
			return err
		}

		// Get outputs from infra-kube stack
		clusterName := infraKubeStack.GetStringOutput(pulumi.String("clusterName"))
		environment := infraKubeStack.GetStringOutput(pulumi.String("environment"))
		awsRegion := awsCfg.Require("region")

		// Initialize API config
		apiCfg := &ApiConfig{
			ProjectName:                cfg.Get("projectName"),
			Domain:                     cfg.Get("domain"),
			Route53ZoneID:              cfg.Get("route53ZoneId"),
			EnableAwsGatewayController: cfg.GetBool("enableAwsGatewayController"),
		}

		// Export outputs
		ctx.Export("clusterName", clusterName)
		ctx.Export("environment", environment)
		ctx.Export("awsRegion", pulumi.String(awsRegion))
		ctx.Export("domain", pulumi.String(apiCfg.Domain))

		return nil
	})
}

// DeployAwsGatewayController deploys AWS Gateway API Controller for VPC Lattice integration
func DeployAwsGatewayController(ctx *pulumi.Context, s *shared.Stack) error {
	// Get VPC Lattice prefix list
	filter := ec2.GetManagedPrefixListsFilter{Name: "prefix-list-name", Values: []string{fmt.Sprintf("com.amazonaws.%s.vpc-lattice", s.Region)}}
	lists, err := ec2.GetManagedPrefixLists(ctx, &ec2.GetManagedPrefixListsArgs{Filters: []ec2.GetManagedPrefixListsFilter{filter}})
	if err != nil {
		fmt.Printf("\nERROR: describe-managed-prefix-lists failed due to: %v\n", err)
		return err
	}

	prefixID := ""
	if lists != nil && len(lists.Ids) > 0 {
		prefixID = lists.Ids[0]
	}
	if prefixID == "" {
		fmt.Printf("\nWARN: describe-managed-prefix-lists returned empty results\n")
		return nil
	}

	// Add security group rule for VPC Lattice
	_, err = ec2.NewSecurityGroupRule(ctx, "sg-ingress-vpclattice", &ec2.SecurityGroupRuleArgs{
		SecurityGroupId: s.Eks.EksCluster.VpcConfig().ClusterSecurityGroupId().Elem(),
		FromPort:        pulumi.Int(-1),
		ToPort:          pulumi.Int(-1),
		Protocol:        pulumi.String("-1"),
		PrefixListIds:   pulumi.ToStringArray([]string{prefixID}),
		Type:            pulumi.String("ingress"),
	})
	if err != nil {
		fmt.Printf("\nERROR: authorize-security-group-ingress failed due to: %v \n", err)
		return err
	}

	// Create namespace
	nsName := "aws-application-networking-system"
	ns, err := s.CreateNamespace(ctx, nsName)
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, ns)

	// IRSA for the controller
	saAndNs := fmt.Sprintf("%s:gateway-api-controller", nsName)
	roleName := s.ClusterScopedResourceName("aws-gateway-api-irsa")
	serviceAccount := awsiam.EKSServiceAccountArgs{Name: s.Eks.EksCluster.Name(), ServiceAccounts: pulumi.ToStringArray([]string{saAndNs})}
	eksRole, err := awsiam.NewEKSRole(ctx, roleName, &awsiam.EKSRoleArgs{
		Role:                   awsiam.RoleArgs{Name: pulumi.String(roleName)},
		ClusterServiceAccounts: awsiam.EKSServiceAccountArray([]awsiam.EKSServiceAccountInput{serviceAccount}),
	}, pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}

	// IAM policy for Gateway API controller
	policyName := s.ClusterScopedResourceName("aws-gateway-api-irsa-policy")
	policyJSON := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Action": [
					"vpc-lattice:*",
					"ec2:DescribeVpcs",
					"ec2:DescribeSubnets",
					"ec2:DescribeTags",
					"ec2:DescribeSecurityGroups",
					"logs:CreateLogDelivery",
					"logs:GetLogDelivery",
					"logs:DescribeLogGroups",
					"logs:PutResourcePolicy",
					"logs:DescribeResourcePolicies",
					"logs:UpdateLogDelivery",
					"logs:DeleteLogDelivery",
					"logs:ListLogDeliveries",
					"tag:GetResources",
					"firehose:TagDeliveryStream",
					"s3:GetBucketPolicy",
					"s3:PutBucketPolicy"
				],
				"Resource": "*"
			},
			{
				"Effect" : "Allow",
				"Action" : "iam:CreateServiceLinkedRole",
				"Resource" : "arn:aws:iam::*:role/aws-service-role/vpc-lattice.amazonaws.com/AWSServiceRoleForVpcLattice",
				"Condition" : {
					"StringLike" : {
						"iam:AWSServiceName" : "vpc-lattice.amazonaws.com"
					}
				}
			},
			{
				"Effect" : "Allow",
				"Action" : "iam:CreateServiceLinkedRole",
				"Resource" : "arn:aws:iam::*:role/aws-service-role/delivery.logs.amazonaws.com/AWSServiceRoleForLogDelivery",
				"Condition" : {
					"StringLike" : {
						"iam:AWSServiceName" : "delivery.logs.amazonaws.com"
					}
				}
			}
		]
	}`
	rolePolicy, err := iam.NewRolePolicy(ctx, policyName,
		&iam.RolePolicyArgs{Name: pulumi.String(policyName), Role: eksRole.Name, Policy: pulumi.String(policyJSON)}, pulumi.DependsOn([]pulumi.Resource{eksRole}))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, rolePolicy)

	// Deploy Helm chart
	customValues := pulumi.Map{
		"fullnameOverride": pulumi.String("gateway-api"),
		"deployment": pulumi.Map{
			"replicas": pulumi.Int(1),
		},
		"serviceAccount": pulumi.Map{
			"annotations": pulumi.StringMap{"eks.amazonaws.com/role-arn": eksRole.Arn},
		},
		"defaultServiceNetwork": pulumi.String("test-grpc-gateway"),
		"resources":             s.Resources.GatewayApi,
	}
	if _, err = s.DeployHelmRelease(ctx, ns, "aws-gateway-controller-chart", awsGatewayAPIControllerChartVers, "", "", customValues); err != nil {
		return err
	}

	// Install Gateway API CRDs
	configGroup, err := yaml.NewConfigGroup(ctx, "gateway-api-crds", &yaml.ConfigGroupArgs{
		Files: []string{"https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/experimental-install.yaml"},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, configGroup)

	// Create GatewayClass
	gatewayClassSpec, err := shared.JSONToMap(`{ "spec": { "controllerName": "application-networking.k8s.aws/gateway-api-controller" }}`)
	if err != nil {
		return err
	}
	gatewayClass, err := apiextensions.NewCustomResource(ctx, "aws-vpc-lattice-gateway", &apiextensions.CustomResourceArgs{
		ApiVersion:  pulumi.String("gateway.networking.k8s.io/v1beta1"),
		Kind:        pulumi.String("GatewayClass"),
		Metadata:    metav1.ObjectMetaArgs{Name: pulumi.String("amazon-vpc-lattice"), Namespace: ns.Metadata.Name()},
		OtherFields: gatewayClassSpec,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, gatewayClass)

	return nil
}

// createRoute53IamRole creates an IAM role for Route53 DNS management
func createRoute53IamRole(ctx *pulumi.Context, roleName string, s *shared.Stack, awsAccountID string) (*iam.Role, error) {
	assumeRolePolicy := pulumi.Sprintf(`{
	  "Version": "2012-10-17",
	  "Statement": [
		{
		  "Effect": "Allow",
		  "Principal": {
			"AWS": "arn:aws:iam::%s:role/%s"
		  },
		  "Action": "sts:AssumeRole"
		}
	  ]
	}`, awsAccountID, s.InstanceRoleName)

	policyJSON := pulumi.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Action": "route53:GetChange",
				"Resource": "arn:aws:route53:::change/*"
			},
			{
				"Effect": "Allow",
				"Action": [
					"route53:ChangeResourceRecordSets",
					"route53:ListResourceRecordSets"
				],
				"Resource": "arn:aws:route53:::hostedzone/%s"
			}
		]
	}`, s.TLSCfg.Route53ZoneID)

	iamRole, err := iam.NewRole(ctx, "Route53", &iam.RoleArgs{Name: pulumi.String(roleName), AssumeRolePolicy: assumeRolePolicy})
	if err != nil {
		return nil, err
	}

	policyName := s.ClusterScopedResourceName("Route53Policy")
	_, err = iam.NewRolePolicy(ctx, "Route53Policy",
		&iam.RolePolicyArgs{Name: pulumi.String(policyName), Role: iamRole.Name, Policy: policyJSON}, pulumi.DependsOn([]pulumi.Resource{iamRole}))

	return iamRole, err
}
