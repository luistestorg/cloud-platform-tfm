package shared

import (
	"fmt"
	"os"
	"strings"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	slackChannelProd = "cloud-platform-notifications"
	slackChannelDev  = "dev-cluster-notifications"

	slackAlertTitle = "{{ range .Alerts }}{{ .Annotations.summary }}\n{{ end }}"
	slackAlertText  = "{{ range .Alerts }}{{ .Annotations.description }}\n{{ end }}"

	PromChartVers              = "56.21.0-5"
	ThanosChartVers            = "15.12.2"
	ElasticSearchChartVers     = "21.6.3-3"
	FluentbitChartVers         = "0.48.9"
	EventExporterChartVers     = "3.4.4"
	cloudWatchMetricsChartVers = "0.26.0"
	OtelKubeStackChartVers     = "0.6.0"
)

type (
	//MonSharedStack defines the Global stack for the monitoring and logging components
	MonSharedStack struct {
		ClusterIssuer string
		RdsZone       string

		BootstrapAdminPassword    pulumi.StringOutput
		GrafanaAdminPassword      pulumi.StringOutput
		PrometheusStorageClass    string
		GrafanaStorageClass       string
		PrometheusStorage         string
		GrafanaStorage            string
		PrometheusStorageSnapshot string
		GrafanaStorageSnapshot    string
		PrometheusMemoryRequests  string
		PrometheusCPURequests     string
		PrometheusReplicas        int

		EnableElasticSearch       bool
		ElasticSearchPassword     pulumi.StringOutput
		KibanaPassword            pulumi.StringOutput
		ElasticSearchStorageSize  string
		ElasticSearchStorageClass string
		KibanaStorageClass        string
		KibanaStorageSize         string
		EnableLogStack            bool
		EnableOTEL                bool
	}
)

func (monSharedStack *MonSharedStack) createThanosSpec(ctx *pulumi.Context, ns *corev1.Namespace, s *Stack) (pulumi.Map, error) {
	triperConfig := `"response_header_timeout": "5m"
"max_idle_conns_per_host": 100
"max_conns_per_host": 100`
	cacheConfig := `type: IN-MEMORY
config:
  max_size: "4096GB"
  validity: "60s"`

	_, err := corev1.NewConfigMap(ctx, "queryfrontend-config", &corev1.ConfigMapArgs{
		ApiVersion: pulumi.String("v1"),
		Data: pulumi.StringMap{
			"tripper-config.yaml": pulumi.String(triperConfig),
			"cache-config.yaml":   pulumi.String(cacheConfig),
		},
		Kind:     pulumi.String("ConfigMap"),
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("frontend-config-files"), Namespace: ns.Metadata.Name()},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return nil, err
	}

	thanosCustomValues := pulumi.Map{
		"nameOverride":     pulumi.String("thanos"),
		"fullnameOverride": pulumi.String("thanos"),
		"query": pulumi.Map{
			"resourcesPreset": s.Resources.ThanosQueryResourcePreset,
			"nodeSelector":    getNodeSelector(false, "", "", ""),
			"tolerations":     nodeTolerationsByPlatformAndDistruptionType(s.Platform, false),
			"dnsDiscovery": pulumi.Map{
				"sidecarsService":   pulumi.String("mon-thanos-discovery"),
				"sidecarsNamespace": pulumi.String("monitoring"),
			},
			"priorityClassName": priorityClassByPlatformAndWorkloadType("deployment"),
		},
		"queryFrontend": pulumi.Map{
			"resourcesPreset":   s.Resources.ThanosQueryFrontendResourcePreset,
			"nodeSelector":      getNodeSelector(false, "", "", ""),
			"tolerations":       nodeTolerationsByPlatformAndDistruptionType(s.Platform, false),
			"priorityClassName": priorityClassByPlatformAndWorkloadType("deployment"),
			"extraFlags": pulumi.StringArray{
				pulumi.String("--query-frontend.compress-responses"),
				pulumi.String("--labels.max-query-parallelism=60"),
				pulumi.String("--query-range.max-query-parallelism=60"),
				pulumi.String("--query-range.split-interval=1h"),
				pulumi.String("--labels.split-interval=1h"),
				pulumi.String("--query-range.max-retries-per-request=5"),
				pulumi.String("--labels.max-retries-per-request=5"),
				pulumi.String("--query-frontend.log-queries-longer-than=30s"),
				pulumi.String("--query-frontend.downstream-tripper-config-file=/frontend-config-files/tripper-config.yaml"),
				pulumi.String("--labels.response-cache-config-file=/frontend-config-files/cache-config.yaml"),
			},
			"extraVolumeMounts": pulumi.MapArray{
				pulumi.Map{
					"name":      pulumi.String("frontend-config-files"),
					"mountPath": pulumi.String("/frontend-config-files"),
					"readOnly":  pulumi.Bool(true),
				},
			},
			"extraVolumes": pulumi.MapArray{
				pulumi.Map{
					"name": pulumi.String("frontend-config-files"),
					"configMap": pulumi.Map{
						"name": pulumi.String("frontend-config-files"),
						"items": pulumi.MapArray{
							pulumi.Map{
								"key":  pulumi.String("tripper-config.yaml"),
								"path": pulumi.String("tripper-config.yaml"),
							},
							pulumi.Map{
								"key":  pulumi.String("cache-config.yaml"),
								"path": pulumi.String("cache-config.yaml"),
							},
						},
					},
				},
			},
		},
		"metrics": pulumi.Map{
			"enabled": pulumi.Bool(true),
			"serviceMonitor": pulumi.Map{
				"enabled": pulumi.Bool(true),
			},
		},
	}

	return thanosCustomValues, nil
}

