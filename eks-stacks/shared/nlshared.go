package shared

import (
	"fmt"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	RedisChartVers        = "19.1.1-3"
	RedisClusterChartVers = "11.0.3"
	MongodbChartVers      = "15.4.3"
	NatsChartVers         = "1.3.6"
)

type (
	NLSharedStack struct {
		MongoStorageClass     string
		MongoStorageSnapshot  string
		MongoUseSts           bool
		MongoExistingClaim    string
		MongoRootPassword     pulumi.StringOutput // loaded at runtime from a secret
		MongoDatabasePassword pulumi.StringOutput // loaded at runtime from a secret

		SharedRedisStorageClass string

		CreateSharedNativeLinkNamespace bool
		SharedRedisZone                 string
		SharedRedisSize                 string
		MtRedisSize                     string
		MtRedisZone                     string
		EnableMongoDB                   bool
		EnableRedisCluster              bool
		ClusterRedisSize                string
		ClusterRedisStorageClass        string
		ClusterRedisZone                string
		EnableNats                      bool
	}
)

func (nlSharedStack *NLSharedStack) deployRedis(ctx *pulumi.Context, ns *corev1.Namespace, s *Stack) error {
	nodeSelectorMap := getNodeSelector(false, "arm64", "linux", nlSharedStack.SharedRedisZone)
	if s.Platform == "aws" {
		nodeSelectorMap["karpenter.k8s.aws/instance-size"] = pulumi.String(nlSharedStack.SharedRedisSize)
	}

	maxMemory := "10000mb"
	diskSize := "20Gi"
	resourcesMap := pulumi.Map{
		"limits": pulumi.Map{
			"ephemeral-storage": pulumi.String("6Gi"),
		},
		"requests": pulumi.Map{
			"ephemeral-storage": pulumi.String("6Gi"),
			"memory":            pulumi.String("10500Mi"),
		},
	}
	if nlSharedStack.SharedRedisSize == "2xlarge" {
		maxMemory = "58600mb"
		diskSize = "100Gi"
		resourcesMap = pulumi.Map{
			"limits": pulumi.Map{
				"cpu":               pulumi.String("7300m"),
				"ephemeral-storage": pulumi.String("12Gi"),
				"memory":            pulumi.String("59800Mi"),
			},
			"requests": pulumi.Map{
				"cpu":               pulumi.String("6900m"),
				"ephemeral-storage": pulumi.String("10Gi"),
				"memory":            pulumi.String("59200Mi"),
			},
		}
	} else if nlSharedStack.SharedRedisSize == "medium" {
		maxMemory = "5800mb"
		diskSize = "8Gi"
		resourcesMap = pulumi.Map{
			"limits": pulumi.Map{
				"ephemeral-storage": pulumi.String(diskSize),
			},
			"requests": pulumi.Map{
				"cpu":               pulumi.String("200m"),
				"ephemeral-storage": pulumi.String(diskSize),
				"memory":            pulumi.String("6000Mi"),
			},
		}
	} else if nlSharedStack.SharedRedisSize == "xlarge" {
		maxMemory = "27500mb"
		diskSize = "100Gi" // hack: to keep the existing size when using 2xlarge
		resourcesMap = pulumi.Map{
			"limits": pulumi.Map{
				"ephemeral-storage": pulumi.String("12Gi"),
			},
			"requests": pulumi.Map{
				"cpu":               pulumi.String("3200m"),
				"ephemeral-storage": pulumi.String("10Gi"),
				"memory":            pulumi.String("28500Mi"),
			},
		}
	}
	extraFlags := []string{"--databases 1", "--maxmemory-policy allkeys-lru", "--maxmemory " + maxMemory}

	isProd := s.Env == "prod"

	masterMap := pulumi.Map{
		"extraFlags":        pulumi.ToStringArray(extraFlags),
		"podAnnotations":    annotationsByPlatform(s.Platform),
		"nodeSelector":      nodeSelectorMap,
		"resources":         resourcesMap,
		"priorityClassName": priorityClassByPlatformAndWorkloadType("statefulset"),
		"persistence": pulumi.Map{
			"enabled":      pulumi.Bool(isProd),
			"storageClass": pulumi.String(nlSharedStack.SharedRedisStorageClass),
			"size":         pulumi.String(diskSize),
		},
	}

	masterMap["tolerations"] = nodeTolerationsByPlatformAndDistruptionType(s.Platform, false)

	// deploy redis helm chart
	customValues := pulumi.Map{
		"global": pulumi.Map{
			"storageClass": pulumi.String(nlSharedStack.SharedRedisStorageClass),
		},
		"master":  masterMap,
		"replica": masterMap,
	}
	if _, err := s.DeployHelmRelease(ctx, ns, "redis", RedisChartVers, "", "redis-values.yaml", customValues); err != nil {
		return err
	}
	return nil
}

