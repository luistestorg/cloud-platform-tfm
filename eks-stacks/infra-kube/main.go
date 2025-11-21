package main

import (
	"encoding/json"
	"fmt"
	"strings"

	awseks "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-eks/sdk/v3/go/eks"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	storagev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/storage/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	ebsCsiDriverChartVers  = "2.32.0"
	metricsServerChartVers = "3.12.2"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")

		// Stack references to infra-aws
		infraAwsRef, err := pulumi.NewStackReference(ctx, cfg.Require("infraAwsStack"), nil)
		if err != nil {
			return err
		}

		// Get outputs from infra-aws
		vpcId := infraAwsRef.GetOutput(pulumi.String("vpcId"))
		privateSubnetIds := infraAwsRef.GetOutput(pulumi.String("privateSubnetIds"))
		publicSubnetIds := infraAwsRef.GetOutput(pulumi.String("publicSubnetIds"))
		instanceRoleArn := infraAwsRef.GetOutput(pulumi.String("instanceRoleArn"))

		// Cluster configuration
		clusterName := cfg.Require("clusterName")
		k8sVersion := cfg.Get("k8sVersion")
		if k8sVersion == "" {
			k8sVersion = "1.30"
		}

		instanceType := cfg.Get("instanceType")
		if instanceType == "" {
			instanceType = "m6i.large"
		}

		minSize := cfg.GetInt("minSize")
		if minSize == 0 {
			minSize = 2
		}

		maxSize := cfg.GetInt("maxSize")
		if maxSize == 0 {
			maxSize = 4
		}

		desiredCapacity := cfg.GetInt("desiredCapacity")
		if desiredCapacity == 0 {
			desiredCapacity = 2
		}

		// Create EKS cluster with Managed Node Group
		cluster, err := eks.NewCluster(ctx, clusterName, &eks.ClusterArgs{
			Name:                         pulumi.String(clusterName),
			Version:                      pulumi.String(k8sVersion),
			CreateOidcProvider:           pulumi.BoolPtr(true),
			VpcId:                        vpcId.AsStringOutput(),
			PublicSubnetIds:              publicSubnetIds.AsStringArrayOutput(),
			PrivateSubnetIds:             privateSubnetIds.AsStringArrayOutput(),
			InstanceType:                 pulumi.String(instanceType),
			MinSize:                      pulumi.Int(0), // Set to 0 to use Managed Node Group
			MaxSize:                      pulumi.Int(0),
			DesiredCapacity:              pulumi.Int(0),
			NodeAssociatePublicIpAddress: pulumi.BoolRef(false),
		})
		if err != nil {
			return err
		}

		// Create Managed Node Group
		nodeGroupName := fmt.Sprintf("%s-ng", clusterName)
		_, err = eks.NewManagedNodeGroup(ctx, nodeGroupName, &eks.ManagedNodeGroupArgs{
			Cluster:       cluster,
			NodeRoleArn:   instanceRoleArn.AsStringOutput(),
			SubnetIds:     privateSubnetIds.AsStringArrayOutput(),
			InstanceTypes: pulumi.StringArray{pulumi.String(instanceType)},
			DiskSize:      pulumi.Int(50),
			ScalingConfig: &awseks.NodeGroupScalingConfigArgs{
				MinSize:     pulumi.Int(minSize),
				MaxSize:     pulumi.Int(maxSize),
				DesiredSize: pulumi.Int(desiredCapacity),
			},
		}, pulumi.DependsOn([]pulumi.Resource{cluster}))
		if err != nil {
			return err
		}

		// Create Kubernetes provider
		k8sProvider, err := kubernetes.NewProvider(ctx, "k8s-provider", &kubernetes.ProviderArgs{
			Kubeconfig: cluster.KubeconfigJson,
		})
		if err != nil {
			return err
		}

		// Get OIDC ID for IRSA
		eksOIDC := cluster.EksCluster.Identities().Index(pulumi.Int(0)).Oidcs().Index(pulumi.Int(0)).Issuer().ApplyT(func(issuer *string) string {
			if issuer == nil {
				return ""
			}
			return strings.TrimPrefix(*issuer, "https://")
		}).(pulumi.StringOutput)

		// Deploy EBS CSI Driver
		ebsCsiRole, err := deployEbsCsiDriver(ctx, cluster, k8sProvider, eksOIDC)
		if err != nil {
			return err
		}

		// Deploy Metrics Server
		_, err = deployMetricsServer(ctx, k8sProvider, cluster)
		if err != nil {
			return err
		}

		// Create Storage Classes
		err = createStorageClasses(ctx, k8sProvider, cluster, ebsCsiRole)
		if err != nil {
			return err
		}

		// Exports
		ctx.Export("clusterName", pulumi.String(clusterName))
		ctx.Export("kubeconfig", cluster.Kubeconfig)
		ctx.Export("kubeconfigJson", cluster.KubeconfigJson)
		ctx.Export("eksOIDC", eksOIDC)
		ctx.Export("clusterEndpoint", cluster.EksCluster.Endpoint())
		ctx.Export("clusterSecurityGroupId", cluster.EksCluster.VpcConfig().ClusterSecurityGroupId())

		return nil
	})
}

