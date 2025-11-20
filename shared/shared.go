package shared

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	awsiam "github.com/pulumi/pulumi-aws-iam/sdk/go/aws-iam"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/iam"
	ec2x "github.com/pulumi/pulumi-awsx/sdk/go/awsx/ec2"
	"github.com/pulumi/pulumi-eks/sdk/go/eks"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	batchv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/batch/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	networkingv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	policyv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/policy/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	schedulingv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/scheduling/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	resourceDef "unir-tfm.com/resources"
)

const (
	revisionMaxHistory   = 3
	GlobalClusterIssuer  = "letsencrypt-tls-issuer"
	Oauth2GlobalScope    = "openid email"
	Oauth2GlobalProvider = "oidc"
)

type (
	Stack struct {
		ClusterName       string
		K8sProvider       *kubernetes.Provider
		DependsOn         []pulumi.Resource
		TLSCfg            *TLSConfig
		OauthConfig       *OauthConfig
		LogGroupRetention int
		Env               string
		ClusterSelfLink   pulumi.StringOutput
		Location          pulumi.StringOutput
		IngressHosts      []string

		GlobalGKEServiceAccount       string
		Platform                      string
		GlobalHelmChartPath           string
		GlobalDashboardPath           string
		GlobalKibanaDashboardPath     string
		GlobalConfigPath              string
		GlobalCrossplanePath          string
		GlobalTemporalImageRepository string
		Region                        string

		CiSupportCfg *CiSupportSharedStack
		NLSharedCfg  *NLSharedStack
		Resources    resourceDef.ResourceConfig

		AlertWebhookURL *pulumi.StringOutput
		SlackWebhookURL *pulumi.StringOutput

		//GCP Specific variables
		Project string

		//AWS Specific variables
		AwsAccountID               string
		InstanceRoleName           string
		Vpc                        *ec2x.Vpc
		OidcID                     pulumi.StringOutput
		InstanceRole               *iam.Role
		Eks                        *eks.Cluster
		Route53IamRole             *iam.Role
		EnableAwsGatewayController bool
		VpcFlowLogsEnabled         bool
		Route53RoleName            string
	}

	TLSConfig struct {
		AcmeServer    string
		Domain        string
		Email         string
		Route53ZoneID string //Just for AWS
	}

	OauthConfig struct {
		Oauth2ValidateURL  string
		OidcIssuerURL      string
		Oauth2AuthURL      string
		Oauth2Provider     string
		Oauth2Scope        string
		Oauth2ClientID     string
		Oauth2GroupsClaim  string
		Oauth2TokenURL     string
		Oauth2ClientSecret pulumi.StringOutput // loaded at runtime from a secret
		Oauth2CookieSecret pulumi.StringOutput
	}
)

func storageClassByPlatform(platform string) pulumi.String {
	if platform == "gke" {
		return "standard"
	}
	//aws
	return "gp3-enc"

}

func nodeTolerationsByPlatformAndDistruptionType(platform string, disruptable bool) pulumi.Array {
	if platform == "gke" {
		if disruptable {
			return pulumi.Array{
				pulumi.Map{
					"effect": pulumi.String("NoSchedule"),
					"key":    pulumi.String("cloud.google.com/gke-spot"),
					"value":  pulumi.String("true"),
				},
			}
		}

		return pulumi.Array{}

	}
	//aws
	if disruptable {
		return pulumi.Array{
			pulumi.Map{
				"effect":   pulumi.String("NoSchedule"),
				"key":      pulumi.String("tfm/tolerates-spot"),
				"operator": pulumi.String("Exists"),
			},
		}
	}
	return pulumi.Array{
		pulumi.Map{
			"effect":   pulumi.String("NoSchedule"),
			"key":      pulumi.String("tfm/not-disruptable"),
			"operator": pulumi.String("Exists"),
		},
	}

}

func priorityClassByPlatformAndWorkloadType(workloadType string) pulumi.String {
	if workloadType == "daemonset" {
		return "system-node-critical"
	}
	return "system-cluster-critical"

}

func annotationsByPlatform(platform string) pulumi.Map {
	if platform == "gke" {
		return pulumi.Map{}
	}
	return pulumi.Map{
		"karpenter.sh/do-not-disrupt": pulumi.String("true"),
	}
}

