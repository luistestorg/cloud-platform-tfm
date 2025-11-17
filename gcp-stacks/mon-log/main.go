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
		monSharedStack := initMonConfig(cfg)

		baseStackName := fmt.Sprintf("tracemachina/gcp-stack/%v", cfg.Require("globalStack"))
		baseStackRef, err := pulumi.NewStackReference(ctx, baseStackName, nil)
		if err != nil {
			return err
		}
		clusterNameRef, err := baseStackRef.GetOutputDetails("clusterName")
		if err != nil {
			return err
		}
		kubeconfig := baseStackRef.GetStringOutput(pulumi.String("kubeconfig"))
		regionRef, err := baseStackRef.GetOutputDetails("gcpRegion")
		if err != nil {
			return err
		}
		envRef, err := baseStackRef.GetOutputDetails("env")
		if err != nil {
			return err
		}

		k8sProvider, err := kubernetes.NewProvider(ctx, "k8s-provider", &kubernetes.ProviderArgs{Kubeconfig: kubeconfig})
		if err != nil {
			return err
		}

		var tlsCfg shared.TLSConfig
		tlsCfg.Domain = cfg.Require("domain")

		oauthConfig := &shared.OauthConfig{
			Oauth2ClientSecret: cfg.RequireSecret("oauth2ClientSecret"),
			Oauth2AuthURL:      cfg.Get("oauth2AuthUrl"),
			Oauth2ClientID:     cfg.Get("oauth2ClientId"),
			Oauth2GroupsClaim:  cfg.Get("oauth2GroupsClaim"),
			OidcIssuerURL:      cfg.Get("oidcIssuerUrl"),
			Oauth2CookieSecret: cfg.RequireSecret("oauth2CookieSecret"),
			Oauth2Provider:     "oidc",
			Oauth2ValidateURL:  cfg.Get("oauth2ValidateUrl"),
			Oauth2Scope:        "openid email",
			Oauth2TokenURL:     cfg.Get("oauth2TokenUrl"),
		}

		s := &shared.Stack{
			ClusterName:               clusterNameRef.Value.(string),
			K8sProvider:               k8sProvider,
			Env:                       envRef.Value.(string),
			TLSCfg:                    &tlsCfg,
			OauthConfig:               oauthConfig,
			Platform:                  globalStack.Platform,
			GlobalHelmChartPath:       globalStack.GlobalHelmChartPath,
			GlobalDashboardPath:       globalStack.GlobalDashboardPath,
			GlobalKibanaDashboardPath: globalStack.GlobalKibanaDashboardPath,
			GlobalConfigPath:          globalStack.GlobalConfigPath,
			Region:                    regionRef.Value.(string),
		}

		alertWebhookURL, err := cfg.TrySecret("alertWebhookUrl")
		if err == nil {
			s.AlertWebhookURL = &alertWebhookURL
			ctx.Export("alertWebhookURL", *s.AlertWebhookURL)
		}

		slackWebhookURL, err := cfg.TrySecret("slackWebhookUrl")
		if err == nil {
			s.SlackWebhookURL = &slackWebhookURL
			ctx.Export("slackWebhookURL", slackWebhookURL)

		}

		//Defines resources based on the environment
		if s.Env == "dev" {
			s.Resources.InitResourcesDev()
		} else {
			if s.Env == "prod" {
				s.Resources.InitResourcesProd()
			}
		}

		if _, err := monSharedStack.DeployMonitoringComponents(ctx, s); err != nil {
			return err
		}

		if err := monSharedStack.DeployLoggingComponents(ctx, s); err != nil {
			return err
		}
		if monSharedStack.EnableOTEL {
			if err := monSharedStack.DeployOTELCollector(ctx, s); err != nil {
				return err
			}
		}

		if err = monSharedStack.DeployCustomDashboards(ctx, s); err != nil {
			return err
		}

		ctx.Export("bootstrapAdminPassword", monSharedStack.BootstrapAdminPassword)
		ctx.Export("grafanaAdminPassword", monSharedStack.GrafanaAdminPassword)
		ctx.Export("oauth2ClientSecret", oauthConfig.Oauth2ClientSecret)
		ctx.Export("oauth2AuthUrl", pulumi.String(oauthConfig.Oauth2AuthURL))
		ctx.Export("oauth2ClientId", pulumi.String(oauthConfig.Oauth2ClientID))
		ctx.Export("oauth2GroupsClaim", pulumi.String(oauthConfig.Oauth2GroupsClaim))
		ctx.Export("oauth2ValidateUrl", pulumi.String(oauthConfig.Oauth2ValidateURL))
		ctx.Export("oidcIssuerUrl", pulumi.String(oauthConfig.OidcIssuerURL))
		ctx.Export("oauth2TokenUrl", pulumi.String(oauthConfig.Oauth2TokenURL))
		ctx.Export("oauth2CookieSecret", oauthConfig.Oauth2CookieSecret)
		ctx.Export("grafanaAdminPassword", monSharedStack.GrafanaAdminPassword)
		ctx.Export("domain", pulumi.String(tlsCfg.Domain))

		return nil
	})
}

func initMonConfig(cfg *config.Config) *shared.MonSharedStack {

	var monSharedStack shared.MonSharedStack

	if monSharedStack.PrometheusStorage == "" {
		monSharedStack.PrometheusStorage = "50Gi"
	}
	if monSharedStack.PrometheusStorageClass == "" {
		monSharedStack.PrometheusStorageClass = "standard"
	}
	if monSharedStack.PrometheusMemoryRequests == "" {
		monSharedStack.PrometheusMemoryRequests = "8Gi"
	}
	if monSharedStack.PrometheusCPURequests == "" {
		monSharedStack.PrometheusCPURequests = "1800m"
	}
	if monSharedStack.GrafanaStorageClass == "" {
		monSharedStack.GrafanaStorageClass = "standard-rwo"
	}
	if monSharedStack.GrafanaStorage == "" {
		monSharedStack.GrafanaStorage = "20Gi"
	}

	monSharedStack.GrafanaAdminPassword = cfg.RequireSecret("grafanaAdminPassword")
	monSharedStack.BootstrapAdminPassword = cfg.RequireSecret("bootstrapAdminPassword")

	monSharedStack.EnableElasticSearch = cfg.GetBool("EnableElasticSearch")
	monSharedStack.EnableOTEL = cfg.GetBool("EnableElasticSearch")
	monSharedStack.ElasticSearchPassword = cfg.RequireSecret("elasticSearchPassword")
	monSharedStack.KibanaPassword = cfg.RequireSecret("kibanaPassword")

	monSharedStack.ElasticSearchStorageSize = cfg.Get("ElasticSearchStorageSize")
	monSharedStack.ElasticSearchStorageClass = cfg.Get("ElasticSearchStorageClass")
	monSharedStack.KibanaStorageClass = cfg.Get("KibanaStorageClass")

	if monSharedStack.ElasticSearchStorageSize == "" {
		monSharedStack.ElasticSearchStorageSize = "50Gi"
	}
	if monSharedStack.KibanaStorageSize == "" {
		monSharedStack.KibanaStorageSize = "50Gi"
	}

	if monSharedStack.PrometheusReplicas <= 0 {
		monSharedStack.PrometheusReplicas = 1
	}

	monSharedStack.ClusterIssuer = globalStack.GlobalClusterIssuer

	return &monSharedStack

}