func (monSharedStack *MonSharedStack) DeployThanos(ctx *pulumi.Context, ns *corev1.Namespace, s *Stack) (*helmv3.Release, error) {

	thanosValuesMap, err := monSharedStack.createThanosSpec(ctx, ns, s)
	if err != nil {
		return nil, err
	}

	thanosRelease, err := s.DeployHelmRelease(ctx, ns, "thanos", ThanosChartVers, "", "", thanosValuesMap)
	if err != nil {
		return nil, err
	}

	return thanosRelease, nil

}

func (monSharedStack *MonSharedStack) DeployMonitoringComponents(ctx *pulumi.Context, s *Stack) (*helmv3.Release, error) {

	// create the "monitoring" namespace holding all monitoring related components, including:
	ns, err := s.CreateNamespace(ctx, "monitoring")
	if err != nil {
		return nil, err
	}

	// Array for receivers
	alertmanagerReceivers := pulumi.Array{
		pulumi.Map{"name": pulumi.String("null")},
	}

	var slackChannel string

	if s.Env == "prod" {
		slackChannel = slackChannelProd
	} else {
		slackChannel = slackChannelDev
	}

	if s.SlackWebhookURL != nil {
		alertmanagerReceivers = append(alertmanagerReceivers, pulumi.Map{
			"name": pulumi.String("slack-notifications"),
			"slack_configs": pulumi.Array{
				pulumi.Map{"api_url": *s.SlackWebhookURL},
				pulumi.Map{"channel": pulumi.String(slackChannel)},
				pulumi.Map{"icon_url": pulumi.String("https://avatars3.githubusercontent.com/u/3380462")},
				pulumi.Map{"send_resolved": pulumi.Bool(true)},
				pulumi.Map{"title": pulumi.String(slackAlertTitle)},
				pulumi.Map{"text": pulumi.String(slackAlertText)},
			},
		})
	}

	alertmanagerRoutes := pulumi.Array{}
	if s.Env == "prod" {
		if s.AlertWebhookURL != nil {
			alertmanagerReceivers = append(alertmanagerReceivers, pulumi.Map{
				"name":            pulumi.String("betteruptime"),
				"webhook_configs": pulumi.Array{pulumi.Map{"url": *s.AlertWebhookURL}},
			})
		}

		alertmanagerRoutes = append(alertmanagerRoutes, pulumi.Map{
			"receiver": pulumi.String("null"),
			"matchers": pulumi.StringArray{
				pulumi.String("severity!~\"critical|error\""),
				pulumi.String("alertname=~\"Watchdog|KubeControllerManagerDown|KubeProxyDown|KubeSchedulerDown|KubeCPUQuotaOvercommit|KubeMemoryQuotaOvercommit|KubeMemoryOvercommit\""),
			},
		})

		alertmanagerRoutes = append(alertmanagerRoutes, pulumi.Map{
			"receiver": pulumi.String("slack-notifications"),
			"matchers": pulumi.StringArray{
				pulumi.String("severity=~\"warning\""),
				pulumi.String("alertname!~\"KubeVersionMismatch|AlertmanagerFailedToSendAlerts|Watchdog|KubeControllerManagerDown|KubeProxyDown|KubeSchedulerDown|KubeCPUQuotaOvercommit|KubeMemoryQuotaOvercommit|KubeMemoryOvercommit\""),
			},
		})

		alertmanagerRoutes = append(alertmanagerRoutes, pulumi.Map{
			"receiver": pulumi.String("betteruptime"),
			"matchers": pulumi.StringArray{
				pulumi.String("severity=~\"critical|error\""),
				pulumi.String("alertname!~\"Watchdog|KubeControllerManagerDown|KubeProxyDown|KubeSchedulerDown|KubeCPUQuotaOvercommit|KubeMemoryQuotaOvercommit|KubeMemoryOvercommit|WorkerImagePullBackOff\""),
			},
		})
	} else {
		alertmanagerRoutes = append(alertmanagerRoutes, pulumi.Map{
			"receiver": pulumi.String("slack-notifications"),
			"matchers": pulumi.StringArray{
				pulumi.String("severity=~\"critical|error|warning\""),
				pulumi.String("alertname!~\"Watchdog|KubeControllerManagerDown|KubeProxyDown|KubeSchedulerDown|KubeCPUQuotaOvercommit|KubeMemoryQuotaOvercommit|KubeMemoryOvercommit\""),
			},
		})
	}

	specMap := pulumi.Map{
		"storageClassName": pulumi.String(monSharedStack.PrometheusStorageClass),
		"accessModes":      pulumi.ToStringArray([]string{"ReadWriteOnce"}),
		"resources": pulumi.Map{
			"requests": pulumi.Map{
				"storage": pulumi.String(monSharedStack.PrometheusStorage),
			},
		},
	}
	if monSharedStack.PrometheusStorageSnapshot != "" {
		specMap["dataSource"] = pulumi.Map{
			"apiGroup": pulumi.String("snapshot.storage.k8s.io"),
			"kind":     pulumi.String("VolumeSnapshot"),
			"name":     pulumi.String(monSharedStack.PrometheusStorageSnapshot),
		}
	}

	// create the PVC here as grafana since grafana uses a deployment instead of a statefulset
	// This causes issues when a helm upgrade is performed
	grafanaPvcSpec := &corev1.PersistentVolumeClaimSpecArgs{
		AccessModes: pulumi.StringArray{
			pulumi.String("ReadWriteOnce"),
		},
		Resources: &corev1.VolumeResourceRequirementsArgs{
			Requests: pulumi.StringMap{
				"storage": pulumi.String(monSharedStack.GrafanaStorage),
			},
		},
		StorageClassName: pulumi.String(monSharedStack.GrafanaStorageClass),
	}
	if monSharedStack.GrafanaStorageSnapshot != "" {
		grafanaPvcSpec.DataSource = &corev1.TypedLocalObjectReferenceArgs{
			Name:     pulumi.String(monSharedStack.GrafanaStorageSnapshot),
			Kind:     pulumi.String("VolumeSnapshot"),
			ApiGroup: pulumi.String("snapshot.storage.k8s.io"),
		}
	}

	grafanaPvc, err := corev1.NewPersistentVolumeClaim(ctx, "grafana-pvc", &corev1.PersistentVolumeClaimArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("grafana-pvc"), Namespace: ns.Metadata.Name()},
		Spec:     grafanaPvcSpec,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return nil, err
	}
	s.DependsOn = append(s.DependsOn, grafanaPvc)

	grafanaURL := fmt.Sprintf("https://grafana.%s", s.TLSCfg.Domain)
	promCustomValues := pulumi.Map{
		"prometheus": pulumi.Map{
			"annotations": annotationsByPlatform(s.Platform),
			"prometheusSpec": pulumi.Map{
				"replicas": pulumi.Int(monSharedStack.PrometheusReplicas),
				"serviceMonitorSelectorNilUsesHelmValues": pulumi.Bool(false),
				"podMonitorSelectorNilUsesHelmValues":     pulumi.Bool(false),
				"enableFeatures":                          pulumi.StringArray{pulumi.String("memory-snapshot-on-shutdown")},
				"priorityClassName":                       priorityClassByPlatformAndWorkloadType("statefulset"),
				"podMetadata": pulumi.Map{
					"annotations": annotationsByPlatform(s.Platform),
				},
				"nodeSelector": getNodeSelector(false, "", "", monSharedStack.RdsZone),
				"thanos": pulumi.Map{
					"resources": s.Resources.PromThanos,
				},
				"tolerations": nodeTolerationsByPlatformAndDistruptionType(s.Platform, false),
				"storageSpec": pulumi.Map{
					"volumeClaimTemplate": pulumi.Map{"spec": specMap},
				},
				"containers": pulumi.Array{
					pulumi.Map{
						"name": pulumi.String("prometheus"),
						"startupProbe": pulumi.Map{
							"initialDelaySeconds": pulumi.Int(60),
							"failureThreshold":    pulumi.Int(90),
							"periodSeconds":       pulumi.Int(10),
							"timeoutSeconds":      pulumi.Int(10),
						},
					},
				},
				"resources": s.Resources.Prometheus,
				"externalLabels": pulumi.Map{
					"cluster": pulumi.String(s.ClusterName),
				},
			},
		},
		"alertmanager": pulumi.Map{
			"annotations": annotationsByPlatform(s.Platform),
			"alertmanagerSpec": pulumi.Map{
				"nodeSelector": getNodeSelector(false, "", "", ""),
				"tolerations":  nodeTolerationsByPlatformAndDistruptionType(s.Platform, false),
			},
			"config": pulumi.Map{
				"receivers": alertmanagerReceivers,
				"route": pulumi.Map{
					"routes": alertmanagerRoutes,
				},
			},
		},
		"prometheus-operator": pulumi.Map{
			"resources": s.Resources.PromOperator,
		},
		"grafana": pulumi.Map{
			"replicas":       pulumi.Int(1),
			"podAnnotations": annotationsByPlatform(s.Platform),
			"nodeSelector":   getNodeSelector(false, "", "", ""),
			"tolerations":    nodeTolerationsByPlatformAndDistruptionType(s.Platform, false),

			"resources": s.Resources.Grafana,
			"sidecar": pulumi.Map{
				"resources": s.Resources.GrafanaSidecars,
				"dashboards": pulumi.Map{
					"ignoreAlreadyProcessed": pulumi.Bool(true),
					"skipReload":             pulumi.Bool(true),
				},
			},
			"persistence": pulumi.Map{
				"type":          pulumi.String("pvc"),
				"enabled":       pulumi.Bool(true),
				"existingClaim": grafanaPvc.Metadata.Name(),
			},
			"grafana.ini": pulumi.Map{
				"analytics": pulumi.Map{
					"check_for_updates": pulumi.Bool(false),
					"reporting_enabled": pulumi.Bool(false),
				},
				"alerting": pulumi.Map{
					"enabled": pulumi.Bool(false),
				},
				"unified_alerting": pulumi.Map{
					"enabled": pulumi.Bool(false),
				},
				"log": pulumi.Map{
					"level": pulumi.String("warn"),
				},
				"security": pulumi.Map{
					"disable_gravatar": pulumi.Bool(true),
				},
				"server": pulumi.Map{
					"root_url":    pulumi.String(grafanaURL),
					"skip_verify": pulumi.Bool(true),
				},
				"auth": pulumi.Map{
					"disable_login_form":   pulumi.Bool(true),
					"oauth_auto_login":     pulumi.Bool(true),
					"signout_redirect_url": pulumi.Sprintf("%s/login", grafanaURL),
				},
				"auth.anonymous": pulumi.Map{
					"enabled":  pulumi.Bool(false),
					"org_role": pulumi.String("Viewer"),
				},
				"auth.proxy": pulumi.Map{
					"enabled":            pulumi.Bool(true),
					"header_name":        pulumi.String("X-Forwarded-User"),
					"header_property":    pulumi.String("username"),
					"headers":            pulumi.String("Email:X-Forwarded-Email Role:X-Forwarded-Groups"),
					"enable_login_token": pulumi.Bool(false),
					"allow_sign_up":      pulumi.Bool(true),
				},
				"auth.generic_oauth": pulumi.Map{
					"enabled":               pulumi.Bool(false),
					"client_id":             pulumi.String(s.OauthConfig.Oauth2ClientID),
					"client_secret":         pulumi.String("$__env{client_secret}"),
					"login_attribute_path":  pulumi.String("sub"),
					"role_attribute_path":   pulumi.String(buildRoleAttributePath(s.OauthConfig.Oauth2GroupsClaim, "api-admin", "Admin")),
					"role_attribute_strict": pulumi.Bool(false),
					"scopes":                pulumi.String(s.OauthConfig.Oauth2Scope),
					"api_url":               pulumi.String(s.OauthConfig.Oauth2ValidateURL),
					"auth_url":              pulumi.String(s.OauthConfig.Oauth2AuthURL),
					"token_url":             pulumi.String(s.OauthConfig.Oauth2TokenURL),
					"allow_sign_up":         pulumi.Bool(true),
				},
				"users": pulumi.Map{
					"allow_sign_up":        pulumi.Bool(false),
					"auto_assign_org":      pulumi.Bool(true),
					"auto_assign_org_role": pulumi.String("Viewer"),
					"viewers_can_edit":     pulumi.Bool(false),
				},
			},
		},
		"nodeExporter": pulumi.Map{
			"priorityClassName": priorityClassByPlatformAndWorkloadType("daemonset"),
		},
		"prometheus-node-exporter": pulumi.Map{
			"priorityClassName": priorityClassByPlatformAndWorkloadType("daemonset"),
			"resources":         s.Resources.PromNodeExporter,
		},
		"kube-state-metrics": pulumi.Map{
			"resources": s.Resources.KubeStateMetrics,
		},
	}
	secret, err := corev1.NewSecret(ctx, "mon-grafana-auth", &corev1.SecretArgs{
		ApiVersion: pulumi.String("v1"),
		Kind:       pulumi.String("Secret"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("mon-grafana-auth"), Namespace: ns.Metadata.Name()},
		StringData: pulumi.StringMap{
			"admin-user":     pulumi.String("admin"),
			"admin-password": monSharedStack.GrafanaAdminPassword,
			"client_secret":  s.OauthConfig.Oauth2ClientSecret,
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{ns}))
	if err != nil {
		return nil, err
	}
	s.DependsOn = append(s.DependsOn, secret)

	// oauth2 proxy sidecar for grafana
	redirectURI := fmt.Sprintf("https://grafana.%s/oauth2/callback", s.TLSCfg.Domain)
	// port 8989 is the Grafana authz filter we wrote to ensure users can't query data outside their namespace
	opConfigMap, err := s.CreateOAuth2ProxyConfig(ctx, ns, "grafana", redirectURI, 8989, "http")
	if err != nil {
		return nil, err
	}
	s.DependsOn = append(s.DependsOn, opConfigMap)

	if s.Platform == "aws" {
		if err = s.deployCloudWatchExporter(ctx, ns); err != nil {
			return nil, err
		}
	}

	// tricky ~ Grafana's chart doesn't let us redirect the service.targetPort to the oauth2 proxy sidecar, so we need this additional service
	opSvc, err := corev1.NewService(ctx, "oauth2-grafana", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("oauth2-grafana"), Namespace: ns.Metadata.Name()},
		Spec: &corev1.ServiceSpecArgs{
			Selector: pulumi.StringMap{"app.kubernetes.io/name": pulumi.String("grafana")},
			Ports: &corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("http-web"),
					Port:       pulumi.Int(80),
					Protocol:   pulumi.String("TCP"),
					TargetPort: pulumi.String("proxy"),
				},
			},
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return nil, err
	}
	s.DependsOn = append(s.DependsOn, opSvc)

	promRules, err := yaml.NewConfigFile(ctx, "prometheus-rules", &yaml.ConfigFileArgs{
		File: fmt.Sprintf("%s/prometheus-rules.yaml", s.GlobalConfigPath),
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return nil, err
	}
	s.DependsOn = append(s.DependsOn, promRules)

	if _, err = monSharedStack.DeployThanos(ctx, ns, s); err != nil {
		return nil, err
	}
	promHelmRelease, err := s.DeployHelmRelease(ctx, ns, "mon", PromChartVers, "", "mon-values.yaml", promCustomValues)
	if err != nil {
		return nil, err
	}
	return promHelmRelease, nil
}
func (monSharedStack *MonSharedStack) DeployLoggingComponents(ctx *pulumi.Context, s *Stack) error {
	nsName := "log-system"
	ns, err := s.CreateNamespace(ctx, nsName)
	if err != nil {
		return err
	}

	// elasticSearch
	var fluentbitOutputs pulumi.StringOutput
	redirectURI := fmt.Sprintf("https://log-analytics.%s/oauth2/callback", s.TLSCfg.Domain)
	opConfigMap, err := s.CreateOAuth2ProxyConfig(ctx, ns, "log-analytics", redirectURI, 5601, "https")
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, opConfigMap)

	if monSharedStack.EnableElasticSearch {

		//Creating log dashboards --- TODO requires process for adding more than one dashboard to a single configmap
		logDashboards := "kibana"
		dashboardJSON := ReadFile(pulumi.NewFileAsset(fmt.Sprintf(s.GlobalKibanaDashboardPath, logDashboards)).Path())
		cmName := fmt.Sprintf("%s-dashboards", logDashboards)
		key := fmt.Sprintf("%s.ndjson", logDashboards)
		dashboardCM, err := corev1.NewConfigMap(ctx, cmName, &corev1.ConfigMapArgs{
			ApiVersion: pulumi.String("v1"),
			Data:       pulumi.StringMap{key: pulumi.String(dashboardJSON)},
			Kind:       pulumi.String("ConfigMap"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(cmName),
				Namespace: ns.Metadata.Name(),
			},
		}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn), pulumi.ReplaceOnChanges([]string{"*"}))
		if err != nil {
			return err
		}

		indexLifeCycleJSON := ReadFile(pulumi.NewFileAsset(fmt.Sprintf("%s/create_es_lc_policy.json", s.GlobalConfigPath)).Path())
		indexTemplateJSON := ReadFile(pulumi.NewFileAsset(fmt.Sprintf("%s/create_es_index_template.json", s.GlobalConfigPath)).Path())

		_, err = corev1.NewConfigMap(ctx, "elastic-init-files", &corev1.ConfigMapArgs{
			ApiVersion: pulumi.String("v1"),
			Data:       pulumi.StringMap{"create_es_lc_policy.json": pulumi.String(indexLifeCycleJSON), "create_es_index_template.json": pulumi.String(indexTemplateJSON)},
			Kind:       pulumi.String("ConfigMap"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("elastic-init-files"),
				Namespace: ns.Metadata.Name(),
			},
		}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn), pulumi.ReplaceOnChanges([]string{"*"}))
		if err != nil {
			return err
		}

		OauthContainerDef := s.CreateOAuth2ProxyContainerDef(opConfigMap.Metadata.Name(), "quay.io/oauth2-proxy/oauth2-proxy:v7.6.0")
		OauthContainerDef["resources"] = s.Resources.OauthProxy

		kibanaCustomValues := pulumi.Map{
			"global": pulumi.Map{
				"defaultStorageClass": pulumi.String(monSharedStack.ElasticSearchStorageClass),
			},
			"configuration": pulumi.Map{
				"server": pulumi.Map{
					"publicBaseUrl": pulumi.Sprintf("https://log-analytics.%s", s.TLSCfg.Domain),
				},
			},
			"priorityClassName": priorityClassByPlatformAndWorkloadType("statefulset"),
			"nameOverride":      pulumi.String("kibana"),
			"fullnameOverride":  pulumi.String("kibana"),
			"namespaceOverride": ns.Metadata.Name(),
			"persistence": pulumi.Map{
				"storageClass": pulumi.String(monSharedStack.KibanaStorageClass),
				"size":         pulumi.String(monSharedStack.KibanaStorageSize),
			},
			"networkPolicy": pulumi.Map{
				"enabled": pulumi.Bool(false),
			},
			"elasticsearch": pulumi.Map{
				"security": pulumi.Map{
					"auth": pulumi.Map{
						"enabled":                     pulumi.Bool(true),
						"kibanaPassword":              monSharedStack.KibanaPassword,
						"createSystemUser":            pulumi.Bool(true),
						"elasticsearchPasswordSecret": pulumi.String("elasticsearch"),
					},
					"tls": pulumi.Map{
						"enabled":        pulumi.Bool(true),
						"existingSecret": pulumi.String("elasticsearch-coordinating-crt"),
						"usePemCerts":    pulumi.Bool(true),
					},
				},
			},
			"tls": pulumi.Map{
				"enabled":       pulumi.Bool(true),
				"autoGenerated": pulumi.Bool(true),
			},
			"podAnnotations": annotationsByPlatform(s.Platform),
			"tolerations":    nodeTolerationsByPlatformAndDistruptionType(s.Platform, false),

			"nodeSelector": getNodeSelector(false, "", "", ""),
			"extraConfiguration": pulumi.Map{
				"xpack.security.authc.providers": pulumi.Map{
					"anonymous.anonymous1": pulumi.Map{
						"order": pulumi.Int(0),
						"credentials": pulumi.Map{
							"username": pulumi.String("elastic"),
							"password": monSharedStack.ElasticSearchPassword,
						},
					},
				},
			},
			"resources": s.Resources.Kibana,
			"sidecars": pulumi.Array{
				OauthContainerDef,
			},
		}

		elasticSearchCustomValues := pulumi.Map{
			"global": pulumi.Map{
				"defaultStorageClass": pulumi.String(monSharedStack.ElasticSearchStorageClass),
			},
			"priorityClassName": priorityClassByPlatformAndWorkloadType("statefulset"),
			"nameOverride":      pulumi.String("elasticsearch"),
			"fullnameOverride":  pulumi.String("elasticsearch"),
			"namespaceOverride": ns.Metadata.Name(),
			"persistence": pulumi.Map{
				"storageClass": pulumi.String(monSharedStack.ElasticSearchStorageClass),
				"size":         pulumi.String(monSharedStack.ElasticSearchStorageSize),
			},
			"security": pulumi.Map{
				"enabled":         pulumi.Bool(true),
				"elasticPassword": monSharedStack.ElasticSearchPassword,
				"tls": pulumi.Map{
					"autoGenerated": pulumi.Bool(true),
					//"verificationMode": pulumi.String("none"),
				},
			},
			"master": pulumi.Map{
				"podAnnotations":    annotationsByPlatform(s.Platform),
				"tolerations":       nodeTolerationsByPlatformAndDistruptionType(s.Platform, false),
				"priorityClassName": priorityClassByPlatformAndWorkloadType("statefulset"),
				"nodeSelector":      getNodeSelector(false, "", "", ""),
				"persistence": pulumi.Map{
					"storageClass": pulumi.String(monSharedStack.ElasticSearchStorageClass),
					"size":         pulumi.String(monSharedStack.ElasticSearchStorageSize),
				},
			},
			"data": pulumi.Map{
				"podAnnotations":    annotationsByPlatform(s.Platform),
				"tolerations":       nodeTolerationsByPlatformAndDistruptionType(s.Platform, false),
				"priorityClassName": priorityClassByPlatformAndWorkloadType("statefulset"),
				"nodeSelector":      getNodeSelector(false, "", "", ""),
				"persistence": pulumi.Map{
					"storageClass": pulumi.String(monSharedStack.ElasticSearchStorageClass),
					"size":         pulumi.String(monSharedStack.ElasticSearchStorageSize),
				},
				"resourcesPreset": s.Resources.ElasticDataResourcePreset,
			},
			"ingest": pulumi.Map{
				"podAnnotations":    annotationsByPlatform(s.Platform),
				"tolerations":       nodeTolerationsByPlatformAndDistruptionType(s.Platform, false),
				"priorityClassName": priorityClassByPlatformAndWorkloadType("deployment"),
				"nodeSelector":      getNodeSelector(false, "", "", ""),
				"resourcesPreset":   s.Resources.ElasticIngestResourcePreset,
				"heapSize":          s.Resources.ElasticIngestHeapSize,
			},
			"coordinating": pulumi.Map{
				"podAnnotations":    annotationsByPlatform(s.Platform),
				"tolerations":       nodeTolerationsByPlatformAndDistruptionType(s.Platform, false),
				"priorityClassName": priorityClassByPlatformAndWorkloadType("deployment"),
				"nodeSelector":      getNodeSelector(false, "", "", ""),
				"resourcesPreset":   s.Resources.ElasticCoordinatingResourcePreset,
				"heapSize":          s.Resources.ElasticCoordinatingHeapSize,
			},
			"metrics": pulumi.Map{
				"resources":         s.Resources.ElasticMetrics,
				"priorityClassName": priorityClassByPlatformAndWorkloadType("deployment"),
			},
			"kibana": kibanaCustomValues,
		}

		elasticHelm, err := s.DeployHelmRelease(ctx, ns, "elasticsearch", ElasticSearchChartVers, "", "elasticsearch-values.yaml", elasticSearchCustomValues)
		if err != nil {
			return err
		}

		// For the Elastic DataSource in Grafana
		// copy the cert needed for mTLS
		if err = copySecret(ctx, s, "elasticsearch-coordinating-crt", "monitoring", elasticHelm); err != nil {
			return err
		}
		// copy the basic auth password too
		if err = copySecret(ctx, s, "elasticsearch", "monitoring", elasticHelm); err != nil {
			return err
		}

		kibanaAutoLoadSvc, err := corev1.NewService(ctx, "kibana-dashboard-load", &corev1.ServiceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Namespace: ns.Metadata.Name(),
				Labels: pulumi.StringMap{
					"app.kubernetes.io/name": pulumi.String("kibana"),
				},
				Name: pulumi.String("kibana-load"),
			},
			Spec: &corev1.ServiceSpecArgs{
				Selector: pulumi.StringMap{
					"app.kubernetes.io/name": pulumi.String("kibana"),
				},
				InternalTrafficPolicy: pulumi.String("Cluster"),
				Ports: &corev1.ServicePortArray{
					&corev1.ServicePortArgs{
						Name:       pulumi.String("https"),
						Port:       pulumi.Int(5601),
						Protocol:   pulumi.String("TCP"),
						TargetPort: pulumi.Int(5601),
					},
				},
			},
		}, pulumi.DependsOn([]pulumi.Resource{
			elasticHelm,
		}))
		if err != nil {
			return err
		}

		s.DependsOn = append(s.DependsOn, kibanaAutoLoadSvc)

		// TODO work with more than one configmap or multiple entries from a single configmap

		// hash is used to check that the file has changed, in that case the autoload pod is recreated
		hash := CalcChecksum(pulumi.NewFileAsset(fmt.Sprintf(s.GlobalKibanaDashboardPath, logDashboards)).Path())
		kibanaAutoLoad, err := corev1.NewPod(ctx, "kibana-dashboard-load", &corev1.PodArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Namespace: ns.Metadata.Name(),
				Labels: pulumi.StringMap{
					"app.kubernetes.io/name": pulumi.String("kibana-autoload"),
					"cmHash":                 pulumi.String(hash),
				},
			},
			Spec: &corev1.PodSpecArgs{
				Volumes: &corev1.VolumeArray{
					&corev1.VolumeArgs{
						Name: pulumi.String("dashboards"),
						ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
							Name: pulumi.String("kibana-dashboards"),
							Items: &corev1.KeyToPathArray{
								&corev1.KeyToPathArgs{
									Key:  pulumi.String("kibana.ndjson"),
									Path: pulumi.String("kibana.ndjson"),
								},
							},
						},
					},
				},
				RestartPolicy: pulumi.String("Never"),
				Containers: corev1.ContainerArray{
					&corev1.ContainerArgs{
						VolumeMounts: &corev1.VolumeMountArray{
							&corev1.VolumeMountArgs{
								Name:      pulumi.String("dashboards"),
								MountPath: pulumi.String("/dashboards/"),
							},
						},
						Command: pulumi.StringArray{
							pulumi.String("curl"),
							pulumi.String("-v"),
							pulumi.String("-s"),
							pulumi.String("-k"),
							pulumi.String("--connect-timeout"),
							pulumi.String("60"),
							pulumi.String("--max-time"),
							pulumi.String("60"),
							pulumi.String("-u"),
							pulumi.String("elastic:$(PASSWORD)"),
							pulumi.String("-XPOST"),
							pulumi.String("https://kibana-load.log-system.svc.cluster.local:5601/api/saved_objects/_import?overwrite=true"),
							pulumi.String("-H"),
							pulumi.String("kbn-xsrf:true"),
							pulumi.String("--form"),
							pulumi.String("file=@/dashboards/kibana.ndjson"),
						},
						Image: pulumi.String("curlimages/curl:8.11.1"),
						Name:  pulumi.String("kibana-dashboard-load"),
						Env: corev1.EnvVarArray{
							&corev1.EnvVarArgs{
								Name: pulumi.String("PASSWORD"),
								ValueFrom: &corev1.EnvVarSourceArgs{
									SecretKeyRef: &corev1.SecretKeySelectorArgs{
										Name: pulumi.String("elasticsearch"),
										Key:  pulumi.String("elasticsearch-password"),
									},
								},
							},
						},
					},
				},
			},
		}, pulumi.DependsOn([]pulumi.Resource{
			kibanaAutoLoadSvc, dashboardCM,
		}), pulumi.ReplaceOnChanges([]string{"*"}))
		if err != nil {
			return err
		}
		s.DependsOn = append(s.DependsOn, kibanaAutoLoad)

		// note: the formatting here is very important as it gets embedded into the fluentbit configmap
		fluentbitOutputs = pulumi.Sprintf(`
[OUTPUT]
    Name es
    Match kube.*
    Host elasticsearch-ingest-hl.log-system.svc.cluster.local
    HTTP_User elastic
    HTTP_Passwd %s
    Port 9200
    Logstash_Format On
    Logstash_Prefix logs
    Suppress_Type_Name On
    Buffer_Size False
    Trace_Error On
    Tls On
    Tls.verify Off
[OUTPUT]
    Name es
    Match events
    Host elasticsearch-ingest-hl.log-system.svc.cluster.local
    HTTP_User elastic
    HTTP_Passwd %s
    Port 9200
    Logstash_Format On
    Logstash_Prefix events
    Suppress_Type_Name On
    Buffer_Size False
    Trace_Error On
    Tls On
    Tls.verify Off`, monSharedStack.ElasticSearchPassword, monSharedStack.ElasticSearchPassword)
	} else {
		fluentbitOutputs = pulumi.Sprintf(`
[OUTPUT]
    name stdout
    match *
    format json_stream
    json_date_key timestamp
    json_date_format epoch`)
	}

	fluentbitCustomParsers := `
[PARSER]
    Name        dockerGKE
    Format      json
    Time_Key    time
    Time_Format %Y-%m-%dT%H:%M:%S%Z
    Time_Keep   On`

	fluentbitFilter := `
[FILTER]
    Name kubernetes
    Match kube.*
    Merge_Log On
    Keep_Log Off
    Labels Off
    Annotations Off
    K8S-Logging.Parser On
    K8S-Logging.Exclude On

[FILTER]
    Name nest
    Match kube.*
    Operation nest
    Wildcard *
    Nest_under log

[FILTER]
    Name lua
    Match kube.*
    script /fluent-bit/scripts/functions.lua
    call set_fields

[FILTER]
    Name lua
    Match events
    script /fluent-bit/scripts/functions.lua
    call set_event_fields`

	var fluentbitCustomFilter string

	var imageRepo string

	if s.Platform == "gke" {
		imageRepo = "fluent/fluent-bit"
		fluentbitCustomFilter = `
[FILTER]
    Name         parser
    Parser       dockerGKE
    Match        events
    Key_Name     log
    Reserve_Data On
    Preserve_Key On	`

	} else {
		imageRepo = "299166832260.dkr.ecr.us-east-2.amazonaws.com/docker-hub/fluent/fluent-bit"
		fluentbitCustomFilter = `
[FILTER]
    Name         parser
    Parser       docker
    Match        events
    Key_Name     log
    Reserve_Data On
    Preserve_Key On	`
	}

	// fluentbit
	fbValues := pulumi.Map{
		"nameOverride":     pulumi.String("fluentbit"),
		"fullnameOverride": pulumi.String("fluentbit"),
		"image": pulumi.Map{
			"repository": pulumi.String(imageRepo),
			"tag":        pulumi.String("latest"),
		},
		"logLevel": pulumi.String("warn"),
		"testFramework": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},
		"resources": s.Resources.FluentBit,
		"config": pulumi.Map{
			"outputs": fluentbitOutputs,
		},
		"rbac": pulumi.Map{
			"nodeAccess":   pulumi.Bool(true),
			"eventsAccess": pulumi.Bool(true),
		},
		"priorityClassName": priorityClassByPlatformAndWorkloadType("daemonset"),
		"extraPorts": pulumi.MapArray{
			pulumi.Map{
				"name":          pulumi.String("otlphttp"),
				"port":          pulumi.Int(4318),
				"protocol":      pulumi.String("TCP"),
				"containerPort": pulumi.Int(4318),
			},
		},
	}
	if s.Platform == "gke" {
		fluentbitFilter += fluentbitFilter + fluentbitCustomFilter

		fbValues["config"] = pulumi.Map{
			"outputs":       fluentbitOutputs,
			"customParsers": pulumi.String(fluentbitCustomParsers),
			"filters":       pulumi.String(fluentbitFilter),
		}
	} else {
		fbValues["config"] = pulumi.Map{
			"outputs": fluentbitOutputs,
		}
	}
	fbValues["tolerations"] = pulumi.Array{
		pulumi.Map{
			"operator": pulumi.String("Exists"),
		},
	}

	_, err = s.DeployHelmRelease(ctx, ns, "fluent-bit", FluentbitChartVers, "", "fluentbit-values.yaml", fbValues)
	if err != nil {
		return err
	}

	eeValues := pulumi.Map{
		"nameOverride":     pulumi.String("event-exporter"),
		"fullnameOverride": pulumi.String("event-exporter"),
		"config": pulumi.Map{
			"logLevel":           pulumi.String("error"),
			"logFormat":          pulumi.String("json"),
			"maxEventAgeSeconds": pulumi.Int(15),
			"route": pulumi.Map{
				"routes": pulumi.Array{
					pulumi.Map{
						"match": pulumi.Array{
							pulumi.Map{"receiver": pulumi.String("dump")},
						},
					},
				},
			},
			"receivers": pulumi.Array{
				pulumi.Map{
					"name": pulumi.String("dump"),
					"stdout": pulumi.Map{
						"deDot": pulumi.Bool(true),
					},
				},
			},
		},
		"resources": s.Resources.EventsExporter,
	}
	_, err = s.DeployHelmRelease(ctx, ns, "event-exporter", EventExporterChartVers, "kubernetes-event-exporter", "", eeValues)
	if err != nil {
		return err
	}

	return nil
}