func getNodeSelector(disruptable bool, arch string, os string, topologyZone string) pulumi.StringMap {
	nodeSelectorMap := pulumi.StringMap{}

	if disruptable {
		nodeSelectorMap["node-role"] = pulumi.String("autoscaled-ondemand")
	} else {
		nodeSelectorMap["node-role"] = pulumi.String("not-disruptable")
	}

	if arch != "" {
		nodeSelectorMap["kubernetes.io/arch"] = pulumi.String(arch)
	}
	if os != "" {
		nodeSelectorMap["kubernetes.io/os"] = pulumi.String(os)
	}

	if topologyZone != "" {
		nodeSelectorMap["topology.kubernetes.io/zone"] = pulumi.String(topologyZone)
	}

	return nodeSelectorMap
}

func CreatePdb(ctx *pulumi.Context, ns *corev1.Namespace, name string, appLabel string) (*policyv1.PodDisruptionBudget, error) {

	pdb, err := policyv1.NewPodDisruptionBudget(ctx, name, &policyv1.PodDisruptionBudgetArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name), Namespace: ns.Metadata.Name()},
		Spec: &policyv1.PodDisruptionBudgetSpecArgs{
			MinAvailable: pulumi.Int(1),
			Selector:     &metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{"app": pulumi.String(appLabel)}},
		},
	})
	if err != nil {
		return nil, err
	}
	return pdb, nil

}

