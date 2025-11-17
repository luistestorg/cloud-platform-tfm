package main

import (
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/compute"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/container"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	globalStack "tracemachina.com/stack"
)

type (
	vpcStack struct {
		project   string
		region    string
		dependsOn []pulumi.Resource
		vpc       *compute.Network
		selfLink  pulumi.StringOutput
	}

	gkeStack struct {
		k8sVersion  string
		ClusterName string
		Id          pulumi.IDOutput

		NodePoolName string
		NodeCount    int
		Endpoint     pulumi.StringOutput
		NodePools    *container.ClusterNodePoolArrayOutput

		region string

		dependsOn []pulumi.Resource
	}
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		gcpConfig := config.New(ctx, "")
		region := gcpConfig.Require("region")
		gpcProject := gcpConfig.Require("project")
		vpcName := gcpConfig.Require("vpcName")
		env := gcpConfig.Require("env")

		s, err := createBase(ctx, gpcProject, vpcName, region)

		if err != nil {
			return err
		}

		gkeStack := &gkeStack{
			k8sVersion:  gcpConfig.Require("kubeVersion"),
			ClusterName: gcpConfig.Require("clusterName"),
			region:      region,
			dependsOn:   []pulumi.Resource{s.vpc},
		}

		err = gkeStack.CreateCluster(ctx, s)
		if err != nil {
			return err
		}

		ctx.Export("vpcName", s.vpc.Name)
		ctx.Export("gcpRegion", pulumi.String(s.region))
		ctx.Export("env", pulumi.String(env))
		ctx.Export("vpcID", s.vpc.ID())

		return nil

	})
}

func createBase(ctx *pulumi.Context, gpcProject string, vpcName string, gcpRegion string) (*vpcStack, error) {

	vpc, err := compute.NewNetwork(ctx, vpcName, &compute.NetworkArgs{
		Project:               pulumi.String(gpcProject),
		Name:                  pulumi.String(vpcName),
		AutoCreateSubnetworks: pulumi.Bool(true),
		Mtu:                   pulumi.Int(1460),
	})

	dependsOn := []pulumi.Resource{vpc}

	if err != nil {
		return nil, err
	}

	vpcStack := &vpcStack{

		project:   gpcProject,
		region:    gcpRegion,
		vpc:       vpc,
		selfLink:  vpc.SelfLink,
		dependsOn: dependsOn,
	}

	return vpcStack, nil

}

func (gkeStack *gkeStack) CreateCluster(ctx *pulumi.Context, vpcStack *vpcStack) error {

	cluster, err := container.NewCluster(ctx, gkeStack.ClusterName, &container.ClusterArgs{
		Name:                  vpcStack.vpc.Name,
		DeletionProtection:    pulumi.Bool(false),
		Location:              pulumi.String(gkeStack.region),
		Network:               vpcStack.vpc.Name,
		RemoveDefaultNodePool: pulumi.Bool(true),
		InitialNodeCount:      pulumi.Int(1),
		MinMasterVersion:      pulumi.String(gkeStack.k8sVersion),
		ReleaseChannel:        &container.ClusterReleaseChannelArgs{Channel: pulumi.String("STABLE")},
		WorkloadIdentityConfig: &container.ClusterWorkloadIdentityConfigArgs{
			WorkloadPool: pulumi.String(globalStack.GlobalWorkloadIdentityPool),
		},
		MaintenancePolicy: &container.ClusterMaintenancePolicyArgs{
			DailyMaintenanceWindow: &container.ClusterMaintenancePolicyDailyMaintenanceWindowArgs{
				StartTime: pulumi.String("06:00"),
			},
		},
		LoggingConfig: container.ClusterLoggingConfigArgs{
			EnableComponents: pulumi.StringArray{
				pulumi.String("SYSTEM_COMPONENTS"),
			},
		},
		MonitoringConfig: container.ClusterMonitoringConfigArgs{
			AdvancedDatapathObservabilityConfig: container.ClusterMonitoringConfigAdvancedDatapathObservabilityConfigArgs{
				EnableMetrics: pulumi.Bool(false),
				EnableRelay:   pulumi.Bool(false),
			},
			EnableComponents: pulumi.StringArray{
				pulumi.String("SYSTEM_COMPONENTS"),
			},
			ManagedPrometheus: container.ClusterMonitoringConfigManagedPrometheusArgs{
				Enabled: pulumi.Bool(false),
			},
		},
	}, pulumi.DependsOn([]pulumi.Resource{vpcStack.vpc}))
	if err != nil {
		return err
	}

	dependsOn := []pulumi.Resource{cluster}

	gkeStack.Endpoint = cluster.Endpoint

	gkeStack.dependsOn = dependsOn
	gkeStack.Id = cluster.ID()

	ctx.Export("clusterName", pulumi.String(gkeStack.ClusterName))
	ctx.Export("endpoint", gkeStack.Endpoint)

	ctx.Export("clusterSelfLink", cluster.SelfLink)
	ctx.Export("Location", cluster.Location)
	ctx.Export("clusterId", cluster.ID())

	ctx.Export("kubeconfig", globalStack.GenerateKubeconfig(cluster.Endpoint, cluster.Name, cluster.MasterAuth))

	return nil

}