func (nlSharedStack *NLSharedStack) DeployRedisCluster(ctx *pulumi.Context, ns *corev1.Namespace, s *Stack) error {
	// additional config (to override defaults in redis-default.conf
	nodeSelectorMap := getNodeSelector(false, "arm64", "linux", nlSharedStack.ClusterRedisZone)
	if s.Platform == "aws" {
		nodeSelectorMap["karpenter.k8s.aws/instance-family"] = pulumi.String("r7g")
		nodeSelectorMap["karpenter.k8s.aws/instance-size"] = pulumi.String(nlSharedStack.ClusterRedisSize)
	}

	// large
	diskSize := "20Gi"
	maxMemory := "13000mb"
	resourcesMap := pulumi.Map{
		"limits": pulumi.Map{
			"cpu":               pulumi.String("1700m"),
			"ephemeral-storage": pulumi.String("6Gi"),
			"memory":            pulumi.String("14100Mi"),
		},
		"requests": pulumi.Map{
			"cpu":               pulumi.String("1400m"),
			"ephemeral-storage": pulumi.String("6Gi"),
			"memory":            pulumi.String("13800Mi"),
		},
	}
	if nlSharedStack.ClusterRedisSize == "2xlarge" {
		diskSize = "70Gi"
		maxMemory = "56500mb"
		resourcesMap = pulumi.Map{
			"limits": pulumi.Map{
				"cpu":               pulumi.String("7000m"),
				"ephemeral-storage": pulumi.String("12Gi"),
				"memory":            pulumi.String("59800Mi"),
			},
			"requests": pulumi.Map{
				"cpu":               pulumi.String("6900m"),
				"ephemeral-storage": pulumi.String("10Gi"),
				"memory":            pulumi.String("57500Mi"),
			},
		}
	}
	confStr := fmt.Sprintf(`maxmemory %s
maxmemory-policy allkeys-lru`, maxMemory)

	// deploy redis helm chart
	customValues := pulumi.Map{
		"redis": pulumi.Map{
			"podAnnotations":    annotationsByPlatform(s.Platform),
			"nodeSelector":      nodeSelectorMap,
			"resources":         resourcesMap,
			"configmap":         pulumi.String(confStr),
			"tolerations":       nodeTolerationsByPlatformAndDistruptionType(s.Platform, false),
			"priorityClassName": priorityClassByPlatformAndWorkloadType("statefulset"),
		},
		"persistence": pulumi.Map{
			"storageClass": pulumi.String(nlSharedStack.ClusterRedisStorageClass),
			"size":         pulumi.String(diskSize),
		},
	}

	if _, err := s.DeployHelmRelease(ctx, ns, "redis-cluster", RedisClusterChartVers, "", "redis-cluster-values.yaml", customValues); err != nil {
		return err
	}
	return nil
}