func (s *Stack) deployCloudWatchExporter(ctx *pulumi.Context, ns *corev1.Namespace) error {
	policyJSON := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Action": [
					"tag:GetResources",
					"cloudwatch:GetMetricData",
					"cloudwatch:GetMetricStatistics",
					"cloudwatch:ListMetrics",
					"apigateway:GET",
					"aps:ListWorkspaces",
					"autoscaling:DescribeAutoScalingGroups",
					"dms:DescribeReplicationInstances",
					"dms:DescribeReplicationTasks",
					"ec2:DescribeTransitGatewayAttachments",
					"ec2:DescribeSpotFleetRequests",
					"shield:ListProtections",
					"storagegateway:ListGateways",
					"storagegateway:ListTagsForResource"
				],
				"Resource": "*"
			}
		]
	}`

	// IRSA
	saName := "cloudwatch-exporter"
	serviceAccount := awsiam.EKSServiceAccountArgs{
		Name:            pulumi.String(s.ClusterName),
		ServiceAccounts: pulumi.ToStringArray([]string{fmt.Sprintf("monitoring:%s", saName)}),
	}

	roleName := s.ClusterScopedResourceName("cloudwatch-metrics")
	eksRole, err := awsiam.NewEKSRole(ctx, roleName, &awsiam.EKSRoleArgs{
		Role:                   awsiam.RoleArgs{Name: pulumi.String(roleName)},
		ClusterServiceAccounts: awsiam.EKSServiceAccountArray([]awsiam.EKSServiceAccountInput{serviceAccount}),
	}, pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	policyName := s.ClusterScopedResourceName("cloudwatch-metrics")
	_, err = iam.NewRolePolicy(ctx, policyName,
		&iam.RolePolicyArgs{Name: pulumi.String(policyName), Role: eksRole.Name, Policy: pulumi.String(policyJSON)}, pulumi.DependsOn([]pulumi.Resource{eksRole}))
	if err != nil {
		return err
	}

	customValues := pulumi.Map{
		"serviceAccount": pulumi.Map{
			"name": pulumi.String(saName),
			"annotations": pulumi.Map{
				"eks.amazonaws.com/role-arn":               eksRole.Arn,
				"eks.amazonaws.com/sts-regional-endpoints": pulumi.String("true"),
			},
		},
		"aws": pulumi.Map{
			"role": eksRole.Name,
		},
		"extraArgs": pulumi.Map{
			"scraping-interval": pulumi.String("120"),
			"debug":             pulumi.String("false"), // crashes the process if debug is enabled (b/c of the p95 stats)
		},
		"resources": s.Resources.CloudWatchMetrics,
	}
	s.DependsOn = append(s.DependsOn, eksRole)

	_, err = s.DeployHelmRelease(ctx, ns, "cloudwatch-metrics", cloudWatchMetricsChartVers, "", "cloudwatch-metrics.yaml", customValues)
	if err != nil {
		return err
	}
	return err
}

func (s *Stack) DeployHelmRelease(ctx *pulumi.Context, ns *corev1.Namespace, name string, vers string, chartPath string, valuesYaml string, customValues pulumi.Map) (*helmv3.Release, error) {
	if chartPath == "" {
		chartPath = name
	}
	args := &helmv3.ReleaseArgs{
		WaitForJobs: pulumi.BoolPtr(true),
		ForceUpdate: pulumi.BoolPtr(true),
		Chart:       pulumi.Sprintf("./%s/%s", s.GlobalHelmChartPath, chartPath),
		Version:     pulumi.String(vers),
		MaxHistory:  pulumi.Int(revisionMaxHistory),
	}
	if s.Platform == "gke" {
		args.Name = pulumi.String(name)
	}

	if ns != nil {
		args.Namespace = ns.Metadata.Name()
		s.DependsOn = append(s.DependsOn, ns)
	} else {
		args.Namespace = pulumi.String("kube-system")
	}

	if valuesYaml != "" {
		args.ValueYamlFiles = pulumi.AssetOrArchiveArray{
			pulumi.NewFileAsset(fmt.Sprintf("./%s/%s", s.GlobalConfigPath, valuesYaml)),
		}
	}

	if len(customValues) > 0 {
		args.Values = customValues
	}

	timeouts := pulumi.Timeouts(&pulumi.CustomTimeouts{Create: "5m"})

	helmRelease, err := helmv3.NewRelease(ctx, name, args, pulumi.Provider(s.K8sProvider),
		timeouts, pulumi.DependsOn(s.DependsOn), pulumi.IgnoreChanges([]string{"checksum"}))
	if err != nil {
		return nil, err
	}

	return helmRelease, nil
}

func (s *Stack) CreateNamespace(ctx *pulumi.Context, nsName string) (*corev1.Namespace, error) {

	ns, err := corev1.NewNamespace(ctx, nsName, &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(nsName)},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{s.K8sProvider}))

	if err != nil {
		return nil, err
	}

	if s.Platform == "gke" {
		_, err = s.createResouceQuota(ctx, nsName)
		if err != nil {
			return nil, err
		}
	}

	return ns, nil
}

// GKE needs us to set a resource quota per namespace to use PriorityClasses
func (s *Stack) createResouceQuota(ctx *pulumi.Context, nsName string) (*corev1.ResourceQuota, error) {

	resourceQuotaName := fmt.Sprintf("%s-critical-pods", nsName)
	return corev1.NewResourceQuota(ctx, nsName, &corev1.ResourceQuotaArgs{
		ApiVersion: pulumi.String("v1"),
		Kind:       pulumi.String("ResourceQuota"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(resourceQuotaName),
			Namespace: pulumi.String(nsName),
		},
		Spec: &corev1.ResourceQuotaSpecArgs{
			Hard: pulumi.StringMap{
				"pods": pulumi.String("1G"),
			},
			ScopeSelector: &corev1.ScopeSelectorArgs{
				MatchExpressions: &corev1.ScopedResourceSelectorRequirementArray{
					&corev1.ScopedResourceSelectorRequirementArgs{
						Operator:  pulumi.String("In"),
						ScopeName: pulumi.String("PriorityClass"),
						Values: pulumi.StringArray{
							pulumi.String("system-node-critical"),
							pulumi.String("system-cluster-critical"),
						},
					},
				},
			},
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{s.K8sProvider}))
}

func (s *Stack) createPriorityClass(ctx *pulumi.Context, name string, value int) (*schedulingv1.PriorityClass, error) {

	return schedulingv1.NewPriorityClass(ctx, name, &schedulingv1.PriorityClassArgs{
		ApiVersion:    pulumi.String("v1"),
		Kind:          pulumi.String("PriorityClass"),
		Metadata:      &metav1.ObjectMetaArgs{Name: pulumi.String(name)},
		GlobalDefault: pulumi.Bool(false),
		Description:   pulumi.String("Priority class used for nativelink claims critical deployments."),
		Value:         pulumi.Int(value),
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{s.K8sProvider}))
}

func (s *Stack) CreateOAuth2ProxyConfig(ctx *pulumi.Context, ns *corev1.Namespace, compName string, redirectURI string, upstreamPort int, protocol string) (*corev1.ConfigMap, error) {
	opSecret, err := s.CreateOAuth2ProxySecret(ctx, ns, compName)
	if err != nil {
		return nil, err
	}

	oauth2EnvMap := pulumi.StringMap{
		"OAUTH2_PROXY_PASS_USER_HEADERS":                      pulumi.String("true"),
		"OAUTH2_PROXY_OIDC_GROUPS_CLAIM":                      pulumi.String(s.OauthConfig.Oauth2GroupsClaim),
		"OAUTH2_PROXY_EMAIL_DOMAINS":                          pulumi.String("*"),
		"OAUTH2_PROXY_AUTH_LOGGING":                           pulumi.String("false"),
		"OAUTH2_PROXY_COOKIE_HTTPONLY":                        pulumi.String("true"),
		"OAUTH2_PROXY_COOKIE_SAMESITE":                        pulumi.String("lax"),
		"OAUTH2_PROXY_COOKIE_SECURE":                          pulumi.String("false"),
		"OAUTH2_PROXY_HTTP_ADDRESS":                           pulumi.String("0.0.0.0:8888"),
		"OAUTH2_PROXY_INSECURE_OIDC_ALLOW_UNVERIFIED_EMAIL":   pulumi.String("true"),
		"OAUTH2_PROXY_INSECURE_OIDC_SKIP_ISSUER_VERIFICATION": pulumi.String("true"),
		"OAUTH2_PROXY_PASS_ACCESS_TOKEN":                      pulumi.String("false"),
		"OAUTH2_PROXY_PROVIDER":                               pulumi.String(s.OauthConfig.Oauth2Provider),
		"OAUTH2_PROXY_REDIRECT_URL":                           pulumi.String(redirectURI),
		"OAUTH2_PROXY_REQUEST_LOGGING":                        pulumi.String("false"),
		"OAUTH2_PROXY_SCOPE":                                  pulumi.String(s.OauthConfig.Oauth2Scope),
		"OAUTH2_PROXY_SESSION_COOKIE_MINIMAL":                 pulumi.String("true"),
		"OAUTH2_PROXY_SHOW_DEBUG_ON_ERROR":                    pulumi.String("true"),
		"OAUTH2_PROXY_SILENCE_PING_LOGGING":                   pulumi.String("true"),
		"OAUTH2_PROXY_SKIP_AUTH_PREFLIGHT":                    pulumi.String("true"),
		"OAUTH2_PROXY_SKIP_AUTH_ROUTES":                       pulumi.String("GET=^/api-auth/v1/verify-api-key/.*$,GET=^/favicon.ico$,GET=^/i18n/locales/.*$,GET=^/_app/version.json$,GET=^/_app/immutable/.*$,GET=^/events-ws$,GET=^/api/v1/accounts/.*/logs$,GET=^/swagger/.*$,GET=^/api/v1/.*/logs$,GET=^/ping$,GET=^/.*.js$,GET=^/.*.css$,GET=^/.*.png$,GET=^/metrics$,GET=^/ready$,GET=^/live$,GET=^/static/*$,GET=^/favicon.ico$,GET=^/files/.*$"),
		"OAUTH2_PROXY_SKIP_JWT_BEARER_TOKENS":                 pulumi.String("true"), // this allows us to pass a JWT to login, needed for API access
		"OAUTH2_PROXY_SKIP_OIDC_DISCOVERY":                    pulumi.String("false"),
		"OAUTH2_PROXY_SKIP_PROVIDER_BUTTON":                   pulumi.String("true"),
		"OAUTH2_PROXY_SSL_INSECURE_SKIP_VERIFY":               pulumi.String("true"),
		"OAUTH2_PROXY_STANDARD_LOGGING":                       pulumi.String("false"),
		"OAUTH2_PROXY_UPSTREAMS":                              pulumi.Sprintf("http://127.0.0.1:%d", upstreamPort),
		"OAUTH2_PROXY_OIDC_ISSUER_URL":                        pulumi.String(s.OauthConfig.OidcIssuerURL),
	}

	if protocol == "https" {
		oauth2EnvMap["OAUTH2_PROXY_UPSTREAMS"] = pulumi.Sprintf("%s://localhost:%d", protocol, upstreamPort)
		oauth2EnvMap["OAUTH2_PROXY_SSL_UPSTREAM_INSECURE_SKIP_VERIFY"] = pulumi.String("true")
	} else {
		oauth2EnvMap["OAUTH2_PROXY_UPSTREAMS"] = pulumi.Sprintf("%s://127.0.0.1:%d", protocol, upstreamPort)

	}

	if s.OauthConfig.Oauth2ValidateURL != "" {
		oauth2EnvMap["OAUTH2_PROXY_VALIDATE_URL"] = pulumi.String(s.OauthConfig.Oauth2ValidateURL)
	}

	configMapName := fmt.Sprintf("oauth2-proxy-config-%s", compName)

	return corev1.NewConfigMap(ctx, configMapName, &corev1.ConfigMapArgs{
		ApiVersion: pulumi.String("v1"),
		Data:       oauth2EnvMap,
		Kind:       pulumi.String("ConfigMap"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String(configMapName), Namespace: ns.Metadata.Name()},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{opSecret}))
}

func (s *Stack) CreateOAuth2ProxySecret(ctx *pulumi.Context, ns *corev1.Namespace, compName string) (*corev1.Secret, error) {
	return corev1.NewSecret(ctx, fmt.Sprintf("oauth2-proxy-client-%s", compName), &corev1.SecretArgs{
		ApiVersion: pulumi.String("v1"),
		Kind:       pulumi.String("Secret"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("oauth2-proxy-client"), Namespace: ns.Metadata.Name()},
		StringData: pulumi.StringMap{
			"OAUTH2_PROXY_CLIENT_ID":     pulumi.String(s.OauthConfig.Oauth2ClientID),
			"OAUTH2_PROXY_CLIENT_SECRET": s.OauthConfig.Oauth2ClientSecret,
			"OAUTH2_PROXY_COOKIE_SECRET": s.OauthConfig.Oauth2CookieSecret,
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{ns}))
}
func (s *Stack) CreateOAuth2ProxyContainerDef(configMap pulumi.StringPtrOutput, image string) pulumi.Map {

	return pulumi.Map{
		"name":            pulumi.String("oauth2-proxy"),
		"image":           pulumi.String(image),
		"imagePullPolicy": pulumi.String("IfNotPresent"),
		"ports": pulumi.MapArray{
			pulumi.Map{
				"containerPort": pulumi.Int(8888),
				"name":          pulumi.String("proxy"),
			},
		},
		"envFrom": pulumi.MapArray{
			pulumi.Map{
				"configMapRef": pulumi.Map{
					"name": configMap,
				},
			},
			pulumi.Map{
				"secretRef": pulumi.Map{
					"name": pulumi.String("oauth2-proxy-client"),
				},
			},
		},
		"livenessProbe": pulumi.Map{
			"failureThreshold": pulumi.Int(3),
			"httpGet": pulumi.Map{
				"path":   pulumi.String("/ping"),
				"port":   pulumi.Int(8888),
				"scheme": pulumi.String("HTTP"),
			},
			"initialDelaySeconds": pulumi.Int(5),
			"periodSeconds":       pulumi.Int(5),
			"successThreshold":    pulumi.Int(1),
			"timeoutSeconds":      pulumi.Int(1),
		},
		"readinessProbe": pulumi.Map{
			"failureThreshold": pulumi.Int(3),
			"httpGet": pulumi.Map{
				"path":   pulumi.String("/ping"),
				"port":   pulumi.Int(8888),
				"scheme": pulumi.String("HTTP"),
			},
			"initialDelaySeconds": pulumi.Int(5),
			"periodSeconds":       pulumi.Int(5),
			"successThreshold":    pulumi.Int(1),
			"timeoutSeconds":      pulumi.Int(1),
		},
		"args": pulumi.StringArray{
			pulumi.String("--whitelist-domain=.amazoncognito.com"),
		},
	}
}

func (s *Stack) CreateIngress(ctx *pulumi.Context, subdomain string, namespace string, backendSvcName string, backendSvcPort int, additionalAnnotations map[string]string) (string, error) {
	backendSvc := &networkingv1.HTTPIngressPathArgs{
		Backend: &networkingv1.IngressBackendArgs{
			Service: &networkingv1.IngressServiceBackendArgs{
				Name: pulumi.String(backendSvcName),
				Port: &networkingv1.ServiceBackendPortArgs{Number: pulumi.Int(backendSvcPort)},
			},
		},
		Path:     pulumi.String("/"),
		PathType: pulumi.String("Prefix"),
	}

	annotations := pulumi.StringMap{
		"cert-manager.io/cluster-issuer":             pulumi.String(GlobalClusterIssuer),
		"nginx.ingress.kubernetes.io/rewrite-target": pulumi.String("/"),
	}
	for k, v := range additionalAnnotations {
		annotations[k] = pulumi.String(v)
	}

	ingName := fmt.Sprintf("%s-ing-%s", subdomain, ctx.Stack())
	host := fmt.Sprintf("%s.%s", subdomain, s.TLSCfg.Domain)
	secretName := fmt.Sprintf("%s-tls", subdomain)
	_, err := networkingv1.NewIngress(ctx, ingName, &networkingv1.IngressArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Annotations: annotations,
			Name:        pulumi.String(ingName),
			Namespace:   pulumi.String(namespace),
		},
		Spec: &networkingv1.IngressSpecArgs{
			IngressClassName: pulumi.String("nginx"),
			Tls: networkingv1.IngressTLSArray{networkingv1.IngressTLSArgs{
				Hosts:      pulumi.StringArray{pulumi.String(host)},
				SecretName: pulumi.String(secretName),
			}},
			Rules: networkingv1.IngressRuleArray{
				&networkingv1.IngressRuleArgs{
					Host: pulumi.String(host),
					Http: &networkingv1.HTTPIngressRuleValueArgs{
						Paths: networkingv1.HTTPIngressPathArray{backendSvc},
					},
				},
			},
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return "", err
	}

	hostURL := fmt.Sprintf("https://%s", host)
	s.IngressHosts = append(s.IngressHosts, hostURL)

	return hostURL, nil
}

// Generate a cluster-scoped account-level resource name
func (s *Stack) ClusterScopedResourceName(suffix string) string {
	return fmt.Sprintf("%s-%s", s.ClusterName, suffix)
}

func (s *Stack) CreateOAuth2ProxyContainerArgs(configMap pulumi.StringPtrOutput, image string) *corev1.ContainerArgs {
	return &corev1.ContainerArgs{
		Name:  pulumi.String("oauth2-proxy"),
		Image: pulumi.String(image),
		Args:  pulumi.ToStringArray([]string{"--whitelist-domain=.amazoncognito.com"}),
		EnvFrom: &corev1.EnvFromSourceArray{
			corev1.EnvFromSourceArgs{
				ConfigMapRef: corev1.ConfigMapEnvSourceArgs{
					Name: configMap,
				},
			},
			corev1.EnvFromSourceArgs{
				SecretRef: corev1.SecretEnvSourceArgs{
					Name: pulumi.String("oauth2-proxy-client"),
				},
			},
		},
		Ports: corev1.ContainerPortArray{
			&corev1.ContainerPortArgs{
				Name:          pulumi.String("proxy"),
				Protocol:      pulumi.String("TCP"),
				ContainerPort: pulumi.Int(8888),
			},
		},
		Resources: s.Resources.OauthProxy,
		LivenessProbe: &corev1.ProbeArgs{
			HttpGet: &corev1.HTTPGetActionArgs{
				Path: pulumi.String("/ping"),
				Port: pulumi.Int(8888),
			},
			FailureThreshold:    pulumi.Int(3),
			InitialDelaySeconds: pulumi.Int(5),
			PeriodSeconds:       pulumi.Int(5),
		},
		ReadinessProbe: &corev1.ProbeArgs{
			HttpGet: &corev1.HTTPGetActionArgs{
				Path: pulumi.String("/ping"),
				Port: pulumi.Int(8888),
			},
			FailureThreshold:    pulumi.Int(3),
			InitialDelaySeconds: pulumi.Int(5),
			PeriodSeconds:       pulumi.Int(5),
		},
	}
}

func (s *Stack) DeployRunbooks(ctx *pulumi.Context, ssAPICfg *SelfServiceAPIConfig) error {
	ns, err := s.CreateNamespace(ctx, "runbooks")
	if err != nil {
		return err
	}
	redirectURI := fmt.Sprintf("https://runbooks.%s/oauth2/callback", s.TLSCfg.Domain)
	opConfigMap, err := s.CreateOAuth2ProxyConfig(ctx, ns, "runbooks", redirectURI, 80, "http")
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, opConfigMap)

	oauth2ProxyContainer := s.CreateOAuth2ProxyContainerArgs(opConfigMap.Metadata.Name(), "quay.io/oauth2-proxy/oauth2-proxy:v7.6.0")

	runbooksContainer := &corev1.ContainerArgs{
		Name:            pulumi.String("runbooks"),
		Image:           pulumi.String("299166832260.dkr.ecr.us-west-2.amazonaws.com/runbooks-site:main"),
		ImagePullPolicy: pulumi.String("Always"),
		Ports: corev1.ContainerPortArray{
			&corev1.ContainerPortArgs{
				Name:          pulumi.String("hugohttp"),
				Protocol:      pulumi.String("TCP"),
				ContainerPort: pulumi.Int(80),
			},
		},
		Resources: s.Resources.Runbooks,
	}

	podTemplate := &corev1.PodTemplateSpecArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Labels: pulumi.StringMap{"app": pulumi.String("runbooks")},
		},
		Spec: &corev1.PodSpecArgs{
			NodeSelector: pulumi.StringMap{
				"kubernetes.io/arch": pulumi.String("amd64"),
				"kubernetes.io/os":   pulumi.String("linux"),
				"node-role":          pulumi.String("autoscaled-ondemand")},
			Containers: corev1.ContainerArray{oauth2ProxyContainer, runbooksContainer},
			Tolerations: corev1.TolerationArray{
				corev1.TolerationArgs{
					Key:      pulumi.String("tfm/tolerates-spot"),
					Operator: pulumi.String("Exists"),
					Effect:   pulumi.String("NoSchedule"),
				},
			},
		},
	}

	replicas := 1
	runbooksDeploy, err := appsv1.NewDeployment(ctx, "runbooks", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("runbooks"), Namespace: ns.Metadata.Name()},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(replicas),
			Selector: metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{"app": pulumi.String("runbooks")}},
			Template: podTemplate,
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, runbooksDeploy)

	runbooksSvc, err := corev1.NewService(ctx, "runbooks", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("runbooks"), Namespace: ns.Metadata.Name()},
		Spec: &corev1.ServiceSpecArgs{
			Selector: pulumi.StringMap{"app": pulumi.String("runbooks")},
			Ports: &corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("proxy"),
					Port:       pulumi.Int(80),
					Protocol:   pulumi.String("TCP"),
					TargetPort: pulumi.String("proxy"),
				},
			},
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, runbooksSvc)

	// expose Runbooks over https
	runbooksHost, err := s.CreateIngress(ctx, "runbooks", "runbooks", "runbooks", 80, nil)
	if err != nil {
		return err
	}
	fmt.Printf("Deployed ingress for host: %s\n", runbooksHost)

	//Create extra entities for renewing runbook pods

	runbookRenewSvcAccount, err := corev1.NewServiceAccount(ctx, "runbooks-renew-pods", &corev1.ServiceAccountArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("runbooks-renew-pods"), Namespace: ns.Metadata.Name()},
	})
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, runbookRenewSvcAccount)

	runbookRenewRole, err := rbacv1.NewRole(ctx, "runbooks-renew-pods", &rbacv1.RoleArgs{
		ApiVersion: pulumi.String("rbac.authorization.k8s.io"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("runbooks-renew-pods"), Namespace: ns.Metadata.Name()},
		Rules: rbacv1.PolicyRuleArray{
			&rbacv1.PolicyRuleArgs{
				Verbs: pulumi.StringArray{
					pulumi.String("get"),
					pulumi.String("patch"),
					pulumi.String("list"),
					pulumi.String("watch"),
					pulumi.String("delete"),
				},
				Resources: pulumi.StringArray{
					pulumi.String("pods"),
				},
				ApiGroups: pulumi.StringArray{
					pulumi.String(""),
				},
			},
		},
	},
	)
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, runbookRenewRole)

	runbookRenewRoleBinding, err := rbacv1.NewRoleBinding(ctx, "runbooks-renew-pods", &rbacv1.RoleBindingArgs{
		ApiVersion: pulumi.String("rbac.authorization.k8s.io"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("runbooks-renew-pods"), Namespace: ns.Metadata.Name()},
		Subjects: &rbacv1.SubjectArray{
			&rbacv1.SubjectArgs{
				Kind: pulumi.String("ServiceAccount"),
				Name: pulumi.String("runbooks-renew-pods"),
			},
		},
		RoleRef: &rbacv1.RoleRefArgs{
			ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
			Kind:     pulumi.String("Role"),
			Name:     pulumi.String("runbooks-renew-pods"),
		},
	})
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, runbookRenewRoleBinding)

	runbooksRenewCronjob, err := batchv1.NewCronJob(ctx, "runbooks-renew-pods", &batchv1.CronJobArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("runbooks-renew-pods"), Namespace: ns.Metadata.Name()},
		Spec: &batchv1.CronJobSpecArgs{
			ConcurrencyPolicy:          pulumi.String("Forbid"),
			Schedule:                   pulumi.String("*/30 * * * *"),
			SuccessfulJobsHistoryLimit: pulumi.Int(1),
			JobTemplate: &batchv1.JobTemplateSpecArgs{
				Spec: &batchv1.JobSpecArgs{
					ActiveDeadlineSeconds: pulumi.Int(300),
					BackoffLimit:          pulumi.Int(3),
					Template: &corev1.PodTemplateSpecArgs{
						Spec: &corev1.PodSpecArgs{
							ServiceAccountName: pulumi.String("runbooks-renew-pods"),
							Containers: corev1.ContainerArray{
								&corev1.ContainerArgs{
									Name:  pulumi.String("kubectl"),
									Image: pulumi.String("bitnami/kubectl:latest"),
									Command: pulumi.StringArray{
										pulumi.String("kubectl"),
										pulumi.String("delete"),
										pulumi.String("pods"),
										pulumi.String("-l"),
										pulumi.String("app=runbooks"),
									},
								},
							},
							RestartPolicy: pulumi.String("OnFailure"),
						},
					},
				},
			},
		},
	})

	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, runbooksRenewCronjob)

	return nil

}

func BuildIDTerms(ids []string) string {
	terms := "["
	for i, id := range ids {
		if i > 0 {
			terms += ","
		}
		terms += fmt.Sprintf("{\"id\":\"%s\"}", id)
	}
	terms += "]"
	return terms
}

func buildRoleAttributePath(groupsClaim string, group string, roleName string) string {
	return fmt.Sprintf("contains(\"%s\"[*], '%s') && '%s'", groupsClaim, group, roleName)
}

func JSONToMap(jsonStr string) (map[string]interface{}, error) {
	parsedMap := make(map[string]interface{})
	if jsonErr := json.Unmarshal([]byte(jsonStr), &parsedMap); jsonErr != nil {
		return nil, jsonErr
	}
	return parsedMap, nil
}

func ReadFile(path string) string {
	fileBytes, ioErr := os.ReadFile(path)
	if ioErr != nil {
		panic(fmt.Sprintf("Failed to read file '%s' due to: %v", path, ioErr))
	}
	return string(fileBytes)
}

func CalcChecksum(path string) string {
	contents, err := os.Open(path)
	if err != nil {
		panic(fmt.Sprintf("Failed to read file '%s' due to: %v", path, err))
	}
	defer func(contents *os.File) {
		_ = contents.Close()
	}(contents)

	// Using md5 because sha functions exceed the number of characters required by a kube label
	hasher := md5.New()
	if _, err := io.Copy(hasher, contents); err != nil {
		panic(fmt.Sprintf("Hasher failed on  '%s' due to: %v", path, err))
	}
	value := hex.EncodeToString(hasher.Sum(nil))
	return value

}
