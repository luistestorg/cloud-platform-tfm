package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/rds"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

type sqlStack struct {
	dbPassword       pulumi.StringOutput
	pgPassword       pulumi.StringOutput
	dbName           string
	dbInstanceType   string
	dbEngineVersion  string
	dbStorage        int
	zone             string
	backupRetention  int
	vpcId            pulumi.StringOutput
	vpcCidr          pulumi.StringOutput
	privateSubnetIds pulumi.StringArrayOutput
	clusterName      pulumi.StringOutput
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")

		// Get stack reference to infra-aws
		infraStackRef := cfg.Require("infraStackRef")
		stackRef, err := pulumi.NewStackReference(ctx, infraStackRef, nil)
		if err != nil {
			return err
		}

		// Initialize SQL stack configuration
		sqlCfg := &sqlStack{
			dbPassword:      cfg.RequireSecret("dbPassword"),
			pgPassword:      cfg.RequireSecret("pgPassword"),
			dbName:          cfg.Require("dbName"),
			dbInstanceType:  cfg.Require("dbInstanceType"),
			dbEngineVersion: cfg.Get("dbEngineVersion"),
			zone:            cfg.Require("zone"),
			backupRetention: 7,
		}

		// Set defaults
		if sqlCfg.dbEngineVersion == "" {
			sqlCfg.dbEngineVersion = "16.3"
		}

		if storage := cfg.GetInt("dbStorage"); storage > 0 {
			sqlCfg.dbStorage = storage
		} else {
			sqlCfg.dbStorage = 100
		}

		if retention := cfg.GetInt("backupRetention"); retention > 0 {
			sqlCfg.backupRetention = retention
		}

		// Get VPC outputs from infra-aws stack
		sqlCfg.vpcId = stackRef.GetStringOutput(pulumi.String("vpcId"))
		sqlCfg.privateSubnetIds = stackRef.GetOutput(pulumi.String("privateSubnetIds")).AsStringArrayOutput()
		sqlCfg.vpcCidr = stackRef.GetStringOutput(pulumi.String("vpcCidr"))
		sqlCfg.clusterName = stackRef.GetStringOutput(pulumi.String("clusterName"))

		// Create RDS instance
		rdsInstance, err := createRDS(ctx, sqlCfg)
		if err != nil {
			return err
		}

		// Export outputs
		ctx.Export("rdsEndpoint", rdsInstance.Endpoint)
		ctx.Export("rdsAddress", rdsInstance.Address)
		ctx.Export("rdsPort", rdsInstance.Port)
		ctx.Export("rdsDbName", rdsInstance.DbName)
		ctx.Export("rdsArn", rdsInstance.Arn)
		ctx.Export("rdsAvailabilityZone", rdsInstance.AvailabilityZone)
		ctx.Export("dbPassword", sqlCfg.dbPassword)
		ctx.Export("dbName", pulumi.String(sqlCfg.dbName))

		return nil
	})
}