func (monSharedStack *MonSharedStack) DeployOTELCollector(ctx *pulumi.Context, s *Stack) error {

	nsName := "otel-operator-system"
	ns, err := s.CreateNamespace(ctx, nsName)
	if err != nil {
		return err
	}

	secret, err := corev1.NewSecret(ctx, "elastic-secret-otel", &corev1.SecretArgs{
		ApiVersion: pulumi.String("v1"),
		Kind:       pulumi.String("Secret"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("elastic-secret-otel"), Namespace: ns.Metadata.Name()},
		StringData: pulumi.StringMap{
			"elastic_endpoint": pulumi.String("https://elasticsearch.log-system.svc.cluster.local:9200"),
			"elastic_user":     pulumi.String("elastic"),
			"elastic_password": monSharedStack.ElasticSearchPassword,
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{ns}))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, secret)

	otelKubeStackValues := pulumi.Map{
		"collectors": pulumi.Map{
			"gateway": pulumi.Map{
				"env": pulumi.MapArray{
					pulumi.Map{
						"name":  pulumi.String("ELASTIC_AGENT_OTEL"),
						"value": pulumi.String("\"true\""),
					},
					pulumi.Map{
						"name": pulumi.String("ELASTIC_ENDPOINT"),
						"valueFrom": pulumi.Map{
							"secretKeyRef": pulumi.Map{
								"name": secret.Metadata.Name(),
								"key":  pulumi.String("elastic_endpoint"),
							},
						},
					},
					pulumi.Map{
						"name": pulumi.String("ELASTIC_USER"),
						"valueFrom": pulumi.Map{
							"secretKeyRef": pulumi.Map{
								"name": secret.Metadata.Name(),
								"key":  pulumi.String("elastic_user"),
							},
						},
					},
					pulumi.Map{
						"name": pulumi.String("ELASTIC_PASSWORD"),
						"valueFrom": pulumi.Map{
							"secretKeyRef": pulumi.Map{
								"name": secret.Metadata.Name(),
								"key":  pulumi.String("elastic_password"),
							},
						},
					},
					pulumi.Map{
						"name": pulumi.String("GOMAXPROCS"),
						"valueFrom": pulumi.Map{
							"resourceFieldRef": pulumi.Map{
								"resource": pulumi.String("limits.cpu"),
							},
						},
					},
					pulumi.Map{
						"name":  pulumi.String("GOMEMLIMIT"),
						"value": pulumi.String("1025MiB"),
					},
				},
			},
		},
	}

	_, err = s.DeployHelmRelease(ctx, ns, "otel-kube-stack", OtelKubeStackChartVers, "", "opentelemetry-kube-stack-values.yaml", otelKubeStackValues)
	if err != nil {
		return err
	}

	return nil
}

