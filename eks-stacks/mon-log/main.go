package main

import (
	"fmt"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	kubePrometheusStackChartVers = "58.2.2"
	lokiChartVers                = "6.6.2"
	promtailChartVers            = "6.15.5"
	monitoringNamespace          = "monitoring"
)

// MonLogConfig holds configuration for monitoring and logging
type MonLogConfig struct {
	Environment string
	ProjectName string
	// Prometheus
	PrometheusStorage      string
	PrometheusStorageClass string
	PrometheusRetention    string
	PrometheusReplicas     int
	// Grafana
	GrafanaStorage       string
	GrafanaStorageClass  string
	GrafanaAdminPassword pulumi.StringOutput
	// Loki
	EnableLoki       bool
	LokiStorage      string
	LokiStorageClass string
	LokiRetention    string
	// AlertManager
	EnableAlertManager bool
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Load configuration
		cfg := config.New(ctx, "")
		monCfg := initMonLogConfig(cfg)

		// Get kubeconfig from infra-kube stack
		infraKubeStackRef := cfg.Require("infraKubeStackRef")
		infraKubeStack, err := pulumi.NewStackReference(ctx, infraKubeStackRef, nil)
		if err != nil {
			return fmt.Errorf("failed to get infra-kube stack reference: %w", err)
		}

		kubeconfig := infraKubeStack.GetStringOutput(pulumi.String("kubeconfig"))

		// Create Kubernetes provider
		k8sProvider, err := kubernetes.NewProvider(ctx, "k8s-provider", &kubernetes.ProviderArgs{
			Kubeconfig: kubeconfig,
		})
		if err != nil {
			return fmt.Errorf("failed to create k8s provider: %w", err)
		}

		// Create monitoring namespace
		monNs, err := createNamespace(ctx, k8sProvider, monitoringNamespace)
		if err != nil {
			return err
		}

		dependsOn := []pulumi.Resource{monNs}

		// Deploy Loki first (if enabled) so Grafana can configure datasource
		var lokiRelease *helmv3.Release
		if monCfg.EnableLoki {
			lokiRelease, err = deployLoki(ctx, k8sProvider, dependsOn, monCfg)
			if err != nil {
				return err
			}
			dependsOn = append(dependsOn, lokiRelease)

			// Deploy Promtail to collect logs from all pods
			promtailRelease, err := deployPromtail(ctx, k8sProvider, dependsOn)
			if err != nil {
				return err
			}
			dependsOn = append(dependsOn, promtailRelease)
		}

		// Deploy kube-prometheus-stack (Prometheus + Grafana + AlertManager)
		promStack, err := deployKubePrometheusStack(ctx, k8sProvider, dependsOn, monCfg)
		if err != nil {
			return err
		}
		dependsOn = append(dependsOn, promStack)

		// Export outputs
		ctx.Export("environment", pulumi.String(monCfg.Environment))
		ctx.Export("monitoringNamespace", pulumi.String(monitoringNamespace))
		ctx.Export("grafanaService", pulumi.String("kube-prometheus-stack-grafana"))
		ctx.Export("prometheusService", pulumi.String("kube-prometheus-stack-prometheus"))

		if monCfg.EnableAlertManager {
			ctx.Export("alertmanagerService", pulumi.String("kube-prometheus-stack-alertmanager"))
		}

		if monCfg.EnableLoki {
			ctx.Export("lokiService", pulumi.String("loki"))
			ctx.Export("promtailDaemonSet", pulumi.String("promtail"))
		}

		fmt.Printf("Monitoring and logging stack deployed successfully in namespace '%s'\n", monitoringNamespace)

		return nil
	})
}

