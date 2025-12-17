package main

import (
	"fmt"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/compute"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/servicenetworking"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/sql"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

type (
	sqlStack struct {
		dbPassword     pulumi.StringOutput
		pgPassword     pulumi.StringOutput
		dbName         string
		dbInstanceType string
		dbEdition      string
		zone           string
		vpc            pulumi.IDOutput
	}
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		sqlConfig := config.New(ctx, "")

		globalStackName := fmt.Sprintf("tfm/gcp-stack/%v", sqlConfig.Require("globalStack"))
		stackRef, err := pulumi.NewStackReference(ctx, globalStackName, nil)
		if err != nil {
			return err
		}

		region := stackRef.GetStringOutput(pulumi.String("gcpRegion"))

		sqlStack := &sqlStack{
			dbPassword:     sqlConfig.RequireSecret("dbPassword"),
			pgPassword:     sqlConfig.RequireSecret("pgPassword"),
			dbName:         sqlConfig.Require("dbName"),
			dbInstanceType: sqlConfig.Require("dbInstanceType"),
			dbEdition:      sqlConfig.Require("dbEdition"),
			zone:           sqlConfig.Require("zone"),
			vpc:            stackRef.GetIDOutput((pulumi.String("vpcID"))),
		}

		_, err = createCloudSql(ctx, sqlStack, region)
		if err != nil {
			return err
		}
		return nil
	})
}

func createCloudSql(ctx *pulumi.Context, s *sqlStack, region pulumi.StringOutput) (*sql.DatabaseInstance, error) {

	privateIpAddress, err := compute.NewGlobalAddress(ctx, "sql_ip_address", &compute.GlobalAddressArgs{
		Name:         pulumi.String(ctx.Stack()),
		Purpose:      pulumi.String("VPC_PEERING"),
		AddressType:  pulumi.String("INTERNAL"),
		PrefixLength: pulumi.Int(16),
		Network:      s.vpc,
	})
	if err != nil {
		return nil, err
	}
	privateVpcConnection, err := servicenetworking.NewConnection(ctx, "sql_vpc_connection", &servicenetworking.ConnectionArgs{
		Network: s.vpc,
		Service: pulumi.String("servicenetworking.googleapis.com"),
		ReservedPeeringRanges: pulumi.StringArray{
			privateIpAddress.Name,
		},
		/* Setting up as ABANDON because of a bug in the GCP provider:
		https://github.com/hashicorp/terraform-provider-google/issues/18834
		https://github.com/hashicorp/terraform-provider-google/issues/16275
		*/
		//DeletionPolicy: pulumi.String("ABANDON"),
	}, pulumi.DependsOn([]pulumi.Resource{
		privateIpAddress,
	}))
	if err != nil {
		return nil, err
	}

	cloudSqlInst, err := sql.NewDatabaseInstance(ctx, "instance", &sql.DatabaseInstanceArgs{
		Name:               pulumi.String(s.dbName),
		Region:             region,
		DatabaseVersion:    pulumi.String("POSTGRES_16"),
		DeletionProtection: pulumi.Bool(false),
		InstanceType:       pulumi.String("CLOUD_SQL_INSTANCE"),
		RootPassword:       s.dbPassword,

		Settings: &sql.DatabaseInstanceSettingsArgs{
			Tier:     pulumi.String("db-custom-2-8192"),
			Edition:  pulumi.String(s.dbEdition),
			DiskSize: pulumi.Int(100),
			LocationPreference: &sql.DatabaseInstanceSettingsLocationPreferenceArgs{
				Zone: pulumi.String(s.zone),
			},
			IpConfiguration: sql.DatabaseInstanceSettingsIpConfigurationArgs{
				PrivateNetwork:                          s.vpc,
				EnablePrivatePathForGoogleCloudServices: pulumi.Bool(true),
			},
		},
	}, pulumi.DependsOn([]pulumi.Resource{
		privateVpcConnection,
	}))
	if err != nil {
		return nil, err
	}

	_, err = sql.NewUser(ctx, "users", &sql.UserArgs{
		Name:     pulumi.String("nlssapidb"),
		Instance: cloudSqlInst.Name,
		Password: s.dbPassword,
		//Setting up as ABANDON because of a postgres restriction
		//CloudSQL instance will be deleted completely so we don't need to delete the user.
		DeletionPolicy: pulumi.String("ABANDON"),
	}, pulumi.DependsOn([]pulumi.Resource{
		cloudSqlInst,
	}))
	if err != nil {
		return nil, err
	}

	apiDb, err := sql.NewDatabase(ctx, "api-database", &sql.DatabaseArgs{
		Name:     pulumi.String("nlssapidb"),
		Instance: cloudSqlInst.Name,
		//Setting up as ABANDON because of a postgres restriction
		//CloudSQL instance will be deleted completely so we don't need to delete the DB.
		DeletionPolicy: pulumi.String("ABANDON"),
	})
	if err != nil {
		return nil, err
	}

	ctx.Export("sqlConnectionName", cloudSqlInst.ConnectionName)
	ctx.Export("sqlDnsName", cloudSqlInst.DnsName)
	ctx.Export("sqlPublicIpAddress", cloudSqlInst.PublicIpAddress)
	ctx.Export("sqlPrivateIpAddress", cloudSqlInst.PrivateIpAddress)
	ctx.Export("sqlZone", pulumi.String(s.zone))
	ctx.Export("dbPassword", s.dbPassword)
	ctx.Export("dbName", apiDb.Name)
	return cloudSqlInst, err
}
