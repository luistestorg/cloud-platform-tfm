package main

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	ec2x "github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	vpcFlowLogsRoleName = "vpc-flow-logs"
)

// InfraAwsConfig holds configuration for AWS infrastructure
type InfraAwsConfig struct {
	Environment      string
	ProjectName      string
	ClusterName      string
	VpcName          string
	AwsAccountID     string
	EnableFlowLogs   bool
	FlowLogRetention int
	AutoTags         map[string]string
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Load AWS region
		awsConfig := config.New(ctx, "aws")
		region := awsConfig.Require("region")

		// Load infrastructure configuration
		cfg := config.New(ctx, "")
		infraCfg := initInfraConfig(cfg)

		// Register auto-tags for all taggable resources
		if len(infraCfg.AutoTags) > 0 {
			if err := RegisterAutoTags(ctx, infraCfg.AutoTags); err != nil {
				return err
			}
		}

		// Get available zones
		available, err := aws.GetAvailabilityZones(ctx, &aws.GetAvailabilityZonesArgs{
			State: pulumi.StringRef("available"),
		}, nil)
		if err != nil {
			return fmt.Errorf("failed to get availability zones: %w", err)
		}
		fmt.Printf("Available zones: %s\n", strings.Join(available.Names, ", "))

		// Create VPC with default CIDR block (10.0.0.0/16)
		vpc, err := ec2x.NewVpc(ctx, infraCfg.VpcName, nil)
		if err != nil {
			return fmt.Errorf("failed to create VPC: %w", err)
		}

		// Create Flow Logs if enabled
		if infraCfg.EnableFlowLogs {
			if err := CreateFlowLogs(ctx, vpc, infraCfg); err != nil {
				return err
			}
		}

		// Create base IAM role for EKS instances
		instanceRoleName := fmt.Sprintf("%s-instance-role", infraCfg.ClusterName)
		instanceRole, err := createEksInstanceRole(ctx, instanceRoleName)
		if err != nil {
			return fmt.Errorf("failed to create instance role: %w", err)
		}

		// Export outputs for Stack References
		ctx.Export("environment", pulumi.String(infraCfg.Environment))
		ctx.Export("projectName", pulumi.String(infraCfg.ProjectName))
		ctx.Export("clusterName", pulumi.String(infraCfg.ClusterName))
		ctx.Export("awsRegion", pulumi.String(region))
		ctx.Export("awsAccountId", pulumi.String(infraCfg.AwsAccountID))
		ctx.Export("vpcId", vpc.VpcId)
		ctx.Export("vpcCidr", vpc.Vpc.CidrBlock())
		ctx.Export("privateSubnetIds", vpc.PrivateSubnetIds)
		ctx.Export("publicSubnetIds", vpc.PublicSubnetIds)
		ctx.Export("instanceRoleName", pulumi.String(instanceRoleName))
		ctx.Export("instanceRoleArn", instanceRole.Arn)
		ctx.Export("enableFlowLogs", pulumi.Bool(infraCfg.EnableFlowLogs))

		fmt.Printf("AWS infrastructure for cluster '%s' created successfully\n", infraCfg.ClusterName)

		return nil
	})
}

// initInfraConfig initializes infrastructure configuration from Pulumi config
func initInfraConfig(cfg *config.Config) *InfraAwsConfig {
	infraCfg := &InfraAwsConfig{
		Environment:      cfg.Get("environment"),
		ProjectName:      cfg.Get("projectName"),
		ClusterName:      cfg.Get("clusterName"),
		VpcName:          cfg.Get("vpcName"),
		AwsAccountID:     cfg.Get("awsAccountId"),
		EnableFlowLogs:   cfg.GetBool("enableFlowLogs"),
		FlowLogRetention: cfg.GetInt("flowLogRetention"),
	}

	// Set defaults
	if infraCfg.Environment == "" {
		infraCfg.Environment = "dev"
	}
	if infraCfg.ProjectName == "" {
		infraCfg.ProjectName = "cloud-platform-tfm"
	}
	if infraCfg.ClusterName == "" {
		infraCfg.ClusterName = "tfm-dev"
	}
	if infraCfg.VpcName == "" {
		infraCfg.VpcName = fmt.Sprintf("%s-vpc", infraCfg.ClusterName)
	}
	if infraCfg.FlowLogRetention == 0 {
		infraCfg.FlowLogRetention = 7
	}

	// Load auto-tags
	var autoTags map[string]string
	if err := cfg.TryObject("autoTags", &autoTags); err == nil {
		infraCfg.AutoTags = autoTags
	}

	return infraCfg
}

// createEksInstanceRole creates the IAM role for EKS worker nodes
func createEksInstanceRole(ctx *pulumi.Context, roleName string) (*iam.Role, error) {
	arns := pulumi.ToStringArray([]string{
		"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
		"arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
		"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
	})

	assumeRolePolicy := pulumi.String(`{
		"Version": "2012-10-17",
		"Statement": [{
			"Sid": "",
			"Effect": "Allow",
			"Principal": {
				"Service": "ec2.amazonaws.com"
			},
			"Action": "sts:AssumeRole"
		}]
	}`)

	return iam.NewRole(ctx, roleName, &iam.RoleArgs{
		Name:              pulumi.String(roleName),
		AssumeRolePolicy:  assumeRolePolicy,
		ManagedPolicyArns: arns,
	})
}

