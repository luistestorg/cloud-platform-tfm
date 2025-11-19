package main

import (
	"fmt"

	"tracemachina.com/shared"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	globalStack "tracemachina.com/stack"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		cfg := config.New(ctx, "")
		apiConfig := initSelfServiceApiConfig(cfg)

		//Refs from other stacks

		baseStackName := fmt.Sprintf("tracemachina/gcp-stack/%v", cfg.Require("globalStack"))
		baseStackRef, err := pulumi.NewStackReference(ctx, baseStackName, nil)
		if err != nil {
			return err
		}

		sharedStackName := fmt.Sprintf("tracemachina/nativelink-shared/%v", ctx.Stack())
		sharedbaseStackRef, err := pulumi.NewStackReference(ctx, sharedStackName, nil)
		if err != nil {
			return err
		}

		monLogStackName := fmt.Sprintf("tracemachina/mon-log/%v", ctx.Stack())
		monLogStackRef, err := pulumi.NewStackReference(ctx, monLogStackName, nil)
		if err != nil {
			return err
		}

		sqlStackName := fmt.Sprintf("tracemachina/sql/%v", ctx.Stack())
		sqlStackRef, err := pulumi.NewStackReference(ctx, sqlStackName, nil)
		if err != nil {
			return err
		}

		//Get outputs from other stacks
		//From globalstack
		kubeconfig := baseStackRef.GetStringOutput(pulumi.String("kubeconfig"))
		clusterNameRef, err := baseStackRef.GetOutputDetails("clusterName")
		if err != nil {
			return err
		}

		//From sql
		apiConfig.SQLPublicIPAddress = sqlStackRef.GetStringOutput(pulumi.String("sqlPublicIpAddress"))
		apiConfig.SQLPrivateIPAddress = sqlStackRef.GetStringOutput(pulumi.String("sqlPrivateIpAddress"))

		apiConfig.DbName = sqlStackRef.GetStringOutput(pulumi.String("dbName"))
		apiConfig.DbPassword = sqlStackRef.GetStringOutput(pulumi.String("dbPassword"))
		apiConfig.PgPassword = sqlStackRef.GetStringOutput(pulumi.String("dbPassword"))

		sqlZoneRef, err := sqlStackRef.GetOutputDetails("sqlZone")
		if err != nil {
			return err
		}

		apiConfig.SQLZone = sqlZoneRef.Value.(string)

		//From nativelink-shared
		SharedRedisZoneRef, err := sharedbaseStackRef.GetOutputDetails("SharedRedisZone")
		if err != nil {
			return err
		}

		apiConfig.SharedRedisZone = SharedRedisZoneRef.Value.(string)
		apiConfig.MongoRootPassword = sharedbaseStackRef.GetStringOutput(pulumi.String("MongoRootPassword"))
		apiConfig.MongoDatabasePassword = sharedbaseStackRef.GetStringOutput(pulumi.String("MongoDatabasePassword"))

		enableMongoDBRef, err := sharedbaseStackRef.GetOutputDetails("EnableMongoDB")
		if err != nil {
			return err
		}
		apiConfig.EnableMongoDB = enableMongoDBRef.Value.(bool)
		//From mon-log

		Oauth2AuthURLRef, err := monLogStackRef.GetOutputDetails("oauth2AuthUrl")
		if err != nil {
			return err
		}

		Oauth2ClientIDRef, err := monLogStackRef.GetOutputDetails("oauth2ClientId")
		if err != nil {
			return err
		}

		Oauth2GroupsClaimRef, err := monLogStackRef.GetOutputDetails("oauth2GroupsClaim")
		if err != nil {
			return err
		}

		Oauth2ValidateUrlRef, err := monLogStackRef.GetOutputDetails("oauth2ValidateUrl")
		if err != nil {
			return err
		}

		OidcIssuerURLRef, err := monLogStackRef.GetOutputDetails("oidcIssuerUrl")
		if err != nil {
			return err
		}

		Oauth2TokenURLRef, err := monLogStackRef.GetOutputDetails("oauth2TokenUrl")
		if err != nil {
			return err
		}

		oauthConfig := &shared.OauthConfig{
			Oauth2ClientSecret: monLogStackRef.GetStringOutput(pulumi.String("oauth2ClientSecret")),
			Oauth2AuthURL:      Oauth2AuthURLRef.Value.(string),
			Oauth2ClientID:     Oauth2ClientIDRef.Value.(string),
			Oauth2GroupsClaim:  Oauth2GroupsClaimRef.Value.(string),
			OidcIssuerURL:      OidcIssuerURLRef.Value.(string),
			Oauth2CookieSecret: cfg.RequireSecret("oauth2CookieSecret"),
			Oauth2Provider:     shared.Oauth2GlobalProvider,
			Oauth2ValidateURL:  Oauth2ValidateUrlRef.Value.(string),
			Oauth2Scope:        shared.Oauth2GlobalScope,
			Oauth2TokenURL:     Oauth2TokenURLRef.Value.(string),
		}

		apiConfig.BootstrapAdminPassword = monLogStackRef.GetStringOutput(pulumi.String("bootstrapAdminPassword"))
		apiConfig.GrafanaAdminPassword = monLogStackRef.GetStringOutput(pulumi.String("grafanaAdminPassword"))

		apiConfig.CachePassword = cfg.RequireSecret("cachePassword")
		apiConfig.SharedCachePassword = cfg.RequireSecret("sharedCachePassword")
		apiConfig.CachePassword = cfg.RequireSecret("cachePassword")
		apiConfig.APIImage = cfg.Require("apiImage")
		apiConfig.EcrRegion = cfg.Require("ecrRegion")
		awsRegion := cfg.Get("awsRegion")
		if awsRegion == "" {
			awsRegion = "us-west-2"
		}
		apiConfig.AwsRegion = awsRegion

		apiConfig.AwsAccessKeyID = cfg.RequireSecret("awsAccessKeyId")
		apiConfig.AwsSecretAccessKey = cfg.RequireSecret("awsSecretAccessKey")
		apiConfig.GithubAuthToken = cfg.RequireSecret("githubAuthToken")

		apiConfig.PgUsername = cfg.Require("pgUsername")

		apiConfig.Project = cfg.Require("project")
		apiConfig.Profile = cfg.Require("profile")
		apiConfig.GCPAWSRoleArn = cfg.Require("gcpAWSRoleArn")
		apiConfig.GCPAWSSub = cfg.Require("gcpAWSSub")

		K8sProvider, err := kubernetes.NewProvider(ctx, "k8s-provider", &kubernetes.ProviderArgs{Kubeconfig: kubeconfig})
		if err != nil {
			return err
		}

		envRef, err := baseStackRef.GetOutputDetails("env")
		if err != nil {
			return err
		}
		env := envRef.Value.(string)

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

		regionRef, err := baseStackRef.GetOutputDetails("gcpRegion")
		if err != nil {
			return err
		}

		s := &shared.Stack{
			ClusterName:                   clusterNameRef.Value.(string),
			K8sProvider:                   K8sProvider,
			ClusterSelfLink:               baseStackRef.GetStringOutput(pulumi.String("clusterSelfLink")),
			Location:                      baseStackRef.GetStringOutput(pulumi.String("Location")),
			OauthConfig:                   oauthConfig,
			TLSCfg:                        &tlsCfg,
			Project:                       cfg.Get("project"),
			GlobalCrossplanePath:          globalStack.GlobalCrossplanePath,
			Env:                           env,
			Region:                        regionRef.Value.(string),
			GlobalGKEServiceAccount:       globalStack.GlobalGKEServiceAccount,
			GlobalHelmChartPath:           globalStack.GlobalHelmChartPath,
			GlobalDashboardPath:           globalStack.GlobalDashboardPath,
			GlobalKibanaDashboardPath:     globalStack.GlobalKibanaDashboardPath,
			GlobalConfigPath:              globalStack.GlobalConfigPath,
			GlobalTemporalImageRepository: globalStack.GlobalTemporalImageRepository,
			Platform:                      globalStack.Platform,
		}

		//Defines resources based on the environment
		if s.Env == "dev" {
			s.Resources.InitResourcesDev()
		} else {
			if s.Env == "prod" {
				s.Resources.InitResourcesProd()
			}
		}

		if _, err = apiConfig.DeployCrossplane(ctx, s); err != nil {
			return err
		}

		if err = apiConfig.DeployNativeLinkCrossplane(ctx, s); err != nil {
			return err
		}

		if err := apiConfig.DeploySelfServiceAPI(ctx, s); err != nil {
			return err
		}

		return nil
	})
}

func initSelfServiceApiConfig(cfg *config.Config) *shared.SelfServiceAPIConfig {
	var ssApiCfg shared.SelfServiceAPIConfig
	_ = cfg.GetObject("", &ssApiCfg)

	if ssApiCfg.APIEnabled {

		if ssApiCfg.SubDomain == "" {
			ssApiCfg.SubDomain = "api"
		}
	}

	if ssApiCfg.SegmentAPIEnabled {
		ssApiCfg.SegmentAPIWriteKey = cfg.RequireSecret("segmentApiWriteKey")
	}

	ssApiCfg.APIImage = cfg.Require("APIImage")

	ssApiCfg.PgUsername = cfg.Require("pgUsername")

	ssApiCfg.EnableTemporal = cfg.GetBool("enableTemporal")

	ssApiCfg.ClusterID = cfg.Require("globalStack")

	return &ssApiCfg
}
