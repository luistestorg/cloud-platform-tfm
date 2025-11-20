package main

import (
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	batchv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/batch/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	"unir-tfm.com/shared"
	globalStack "unir-tfm.com/shared-gcp"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")
		ciSupportCfg, enableUISecretUpdater, uiNamespace := initCiSupportConfig(cfg)

		baseStackName := fmt.Sprintf("tracemachina/gcp-stack/%v", cfg.Require("globalStack"))
		baseStackRef, err := pulumi.NewStackReference(ctx, baseStackName, nil)
		if err != nil {
			return err
		}

		kubeconfig := baseStackRef.GetStringOutput(pulumi.String("kubeconfig"))
		//We need to get this process in order to get the real value from the Outputs
		clusterNameRef, err := baseStackRef.GetOutputDetails("clusterName")
		if err != nil {
			return err
		}

		envRef, err := baseStackRef.GetOutputDetails("env")
		if err != nil {
			return err
		}
		env := envRef.Value.(string)

		monLogStackName := fmt.Sprintf("tracemachina/mon-log/%v", ctx.Stack())
		monLogStackRef, err := pulumi.NewStackReference(ctx, monLogStackName, nil)
		if err != nil {
			return err
		}
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
		Oauth2CookieSecret := monLogStackRef.GetStringOutput(pulumi.String("oauth2CookieSecret"))

		oauthConfig := &shared.OauthConfig{
			Oauth2ClientSecret: monLogStackRef.GetStringOutput(pulumi.String("oauth2ClientSecret")),
			Oauth2AuthURL:      Oauth2AuthURLRef.Value.(string),
			Oauth2ClientID:     Oauth2ClientIDRef.Value.(string),
			Oauth2GroupsClaim:  Oauth2GroupsClaimRef.Value.(string),
			OidcIssuerURL:      OidcIssuerURLRef.Value.(string),
			Oauth2CookieSecret: Oauth2CookieSecret,
			Oauth2Provider:     shared.Oauth2GlobalProvider,
			Oauth2ValidateURL:  Oauth2ValidateUrlRef.Value.(string),
			Oauth2Scope:        shared.Oauth2GlobalScope,
			Oauth2TokenURL:     Oauth2TokenURLRef.Value.(string),
		}

		infraKubeStackName := fmt.Sprintf("tracemachina/infra-kube/%v", ctx.Stack())
		infraKubeStackRef, err := pulumi.NewStackReference(ctx, infraKubeStackName, nil)

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
		acmeServerRef, err := infraKubeStackRef.GetOutputDetails("AcmeServer")
		if err != nil {
			return err
		}

		var tlsCfg shared.TLSConfig
		tlsCfg.Domain = domainRef.Value.(string)
		tlsCfg.Email = emailRef.Value.(string)
		tlsCfg.AcmeServer = acmeServerRef.Value.(string)

		s := &shared.Stack{

			ClusterName:               clusterNameRef.Value.(string),
			Project:                   cfg.Get("project"),
			Env:                       env,
			TLSCfg:                    &tlsCfg,
			OauthConfig:               oauthConfig,
			Platform:                  globalStack.Platform,
			GlobalHelmChartPath:       globalStack.GlobalHelmChartPath,
			GlobalDashboardPath:       globalStack.GlobalDashboardPath,
			GlobalKibanaDashboardPath: globalStack.GlobalKibanaDashboardPath,
			GlobalConfigPath:          globalStack.GlobalConfigPath,
			CiSupportCfg:              ciSupportCfg,
		}

		k8sProvider, err := kubernetes.NewProvider(ctx, "k8s-provider", &kubernetes.ProviderArgs{Kubeconfig: kubeconfig})
		if err != nil {
			return err
		}

		dependsOn := []pulumi.Resource{k8sProvider}
		s.DependsOn = dependsOn
		s.K8sProvider = k8sProvider

		//Defines resources based on the environment
		if s.Env == "dev" {
			s.Resources.InitResourcesDev()
		} else {
			if s.Env == "prod" {
				s.Resources.InitResourcesProd()
			}
		}

		if s.CiSupportCfg.EnableActionsRunnerController {
			if err := s.CiSupportCfg.DeployActionsRunnerController(ctx, s); err != nil {
				return err
			}
		}

		if enableUISecretUpdater {

			err = createGCPAuthJobs(ctx, uiNamespace, s)
			if err != nil {
				return err
			}
		}

		ctx.Export("env", pulumi.String(s.Env))
		return nil
	})
}