// CreateFlowLogs creates VPC Flow Logs for network traffic analysis
func CreateFlowLogs(ctx *pulumi.Context, vpc *ec2x.Vpc, cfg *InfraAwsConfig) error {
	// Look up the existing Flow Logs IAM role
	flowLogsRole, err := iam.LookupRole(ctx, &iam.LookupRoleArgs{
		Name: vpcFlowLogsRoleName,
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to lookup flow logs role: %w", err)
	}

	// Create CloudWatch Log Group for Flow Logs
	logGroup, err := cloudwatch.NewLogGroup(ctx, vpcFlowLogsRoleName, &cloudwatch.LogGroupArgs{
		Name:            pulumi.String(vpcFlowLogsRoleName),
		RetentionInDays: pulumi.Int(cfg.FlowLogRetention),
		Tags: pulumi.StringMap{
			"VpcName":     pulumi.String(cfg.VpcName),
			"ClusterName": pulumi.String(cfg.ClusterName),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create log group: %w", err)
	}

	// Create the VPC Flow Log
	_, err = ec2.NewFlowLog(ctx, vpcFlowLogsRoleName, &ec2.FlowLogArgs{
		IamRoleArn:     pulumi.String(flowLogsRole.Arn),
		LogDestination: logGroup.Arn,
		TrafficType:    pulumi.String("ALL"),
		VpcId:          vpc.VpcId,
	})
	if err != nil {
		return fmt.Errorf("failed to create flow log: %w", err)
	}

	// Export flow log information
	ctx.Export("flowLogGroupId", logGroup.Name)
	ctx.Export("flowLogRoleArn", pulumi.String(flowLogsRole.Arn))

	return nil
}

// RegisterAutoTags registers a stack transformation to auto-tag all taggable resources
func RegisterAutoTags(ctx *pulumi.Context, autoTags map[string]string) error {
	return ctx.RegisterStackTransformation(
		func(args *pulumi.ResourceTransformationArgs) *pulumi.ResourceTransformationResult {
			if args.Props != nil && isTaggable(args.Type) {
				ptr := reflect.ValueOf(args.Props)
				if !ptr.IsZero() {
					val := ptr.Elem()
					if !val.IsZero() {
						tags := val.FieldByName("Tags")

						var tagsMap pulumi.StringMap
						if !tags.IsZero() {
							tagsMap = tags.Interface().(pulumi.StringMap)
						} else {
							tagsMap = pulumi.StringMap{}
						}
						for k, v := range autoTags {
							tagsMap[k] = pulumi.String(v)
						}
						tags.Set(reflect.ValueOf(tagsMap))

						return &pulumi.ResourceTransformationResult{Props: args.Props, Opts: args.Opts}
					}
				}
			}
			return nil
		},
	)
}

// isTaggable checks if a resource type supports tagging
func isTaggable(t string) bool {
	for _, trt := range taggableResourceTypes {
		if t == trt {
			return true
		}
	}
	return false
}

// taggableResourceTypes is a list of AWS resource types that support tagging
// Reduced list for TFM demo - includes only commonly used resources
var taggableResourceTypes = []string{
	"aws:cloudwatch/logGroup:LogGroup",
	"aws:ec2/customerGateway:CustomerGateway",
	"aws:ec2/defaultNetworkAcl:DefaultNetworkAcl",
	"aws:ec2/defaultRouteTable:DefaultRouteTable",
	"aws:ec2/defaultSecurityGroup:DefaultSecurityGroup",
	"aws:ec2/defaultSubnet:DefaultSubnet",
	"aws:ec2/defaultVpc:DefaultVpc",
	"aws:ec2/eip:Eip",
	"aws:ec2/instance:Instance",
	"aws:ec2/internetGateway:InternetGateway",
	"aws:ec2/keyPair:KeyPair",
	"aws:ec2/launchTemplate:LaunchTemplate",
	"aws:ec2/natGateway:NatGateway",
	"aws:ec2/networkAcl:NetworkAcl",
	"aws:ec2/networkInterface:NetworkInterface",
	"aws:ec2/routeTable:RouteTable",
	"aws:ec2/securityGroup:SecurityGroup",
	"aws:ec2/subnet:Subnet",
	"aws:ec2/vpc:Vpc",
	"aws:ec2/vpcEndpoint:VpcEndpoint",
	"aws:eks/cluster:Cluster",
	"aws:eks/nodeGroup:NodeGroup",
	"aws:iam/role:Role",
	"aws:iam/user:User",
	"aws:lb/loadBalancer:LoadBalancer",
	"aws:lb/targetGroup:TargetGroup",
	"aws:route53/zone:Zone",
	"aws:s3/bucket:Bucket",
}