func createRDS(ctx *pulumi.Context, s *sqlStack) (*rds.Instance, error) {
	// Create Security Group for RDS
	rdsSg, err := ec2.NewSecurityGroup(ctx, fmt.Sprintf("%s-rds-sg", s.dbName), &ec2.SecurityGroupArgs{
		Name:        pulumi.Sprintf("%s-rds-sg", s.dbName),
		Description: pulumi.String("Security group for RDS PostgreSQL"),
		VpcId:       s.vpcId,
		Ingress: ec2.SecurityGroupIngressArray{
			&ec2.SecurityGroupIngressArgs{
				Description: pulumi.String("PostgreSQL from VPC"),
				FromPort:    pulumi.Int(5432),
				ToPort:      pulumi.Int(5432),
				Protocol:    pulumi.String("tcp"),
				CidrBlocks:  pulumi.StringArray{s.vpcCidr},
			},
		},
		Egress: ec2.SecurityGroupEgressArray{
			&ec2.SecurityGroupEgressArgs{
				FromPort:   pulumi.Int(0),
				ToPort:     pulumi.Int(0),
				Protocol:   pulumi.String("-1"),
				CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
			},
		},
		Tags: pulumi.StringMap{
			"Name": pulumi.Sprintf("%s-rds-sg", s.dbName),
		},
	})
	if err != nil {
		return nil, err
	}

	// Create Subnet Group for RDS
	subnetGroup, err := rds.NewSubnetGroup(ctx, fmt.Sprintf("%s-rds-subnets", s.dbName), &rds.SubnetGroupArgs{
		Name:        pulumi.Sprintf("%s-rds-subnets", s.dbName),
		Description: pulumi.Sprintf("Subnet group for %s RDS instance", s.dbName),
		SubnetIds:   s.privateSubnetIds,
		Tags: pulumi.StringMap{
			"Name": pulumi.Sprintf("%s-rds-subnets", s.dbName),
		},
	})
	if err != nil {
		return nil, err
	}

	// Create Parameter Group for PostgreSQL
	paramGroup, err := rds.NewParameterGroup(ctx, fmt.Sprintf("%s-pg-params", s.dbName), &rds.ParameterGroupArgs{
		Name:        pulumi.Sprintf("%s-pg-params", s.dbName),
		Family:      pulumi.String("postgres16"),
		Description: pulumi.Sprintf("Parameter group for %s", s.dbName),
		Parameters: rds.ParameterGroupParameterArray{
			&rds.ParameterGroupParameterArgs{
				Name:  pulumi.String("log_connections"),
				Value: pulumi.String("1"),
			},
			&rds.ParameterGroupParameterArgs{
				Name:  pulumi.String("log_disconnections"),
				Value: pulumi.String("1"),
			},
		},
		Tags: pulumi.StringMap{
			"Name": pulumi.Sprintf("%s-pg-params", s.dbName),
		},
	})
	if err != nil {
		return nil, err
	}

	// Create RDS Instance
	rdsInst, err := rds.NewInstance(ctx, fmt.Sprintf("%s-rds", s.dbName), &rds.InstanceArgs{
		Identifier:               pulumi.String(s.dbName),
		AvailabilityZone:         pulumi.String(s.zone),
		AllocatedStorage:         pulumi.Int(s.dbStorage),
		MaxAllocatedStorage:      pulumi.Int(s.dbStorage * 2), // Enable storage autoscaling
		AllowMajorVersionUpgrade: pulumi.BoolPtr(false),
		AutoMinorVersionUpgrade:  pulumi.BoolPtr(true),
		BackupRetentionPeriod:    pulumi.Int(s.backupRetention),
		BackupWindow:             pulumi.String("03:00-04:00"),
		MaintenanceWindow:        pulumi.String("Mon:04:00-Mon:05:00"),
		Engine:                   pulumi.String("postgres"),
		EngineVersion:            pulumi.String(s.dbEngineVersion),
		InstanceClass:            pulumi.String(s.dbInstanceType),
		DbName:                   pulumi.String("tfmdb"),
		Username:                 pulumi.String("tfmdb"),
		Password:                 s.dbPassword,
		ParameterGroupName:       paramGroup.Name,
		ApplyImmediately:         pulumi.Bool(true),
		StorageEncrypted:         pulumi.Bool(true),
		StorageType:              pulumi.String("gp3"),
		Iops:                     pulumi.Int(3000),
		VpcSecurityGroupIds:      pulumi.StringArray{rdsSg.ID()},
		DbSubnetGroupName:        subnetGroup.Name,
		PubliclyAccessible:       pulumi.Bool(false),
		MultiAz:                  pulumi.Bool(false), // Set to true for production
		DeletionProtection:       pulumi.Bool(false), // Set to true for production
		SkipFinalSnapshot:        pulumi.BoolPtr(true),
		FinalSnapshotIdentifier:  pulumi.Sprintf("%s-final-snapshot", s.dbName),
		CopyTagsToSnapshot:       pulumi.Bool(true),
		EnabledCloudwatchLogsExports: pulumi.ToStringArray([]string{
			"postgresql",
			"upgrade",
		}),
		PerformanceInsightsEnabled:         pulumi.Bool(true),
		PerformanceInsightsRetentionPeriod: pulumi.Int(7),
		Tags: pulumi.StringMap{
			"Name":        pulumi.String(s.dbName),
			"Environment": pulumi.String(ctx.Stack()),
		},
	}, pulumi.DependsOn([]pulumi.Resource{rdsSg, subnetGroup, paramGroup}))
	if err != nil {
		return nil, err
	}

	// Create CloudWatch alarms for RDS
	_, err = cloudwatch.NewMetricAlarm(ctx, fmt.Sprintf("%s-cpu-alarm", s.dbName), &cloudwatch.MetricAlarmArgs{
		Name:               pulumi.Sprintf("%s-rds-high-cpu", s.dbName),
		ComparisonOperator: pulumi.String("GreaterThanThreshold"),
		EvaluationPeriods:  pulumi.Int(2),
		MetricName:         pulumi.String("CPUUtilization"),
		Namespace:          pulumi.String("AWS/RDS"),
		Period:             pulumi.Int(300),
		Statistic:          pulumi.String("Average"),
		Threshold:          pulumi.Float64(80),
		AlarmDescription:   pulumi.Sprintf("CPU utilization is high for %s", s.dbName),
		Dimensions: pulumi.StringMap{
			"DBInstanceIdentifier": rdsInst.Identifier,
		},
		Tags: pulumi.StringMap{
			"Name": pulumi.Sprintf("%s-cpu-alarm", s.dbName),
		},
	})
	if err != nil {
		return nil, err
	}

	// Create alarm for free storage space
	_, err = cloudwatch.NewMetricAlarm(ctx, fmt.Sprintf("%s-storage-alarm", s.dbName), &cloudwatch.MetricAlarmArgs{
		Name:               pulumi.Sprintf("%s-rds-low-storage", s.dbName),
		ComparisonOperator: pulumi.String("LessThanThreshold"),
		EvaluationPeriods:  pulumi.Int(2),
		MetricName:         pulumi.String("FreeStorageSpace"),
		Namespace:          pulumi.String("AWS/RDS"),
		Period:             pulumi.Int(300),
		Statistic:          pulumi.String("Average"),
		Threshold:          pulumi.Float64(10737418240), // 10 GB in bytes
		AlarmDescription:   pulumi.Sprintf("Free storage space is low for %s", s.dbName),
		Dimensions: pulumi.StringMap{
			"DBInstanceIdentifier": rdsInst.Identifier,
		},
		Tags: pulumi.StringMap{
			"Name": pulumi.Sprintf("%s-storage-alarm", s.dbName),
		},
	})
	if err != nil {
		return nil, err
	}

	return rdsInst, nil
}