func initCiSupportConfig(cfg *config.Config) (*shared.CiSupportSharedStack, bool, string) {

	var secCompConfig shared.CiSupportSharedStack

	secCompConfig.EnableActionsRunnerController = cfg.GetBool("enableActionsController")
	if secCompConfig.EnableActionsRunnerController {

		secCompConfig.GithubConfigURL = cfg.Require("githubConfigUrl")
		secCompConfig.GithubActionsAppID = cfg.RequireSecret("github_app_id")
		secCompConfig.GithubActionsAppInstID = cfg.RequireSecret("github_app_installation_id")
		secCompConfig.GithubActionsPrivateKey = cfg.RequireSecret("github_app_private_key")

	}

	enableUISecretUpdater := cfg.GetBool("enableUISecretUpdater")
	return &secCompConfig, enableUISecretUpdater, ""

}

func createGCPAuthJobs(ctx *pulumi.Context, ns string, s *shared.Stack) error {
	// Creating cronjob for updating ECR secret for image pull, ECR token needs to be refreshed every 12 hours.

	//Create extra entities for renewing runbook pods

	ecrSecretRenewSvcAccount, err := corev1.NewServiceAccount(ctx, "ecr-renew-secret", &corev1.ServiceAccountArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("ecr-renew-secret"),
			Namespace: pulumi.String(ns),
			Annotations: pulumi.StringMap{
				"iam.gke.io/gcp-service-account": pulumi.String("gke-cloud-platform-deployer@cloud-platform-tfm.iam.gserviceaccount.com"),
			},
		},
	}, pulumi.Provider(s.K8sProvider))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, ecrSecretRenewSvcAccount)

	ecrSecretRenewRole, err := rbacv1.NewRole(ctx, "ecr-renew-secret", &rbacv1.RoleArgs{
		ApiVersion: pulumi.String("rbac.authorization.k8s.io"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("ecr-renew-secret"), Namespace: pulumi.String(ns)},
		Rules: rbacv1.PolicyRuleArray{
			&rbacv1.PolicyRuleArgs{
				Verbs: pulumi.StringArray{
					pulumi.String("create"),
					pulumi.String("get"),
					pulumi.String("list"),
				},
				Resources: pulumi.StringArray{
					pulumi.String("secrets"),
				},
				ApiGroups: pulumi.StringArray{
					pulumi.String(""),
				},
			},
			&rbacv1.PolicyRuleArgs{
				Verbs: pulumi.StringArray{
					pulumi.String("*"),
				},
				Resources: pulumi.StringArray{
					pulumi.String("secrets"),
				},
				ResourceNames: pulumi.StringArray{
					pulumi.String("ecr-secret"),
				},
				ApiGroups: pulumi.StringArray{
					pulumi.String(""),
				},
			},
		},
	}, pulumi.Provider(s.K8sProvider),
	)
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, ecrSecretRenewRole)

	ecrSecretRenewRoleBinding, err := rbacv1.NewRoleBinding(ctx, "ecr-renew-secret", &rbacv1.RoleBindingArgs{
		ApiVersion: pulumi.String("rbac.authorization.k8s.io"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("ecr-renew-secret"), Namespace: pulumi.String(ns)},
		Subjects: &rbacv1.SubjectArray{
			&rbacv1.SubjectArgs{
				Kind: pulumi.String("ServiceAccount"),
				Name: pulumi.String("ecr-renew-secret"),
			},
		},
		RoleRef: &rbacv1.RoleRefArgs{
			ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
			Kind:     pulumi.String("Role"),
			Name:     pulumi.String("ecr-renew-secret"),
		},
	}, pulumi.Provider(s.K8sProvider))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, ecrSecretRenewRoleBinding)

	// Configmap for environment values
	envConfig, err := corev1.NewConfigMap(ctx, "env-config", &corev1.ConfigMapArgs{
		ApiVersion: pulumi.String("v1"),
		Data: pulumi.StringMap{
			"DOCKER_SECRET_NAME": pulumi.String("ecr-secret"),
			"NAMESPACE":          pulumi.String(ns),
			"ECR_REGION":         pulumi.String("us-east-2"),
		},
		Kind:     pulumi.String("ConfigMap"),
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("env-config"), Namespace: pulumi.String(ns)},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, envConfig)

	jobCommand := []string{
		"ECR_TOKEN=`gcloud secrets versions access latest --secret=CloudEngSA_ECR --quiet`",
		"kubectl delete secret --ignore-not-found $DOCKER_SECRET_NAME -n $NAMESPACE",
		"kubectl create secret docker-registry $DOCKER_SECRET_NAME --docker-server=https://299166832260.dkr.ecr.${ECR_REGION}.amazonaws.com --docker-username=AWS --docker-password=${ECR_TOKEN} --namespace=$NAMESPACE_NAME",
		"kubectl label secret $DOCKER_SECRET_NAME -n $NAMESPACE tfm/secret-type=regcred",
		"kubectl annotate secret $DOCKER_SECRET_NAME -n $NAMESPACE tfm/registry-url=299166832260.dkr.ecr.${ECR_REGION}.amazonaws.com",
		"echo \"Secret was successfully updated at $(date)\"",
	}

	jobPodTemplate := &corev1.PodTemplateSpecArgs{
		Spec: &corev1.PodSpecArgs{
			ServiceAccountName: pulumi.String("ecr-renew-secret"),
			Containers: corev1.ContainerArray{
				&corev1.ContainerArgs{
					Name:  pulumi.String("ecr-renew-secret"),
					Image: pulumi.String("claranet/gcloud-kubectl-docker"),
					EnvFrom: &corev1.EnvFromSourceArray{
						&corev1.EnvFromSourceArgs{
							ConfigMapRef: &corev1.ConfigMapEnvSourceArgs{
								Name: envConfig.Metadata.Name(),
							},
						},
					},
					Command: pulumi.StringArray{
						pulumi.String("/bin/sh"),
						pulumi.String("-c"),
						pulumi.String(strings.Join(jobCommand, "; ")),
					},
				},
			},
			RestartPolicy: pulumi.String("OnFailure"),
		},
	}

	ecrSecretSingleJob, err := batchv1.NewJob(ctx, "ecr-renew-secret", &batchv1.JobArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("ecr-renew-secret"), Namespace: pulumi.String(ns)},
		Spec:     &batchv1.JobSpecArgs{Template: jobPodTemplate},
	}, pulumi.Provider(s.K8sProvider))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, ecrSecretSingleJob)

	ecrSecretRenewCronjob, err := batchv1.NewCronJob(ctx, "ecr-renew-secret", &batchv1.CronJobArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("ecr-renew-secret"), Namespace: pulumi.String(ns)},
		Spec: &batchv1.CronJobSpecArgs{
			ConcurrencyPolicy:          pulumi.String("Forbid"),
			Schedule:                   pulumi.String("0 */2 * * *"),
			SuccessfulJobsHistoryLimit: pulumi.Int(1),
			JobTemplate: &batchv1.JobTemplateSpecArgs{
				Spec: &batchv1.JobSpecArgs{
					ActiveDeadlineSeconds: pulumi.Int(300),
					BackoffLimit:          pulumi.Int(3),
					Template:              jobPodTemplate,
				},
			},
		},
	}, pulumi.Provider(s.K8sProvider))
	s.DependsOn = append(s.DependsOn, ecrSecretRenewCronjob)

	if err != nil {
		return err
	}
	return nil
}
