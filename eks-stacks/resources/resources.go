package resources

import (
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type (
	ResourceConfig struct {
		ArcController                     pulumi.Map
		ArcRunner                         pulumi.Map
		GatewayApi                        pulumi.Map
		Crossplane                        pulumi.Map
		CrossplaneRBAC                    pulumi.Map
		CertManagerController             pulumi.Map
		CertManagerCAInjector             pulumi.Map
		CertManagerWebhook                pulumi.Map
		IngressNginx                      pulumi.Map
		Karpenter                         pulumi.Map
		KubecostModel                     pulumi.Map
		KubecostForecasting               pulumi.Map
		KubecostNetworkCosts              pulumi.Map
		KubecostAggregator                pulumi.Map
		KubecostFrontend                  pulumi.Map
		KubecostCloudCost                 pulumi.Map
		FluentBit                         pulumi.Map
		Kibana                            pulumi.Map
		ElasticMetrics                    pulumi.Map
		ElasticDataResourcePreset         pulumi.String
		ElasticIngestResourcePreset       pulumi.String
		ElasticIngestHeapSize             pulumi.String
		ElasticCoordinatingResourcePreset pulumi.String
		ElasticCoordinatingHeapSize       pulumi.String
		EventsExporter                    pulumi.Map
		CloudWatchMetrics                 pulumi.Map
		PromNodeExporter                  pulumi.Map
		KubeStateMetrics                  pulumi.Map
		GrafanaSidecars                   pulumi.Map
		PromOperator                      pulumi.Map
		Grafana                           pulumi.Map
		Prometheus                        pulumi.Map
		PromThanos                        pulumi.Map
		Api                               corev1.ResourceRequirementsArgs
		DragonFly                         pulumi.Map
		TemporalAdminTools                pulumi.Map
		TemporalFrontend                  pulumi.Map
		TemporalHistory                   pulumi.Map
		TemporalMatching                  pulumi.Map
		TemporalWeb                       pulumi.Map
		TemporalWorker                    pulumi.Map
		Runbooks                          corev1.ResourceRequirementsArgs
		OauthProxy                        corev1.ResourceRequirementsArgs
		ThanosQueryResourcePreset         pulumi.String
		ThanosQueryFrontendResourcePreset pulumi.String
	}
)

func (s *ResourceConfig) InitResourcesDev() {
	s.ArcController = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("40Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
	}
	s.ArcRunner = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("2"),
			"memory": pulumi.String("3Gi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("4"),
			"memory": pulumi.String("66Gi"),
		},
	}
	s.GatewayApi = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("50Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
	}
	s.CertManagerController = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("60m"),
			"memory": pulumi.String("100Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("200m"),
			"memory": pulumi.String("300Mi"),
		},
	}
	s.CertManagerCAInjector = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("50m"),
			"memory": pulumi.String("300Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("300m"),
			"memory": pulumi.String("1000Mi"),
		},
	}
	s.CertManagerWebhook = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("15Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
	}
	s.Crossplane = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("650m"),
			"memory": pulumi.String("450Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("3"),
			"memory": pulumi.String("2Gi"),
		},
	}
	s.CrossplaneRBAC = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("30Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
	}
	s.IngressNginx = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("300m"),
			"memory": pulumi.String("1000Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("900m"),
			"memory": pulumi.String("1500Mi"),
		},
	}
	s.Karpenter = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("500Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("300m"),
			"memory": pulumi.String("1500Mi"),
		},
	}
	s.KubecostForecasting = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("70m"),
			"memory": pulumi.String("170Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("210m"),
			"memory": pulumi.String("510Mi"),
		},
	}
	s.KubecostNetworkCosts = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("10Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
	}
	s.KubecostAggregator = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("500m"),
			"memory": pulumi.String("1600Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("1500m"),
			"memory": pulumi.String("3200Mi"),
		},
	}
	s.KubecostCloudCost = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("60Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
	}
	s.KubecostFrontend = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("15Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
	}
	s.KubecostModel = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("180m"),
			"memory": pulumi.String("220Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("540m"),
			"memory": pulumi.String("660Mi"),
		},
	}

	s.FluentBit = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("15m"),
			"memory": pulumi.String("25Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
	}
	s.ElasticDataResourcePreset = pulumi.String("large")
	s.ElasticIngestResourcePreset = pulumi.String("medium")
	s.ElasticIngestHeapSize = pulumi.String("512m")
	s.ElasticCoordinatingResourcePreset = pulumi.String("medium")
	s.ElasticCoordinatingHeapSize = pulumi.String("512m")
	s.ElasticMetrics = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("20Mi"),
		},
	}
	s.EventsExporter = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("40Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
	}
	s.Kibana = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("200m"),
			"memory": pulumi.String("800Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("1"),
			"memory": pulumi.String("2400Mi"),
		},
	}
	s.CloudWatchMetrics = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("25Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
	}
	s.PromNodeExporter = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("15Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
	}
	s.KubeStateMetrics = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("200m"),
			"memory": pulumi.String("300Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("400m"),
			"memory": pulumi.String("600Mi"),
		},
	}
	s.GrafanaSidecars = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("120Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("360Mi"),
		},
	}
	s.Grafana = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("600m"),
			"memory": pulumi.String("1200Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("1200m"),
			"memory": pulumi.String("2400Mi"),
		},
	}
	s.PromOperator = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("40Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("120Mi"),
		},
	}
	s.Prometheus = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("3000m"),
			"memory": pulumi.String("20Gi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("3500m"),
			"memory": pulumi.String("25Gi"),
		},
	}

	s.ThanosQueryResourcePreset = "large"
	s.ThanosQueryFrontendResourcePreset = "large"

	//Api resources
	s.OauthProxy = corev1.ResourceRequirementsArgs{
		Requests: pulumi.StringMap{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("50Mi"),
		},
		Limits: pulumi.StringMap{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
	}

	s.Api = corev1.ResourceRequirementsArgs{
		Requests: pulumi.StringMap{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("100Mi"),
		},
		Limits: pulumi.StringMap{
			"cpu":    pulumi.String("200m"),
			"memory": pulumi.String("200Mi"),
		},
	}
	s.TemporalAdminTools = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("50m"),
			"memory": pulumi.String("500Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("150m"),
			"memory": pulumi.String("1500Mi"),
		},
	}
	s.TemporalFrontend = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("55Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
	}
	s.TemporalHistory = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("60m"),
			"memory": pulumi.String("300Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("180m"),
			"memory": pulumi.String("900Mi"),
		},
	}
	s.TemporalMatching = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("60Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("180Mi"),
		},
	}
	s.TemporalWeb = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("50m"),
			"memory": pulumi.String("50Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("150m"),
			"memory": pulumi.String("150Mi"),
		},
	}

	s.TemporalWorker = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("55Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("155Mi"),
		},
	}

	s.DragonFly = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("200m"),
			"memory": pulumi.String("500Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("400m"),
			"memory": pulumi.String("1000Mi"),
		},
	}

	s.Runbooks = corev1.ResourceRequirementsArgs{
		Requests: pulumi.StringMap{
			"cpu":    pulumi.String("10m"),
			"memory": pulumi.String("15Mi"),
		},
		Limits: pulumi.StringMap{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
	}

}