func deployEbsCsiDriver(ctx *pulumi.Context, cluster *eks.Cluster, k8sProvider *kubernetes.Provider, eksOIDC pulumi.StringOutput) (*iam.Role, error) {
	// Create IRSA role for EBS CSI Driver
	roleName := "ebs-csi-controller"

	// Create assume role policy for IRSA
	assumeRolePolicy := eksOIDC.ApplyT(func(oidc string) (string, error) {
		policy := map[string]interface{}{
			"Version": "2012-10-17",
			"Statement": []map[string]interface{}{
				{
					"Effect": "Allow",
					"Principal": map[string]interface{}{
						"Federated": fmt.Sprintf("arn:aws:iam::*:oidc-provider/%s", oidc),
					},
					"Action": "sts:AssumeRoleWithWebIdentity",
					"Condition": map[string]interface{}{
						"StringEquals": map[string]string{
							fmt.Sprintf("%s:sub", oidc): "system:serviceaccount:kube-system:ebs-csi-controller-sa",
							fmt.Sprintf("%s:aud", oidc): "sts.amazonaws.com",
						},
					},
				},
			},
		}

		policyJSON, err := json.Marshal(policy)
		return string(policyJSON), err
	}).(pulumi.StringOutput)

	// Create IAM role
	eksRole, err := iam.NewRole(ctx, roleName, &iam.RoleArgs{
		Name:             pulumi.String(roleName),
		AssumeRolePolicy: assumeRolePolicy,
		ManagedPolicyArns: pulumi.StringArray{
			pulumi.String("arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy"),
		},
	}, pulumi.DependsOn([]pulumi.Resource{cluster}))
	if err != nil {
		return nil, err
	}

	// Add KMS policy for encrypted volumes
	kmsPolicy := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Action": [
					"kms:Decrypt",
					"kms:GenerateDataKeyWithoutPlaintext",
					"kms:CreateGrant"
				],
				"Resource": "*"
			}
		]
	}`

	_, err = iam.NewRolePolicy(ctx, "ebs-csi-kms-policy", &iam.RolePolicyArgs{
		Name:   pulumi.String("ebs-csi-kms-policy"),
		Role:   eksRole.Name,
		Policy: pulumi.String(kmsPolicy),
	}, pulumi.DependsOn([]pulumi.Resource{eksRole}))
	if err != nil {
		return nil, err
	}

	// Deploy CSI Snapshotter CRDs
	_, err = yaml.NewConfigGroup(ctx, "snapshotter-crds", &yaml.ConfigGroupArgs{
		Files: []string{
			"https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/master/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml",
			"https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/master/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml",
			"https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/master/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml",
		},
	}, pulumi.Provider(k8sProvider), pulumi.DependsOn([]pulumi.Resource{cluster}))
	if err != nil {
		return nil, err
	}

	// Deploy EBS CSI Driver Helm chart
	_, err = helmv3.NewRelease(ctx, "aws-ebs-csi-driver", &helmv3.ReleaseArgs{
		Name:      pulumi.String("aws-ebs-csi-driver"),
		Namespace: pulumi.String("kube-system"),
		Chart:     pulumi.String("aws-ebs-csi-driver"),
		Version:   pulumi.String(ebsCsiDriverChartVers),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{
			Repo: pulumi.String("https://kubernetes-sigs.github.io/aws-ebs-csi-driver"),
		},
		Values: pulumi.Map{
			"controller": pulumi.Map{
				"replicaCount": pulumi.Int(2),
				"serviceAccount": pulumi.Map{
					"annotations": pulumi.Map{
						"eks.amazonaws.com/role-arn": eksRole.Arn,
					},
				},
			},
			"sidecars": pulumi.Map{
				"snapshotter": pulumi.Map{
					"forceEnable": pulumi.Bool(true),
				},
			},
		},
	}, pulumi.Provider(k8sProvider), pulumi.DependsOn([]pulumi.Resource{cluster, eksRole}))
	if err != nil {
		return nil, err
	}

	return eksRole, nil
}

func deployMetricsServer(ctx *pulumi.Context, k8sProvider *kubernetes.Provider, cluster *eks.Cluster) (*helmv3.Release, error) {
	return helmv3.NewRelease(ctx, "metrics-server", &helmv3.ReleaseArgs{
		Name:      pulumi.String("metrics-server"),
		Namespace: pulumi.String("kube-system"),
		Chart:     pulumi.String("metrics-server"),
		Version:   pulumi.String(metricsServerChartVers),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{
			Repo: pulumi.String("https://kubernetes-sigs.github.io/metrics-server/"),
		},
	}, pulumi.Provider(k8sProvider), pulumi.DependsOn([]pulumi.Resource{cluster}))
}

func createStorageClasses(ctx *pulumi.Context, k8sProvider *kubernetes.Provider, cluster *eks.Cluster, ebsCsiRole *iam.Role) error {
	dependsOn := []pulumi.Resource{cluster, ebsCsiRole}

	// GP3 Storage Class (default)
	_, err := storagev1.NewStorageClass(ctx, "gp3", &storagev1.StorageClassArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String("gp3"),
			Annotations: pulumi.StringMap{
				"storageclass.kubernetes.io/is-default-class": pulumi.String("true"),
			},
		},
		Provisioner: pulumi.String("ebs.csi.aws.com"),
		Parameters: pulumi.StringMap{
			"type":                      pulumi.String("gp3"),
			"csi.storage.k8s.io/fstype": pulumi.String("ext4"),
		},
		ReclaimPolicy:        pulumi.String("Delete"),
		AllowVolumeExpansion: pulumi.Bool(true),
		VolumeBindingMode:    pulumi.String("WaitForFirstConsumer"),
	}, pulumi.Provider(k8sProvider), pulumi.DependsOn(dependsOn))
	if err != nil {
		return err
	}

	// GP3 Encrypted Storage Class
	_, err = storagev1.NewStorageClass(ctx, "gp3-enc", &storagev1.StorageClassArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String("gp3-enc"),
		},
		Provisioner: pulumi.String("ebs.csi.aws.com"),
		Parameters: pulumi.StringMap{
			"type":                      pulumi.String("gp3"),
			"csi.storage.k8s.io/fstype": pulumi.String("ext4"),
			"encrypted":                 pulumi.String("true"),
		},
		ReclaimPolicy:        pulumi.String("Delete"),
		AllowVolumeExpansion: pulumi.Bool(true),
		VolumeBindingMode:    pulumi.String("WaitForFirstConsumer"),
	}, pulumi.Provider(k8sProvider), pulumi.DependsOn(dependsOn))
	if err != nil {
		return err
	}

	// GP3 High IOPS Encrypted Storage Class
	_, err = storagev1.NewStorageClass(ctx, "gp3-8k-iops-enc", &storagev1.StorageClassArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String("gp3-8k-iops-enc"),
		},
		Provisioner: pulumi.String("ebs.csi.aws.com"),
		Parameters: pulumi.StringMap{
			"type":                      pulumi.String("gp3"),
			"csi.storage.k8s.io/fstype": pulumi.String("ext4"),
			"iops":                      pulumi.String("8000"),
			"throughput":                pulumi.String("1000"),
			"encrypted":                 pulumi.String("true"),
		},
		ReclaimPolicy:        pulumi.String("Retain"),
		AllowVolumeExpansion: pulumi.Bool(true),
		VolumeBindingMode:    pulumi.String("WaitForFirstConsumer"),
	}, pulumi.Provider(k8sProvider), pulumi.DependsOn(dependsOn))
	if err != nil {
		return err
	}

	// Volume Snapshot Class
	_, err = corev1.NewConfigMap(ctx, "snapshot-class-placeholder", &corev1.ConfigMapArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("ebs-snapshot-info"),
			Namespace: pulumi.String("kube-system"),
		},
		Data: pulumi.StringMap{
			"info": pulumi.String("VolumeSnapshotClass 'gp3-snapshotclass' available for EBS snapshots"),
		},
	}, pulumi.Provider(k8sProvider), pulumi.DependsOn(dependsOn))

	return err
}