// initMonLogConfig initializes configuration from Pulumi config
func initMonLogConfig(cfg *config.Config) *MonLogConfig {
	monCfg := &MonLogConfig{
		Environment:            cfg.Get("environment"),
		ProjectName:            cfg.Get("projectName"),
		PrometheusStorage:      cfg.Get("prometheusStorage"),
		PrometheusStorageClass: cfg.Get("prometheusStorageClass"),
		PrometheusRetention:    cfg.Get("prometheusRetention"),
		PrometheusReplicas:     cfg.GetInt("prometheusReplicas"),
		GrafanaStorage:         cfg.Get("grafanaStorage"),
		GrafanaStorageClass:    cfg.Get("grafanaStorageClass"),
		GrafanaAdminPassword:   cfg.RequireSecret("grafanaAdminPassword"),
		EnableLoki:             cfg.GetBool("enableLoki"),
		LokiStorage:            cfg.Get("lokiStorage"),
		LokiStorageClass:       cfg.Get("lokiStorageClass"),
		LokiRetention:          cfg.Get("lokiRetention"),
		EnableAlertManager:     cfg.GetBool("enableAlertManager"),
	}

	// Set defaults
	if monCfg.Environment == "" {
		monCfg.Environment = "dev"
	}
	if monCfg.ProjectName == "" {
		monCfg.ProjectName = "cloud-platform-tfm"
	}
	if monCfg.PrometheusStorage == "" {
		monCfg.PrometheusStorage = "20Gi"
	}
	if monCfg.PrometheusStorageClass == "" {
		monCfg.PrometheusStorageClass = "gp3-enc"
	}
	if monCfg.PrometheusRetention == "" {
		monCfg.PrometheusRetention = "7d"
	}
	if monCfg.PrometheusReplicas <= 0 {
		monCfg.PrometheusReplicas = 1
	}
	if monCfg.GrafanaStorage == "" {
		monCfg.GrafanaStorage = "10Gi"
	}
	if monCfg.GrafanaStorageClass == "" {
		monCfg.GrafanaStorageClass = "gp3-enc"
	}
	if monCfg.LokiStorage == "" {
		monCfg.LokiStorage = "10Gi"
	}
	if monCfg.LokiStorageClass == "" {
		monCfg.LokiStorageClass = "gp3-enc"
	}
	if monCfg.LokiRetention == "" {
		monCfg.LokiRetention = "168h" // 7 days
	}

	return monCfg
}

// createNamespace creates a Kubernetes namespace
func createNamespace(ctx *pulumi.Context, k8sProvider *kubernetes.Provider, name string) (*corev1.Namespace, error) {
	ns, err := corev1.NewNamespace(ctx, name, &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(name),
			Labels: pulumi.StringMap{
				"name": pulumi.String(name),
			},
		},
	}, pulumi.Provider(k8sProvider))
	if err != nil {
		return nil, fmt.Errorf("failed to create namespace '%s': %w", name, err)
	}
	return ns, nil
}

// deployLoki deploys Loki for log aggregation
func deployLoki(ctx *pulumi.Context, k8sProvider *kubernetes.Provider, dependsOn []pulumi.Resource, cfg *MonLogConfig) (*helmv3.Release, error) {
	customValues := pulumi.Map{
		"fullnameOverride": pulumi.String("loki"),

		// Single binary mode - lightweight for demo
		"deploymentMode": pulumi.String("SingleBinary"),

		"singleBinary": pulumi.Map{
			"replicas": pulumi.Int(1),
			"persistence": pulumi.Map{
				"enabled":      pulumi.Bool(true),
				"size":         pulumi.String(cfg.LokiStorage),
				"storageClass": pulumi.String(cfg.LokiStorageClass),
			},
			"resources": pulumi.Map{
				"requests": pulumi.Map{
					"cpu":    pulumi.String("100m"),
					"memory": pulumi.String("256Mi"),
				},
				"limits": pulumi.Map{
					"cpu":    pulumi.String("500m"),
					"memory": pulumi.String("512Mi"),
				},
			},
		},

		// Loki configuration
		"loki": pulumi.Map{
			"auth_enabled": pulumi.Bool(false),
			"commonConfig": pulumi.Map{
				"replication_factor": pulumi.Int(1),
			},
			"storage": pulumi.Map{
				"type": pulumi.String("filesystem"),
			},
			"limits_config": pulumi.Map{
				"retention_period":        pulumi.String(cfg.LokiRetention),
				"ingestion_rate_mb":       pulumi.Int(10),
				"ingestion_burst_size_mb": pulumi.Int(20),
				"max_streams_per_user":    pulumi.Int(10000),
			},
			"schemaConfig": pulumi.Map{
				"configs": pulumi.Array{
					pulumi.Map{
						"from":         pulumi.String("2024-01-01"),
						"store":        pulumi.String("tsdb"),
						"object_store": pulumi.String("filesystem"),
						"schema":       pulumi.String("v13"),
						"index": pulumi.Map{
							"prefix": pulumi.String("index_"),
							"period": pulumi.String("24h"),
						},
					},
				},
			},
		},

		// Disable distributed components (using SingleBinary)
		"backend": pulumi.Map{
			"replicas": pulumi.Int(0),
		},
		"read": pulumi.Map{
			"replicas": pulumi.Int(0),
		},
		"write": pulumi.Map{
			"replicas": pulumi.Int(0),
		},

		// Gateway not needed for SingleBinary
		"gateway": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},

		// Monitoring
		"monitoring": pulumi.Map{
			"selfMonitoring": pulumi.Map{
				"enabled": pulumi.Bool(false),
			},
			"lokiCanary": pulumi.Map{
				"enabled": pulumi.Bool(false),
			},
		},

		// Test disabled
		"test": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},
	}

	helmRel, err := helmv3.NewRelease(ctx, "loki", &helmv3.ReleaseArgs{
		Name:      pulumi.String("loki"),
		Namespace: pulumi.String(monitoringNamespace),
		Chart:     pulumi.String("loki"),
		Version:   pulumi.String(lokiChartVers),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{
			Repo: pulumi.String("https://grafana.github.io/helm-charts"),
		},
		Values:          customValues,
		CreateNamespace: pulumi.Bool(false),
		Timeout:         pulumi.Int(300),
	}, pulumi.Provider(k8sProvider), pulumi.DependsOn(dependsOn))

	if err != nil {
		return nil, fmt.Errorf("failed to deploy Loki: %w", err)
	}

	return helmRel, nil
}

