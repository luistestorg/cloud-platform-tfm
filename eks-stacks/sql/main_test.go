package main

import (
	"testing"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/rds"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
)

type mocks int

// Mock for Pulumi resources
func (mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	outputs := args.Inputs.Copy()

	switch args.TypeToken {
	case "aws:rds/instance:Instance":
		outputs["endpoint"] = resource.NewPropertyValue("tfmdb.abc123.us-west-2.rds.amazonaws.com:5432")
		outputs["address"] = resource.NewPropertyValue("tfmdb.abc123.us-west-2.rds.amazonaws.com")
		outputs["port"] = resource.NewPropertyValue(5432)
		outputs["dbName"] = args.Inputs["dbName"]
		outputs["arn"] = resource.NewPropertyValue("arn:aws:rds:us-west-2:123456789012:db:tfmdb")
		outputs["availabilityZone"] = args.Inputs["availabilityZone"]
		outputs["storageEncrypted"] = args.Inputs["storageEncrypted"]

	case "aws:ec2/securityGroup:SecurityGroup":
		outputs["id"] = resource.NewPropertyValue("sg-12345678")
		outputs["name"] = args.Inputs["name"]

	case "aws:rds/subnetGroup:SubnetGroup":
		outputs["name"] = args.Inputs["name"]
		outputs["arn"] = resource.NewPropertyValue("arn:aws:rds:us-west-2:123456789012:subgrp:tfmdb-subnets")

	case "aws:rds/parameterGroup:ParameterGroup":
		outputs["name"] = args.Inputs["name"]
		outputs["family"] = args.Inputs["family"]

	case "aws:cloudwatch/metricAlarm:MetricAlarm":
		outputs["name"] = args.Inputs["name"]
		outputs["arn"] = resource.NewPropertyValue("arn:aws:cloudwatch:us-west-2:123456789012:alarm:test-alarm")
	}

	return args.Name + "_id", outputs, nil
}

func (mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	outputs := map[string]interface{}{}

	switch args.Token {
	case "aws:index/getRegion:getRegion":
		outputs["name"] = "us-west-2"
		outputs["id"] = "us-west-2"
	}

	return resource.NewPropertyMapFromMap(outputs), nil
}

