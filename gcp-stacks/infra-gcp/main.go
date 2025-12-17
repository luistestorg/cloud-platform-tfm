package main

import (
	"fmt"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/container"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	globalStack "unir-tfm.com/shared-gcp"
)

type (
	nodePoolStack struct {
		k8sVersion  string
		ClusterName string
		Id          pulumi.IDOutput

		NodePoolName string
		NodeCount    int
		Endpoint     pulumi.StringOutput
		NodePools    *container.ClusterNodePoolArrayOutput

		baseMachineType      string
		baseMachineType32cpu string
		armMachineType       string
		gpuMachineType       string

		region   pulumi.StringOutput
		armZone1 string
		armZone2 string
		armZone3 string

		gpuZone1 string
		gpuZone2 string

		vpc pulumi.StringOutput

		armMinNodes  int
		armMaxNodes  int
		baseMinNodes int
		baseMaxNodes int
		enableGPU    bool
		gpuMinNodes  int
		gpuMaxNodes  int
	}

	nodePoolConfig struct {
		key         string
		minNodes    int
		maxNodes    int
		machineType string
		spot        bool
		labels      pulumi.StringMap
		preemptible bool
		zones       []string
		diskSizeGb  int
		taints      *container.NodePoolNodeConfigTaintArray
		//In case we need to attach more disks and avoid adding more local storage
		localSSDCount int
	}
)

func main() {

	pulumi.Run(func(ctx *pulumi.Context) error {

		gcpConfig := config.New(ctx, "")

		baseStackName := fmt.Sprintf("tfm/gcp-stack/%v", gcpConfig.Require("globalStack"))
		baseStackRef, err := pulumi.NewStackReference(ctx, baseStackName, nil)
		if err != nil {
			return err
		}

		s := &nodePoolStack{
			k8sVersion:           gcpConfig.Require("kubeVersion"),
			ClusterName:          ctx.Stack(),
			vpc:                  baseStackRef.GetStringOutput(pulumi.String("vpcName")),
			baseMachineType:      gcpConfig.Require("baseMachineType"),
			baseMachineType32cpu: gcpConfig.Require("baseMachineType32cpu"),
			armMachineType:       gcpConfig.Require("armMachineType"),
			region:               baseStackRef.GetStringOutput(pulumi.String("gcpRegion")),
			armZone1:             gcpConfig.Require("armZone1"),
			armZone2:             gcpConfig.Require("armZone2"),
			armZone3:             gcpConfig.Require("armZone3"),
			gpuZone1:             gcpConfig.Require("gpuZone1"),
			gpuZone2:             gcpConfig.Require("gpuZone2"),
			gpuMachineType:       gcpConfig.Require("gpuMachineType"),
			armMinNodes:          gcpConfig.RequireInt("armMinNodes"),
			armMaxNodes:          gcpConfig.RequireInt("armMaxNodes"),
			baseMinNodes:         gcpConfig.RequireInt("baseMinNodes"),
			baseMaxNodes:         gcpConfig.RequireInt("baseMaxNodes"),
			gpuMinNodes:          gcpConfig.RequireInt("gpuMinNodes"),
			gpuMaxNodes:          gcpConfig.RequireInt("gpuMaxNodes"),
			enableGPU:            gcpConfig.RequireBool("enableGPU"),
			Id:                   baseStackRef.GetIDOutput(pulumi.String("clusterId")),
		}

		baseCfg := &nodePoolConfig{
			key:         "base",
			minNodes:    s.baseMinNodes,
			maxNodes:    s.baseMaxNodes,
			machineType: s.baseMachineType,
			labels:      pulumi.StringMap{"node-role": pulumi.String("not-disruptable")},
			diskSizeGb:  200,
			//localSSDCount: 2,
		}
		if err = s.deployNodePool(ctx, baseCfg); err != nil {
			return err
		}

		spotCfg := &nodePoolConfig{
			key:         "spot",
			minNodes:    0,
			maxNodes:    s.baseMaxNodes,
			machineType: s.baseMachineType,
			spot:        true,
			labels:      pulumi.StringMap{"node-role": pulumi.String("autoscaled-ondemand")},
			preemptible: false,
			diskSizeGb:  200,
			taints: &container.NodePoolNodeConfigTaintArray{
				&container.NodePoolNodeConfigTaintArgs{
					Effect: pulumi.String("NO_SCHEDULE"),
					Key:    pulumi.String("tfm/tolerates-spot"),
					Value:  pulumi.String("true"),
				},
			},
			//localSSDCount: 2,
		}
		if err = s.deployNodePool(ctx, spotCfg); err != nil {
			return err
		}

		spotCfg32CPU := &nodePoolConfig{
			key:         "spot32cpu",
			minNodes:    0,
			maxNodes:    s.baseMaxNodes,
			machineType: s.baseMachineType32cpu,
			spot:        true,
			labels:      pulumi.StringMap{"node-role": pulumi.String("autoscaled-ondemand")},
			preemptible: false,
			diskSizeGb:  200,
			taints: &container.NodePoolNodeConfigTaintArray{
				&container.NodePoolNodeConfigTaintArgs{
					Effect: pulumi.String("NO_SCHEDULE"),
					Key:    pulumi.String("tfm/tolerates-spot"),
					Value:  pulumi.String("true"),
				},
			},
			//localSSDCount: 0,
		}
		if err = s.deployNodePool(ctx, spotCfg32CPU); err != nil {
			return err
		}

		armCfg := &nodePoolConfig{
			key:         "arm",
			minNodes:    s.armMinNodes,
			maxNodes:    s.armMaxNodes,
			machineType: s.armMachineType,
			spot:        false,
			labels:      pulumi.StringMap{"node-role": pulumi.String("not-disruptable")},
			preemptible: false,
			zones:       []string{s.armZone1, s.armZone2, s.armZone3},
			diskSizeGb:  400,
			//For ARM this value needs to be 0, machines don't support manual set of localSSDCount
			//localSSDCount: 0,
		}
		if err = s.deployNodePool(ctx, armCfg); err != nil {
			return err
		}

		if s.enableGPU {
			gpuCfg := &nodePoolConfig{
				key:         "gpu",
				minNodes:    s.gpuMinNodes,
				maxNodes:    s.gpuMaxNodes,
				machineType: s.gpuMachineType,
				spot:        true,
				labels:      pulumi.StringMap{"nvidia.com/gpu": pulumi.String("present")},
				preemptible: false,
				zones:       []string{s.gpuZone1},
			}
			if err = s.deployNodePool(ctx, gpuCfg); err != nil {
				return err
			}
		}

		ctx.Export("enableGPU", pulumi.Bool(s.enableGPU))

		demoBuildsCfg := &nodePoolConfig{
			key:         "demo-builds",
			minNodes:    0,
			maxNodes:    10,
			machineType: s.baseMachineType,
			spot:        true,
			labels:      pulumi.StringMap{"node-role": pulumi.String("demo-builds")},
			preemptible: false,
			diskSizeGb:  200,
			taints: &container.NodePoolNodeConfigTaintArray{
				&container.NodePoolNodeConfigTaintArgs{
					Effect: pulumi.String("NO_SCHEDULE"),
					Key:    pulumi.String("tfm/tolerates-spot"),
					Value:  pulumi.String("true"),
				},
			},
			//localSSDCount: 2,
		}
		if err = s.deployNodePool(ctx, demoBuildsCfg); err != nil {
			return err
		}

		return nil

	})
}