func (s *ResourceConfig) InitResourcesProd() {
	s.CertManagerController = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("80m"),
			"memory": pulumi.String("120Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("200m"),
			"memory": pulumi.String("400Mi"),
		},
	}
	s.CertManagerCAInjector = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("50m"),
			"memory": pulumi.String("300Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("300m"),
			"memory": pulumi.String("1000Mi"),
		},
	}
	s.CertManagerWebhook = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("15Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
	}
	s.Crossplane = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("1900m"),
			"memory": pulumi.String("2000Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("3"),
			"memory": pulumi.String("3000Mi"),
		},
	}
	s.CrossplaneRBAC = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("30Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
	}
	s.IngressNginx = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("500m"),
			"memory": pulumi.String("2000Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("2500m"),
			"memory": pulumi.String("2500Mi"),
		},
	}
	s.Karpenter = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("150m"),
			"memory": pulumi.String("600Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("450m"),
			"memory": pulumi.String("1800Mi"),
		},
	}
	s.KubecostForecasting = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("250Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("210m"),
			"memory": pulumi.String("510Mi"),
		},
	}
	s.KubecostNetworkCosts = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("120m"),
			"memory": pulumi.String("60Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("360m"),
			"memory": pulumi.String("180Mi"),
		},
	}
	s.KubecostAggregator = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("1300m"),
			"memory": pulumi.String("2500Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("3600m"),
			"memory": pulumi.String("6000Mi"),
		},
	}
	s.KubecostCloudCost = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("80Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("240Mi"),
		},
	}
	s.KubecostFrontend = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("20Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("200m"),
			"memory": pulumi.String("200Mi"),
		},
	}
	s.KubecostModel = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("220m"),
			"memory": pulumi.String("600Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("660m"),
			"memory": pulumi.String("1800Mi"),
		},
	}

	s.FluentBit = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("30Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("120m"),
			"memory": pulumi.String("180Mi"),
		},
	}
	s.ElasticDataResourcePreset = pulumi.String("large")
	s.ElasticIngestResourcePreset = pulumi.String("medium")
	s.ElasticIngestHeapSize = pulumi.String("512m")
	s.ElasticCoordinatingResourcePreset = pulumi.String("medium")
	s.ElasticCoordinatingHeapSize = pulumi.String("512m")

	s.ElasticMetrics = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("20Mi"),
		},
	}

	s.EventsExporter = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("30m"),
			"memory": pulumi.String("100Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("120m"),
			"memory": pulumi.String("210Mi"),
		},
	}
	s.Kibana = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("200m"),
			"memory": pulumi.String("1000Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("1"),
			"memory": pulumi.String("3000Mi"),
		},
	}
	//MONITORING
	s.CloudWatchMetrics = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("25Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
	}
	s.PromNodeExporter = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("30m"),
			"memory": pulumi.String("30Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
	}
	s.KubeStateMetrics = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("200m"),
			"memory": pulumi.String("300Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("400m"),
			"memory": pulumi.String("600Mi"),
		},
	}
	s.GrafanaSidecars = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("150Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("450Mi"),
		},
	}
	s.Grafana = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("800m"),
			"memory": pulumi.String("1400Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("2000m"),
			"memory": pulumi.String("2400Mi"),
		},
	}
	s.PromOperator = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("20m"),
			"memory": pulumi.String("70Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("210Mi"),
		},
	}
	s.Prometheus = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("3000m"),
			"memory": pulumi.String("20Gi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("3500m"),
			"memory": pulumi.String("25Gi"),
		},
	}
	s.PromThanos = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("700m"),
			"memory": pulumi.String("100Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("2100m"),
			"memory": pulumi.String("300Mi"),
		},
	}

	s.ThanosQueryResourcePreset = "large"
	s.ThanosQueryFrontendResourcePreset = "large"

	s.OauthProxy = corev1.ResourceRequirementsArgs{
		Requests: pulumi.StringMap{
			"cpu":    pulumi.String("50m"),
			"memory": pulumi.String("50Mi"),
		},
		Limits: pulumi.StringMap{
			"cpu":    pulumi.String("150m"),
			"memory": pulumi.String("150Mi"),
		},
	}

	s.Api = corev1.ResourceRequirementsArgs{
		Requests: pulumi.StringMap{
			"cpu":    pulumi.String("200m"),
			"memory": pulumi.String("200Mi"),
		},
		Limits: pulumi.StringMap{
			"cpu":    pulumi.String("200m"),
			"memory": pulumi.String("1000Mi"),
		},
	}
	s.TemporalAdminTools = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("150m"),
			"memory": pulumi.String("500Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("300m"),
			"memory": pulumi.String("1500Mi"),
		},
	}
	s.TemporalFrontend = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("200Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("200m"),
			"memory": pulumi.String("2100Mi"),
		},
	}
	s.TemporalHistory = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("200m"),
			"memory": pulumi.String("400Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("400m"),
			"memory": pulumi.String("1050Mi"),
		},
	}
	s.TemporalMatching = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("120Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("200m"),
			"memory": pulumi.String("240Mi"),
		},
	}
	s.TemporalWeb = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("100m"),
			"memory": pulumi.String("150Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("200m"),
			"memory": pulumi.String("300Mi"),
		},
	}
	s.TemporalWorker = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("200m"),
			"memory": pulumi.String("200Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("400m"),
			"memory": pulumi.String("400Mi"),
		},
	}
	s.DragonFly = pulumi.Map{
		"requests": pulumi.Map{
			"cpu":    pulumi.String("200m"),
			"memory": pulumi.String("500Mi"),
		},
		"limits": pulumi.Map{
			"cpu":    pulumi.String("400m"),
			"memory": pulumi.String("1000Mi"),
		},
	}

}