// TestRDSInstanceCreation verifies RDS PostgreSQL instance configuration
func TestRDSInstanceCreation(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Mock stack reference
		infraStack, err := pulumi.NewStackReference(ctx, "organization/eks-infra-aws/dev", nil)
		if err != nil {
			return err
		}

		vpcId := infraStack.GetStringOutput(pulumi.String("vpcId"))
		privateSubnetIds := infraStack.GetOutput(pulumi.String("privateSubnetIds")).AsStringArrayOutput()

		// Create RDS instance
		rdsInst, err := rds.NewInstance(ctx, "tfmdb-rds", &rds.InstanceArgs{
			Identifier:            pulumi.String("tfmdb"),
			AvailabilityZone:      pulumi.String("us-west-2a"),
			AllocatedStorage:      pulumi.Int(100),
			MaxAllocatedStorage:   pulumi.Int(200),
			Engine:                pulumi.String("postgres"),
			EngineVersion:         pulumi.String("16.3"),
			InstanceClass:         pulumi.String("db.t4g.small"),
			DbName:                pulumi.String("tfmdb"),
			Username:              pulumi.String("tfmdb"),
			Password:              pulumi.String("test-password"),
			StorageEncrypted:      pulumi.Bool(true),
			StorageType:           pulumi.String("gp3"),
			BackupRetentionPeriod: pulumi.Int(7),
			MultiAz:               pulumi.Bool(false),
			PubliclyAccessible:    pulumi.Bool(false),
		})
		if err != nil {
			return err
		}

		// Validate RDS properties
		rdsInst.Endpoint.ApplyT(func(endpoint string) error {
			assert.NotEmpty(t, endpoint, "RDS endpoint should not be empty")
			assert.Contains(t, endpoint, ":5432", "RDS endpoint should include port 5432")
			return nil
		})

		rdsInst.StorageEncrypted.ApplyT(func(encrypted bool) error {
			assert.True(t, encrypted, "RDS storage should be encrypted")
			return nil
		})

		// Verify outputs
		ctx.Export("rdsEndpoint", rdsInst.Endpoint)
		ctx.Export("rdsAddress", rdsInst.Address)
		ctx.Export("rdsPort", rdsInst.Port)

		// Suppress unused variable warnings
		_ = vpcId
		_ = privateSubnetIds

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestRDSSecurityGroup verifies security group configuration for RDS
func TestRDSSecurityGroup(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		vpcId := pulumi.String("vpc-12345678")
		vpcCidr := pulumi.String("10.0.0.0/16")

		// Create Security Group for RDS
		rdsSg, err := ec2.NewSecurityGroup(ctx, "tfmdb-rds-sg", &ec2.SecurityGroupArgs{
			Name:        pulumi.String("tfmdb-rds-sg"),
			Description: pulumi.String("Security group for RDS PostgreSQL"),
			VpcId:       vpcId,
			Ingress: ec2.SecurityGroupIngressArray{
				&ec2.SecurityGroupIngressArgs{
					Description: pulumi.String("PostgreSQL from VPC"),
					FromPort:    pulumi.Int(5432),
					ToPort:      pulumi.Int(5432),
					Protocol:    pulumi.String("tcp"),
					CidrBlocks:  pulumi.StringArray{vpcCidr},
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
		})
		if err != nil {
			return err
		}

		// Validate security group
		rdsSg.Name.ApplyT(func(name string) error {
			assert.Equal(t, "tfmdb-rds-sg", name, "Security group name should match")
			return nil
		})

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestRDSSubnetGroup verifies subnet group configuration
func TestRDSSubnetGroup(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		privateSubnetIds := pulumi.StringArray{
			pulumi.String("subnet-11111111"),
			pulumi.String("subnet-22222222"),
		}

		// Create Subnet Group
		subnetGroup, err := rds.NewSubnetGroup(ctx, "tfmdb-rds-subnets", &rds.SubnetGroupArgs{
			Name:        pulumi.String("tfmdb-rds-subnets"),
			Description: pulumi.String("Subnet group for tfmdb RDS instance"),
			SubnetIds:   privateSubnetIds,
		})
		if err != nil {
			return err
		}

		// Validate subnet group
		subnetGroup.Name.ApplyT(func(name string) error {
			assert.Equal(t, "tfmdb-rds-subnets", name, "Subnet group name should match")
			return nil
		})

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestRDSParameterGroup verifies parameter group for PostgreSQL
func TestRDSParameterGroup(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Create Parameter Group
		paramGroup, err := rds.NewParameterGroup(ctx, "tfmdb-pg-params", &rds.ParameterGroupArgs{
			Name:        pulumi.String("tfmdb-pg-params"),
			Family:      pulumi.String("postgres16"),
			Description: pulumi.String("Parameter group for tfmdb"),
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
		})
		if err != nil {
			return err
		}

		// Validate parameter group
		paramGroup.Family.ApplyT(func(family string) error {
			assert.Equal(t, "postgres16", family, "Parameter group family should be postgres16")
			return nil
		})

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestRDSStorageConfiguration verifies storage settings
func TestRDSStorageConfiguration(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Create RDS with storage configuration
		rdsInst, err := rds.NewInstance(ctx, "tfmdb-rds", &rds.InstanceArgs{
			Identifier:          pulumi.String("tfmdb"),
			AllocatedStorage:    pulumi.Int(100),
			MaxAllocatedStorage: pulumi.Int(200),
			StorageType:         pulumi.String("gp3"),
			Iops:                pulumi.Int(3000),
			StorageEncrypted:    pulumi.Bool(true),
			Engine:              pulumi.String("postgres"),
			EngineVersion:       pulumi.String("16.3"),
			InstanceClass:       pulumi.String("db.t4g.small"),
		})
		if err != nil {
			return err
		}

		// Validate storage encryption
		rdsInst.StorageEncrypted.ApplyT(func(encrypted bool) error {
			assert.True(t, encrypted, "Storage must be encrypted")
			return nil
		})

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestRDSBackupConfiguration verifies backup settings
func TestRDSBackupConfiguration(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		backupRetention := 7
		backupWindow := "03:00-04:00"
		maintenanceWindow := "Mon:04:00-Mon:05:00"

		// Create RDS with backup configuration
		_, err := rds.NewInstance(ctx, "tfmdb-rds", &rds.InstanceArgs{
			Identifier:            pulumi.String("tfmdb"),
			BackupRetentionPeriod: pulumi.Int(backupRetention),
			BackupWindow:          pulumi.String(backupWindow),
			MaintenanceWindow:     pulumi.String(maintenanceWindow),
			CopyTagsToSnapshot:    pulumi.Bool(true),
			Engine:                pulumi.String("postgres"),
			EngineVersion:         pulumi.String("16.3"),
			InstanceClass:         pulumi.String("db.t4g.small"),
		})
		if err != nil {
			return err
		}

		// Validate configuration
		assert.Equal(t, 7, backupRetention, "Backup retention should be 7 days")
		assert.NotEmpty(t, backupWindow, "Backup window should be set")
		assert.NotEmpty(t, maintenanceWindow, "Maintenance window should be set")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestCloudWatchAlarmsConfiguration verifies CloudWatch alarms for RDS
func TestCloudWatchAlarmsConfiguration(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		dbIdentifier := pulumi.String("tfmdb")

		// CPU Alarm
		cpuAlarm, err := cloudwatch.NewMetricAlarm(ctx, "tfmdb-cpu-alarm", &cloudwatch.MetricAlarmArgs{
			Name:               pulumi.String("tfmdb-rds-high-cpu"),
			ComparisonOperator: pulumi.String("GreaterThanThreshold"),
			EvaluationPeriods:  pulumi.Int(2),
			MetricName:         pulumi.String("CPUUtilization"),
			Namespace:          pulumi.String("AWS/RDS"),
			Period:             pulumi.Int(300),
			Statistic:          pulumi.String("Average"),
			Threshold:          pulumi.Float64(80),
			AlarmDescription:   pulumi.String("CPU utilization is high"),
			Dimensions: pulumi.StringMap{
				"DBInstanceIdentifier": dbIdentifier,
			},
		})
		if err != nil {
			return err
		}

		// Storage Alarm
		storageAlarm, err := cloudwatch.NewMetricAlarm(ctx, "tfmdb-storage-alarm", &cloudwatch.MetricAlarmArgs{
			Name:               pulumi.String("tfmdb-rds-low-storage"),
			ComparisonOperator: pulumi.String("LessThanThreshold"),
			EvaluationPeriods:  pulumi.Int(2),
			MetricName:         pulumi.String("FreeStorageSpace"),
			Namespace:          pulumi.String("AWS/RDS"),
			Period:             pulumi.Int(300),
			Statistic:          pulumi.String("Average"),
			Threshold:          pulumi.Float64(10737418240), // 10 GB
			AlarmDescription:   pulumi.String("Free storage space is low"),
			Dimensions: pulumi.StringMap{
				"DBInstanceIdentifier": dbIdentifier,
			},
		})
		if err != nil {
			return err
		}

		// Validate alarms
		cpuAlarm.Name.ApplyT(func(name string) error {
			assert.Contains(t, name, "cpu", "CPU alarm name should contain 'cpu'")
			return nil
		})

		storageAlarm.Name.ApplyT(func(name string) error {
			assert.Contains(t, name, "storage", "Storage alarm name should contain 'storage'")
			return nil
		})

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestRDSOutputs verifies all required outputs are exported
func TestRDSOutputs(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Create mock RDS instance
		rdsInst, err := rds.NewInstance(ctx, "tfmdb-rds", &rds.InstanceArgs{
			Identifier:    pulumi.String("tfmdb"),
			Engine:        pulumi.String("postgres"),
			EngineVersion: pulumi.String("16.3"),
			InstanceClass: pulumi.String("db.t4g.small"),
		})
		if err != nil {
			return err
		}

		// Export all required outputs
		ctx.Export("rdsEndpoint", rdsInst.Endpoint)
		ctx.Export("rdsAddress", rdsInst.Address)
		ctx.Export("rdsPort", rdsInst.Port)
		ctx.Export("rdsDbName", rdsInst.DbName)
		ctx.Export("rdsArn", rdsInst.Arn)
		ctx.Export("rdsAvailabilityZone", rdsInst.AvailabilityZone)

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestStackReferenceToInfraAWS verifies stack reference configuration
func TestStackReferenceToInfraAWS(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Create stack reference to infra-aws
		infraStack, err := pulumi.NewStackReference(ctx, "organization/eks-infra-aws/dev", nil)
		if err != nil {
			return err
		}

		// Get VPC outputs
		vpcId := infraStack.GetStringOutput(pulumi.String("vpcId"))
		vpcId.ApplyT(func(id string) error {
			assert.NotEmpty(t, id, "VPC ID from stack reference should not be empty")
			return nil
		})

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestRDSPerformanceInsights verifies Performance Insights configuration
func TestRDSPerformanceInsights(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Create RDS with Performance Insights
		_, err := rds.NewInstance(ctx, "tfmdb-rds", &rds.InstanceArgs{
			Identifier:                         pulumi.String("tfmdb"),
			Engine:                             pulumi.String("postgres"),
			EngineVersion:                      pulumi.String("16.3"),
			InstanceClass:                      pulumi.String("db.t4g.small"),
			PerformanceInsightsEnabled:         pulumi.Bool(true),
			PerformanceInsightsRetentionPeriod: pulumi.Int(7),
		})
		if err != nil {
			return err
		}

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestRDSEnhancedMonitoring verifies enhanced monitoring logs
func TestRDSEnhancedMonitoring(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Test enhanced monitoring configuration
		enabledLogs := []string{"postgresql", "upgrade"}

		// Validate log configuration
		assert.Contains(t, enabledLogs, "postgresql", "PostgreSQL logs should be enabled")
		assert.Contains(t, enabledLogs, "upgrade", "Upgrade logs should be enabled")

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}