func (nodePoolStack *nodePoolStack) deployNodePool(ctx *pulumi.Context, nodeCfg *nodePoolConfig) error {
	nodePoolName := fmt.Sprintf("%s-nodepool-%v", nodeCfg.key, nodePoolStack.ClusterName)
	location := "BALANCED"
	if nodeCfg.spot {
		location = "ANY"
	}

	if nodeCfg.diskSizeGb < 20 {
		nodeCfg.diskSizeGb = 20
	}

	args := &container.NodePoolArgs{
		Name:             pulumi.String(nodePoolName),
		Cluster:          nodePoolStack.Id,
		Location:         nodePoolStack.region,
		Version:          pulumi.String(nodePoolStack.k8sVersion),
		InitialNodeCount: pulumi.Int(nodeCfg.minNodes),
		Management: &container.NodePoolManagementArgs{
			AutoRepair:  pulumi.Bool(true),
			AutoUpgrade: pulumi.Bool(true),
		},
		Autoscaling: &container.NodePoolAutoscalingArgs{
			MaxNodeCount:   pulumi.Int(nodeCfg.maxNodes),
			MinNodeCount:   pulumi.Int(nodeCfg.minNodes),
			LocationPolicy: pulumi.String(location),
		},
		NodeConfig: &container.NodePoolNodeConfigArgs{
			Spot:           pulumi.Bool(nodeCfg.spot),
			Preemptible:    pulumi.Bool(nodeCfg.preemptible),
			MachineType:    pulumi.String(nodeCfg.machineType),
			ServiceAccount: pulumi.String(globalStack.GlobalGKEServiceAccount),
			OauthScopes: pulumi.StringArray{
				pulumi.String("https://www.googleapis.com/auth/cloud-platform"),
			},
			Labels:     nodeCfg.labels,
			DiskSizeGb: pulumi.Int(nodeCfg.diskSizeGb),
			Taints:     nodeCfg.taints,
			EphemeralStorageConfig: &container.NodePoolNodeConfigEphemeralStorageConfigArgs{
				LocalSsdCount: pulumi.Int(nodeCfg.localSSDCount),
			},
		},
	}
	if len(nodeCfg.zones) > 0 {
		args.NodeLocations = pulumi.ToStringArray(nodeCfg.zones)
	}
	_, err := container.NewNodePool(ctx, nodePoolName, args)
	if err != nil {
		fmt.Printf("\nERROR: failed to create node pool %s due to: %v\n", nodePoolName, err)
	}
	return err
}