// deployPromtail deploys Promtail DaemonSet to collect logs from all nodes
func deployPromtail(ctx *pulumi.Context, k8sProvider *kubernetes.Provider, dependsOn []pulumi.Resource) (*helmv3.Release, error) {
	customValues := pulumi.Map{
		"fullnameOverride": pulumi.String("promtail"),

		// Loki endpoint
		"config": pulumi.Map{
			"clients": pulumi.Array{
				pulumi.Map{
					"url": pulumi.String(fmt.Sprintf("http://loki.%s.svc.cluster.local:3100/loki/api/v1/push", monitoringNamespace)),
				},
			},
		},

		// Resources for DaemonSet
		"resources": pulumi.Map{
			"requests": pulumi.Map{
				"cpu":    pulumi.String("50m"),
				"memory": pulumi.String("64Mi"),
			},
			"limits": pulumi.Map{
				"cpu":    pulumi.String("200m"),
				"memory": pulumi.String("128Mi"),
			},
		},

		// Tolerations to run on all nodes
		"tolerations": pulumi.Array{
			pulumi.Map{
				"effect":   pulumi.String("NoSchedule"),
				"operator": pulumi.String("Exists"),
			},
		},
	}

	helmRel, err := helmv3.NewRelease(ctx, "promtail", &helmv3.ReleaseArgs{
		Name:      pulumi.String("promtail"),
		Namespace: pulumi.String(monitoringNamespace),
		Chart:     pulumi.String("promtail"),
		Version:   pulumi.String(promtailChartVers),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{
			Repo: pulumi.String("https://grafana.github.io/helm-charts"),
		},
		Values:          customValues,
		CreateNamespace: pulumi.Bool(false),
		Timeout:         pulumi.Int(300),
	}, pulumi.Provider(k8sProvider), pulumi.DependsOn(dependsOn))

	if err != nil {
		return nil, fmt.Errorf("failed to deploy Promtail: %w", err)
	}

	return helmRel, nil
}

