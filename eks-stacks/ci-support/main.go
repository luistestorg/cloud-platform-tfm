package main

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	"unir-tfm.com/shared"
)

type CiSupportConfig struct {
	ProjectName                   string
	Environment                   string
	EnableActionsRunnerController bool
	EnableTekton                  bool
	GithubConfigURL               string
	GithubOrg                     string
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Load configuration
		cfg := config.New(ctx, "")
		awsCfg := config.New(ctx, "aws")

		// Get stack reference to infra-kube
		infraKubeStackRef := cfg.Require("infraKubeStackRef")
		infraKubeStack, err := pulumi.NewStackReference(ctx, infraKubeStackRef, nil)
		if err != nil {
			return err
		}

		// Get outputs from infra-kube stack
		clusterName := infraKubeStack.GetStringOutput(pulumi.String("clusterName"))
		environment := infraKubeStack.GetStringOutput(pulumi.String("environment"))
		awsRegion := awsCfg.Require("region")

		// Initialize CI Support config
		ciCfg := &CiSupportConfig{
			ProjectName:                   cfg.Get("projectName"),
			EnableActionsRunnerController: cfg.GetBool("enableActionsRunnerController"),
			EnableTekton:                  cfg.GetBool("enableTekton"),
			GithubOrg:                     cfg.Get("githubOrg"),
		}

		// Export outputs
		ctx.Export("clusterName", clusterName)
		ctx.Export("environment", environment)
		ctx.Export("awsRegion", pulumi.String(awsRegion))
		ctx.Export("enableActionsRunner", pulumi.Bool(ciCfg.EnableActionsRunnerController))
		ctx.Export("enableTekton", pulumi.Bool(ciCfg.EnableTekton))

		return nil
	})
}

// initCiSupportConfig initializes CI support configuration from Pulumi config
func initCiSupportConfig(cfg *config.Config) *shared.CiSupportSharedStack {
	var ciConfig shared.CiSupportSharedStack
	ciConfig.EnableActionsRunnerController = cfg.GetBool("enableActionsController")

	if ciConfig.EnableActionsRunnerController {
		ciConfig.GithubConfigURL = cfg.Require("githubConfigUrl")
		ciConfig.GithubActionsAppID = cfg.RequireSecret("github_app_id")
		ciConfig.GithubActionsAppInstID = cfg.RequireSecret("github_app_installation_id")
		ciConfig.GithubActionsPrivateKey = cfg.RequireSecret("github_app_private_key")
	}

	ciConfig.EnableTekton = cfg.GetBool("enableTekton")

	return &ciConfig
}

// DeployCiSupportComponents deploys CI/CD support components
func DeployCiSupportComponents(ctx *pulumi.Context, s *shared.Stack, ciCfg *shared.CiSupportSharedStack) error {
	// Deploy Actions Runner Controller if enabled
	if ciCfg.EnableActionsRunnerController {
		if err := deployActionsRunnerController(ctx, s, ciCfg); err != nil {
			return err
		}
	}

	return nil
}

// deployActionsRunnerController deploys GitHub Actions Runner Controller
func deployActionsRunnerController(ctx *pulumi.Context, s *shared.Stack, ciCfg *shared.CiSupportSharedStack) error {
	ns, err := s.CreateNamespace(ctx, "actions-runner-system")
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, ns)

	// Deploy actions-runner-controller helm chart
	customValues := pulumi.Map{
		"authSecret": pulumi.Map{
			"create":                     pulumi.Bool(true),
			"github_app_id":              ciCfg.GithubActionsAppID,
			"github_app_installation_id": ciCfg.GithubActionsAppInstID,
			"github_app_private_key":     ciCfg.GithubActionsPrivateKey,
		},
	}

	_, err = s.DeployHelmRelease(ctx, ns, "actions-runner-controller", shared.ActionsRunnerControllerChartVers, "", "", customValues)
	if err != nil {
		return err
	}

	return nil
}