func (monSharedStack *MonSharedStack) DeployCustomDashboards(ctx *pulumi.Context, s *Stack) error {
	dashboards := []string{"nativelink", "ingress", "redis", "redis-thanos", "node-exporter-full"}
	if s.Platform == "aws" {
		dashboards = append(dashboards, "karpenter", "s3")
	}
	for _, db := range dashboards {
		dashboardJSON := ReadFile(pulumi.NewFileAsset(fmt.Sprintf("%s/%s-dashboard.json", s.GlobalDashboardPath, db)).Path())
		cmName := fmt.Sprintf("%s-dashboard", db)
		key := fmt.Sprintf("%s.json", db)
		_, err := corev1.NewConfigMap(ctx, cmName, &corev1.ConfigMapArgs{
			ApiVersion: pulumi.String("v1"),
			Data:       pulumi.StringMap{key: pulumi.String(dashboardJSON)},
			Kind:       pulumi.String("ConfigMap"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(cmName),
				Namespace: pulumi.String("monitoring"),
				Labels:    pulumi.StringMap{"grafana_dashboard": pulumi.String("1")},
				Annotations: pulumi.StringMap{
					"grafana_folder": pulumi.String("nl-internal"),
				},
			},
		}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
		if err != nil {
			return err
		}
	}
	return nil
}

func (monSharedStack *MonSharedStack) DeployKubeCostDashboards(ctx *pulumi.Context, s *Stack) error {
	kubecostDashboardFolder := fmt.Sprintf("%s/kubecost", s.GlobalDashboardPath)

	files, err := os.ReadDir(kubecostDashboardFolder)
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.Type().IsRegular() {
			dashboardJSON := ReadFile(pulumi.NewFileAsset(fmt.Sprintf("%s/%s", kubecostDashboardFolder, file.Name())).Path())
			fileName := strings.TrimSuffix(file.Name(), ".json")
			cmName := fmt.Sprintf("%s-dashboard", fileName)
			key := file.Name()
			_, err := corev1.NewConfigMap(ctx, cmName, &corev1.ConfigMapArgs{
				ApiVersion: pulumi.String("v1"),
				Data:       pulumi.StringMap{key: pulumi.String(dashboardJSON)},
				Kind:       pulumi.String("ConfigMap"),
				Metadata: &metav1.ObjectMetaArgs{
					Name:      pulumi.String(cmName),
					Namespace: pulumi.String("monitoring"),
					Labels:    pulumi.StringMap{"grafana_dashboard": pulumi.String("2")},
					Annotations: pulumi.StringMap{
						"grafana_folder": pulumi.String("kubecost"),
					},
				},
			}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func copySecret(ctx *pulumi.Context, s *Stack, sourceName string, destNamespace string, dependsOn pulumi.Resource) error {
	sourceSecret, err := corev1.GetSecret(ctx, "source-secret-"+sourceName, pulumi.ID("log-system/"+sourceName),
		&corev1.SecretState{}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{dependsOn}))
	if err != nil {
		return err
	}
	_, err = corev1.NewSecret(ctx, "copy-"+sourceName, &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      sourceSecret.Metadata.Name(),
			Namespace: pulumi.String(destNamespace),
		},
		Type: sourceSecret.Type,
		Data: sourceSecret.Data,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{sourceSecret}))
	return err
}