// deployKubePrometheusStack deploys the kube-prometheus-stack Helm chart
func deployKubePrometheusStack(ctx *pulumi.Context, k8sProvider *kubernetes.Provider, dependsOn []pulumi.Resource, cfg *MonLogConfig) (*helmv3.Release, error) {
	// Build additional datasources for Grafana
	additionalDataSources := pulumi.Array{}

	if cfg.EnableLoki {
		additionalDataSources = append(additionalDataSources, pulumi.Map{
			"name":      pulumi.String("Loki"),
			"type":      pulumi.String("loki"),
			"url":       pulumi.String(fmt.Sprintf("http://loki.%s.svc.cluster.local:3100", monitoringNamespace)),
			"access":    pulumi.String("proxy"),
			"isDefault": pulumi.Bool(false),
		})
	}

	customValues := pulumi.Map{
		"fullnameOverride": pulumi.String("kube-prometheus-stack"),

		// Prometheus configuration
		"prometheus": pulumi.Map{
			"prometheusSpec": pulumi.Map{
				"retention": pulumi.String(cfg.PrometheusRetention),
				"replicas":  pulumi.Int(cfg.PrometheusReplicas),
				"storageSpec": pulumi.Map{
					"volumeClaimTemplate": pulumi.Map{
						"spec": pulumi.Map{
							"storageClassName": pulumi.String(cfg.PrometheusStorageClass),
							"accessModes":      pulumi.StringArray{pulumi.String("ReadWriteOnce")},
							"resources": pulumi.Map{
								"requests": pulumi.Map{
									"storage": pulumi.String(cfg.PrometheusStorage),
								},
							},
						},
					},
				},
				"resources": pulumi.Map{
					"requests": pulumi.Map{
						"cpu":    pulumi.String("200m"),
						"memory": pulumi.String("512Mi"),
					},
					"limits": pulumi.Map{
						"cpu":    pulumi.String("500m"),
						"memory": pulumi.String("1Gi"),
					},
				},
			},
		},

		// Grafana configuration with Loki datasource
		"grafana": pulumi.Map{
			"adminPassword": cfg.GrafanaAdminPassword,
			"persistence": pulumi.Map{
				"enabled":          pulumi.Bool(true),
				"storageClassName": pulumi.String(cfg.GrafanaStorageClass),
				"size":             pulumi.String(cfg.GrafanaStorage),
			},
			"resources": pulumi.Map{
				"requests": pulumi.Map{
					"cpu":    pulumi.String("100m"),
					"memory": pulumi.String("256Mi"),
				},
				"limits": pulumi.Map{
					"cpu":    pulumi.String("300m"),
					"memory": pulumi.String("512Mi"),
				},
			},
			"sidecar": pulumi.Map{
				"dashboards": pulumi.Map{
					"enabled": pulumi.Bool(true),
				},
			},
			"additionalDataSources": additionalDataSources,
		},

		// AlertManager configuration
		"alertmanager": pulumi.Map{
			"enabled": pulumi.Bool(cfg.EnableAlertManager),
			"alertmanagerSpec": pulumi.Map{
				"resources": pulumi.Map{
					"requests": pulumi.Map{
						"cpu":    pulumi.String("50m"),
						"memory": pulumi.String("64Mi"),
					},
					"limits": pulumi.Map{
						"cpu":    pulumi.String("100m"),
						"memory": pulumi.String("128Mi"),
					},
				},
			},
		},

		// Node Exporter for node metrics
		"nodeExporter": pulumi.Map{
			"enabled": pulumi.Bool(true),
		},

		// Kube State Metrics
		"kubeStateMetrics": pulumi.Map{
			"enabled": pulumi.Bool(true),
		},

		// Disable components not needed for TFM demo (not accessible in EKS)
		"kubeEtcd": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},
		"kubeControllerManager": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},
		"kubeScheduler": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},
		"kubeProxy": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},
	}

	helmRel, err := helmv3.NewRelease(ctx, "kube-prometheus-stack", &helmv3.ReleaseArgs{
		Name:      pulumi.String("kube-prometheus-stack"),
		Namespace: pulumi.String(monitoringNamespace),
		Chart:     pulumi.String("kube-prometheus-stack"),
		Version:   pulumi.String(kubePrometheusStackChartVers),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{
			Repo: pulumi.String("https://prometheus-community.github.io/helm-charts"),
		},
		Values:          customValues,
		CreateNamespace: pulumi.Bool(false),
		Timeout:         pulumi.Int(600),
	}, pulumi.Provider(k8sProvider), pulumi.DependsOn(dependsOn))

	if err != nil {
		return nil, fmt.Errorf("failed to deploy kube-prometheus-stack: %w", err)
	}

	return helmRel, nil
}
