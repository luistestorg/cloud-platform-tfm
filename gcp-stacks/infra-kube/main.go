package main

import (
	"fmt"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	"tracemachina.com/shared"
	globalStack "tracemachina.com/stack"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		cfg := config.New(ctx, "")
		tlsCfg := InitTLSConfig(cfg)

		baseStackName := fmt.Sprintf("tracemachina/gcp-stack/%v", cfg.Require("globalStack"))
		baseStackRef, err := pulumi.NewStackReference(ctx, baseStackName, nil)

		if err != nil {
			return err
		}

		infraStackName := fmt.Sprintf("tracemachina/infra-gcp/%v", ctx.Stack())
		infraStackRef, err := pulumi.NewStackReference(ctx, infraStackName, nil)

		if err != nil {
			return err
		}

		monlogStackName := fmt.Sprintf("tracemachina/mon-log/%v", ctx.Stack())
		monlogStackRef, err := pulumi.NewStackReference(ctx, monlogStackName, nil)

		if err != nil {
			return err
		}

		domainRef, err := monlogStackRef.GetOutputDetails("domain")
		if err != nil {
			return err
		}

		tlsCfg.Domain = domainRef.Value.(string)

		kubeconfig := baseStackRef.GetStringOutput(pulumi.String("kubeconfig"))
		clusterNameRef, err := baseStackRef.GetOutputDetails("clusterName")
		if err != nil {
			return err
		}

		deployGpuRef, err := infraStackRef.GetOutputDetails("enableGPU")
		if err != nil {
			return err
		}
		deployGpu := deployGpuRef.Value.(bool)

		k8sProvider, err := kubernetes.NewProvider(ctx, "k8s-provider", &kubernetes.ProviderArgs{Kubeconfig: kubeconfig})
		if err != nil {
			return err
		}
		dependsOn := []pulumi.Resource{k8sProvider}

		envRef, err := baseStackRef.GetOutputDetails("env")
		if err != nil {
			return err
		}

		s := &shared.Stack{
			ClusterName:               clusterNameRef.Value.(string),
			K8sProvider:               k8sProvider,
			Env:                       envRef.Value.(string),
			TLSCfg:                    tlsCfg,
			Platform:                  globalStack.Platform,
			GlobalHelmChartPath:       globalStack.GlobalHelmChartPath,
			GlobalDashboardPath:       globalStack.GlobalDashboardPath,
			GlobalKibanaDashboardPath: globalStack.GlobalKibanaDashboardPath,
			GlobalConfigPath:          globalStack.GlobalConfigPath,
			Project:                   cfg.Get("project"),
		}

		s.DependsOn = dependsOn
		//Defines resources based on the environment
		if s.Env == "dev" {
			s.Resources.InitResourcesDev()
		} else {
			if s.Env == "prod" {
				s.Resources.InitResourcesProd()
			}
		}

		if _, err := s.DeployIngressNginxController(ctx); err != nil {
			return err
		}
		if _, err := s.DeployCertManager(ctx); err != nil {
			return err
		}

		err = s.LinkCertManagerSa(ctx, globalStack.GlobalGKEServiceAccount)

		if err != nil {
			return err
		}

		if _, err = s.CreateTLSCertIssuer(ctx); err != nil {
			return err
		}

		if deployGpu {

			if err = s.DeployGpuPlugin(ctx); err != nil {
				return err
			}
		}

		// Deploying ingress after cert-manager is installed - this is because we want to enable servicemonitors

		// expose Grafana over https
		annotationMap := map[string]string{"nginx.ingress.kubernetes.io/affinity-mode": "persistent"}
		host, err := s.CreateIngress(ctx, "grafana", "monitoring", "oauth2-grafana", 80, annotationMap)
		if err != nil {
			return err
		}
		fmt.Printf("Deployed ingress for host: %s\n", host)

		// Also deploying kibana ingress here because certs are not available in the mon-log stack
		kibanaHost, err := s.CreateIngress(ctx, "log-analytics", "log-system", "kibana", 5601, annotationMap)
		if err != nil {
			return err
		}
		fmt.Printf("Deployed ingress for host: %s\n", kibanaHost)

		ctx.Export("AcmeServer", pulumi.String(tlsCfg.AcmeServer))
		ctx.Export("Domain", pulumi.String(tlsCfg.Domain))
		ctx.Export("Email", pulumi.String(tlsCfg.Email))

		return nil
	})
}

func InitTLSConfig(cfg *config.Config) *shared.TLSConfig {
	var tlsCfg shared.TLSConfig
	cfg.RequireObject("tls", &tlsCfg)
	if tlsCfg.AcmeServer == "" {
		// fallback to using the let's encrypt staging server
		tlsCfg.AcmeServer = "https://acme-staging-v02.api.letsencrypt.org/directory"
	}
	if tlsCfg.Email == "" {
		tlsCfg.Email = "lmunozes@gmail.com"
	}
	return &tlsCfg
}