func (nlSharedStack *NLSharedStack) deployMultiTenantRedis(ctx *pulumi.Context, ns *corev1.Namespace, s *Stack) error {
	nodeSelectorMap := getNodeSelector(false, "arm64", "linux", nlSharedStack.MtRedisZone)

	var nodeRole string
	if s.Platform == "aws" {
		nodeSelectorMap["karpenter.k8s.aws/instance-size"] = pulumi.String(nlSharedStack.MtRedisSize)
		nodeRole = "not-disruptable-50g-ebs"
	} else {
		nodeRole = "not-disruptable"
	}

	// r7g.xlarge

	maxMemory := "7500mb"
	diskSize := "70Gi"
	resourcesMap := pulumi.Map{
		"limits": pulumi.Map{
			"ephemeral-storage": pulumi.String("20Gi"),
		},
		"requests": pulumi.Map{
			"ephemeral-storage": pulumi.String("20Gi"),
			"memory":            pulumi.String("8500Mi"),
		},
	}
	// r7g.2xlarge
	if nlSharedStack.MtRedisSize == "2xlarge" {
		maxMemory = "56500mb"
		diskSize = "70Gi"
		resourcesMap = pulumi.Map{
			"limits": pulumi.Map{
				"ephemeral-storage": pulumi.String("30Gi"),
			},
			"requests": pulumi.Map{
				"cpu":               pulumi.String("6800m"),
				"ephemeral-storage": pulumi.String("30Gi"),
				"memory":            pulumi.String("57500Mi"),
			},
		}
	}
	// r7g.large (2 cpu / 16g mem)
	if nlSharedStack.MtRedisSize == "large" {
		nodeRole = "not-disruptable"
		maxMemory = "10000mb"
		diskSize = "20Gi"
		resourcesMap = pulumi.Map{
			"limits": pulumi.Map{
				"ephemeral-storage": pulumi.String("15Gi"),
			},
			"requests": pulumi.Map{
				"ephemeral-storage": pulumi.String("15Gi"),
				"memory":            pulumi.String("10500Mi"),
			},
		}
	}

	// r7g.medium
	if nlSharedStack.MtRedisSize == "medium" {
		nodeRole = "not-disruptable"
		maxMemory = "5800mb"
		diskSize = "8Gi"
		resourcesMap = pulumi.Map{
			"limits": pulumi.Map{
				"ephemeral-storage": pulumi.String("8Gi"),
			},
			"requests": pulumi.Map{
				"cpu":               pulumi.String("200m"),
				"ephemeral-storage": pulumi.String("8Gi"),
				"memory":            pulumi.String("6000Mi"),
			},
		}
	}

	nodeSelectorMap["node-role"] = pulumi.String(nodeRole)
	extraFlags := []string{"--databases 100", "--maxmemory-policy allkeys-lru", "--maxmemory " + maxMemory}

	masterMap := pulumi.Map{
		"podAnnotations":    annotationsByPlatform(s.Platform),
		"priorityClassName": priorityClassByPlatformAndWorkloadType("statefulset"),
		"extraFlags":        pulumi.ToStringArray(extraFlags),
		"nodeSelector":      nodeSelectorMap,
		"resources":         resourcesMap,
		"persistence": pulumi.Map{
			"enabled":      pulumi.Bool(false),
			"storageClass": storageClassByPlatform(s.Platform),
			"size":         pulumi.String(diskSize),
		},
	}

	masterMap["tolerations"] = nodeTolerationsByPlatformAndDistruptionType(s.Platform, false)

	// deploy redis helm chart
	customValues := pulumi.Map{
		"global": pulumi.Map{
			"storageClass": storageClassByPlatform(s.Platform),
		},
		"master":  masterMap,
		"replica": masterMap,
	}

	if _, err := s.DeployHelmRelease(ctx, ns, "mt-redis", RedisChartVers, "redis", "mt-redis-values.yaml", customValues); err != nil {
		return err
	}
	return nil
}

func (nlSharedStack *NLSharedStack) DeploySharedNamespace(ctx *pulumi.Context, s *Stack) error {
	ns, err := s.CreateNamespace(ctx, "nativelink-shared")
	if err != nil {
		return err
	}

	// create a wildcard cert for shared tier
	if err = s.CreateWildCardCert(ctx, ns); err != nil {
		return err
	}

	if err = nlSharedStack.deployRedis(ctx, ns, s); err != nil {
		return err
	}

	mtNs, err := s.CreateNamespace(ctx, "nativelink-shared-mt")
	if err != nil {
		return err
	}
	if err = nlSharedStack.deployMultiTenantRedis(ctx, mtNs, s); err != nil {
		return err
	}

	return nil
}

