package main

import (
	"fmt"

	"tracemachina.com/shared"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	globalStack "tracemachina.com/stack"
)

type (
	sharedStack struct {
		mongoStorageClass     string
		mongoStorageSnapshot  string
		mongoUseSts           bool
		mongoExistingClaim    string
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
		EnableNats                      bool
		ClusterRedisSize                string
		ClusterRedisStorageClass        string
		ClusterRedisZone                string
	}
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		cfg := config.New(ctx, "")
		sharedCfg := initSharedConfig(cfg)

		baseStackName := fmt.Sprintf("tracemachina/gcp-stack/%v", cfg.Require("globalStack"))
		baseStackRef, err := pulumi.NewStackReference(ctx, baseStackName, nil)
		if err != nil {
			return err
		}
		kubeconfig := baseStackRef.GetStringOutput(pulumi.String("kubeconfig"))
		clusterNameRef, err := baseStackRef.GetOutputDetails("clusterName")
		if err != nil {
			return err
		}

		k8sProvider, err := kubernetes.NewProvider(ctx, "k8s-provider", &kubernetes.ProviderArgs{Kubeconfig: kubeconfig})
		if err != nil {
			return err
		}

		envRef, err := baseStackRef.GetOutputDetails("env")
		if err != nil {
			return err
		}

		infraKubeStackName := fmt.Sprintf("tracemachina/infra-kube/%v", cfg.Require("globalStack"))
		infraKubeStackRef, err := pulumi.NewStackReference(ctx, infraKubeStackName, nil)
		if err != nil {
			return err
		}

		acmeServerRef, err := infraKubeStackRef.GetOutputDetails("AcmeServer")
		if err != nil {
			return err
		}

		domainRef, err := infraKubeStackRef.GetOutputDetails("Domain")
		if err != nil {
			return err
		}

		emailRef, err := infraKubeStackRef.GetOutputDetails("Email")
		if err != nil {
			return err
		}

		var tlsCfg shared.TLSConfig

		tlsCfg.AcmeServer = acmeServerRef.Value.(string)
		tlsCfg.Domain = domainRef.Value.(string)
		tlsCfg.Email = emailRef.Value.(string)

		dependsOn := []pulumi.Resource{k8sProvider}

		s := &shared.Stack{
			ClusterName:               clusterNameRef.Value.(string),
			K8sProvider:               k8sProvider,
			Env:                       envRef.Value.(string),
			DependsOn:                 dependsOn,
			TLSCfg:                    &tlsCfg,
			Platform:                  globalStack.Platform,
			GlobalHelmChartPath:       globalStack.GlobalHelmChartPath,
			GlobalDashboardPath:       globalStack.GlobalDashboardPath,
			GlobalKibanaDashboardPath: globalStack.GlobalKibanaDashboardPath,
			GlobalConfigPath:          globalStack.GlobalConfigPath,
		}

		if sharedCfg.EnableMongoDB {

			if _, err := sharedCfg.DeployMongoDB(ctx, s); err != nil {
				return err
			}
		}

		if sharedCfg.EnableNats {
			if _, err = sharedCfg.DeployNats(ctx, s); err != nil {
				return err
			}
		}

		fmt.Printf("CreateSharedNativeLinkNamespace? %t\n", sharedCfg.CreateSharedNativeLinkNamespace)
		if sharedCfg.CreateSharedNativeLinkNamespace {
			// we need to create the nativelink-shared namespacglobalStacke and wildcard cert ...
			// the API creates the NativeLink claim (since it does that for all other claims)

			if err = sharedCfg.DeploySharedNamespace(ctx, s); err != nil {
				return err
			}
		}

		ctx.Export("SharedRedisZone", pulumi.String(sharedCfg.SharedRedisZone))
		ctx.Export("EnableMongoDB", pulumi.Bool(sharedCfg.EnableMongoDB))
		ctx.Export("MongoRootPassword", sharedCfg.MongoRootPassword)
		ctx.Export("MongoDatabasePassword", sharedCfg.MongoDatabasePassword)
		return nil
	})
}

func initSharedConfig(cfg *config.Config) *shared.NLSharedStack {

	var sharedConfig shared.NLSharedStack

	cfg.RequireObject("shared", &sharedConfig)

	if sharedConfig.EnableMongoDB {
		sharedConfig.MongoRootPassword = cfg.RequireSecret("mongoRootPassword")
		sharedConfig.MongoDatabasePassword = cfg.RequireSecret("mongoDatabasePassword")
	}

	if sharedConfig.SharedRedisStorageClass == "" {
		sharedConfig.SharedRedisStorageClass = "standard"
	}

	return &sharedConfig

}