func (nlSharedStack *NLSharedStack) DeployMongoDB(ctx *pulumi.Context, s *Stack) (*helmv3.Release, error) {
	ns, err := s.CreateNamespace(ctx, "mongodb")
	if err != nil {
		return nil, err
	}

	// @param auth.existingSecret Existing secret with MongoDB(&reg;)
	// credentials (keys: `mongodb-passwords`, `mongodb-root-password`, `mongodb-metrics-password`, `mongodb-replica-set-key`)
	// NOTE: When it's set the previous parameters are ignored.
	//
	secret, err := corev1.NewSecret(ctx, "mongodb-auth", &corev1.SecretArgs{
		ApiVersion: pulumi.String("v1"),
		Kind:       pulumi.String("Secret"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("mongodb-auth"), Namespace: ns.Metadata.Name()},
		StringData: pulumi.StringMap{
			"mongodb-root-password": nlSharedStack.MongoRootPassword,
			"mongodb-passwords":     nlSharedStack.MongoDatabasePassword,
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{ns}))
	if err != nil {
		return nil, err
	}

	nodeSelectorMap := getNodeSelector(false, "amd64", "linux", "")

	// TODO: need to configure a mongodb database to get deployed?

	persistenceMap := pulumi.Map{
		"enabled": pulumi.Bool(true),
		"size":    pulumi.String("100Gi"), // TODO: Size needs to be a config param in the YAML
	}
	if nlSharedStack.MongoStorageSnapshot != "" {
		persistenceMap["volumeClaimTemplates"] = pulumi.Map{
			"dataSource": pulumi.Map{
				"apiGroup": pulumi.String("snapshot.storage.k8s.io"),
				"kind":     pulumi.String("VolumeSnapshot"),
				"name":     pulumi.String(nlSharedStack.MongoStorageSnapshot),
			},
		}
	}
	if nlSharedStack.MongoExistingClaim != "" {
		persistenceMap["existingClaim"] = pulumi.String(nlSharedStack.MongoExistingClaim)
	}

	customValues := pulumi.Map{
		"global": pulumi.Map{
			"storageClass": pulumi.String(nlSharedStack.MongoStorageClass),
		},
		"annotations":       annotationsByPlatform(s.Platform),
		"podAnnotations":    annotationsByPlatform(s.Platform),
		"tolerations":       nodeTolerationsByPlatformAndDistruptionType(s.Platform, false),
		"priorityClassName": priorityClassByPlatformAndWorkloadType("statefulset"),
		"resourcesPreset":   pulumi.String("large"),
		"kubeVersion":       pulumi.String("1.28"),
		"nameOverride":      pulumi.String("mdb"),
		"fullnameOverride":  pulumi.String("mdb"),
		"nodeSelector":      nodeSelectorMap,
		"auth": pulumi.Map{
			"existingSecret": secret.Metadata.Name(),
			"usernames":      pulumi.ToStringArray([]string{"bep"}),
			"databases":      pulumi.ToStringArray([]string{"bep"}),
		},
		"persistence": persistenceMap,
	}
	if nlSharedStack.MongoUseSts {
		customValues["useStatefulSet"] = pulumi.Bool(true)
	}
	return s.DeployHelmRelease(ctx, ns, "mongodb", MongodbChartVers, "", "", customValues)
}

func (nlSharedStack *NLSharedStack) DeployNats(ctx *pulumi.Context, s *Stack) (*helmv3.Release, error) {
	name := "nats"
	ns, err := s.CreateNamespace(ctx, name)
	if err != nil {
		return nil, err
	}

	customValues := pulumi.Map{
		"nameOverride":      pulumi.String(name),
		"fullnameOverride":  pulumi.String(name),
		"namespaceOverride": ns.Metadata.Name(),
	}

	return s.DeployHelmRelease(ctx, ns, name, NatsChartVers, "nats", "", customValues)
}
