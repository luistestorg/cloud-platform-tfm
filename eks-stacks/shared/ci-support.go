package shared

import (
	"fmt"

	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/kustomize"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	autoscalingv2 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/autoscaling/v2"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	ActionsRunnerControllerChartVers = "0.9.3"
	TektonVers                       = "v0.72.0"
)

type (
	CiSupportSharedStack struct {
		GithubConfigURL               string
		GithubActionsAppID            pulumi.StringOutput
		GithubActionsAppInstID        pulumi.StringOutput
		GithubActionsPrivateKey       pulumi.StringOutput
		GithubHelmConfigUrl           string
		GithubUI3ConfigUrl            string
		EnableTekton                  bool
		EnableActionsRunnerController bool
		EnableHelmActionsSet          bool
		EnableUI3ActionsSet           bool
		EnableBuildBarn               bool
	}
)

func (ciSupportStack *CiSupportSharedStack) DeployActionsRunnerController(ctx *pulumi.Context, s *Stack) error {
	nsName := "arc-system"
	ns, err := s.CreateNamespace(ctx, nsName)
	if err != nil {
		return err
	}

	customCtrlValues := pulumi.Map{
		"nameOverride":     pulumi.String("arc"),
		"fullnameOverride": pulumi.String("arc"),
		"serviceAccount": pulumi.Map{
			"name": pulumi.String("arc"),
		},
		"resources": s.Resources.ArcController,
	}
	_, err = s.DeployHelmRelease(ctx, ns, "gha-runner-scale-set-controller", ActionsRunnerControllerChartVers, "", "", customCtrlValues)
	if err != nil {
		return err
	}

	// create the secret holding the PAT
	secret, err := corev1.NewSecret(ctx, "arc-github-app", &corev1.SecretArgs{
		ApiVersion: pulumi.String("v1"),
		Kind:       pulumi.String("Secret"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("arc-github-app"), Namespace: ns.Metadata.Name()},
		StringData: pulumi.StringMap{
			"github_app_id":              ciSupportStack.GithubActionsAppID,
			"github_app_installation_id": ciSupportStack.GithubActionsAppInstID,
			"github_app_private_key":     ciSupportStack.GithubActionsPrivateKey,
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{ns}))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, secret)

	// ARM
	installationARM := fmt.Sprintf("%s-arc-runner-set-arm64", s.ClusterName)
	customValuesARM := pulumi.Map{
		"githubConfigUrl":    pulumi.String(ciSupportStack.GithubConfigURL),
		"githubConfigSecret": secret.Metadata.Name(),
		"runnerScaleSetName": pulumi.String(installationARM),
		"maxRunners":         pulumi.Int(10),
		"minRunners":         pulumi.Int(0),
		"containerMode": pulumi.Map{
			"type": pulumi.String("dind"),
		},
		"template": ciSupportStack.CreateArcSpec(ctx, s.Platform, "arm64", s.Resources.ArcRunner),
	}
	_, err = s.DeployHelmRelease(ctx, ns, installationARM, ActionsRunnerControllerChartVers, "gha-runner-scale-set", "", customValuesARM)
	if err != nil {
		return err
	}

	// AMD / x86
	installationAMD := fmt.Sprintf("%s-arc-runner-set-amd64", s.ClusterName)
	customValuesAMD := pulumi.Map{
		"githubConfigUrl":    pulumi.String(ciSupportStack.GithubConfigURL),
		"githubConfigSecret": secret.Metadata.Name(),
		"runnerScaleSetName": pulumi.String(installationAMD),
		"maxRunners":         pulumi.Int(10),
		"minRunners":         pulumi.Int(0),
		"containerMode": pulumi.Map{
			"type": pulumi.String("dind"),
		},
		"template": ciSupportStack.CreateArcSpec(ctx, s.Platform, "amd64", s.Resources.ArcRunner),
	}
	_, err = s.DeployHelmRelease(ctx, ns, installationAMD, ActionsRunnerControllerChartVers, "gha-runner-scale-set", "", customValuesAMD)
	if err != nil {
		return err
	}

	if ciSupportStack.EnableHelmActionsSet {
		// AMD / x86
		installationHelmAMD := fmt.Sprintf("%s-arc-runner-set-helm-amd64", s.ClusterName)
		customValuesHelmAMD := pulumi.Map{
			"githubConfigUrl":    pulumi.String(ciSupportStack.GithubHelmConfigUrl),
			"githubConfigSecret": secret.Metadata.Name(),
			"runnerScaleSetName": pulumi.String(installationHelmAMD),
			"maxRunners":         pulumi.Int(10),
			"minRunners":         pulumi.Int(0),
			"containerMode": pulumi.Map{
				"type": pulumi.String("dind"),
			},
			"template": ciSupportStack.CreateArcSpec(ctx, s.Platform, "amd64", s.Resources.ArcRunner),
		}
		_, err = s.DeployHelmRelease(ctx, ns, installationHelmAMD, ActionsRunnerControllerChartVers, "gha-runner-scale-set", "", customValuesHelmAMD)
		if err != nil {
			return err
		}
	}

	if ciSupportStack.EnableUI3ActionsSet {
		// AMD / x86
		installationUI3AMD := fmt.Sprintf("%s-arc-runner-set-ui3-amd64", s.ClusterName)
		customValuesUI3AMD := pulumi.Map{
			"githubConfigUrl":    pulumi.String(ciSupportStack.GithubUI3ConfigUrl),
			"githubConfigSecret": secret.Metadata.Name(),
			"runnerScaleSetName": pulumi.String(installationUI3AMD),
			"maxRunners":         pulumi.Int(10),
			"minRunners":         pulumi.Int(0),
			"containerMode": pulumi.Map{
				"type": pulumi.String("dind"),
			},
			"template": ciSupportStack.CreateArcSpec(ctx, s.Platform, "amd64", s.Resources.ArcRunner),
		}
		_, err = s.DeployHelmRelease(ctx, ns, installationUI3AMD, ActionsRunnerControllerChartVers, "gha-runner-scale-set", "", customValuesUI3AMD)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ciSupportStack *CiSupportSharedStack) DeployTekton(ctx *pulumi.Context, s *Stack) error {
	_, err := yaml.NewConfigGroup(ctx, "tekton-operator", &yaml.ConfigGroupArgs{
		Files: []string{
			fmt.Sprintf("https://storage.googleapis.com/tekton-releases/operator/previous/%s/release.yaml", TektonVers),
			fmt.Sprintf("https://raw.githubusercontent.com/tektoncd/operator/%s/config/crs/kubernetes/config/all/operator_v1alpha1_config_cr.yaml", TektonVers),
		},
		Transformations: []yaml.Transformation{
			func(state map[string]interface{}, _ ...pulumi.ResourceOption) {
				if kind, kindExists := state["kind"].(string); kindExists &&
					kind == "TektonConfig" {
					spec, specExists := state["spec"].(map[string]interface{})
					if !specExists {
						return
					}
					pipeline, pipelineExists := spec["pipeline"].(map[string]interface{})
					if !pipelineExists {
						pipeline = map[string]interface{}{}
						spec["pipeline"] = pipeline
					}
					pipeline["disable-affinity-assistant"] = true
					pipeline["coschedule"] = "pipelineruns"
				}
			},
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	return err
}
func (ciSupportStack *CiSupportSharedStack) CreateArcSpec(ctx *pulumi.Context, platform string, arch string, resources pulumi.Map) pulumi.Map {
	if platform == "gke" {
		return pulumi.Map{
			"spec": pulumi.Map{
				"containers": pulumi.MapArray{
					pulumi.Map{
						"name":     pulumi.String("runner"),
						"resouces": resources,
						"image":    pulumi.String("ghcr.io/actions/actions-runner:latest"),
						"command": pulumi.StringArray{
							pulumi.String("/home/runner/run.sh"),
						},
					},
				},

				"affinity": pulumi.Map{
					"nodeAffinity": pulumi.Map{
						"preferredDuringSchedulingIgnoredDuringExecution": pulumi.MapArray{
							pulumi.Map{
								"preference": pulumi.Map{
									"matchExpressions": pulumi.MapArray{
										pulumi.Map{
											"key":      pulumi.String("cloud.google.com/gke-provisioning"),
											"operator": pulumi.String("In"),
											"values": pulumi.StringArray{
												pulumi.String("standard"),
											},
										},
									},
								},
								"weight": pulumi.Int(100),
							},
						},
					},
				},
				// Using false for disruptable because we currently don't have arm spots in GKE
				"nodeSelector": getNodeSelector(false, arch, "linux", ""),
				"tolerations":  nodeTolerationsByPlatformAndDistruptionType(platform, false),
				"securityContext": pulumi.Map{
					"fsGroup": pulumi.Int(123),
				},
			},
		}
	}
	//aws
	return pulumi.Map{
		"spec": pulumi.Map{
			"containers": pulumi.MapArray{
				pulumi.Map{
					"name":     pulumi.String("runner"),
					"resouces": resources,
					"image":    pulumi.String("ghcr.io/actions/actions-runner:latest"),
					"command": pulumi.StringArray{
						pulumi.String("/home/runner/run.sh"),
					},
				},
			},
			"affinity": pulumi.Map{
				"nodeAffinity": pulumi.Map{
					"preferredDuringSchedulingIgnoredDuringExecution": pulumi.MapArray{
						pulumi.Map{
							"preference": pulumi.Map{
								"matchExpressions": pulumi.MapArray{
									pulumi.Map{
										"key":      pulumi.String("eks.amazonaws.com/capacityType"),
										"operator": pulumi.String("In"),
										"values": pulumi.StringArray{
											pulumi.String("SPOT"),
										},
									},
								},
							},
							"weight": pulumi.Int(100),
						},
					},
				},
			},
			"nodeSelector": getNodeSelector(true, arch, "linux", ""),
			"tolerations":  nodeTolerationsByPlatformAndDistruptionType(platform, true),
			"securityContext": pulumi.Map{
				"fsGroup": pulumi.Int(123),
			},
		},
	}

}

func (ciSupportStack *CiSupportSharedStack) DeployBuildBarn(ctx *pulumi.Context, s *Stack, configPath string) error {

	ns, err := s.CreateNamespace(ctx, "buildbarn")
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, ns)

	builbarnConfig, err := kustomize.NewDirectory(ctx, "buildbarn",
		kustomize.DirectoryArgs{
			Directory: pulumi.String(configPath),
		},
	)
	if err != nil {
		return err
	}

	s.DependsOn = append(s.DependsOn, builbarnConfig)

	redirectURIBrowser := fmt.Sprintf("https://buildbrowser.%s/oauth2/callback", s.TLSCfg.Domain)
	opConfigMapBrowser, err := s.CreateOAuth2ProxyConfig(ctx, ns, "buildbrowser", redirectURIBrowser, 7984, "http")
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, opConfigMapBrowser)

	oauth2ProxyContainerBrowser := s.CreateOAuth2ProxyContainerArgs(opConfigMapBrowser.Metadata.Name(), "quay.io/oauth2-proxy/oauth2-proxy:v7.6.0")

	//Browser deployment

	buildbarnBrowserContainer := &corev1.ContainerArgs{
		Name:            pulumi.String("browser"),
		Image:           pulumi.String("ghcr.io/buildbarn/bb-browser:20240613T055327Z-f0fbe96"),
		ImagePullPolicy: pulumi.String("Always"),
		Ports: corev1.ContainerPortArray{
			&corev1.ContainerPortArgs{
				Name:          pulumi.String("browser"),
				Protocol:      pulumi.String("TCP"),
				ContainerPort: pulumi.Int(7984),
			},
		},
		Resources: &corev1.ResourceRequirementsArgs{
			Requests: pulumi.StringMap{
				"memory": pulumi.String("80Mi"),
				"cpu":    pulumi.String("20m"),
			},
		},
		Args: pulumi.StringArray{
			pulumi.String("/config/browser.jsonnet"),
		},
		VolumeMounts: &corev1.VolumeMountArray{
			&corev1.VolumeMountArgs{
				MountPath: pulumi.String("/config/"),
				Name:      pulumi.String("configs"),
				ReadOnly:  pulumi.Bool(true),
			},
		},
	}
	browserPodTemplate := &corev1.PodTemplateSpecArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Labels: pulumi.StringMap{"app": pulumi.String("browser")},
		},
		Spec: &corev1.PodSpecArgs{
			NodeSelector: pulumi.StringMap{
				"kubernetes.io/arch": pulumi.String("amd64"),
				"kubernetes.io/os":   pulumi.String("linux"),
				"node-role":          pulumi.String("autoscaled-ondemand")},
			Containers: corev1.ContainerArray{oauth2ProxyContainerBrowser, buildbarnBrowserContainer},
			Tolerations: corev1.TolerationArray{
				&corev1.TolerationArgs{
					Key:      pulumi.String("nativelink/tolerates-spot"),
					Operator: pulumi.String("Exists"),
					Effect:   pulumi.String("NoSchedule"),
				},
			},
			Volumes: &corev1.VolumeArray{
				&corev1.VolumeArgs{
					Name: pulumi.String("configs"),
					ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
						Name: pulumi.String("buildbarn-config"),
						Items: &corev1.KeyToPathArray{
							&corev1.KeyToPathArgs{
								Key:  pulumi.String("common.libsonnet"),
								Path: pulumi.String("common.libsonnet"),
							},
							&corev1.KeyToPathArgs{
								Key:  pulumi.String("browser.jsonnet"),
								Path: pulumi.String("browser.jsonnet"),
							},
						},
					},
				},
			},
		},
	}
	browserReplicas := 1
	browserDeploy, err := appsv1.NewDeployment(ctx, "browser", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("browser"), Namespace: ns.Metadata.Name()},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(browserReplicas),
			Selector: metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{"app": pulumi.String("browser")}},
			Template: browserPodTemplate,
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, browserDeploy)

	browserHpa, err := autoscalingv2.NewHorizontalPodAutoscaler(ctx, "browser",
		&autoscalingv2.HorizontalPodAutoscalerArgs{
			ApiVersion: pulumi.String("v2"),
			Kind:       pulumi.String("HorizontalPodAutoscaler"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("browser-hpa"),
				Namespace: pulumi.String("buildbarn"),
			},
			Spec: &autoscalingv2.HorizontalPodAutoscalerSpecArgs{
				MaxReplicas: pulumi.Int(3),
				MinReplicas: pulumi.Int(1),
				ScaleTargetRef: &autoscalingv2.CrossVersionObjectReferenceArgs{
					ApiVersion: pulumi.String("apps/v1"),
					Kind:       pulumi.String("Deployment"),
					Name:       pulumi.String("browser-hpa"),
				},
				Behavior: &autoscalingv2.HorizontalPodAutoscalerBehaviorArgs{
					ScaleUp: &autoscalingv2.HPAScalingRulesArgs{
						SelectPolicy:               pulumi.String("Max"),
						StabilizationWindowSeconds: pulumi.Int(60),
						Policies: &autoscalingv2.HPAScalingPolicyArray{
							&autoscalingv2.HPAScalingPolicyArgs{
								PeriodSeconds: pulumi.Int(15),
								Type:          pulumi.String("Percent"),
								Value:         pulumi.Int(100),
							},
							&autoscalingv2.HPAScalingPolicyArgs{
								PeriodSeconds: pulumi.Int(15),
								Type:          pulumi.String("Pods"),
								Value:         pulumi.Int(4),
							},
						},
					},
				},
				Metrics: &autoscalingv2.MetricSpecArray{
					&autoscalingv2.MetricSpecArgs{
						Resource: &autoscalingv2.ResourceMetricSourceArgs{
							Name: pulumi.String("cpu"),
							Target: &autoscalingv2.MetricTargetArgs{
								AverageUtilization: pulumi.Int(85),
								Type:               pulumi.String("Utilization"),
							},
						},
						Type: pulumi.String("Resource"),
					},
					&autoscalingv2.MetricSpecArgs{
						Resource: &autoscalingv2.ResourceMetricSourceArgs{
							Name: pulumi.String("memory"),
							Target: &autoscalingv2.MetricTargetArgs{
								AverageUtilization: pulumi.Int(80),
								Type:               pulumi.String("Utilization"),
							},
						},
						Type: pulumi.String("Resource"),
					},
				},
			},
		})
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, browserHpa)

	browserSvc, err := corev1.NewService(ctx, "browser", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("browser"),
			Namespace: ns.Metadata.Name(),
			Annotations: pulumi.StringMap{
				"prometheus.io/port":   pulumi.String("80"),
				"prometheus.io/scrape": pulumi.String("true"),
			},
		},
		Spec: &corev1.ServiceSpecArgs{
			Selector: pulumi.StringMap{"app": pulumi.String("browser")},
			Ports: &corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("proxy"),
					Port:       pulumi.Int(7984),
					Protocol:   pulumi.String("TCP"),
					TargetPort: pulumi.String("proxy"),
				},
			},
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, browserSvc)

	// expose browser over https
	browserHost, err := s.CreateIngress(ctx, "buildbrowser", "buildbarn", "browser", 7984, nil)
	if err != nil {
		return err
	}
	fmt.Printf("Deployed ingress for host: %s\n", browserHost)

	//Frontend deployment

	buildbarnFrontEndContainer := &corev1.ContainerArgs{
		Name:            pulumi.String("frontend"),
		Image:           pulumi.String("ghcr.io/buildbarn/bb-storage:20240810T092106Z-3f5e30c"),
		ImagePullPolicy: pulumi.String("Always"),
		Ports: corev1.ContainerPortArray{
			&corev1.ContainerPortArgs{
				Name:          pulumi.String("frontend"),
				Protocol:      pulumi.String("TCP"),
				ContainerPort: pulumi.Int(8980),
			},
		},
		Resources: &corev1.ResourceRequirementsArgs{
			Requests: pulumi.StringMap{
				"memory": pulumi.String("80Mi"),
				"cpu":    pulumi.String("20m"),
			},
		},
		Args: pulumi.StringArray{
			pulumi.String("/config/frontend.jsonnet"),
		},
		VolumeMounts: &corev1.VolumeMountArray{
			&corev1.VolumeMountArgs{
				MountPath: pulumi.String("/config/"),
				Name:      pulumi.String("configs"),
				ReadOnly:  pulumi.Bool(true),
			},
		},
	}

	frontendPodTemplate := &corev1.PodTemplateSpecArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Labels: pulumi.StringMap{"app": pulumi.String("frontend")},
		},
		Spec: &corev1.PodSpecArgs{
			NodeSelector: pulumi.StringMap{
				"kubernetes.io/arch": pulumi.String("amd64"),
				"kubernetes.io/os":   pulumi.String("linux"),
				"node-role":          pulumi.String("autoscaled-ondemand")},
			Containers: corev1.ContainerArray{buildbarnFrontEndContainer},
			Tolerations: corev1.TolerationArray{
				&corev1.TolerationArgs{
					Key:      pulumi.String("nativelink/tolerates-spot"),
					Operator: pulumi.String("Exists"),
					Effect:   pulumi.String("NoSchedule"),
				},
			},
			Volumes: &corev1.VolumeArray{
				&corev1.VolumeArgs{
					Name: pulumi.String("configs"),
					ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
						Name: pulumi.String("buildbarn-config"),
						Items: &corev1.KeyToPathArray{
							&corev1.KeyToPathArgs{
								Key:  pulumi.String("common.libsonnet"),
								Path: pulumi.String("common.libsonnet"),
							},
							&corev1.KeyToPathArgs{
								Key:  pulumi.String("frontend.jsonnet"),
								Path: pulumi.String("frontend.jsonnet"),
							},
						},
					},
				},
			},
		},
	}

	frontendReplicas := 1
	frontendDeploy, err := appsv1.NewDeployment(ctx, "frontend", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("frontend"), Namespace: ns.Metadata.Name()},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(frontendReplicas),
			Selector: metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{"app": pulumi.String("frontend")}},
			Template: frontendPodTemplate,
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, frontendDeploy)

	frontendHpa, err := autoscalingv2.NewHorizontalPodAutoscaler(ctx, "frontend",
		&autoscalingv2.HorizontalPodAutoscalerArgs{
			ApiVersion: pulumi.String("v2"),
			Kind:       pulumi.String("HorizontalPodAutoscaler"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("frontend-hpa"),
				Namespace: pulumi.String("buildbarn"),
			},
			Spec: &autoscalingv2.HorizontalPodAutoscalerSpecArgs{
				MaxReplicas: pulumi.Int(3),
				MinReplicas: pulumi.Int(1),
				ScaleTargetRef: &autoscalingv2.CrossVersionObjectReferenceArgs{
					ApiVersion: pulumi.String("apps/v1"),
					Kind:       pulumi.String("Deployment"),
					Name:       pulumi.String("frontend-hpa"),
				},
				Behavior: &autoscalingv2.HorizontalPodAutoscalerBehaviorArgs{
					ScaleUp: &autoscalingv2.HPAScalingRulesArgs{
						SelectPolicy:               pulumi.String("Max"),
						StabilizationWindowSeconds: pulumi.Int(60),
						Policies: &autoscalingv2.HPAScalingPolicyArray{
							&autoscalingv2.HPAScalingPolicyArgs{
								PeriodSeconds: pulumi.Int(15),
								Type:          pulumi.String("Percent"),
								Value:         pulumi.Int(100),
							},
							&autoscalingv2.HPAScalingPolicyArgs{
								PeriodSeconds: pulumi.Int(15),
								Type:          pulumi.String("Pods"),
								Value:         pulumi.Int(4),
							},
						},
					},
				},
				Metrics: &autoscalingv2.MetricSpecArray{
					&autoscalingv2.MetricSpecArgs{
						Resource: &autoscalingv2.ResourceMetricSourceArgs{
							Name: pulumi.String("cpu"),
							Target: &autoscalingv2.MetricTargetArgs{
								AverageUtilization: pulumi.Int(85),
								Type:               pulumi.String("Utilization"),
							},
						},
						Type: pulumi.String("Resource"),
					},
					&autoscalingv2.MetricSpecArgs{
						Resource: &autoscalingv2.ResourceMetricSourceArgs{
							Name: pulumi.String("memory"),
							Target: &autoscalingv2.MetricTargetArgs{
								AverageUtilization: pulumi.Int(80),
								Type:               pulumi.String("Utilization"),
							},
						},
						Type: pulumi.String("Resource"),
					},
				},
			},
		})
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, frontendHpa)

	frontendSvc, err := corev1.NewService(ctx, "frontend", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("frontend"),
			Namespace: ns.Metadata.Name(),
			Annotations: pulumi.StringMap{
				"prometheus.io/port":   pulumi.String("80"),
				"prometheus.io/scrape": pulumi.String("true"),
			},
		},
		Spec: &corev1.ServiceSpecArgs{
			Selector: pulumi.StringMap{"app": pulumi.String("frontend")},
			Type:     pulumi.String("LoadBalancer"),
			Ports: &corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("proxy"),
					Port:       pulumi.Int(8980),
					Protocol:   pulumi.String("TCP"),
					TargetPort: pulumi.Int(8980),
				},
			},
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, frontendSvc)

	//Scheduler deployment

	redirectURIScheduler := fmt.Sprintf("https://buildscheduler.%s/oauth2/callback", s.TLSCfg.Domain)
	opConfigMapScheduler, err := s.CreateOAuth2ProxyConfig(ctx, ns, "buildscheduler", redirectURIScheduler, 7982, "http")
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, opConfigMapBrowser)

	oauth2ProxyContainerScheduler := s.CreateOAuth2ProxyContainerArgs(opConfigMapScheduler.Metadata.Name(), "quay.io/oauth2-proxy/oauth2-proxy:v7.6.0")

	schedulerContainer := &corev1.ContainerArgs{
		Name:            pulumi.String("scheduler"),
		Image:           pulumi.String("ghcr.io/buildbarn/bb-scheduler:20240716T044555Z-9850e82"),
		ImagePullPolicy: pulumi.String("Always"),
		Ports: corev1.ContainerPortArray{
			&corev1.ContainerPortArgs{
				Protocol:      pulumi.String("TCP"),
				ContainerPort: pulumi.Int(8982),
			},
			&corev1.ContainerPortArgs{
				Protocol:      pulumi.String("TCP"),
				ContainerPort: pulumi.Int(8983),
			},
			&corev1.ContainerPortArgs{
				Protocol:      pulumi.String("TCP"),
				ContainerPort: pulumi.Int(7982),
			},
		},
		Resources: &corev1.ResourceRequirementsArgs{
			Requests: pulumi.StringMap{
				"memory": pulumi.String("80Mi"),
				"cpu":    pulumi.String("20m"),
			},
		},
		Args: pulumi.StringArray{
			pulumi.String("/config/scheduler.jsonnet"),
		},
		VolumeMounts: &corev1.VolumeMountArray{
			&corev1.VolumeMountArgs{
				MountPath: pulumi.String("/config/"),
				Name:      pulumi.String("configs"),
				ReadOnly:  pulumi.Bool(true),
			},
		},
	}

	schedulerPodTemplate := &corev1.PodTemplateSpecArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Labels: pulumi.StringMap{"app": pulumi.String("scheduler")},
		},
		Spec: &corev1.PodSpecArgs{
			NodeSelector: pulumi.StringMap{
				"kubernetes.io/arch": pulumi.String("amd64"),
				"kubernetes.io/os":   pulumi.String("linux"),
				"node-role":          pulumi.String("autoscaled-ondemand")},
			Containers: corev1.ContainerArray{oauth2ProxyContainerScheduler, schedulerContainer},
			Tolerations: corev1.TolerationArray{
				&corev1.TolerationArgs{
					Key:      pulumi.String("nativelink/tolerates-spot"),
					Operator: pulumi.String("Exists"),
					Effect:   pulumi.String("NoSchedule"),
				},
			},
			Volumes: &corev1.VolumeArray{
				&corev1.VolumeArgs{
					Name: pulumi.String("configs"),
					ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
						Name: pulumi.String("buildbarn-config"),
						Items: &corev1.KeyToPathArray{
							&corev1.KeyToPathArgs{
								Key:  pulumi.String("common.libsonnet"),
								Path: pulumi.String("common.libsonnet"),
							},
							&corev1.KeyToPathArgs{
								Key:  pulumi.String("scheduler.jsonnet"),
								Path: pulumi.String("scheduler.jsonnet"),
							},
						},
					},
				},
			},
		},
	}
	schedulerReplicas := 1
	schedulerDeploy, err := appsv1.NewDeployment(ctx, "scheduler", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("scheduler"), Namespace: ns.Metadata.Name()},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(schedulerReplicas),
			Selector: metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{"app": pulumi.String("scheduler")}},
			Template: schedulerPodTemplate,
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, schedulerDeploy)

	schedulerSvc, err := corev1.NewService(ctx, "scheduler", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("scheduler"),
			Namespace: ns.Metadata.Name(),
			Annotations: pulumi.StringMap{
				"prometheus.io/port":   pulumi.String("80"),
				"prometheus.io/scrape": pulumi.String("true"),
			},
		},
		Spec: &corev1.ServiceSpecArgs{
			Selector: pulumi.StringMap{"app": pulumi.String("scheduler")},
			Type:     pulumi.String("ClusterIP"),
			Ports: &corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("http"),
					Port:       pulumi.Int(7982),
					Protocol:   pulumi.String("TCP"),
					TargetPort: pulumi.String("proxy"),
				},
				&corev1.ServicePortArgs{
					Name:     pulumi.String("client-grpc"),
					Port:     pulumi.Int(8982),
					Protocol: pulumi.String("TCP"),
				},
				&corev1.ServicePortArgs{
					Name:     pulumi.String("worker-grpc"),
					Port:     pulumi.Int(8983),
					Protocol: pulumi.String("TCP"),
				},
			},
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, schedulerSvc)

	// expose scheduler over https
	schedulerHost, err := s.CreateIngress(ctx, "buildscheduler", "buildbarn", "scheduler", 7982, nil)
	if err != nil {
		return err
	}
	fmt.Printf("Deployed ingress for host: %s\n", schedulerHost)

	// Storage statefulset

	storageContainer := &corev1.ContainerArgs{
		Name:            pulumi.String("storage"),
		Image:           pulumi.String("ghcr.io/buildbarn/bb-storage:20240810T092106Z-3f5e30c"),
		ImagePullPolicy: pulumi.String("Always"),
		Resources: &corev1.ResourceRequirementsArgs{
			Requests: pulumi.StringMap{
				"memory": pulumi.String("80Mi"),
				"cpu":    pulumi.String("20m"),
			},
		},
		Args: pulumi.StringArray{
			pulumi.String("/config/storage.jsonnet"),
		},
		VolumeMounts: &corev1.VolumeMountArray{
			&corev1.VolumeMountArgs{
				MountPath: pulumi.String("/config/"),
				Name:      pulumi.String("configs"),
				ReadOnly:  pulumi.Bool(true),
			},
			&corev1.VolumeMountArgs{
				MountPath: pulumi.String("/storage-cas"),
				Name:      pulumi.String("cas"),
			},
			&corev1.VolumeMountArgs{
				MountPath: pulumi.String("/storage-ac"),
				Name:      pulumi.String("ac"),
			},
		},
		Ports: &corev1.ContainerPortArray{
			&corev1.ContainerPortArgs{
				ContainerPort: pulumi.Int(8981),
				Protocol:      pulumi.String("TCP"),
			},
		},
	}

	volumeStorageInitContainer := &corev1.ContainerArgs{
		Name:            pulumi.String("volume-init"),
		Image:           pulumi.String("busybox:1.31.1-uclibc"),
		ImagePullPolicy: pulumi.String("Always"),
		Resources: &corev1.ResourceRequirementsArgs{
			Requests: pulumi.StringMap{
				"memory": pulumi.String("80Mi"),
				"cpu":    pulumi.String("20m"),
			},
		},
		VolumeMounts: &corev1.VolumeMountArray{
			&corev1.VolumeMountArgs{
				MountPath: pulumi.String("/storage-cas"),
				Name:      pulumi.String("cas"),
			},
			&corev1.VolumeMountArgs{
				MountPath: pulumi.String("/storage-ac"),
				Name:      pulumi.String("ac"),
			},
		},
		Command: pulumi.StringArray{
			pulumi.String("sh"),
			pulumi.String("-c"),
			pulumi.String("mkdir -m 0700 -p /storage-cas/persistent_state /storage-ac/persistent_state"),
		},
	}

	storagePodTemplate := &corev1.PodTemplateSpecArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Labels: pulumi.StringMap{"app": pulumi.String("storage")},
		},
		Spec: &corev1.PodSpecArgs{
			NodeSelector: pulumi.StringMap{
				"kubernetes.io/arch": pulumi.String("amd64"),
				"kubernetes.io/os":   pulumi.String("linux"),
				"node-role":          pulumi.String("autoscaled-ondemand")},
			Containers:     corev1.ContainerArray{storageContainer},
			InitContainers: corev1.ContainerArray{volumeStorageInitContainer},
			Tolerations: corev1.TolerationArray{
				&corev1.TolerationArgs{
					Key:      pulumi.String("nativelink/tolerates-spot"),
					Operator: pulumi.String("Exists"),
					Effect:   pulumi.String("NoSchedule"),
				},
			},
			Volumes: &corev1.VolumeArray{
				&corev1.VolumeArgs{
					Name: pulumi.String("configs"),
					ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
						Name: pulumi.String("buildbarn-config"),
						Items: &corev1.KeyToPathArray{
							&corev1.KeyToPathArgs{
								Key:  pulumi.String("common.libsonnet"),
								Path: pulumi.String("common.libsonnet"),
							},
							&corev1.KeyToPathArgs{
								Key:  pulumi.String("storage.jsonnet"),
								Path: pulumi.String("storage.jsonnet"),
							},
						},
					},
				},
			},
		},
	}

	storageReplicas := 1
	storageStatefulset, err := appsv1.NewStatefulSet(ctx, "storage", &appsv1.StatefulSetArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("storage"), Namespace: ns.Metadata.Name()},
		Spec: &appsv1.StatefulSetSpecArgs{
			Replicas: pulumi.Int(storageReplicas),
			Selector: metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{"app": pulumi.String("storage")}},
			Template: storagePodTemplate,
			VolumeClaimTemplates: &corev1.PersistentVolumeClaimTypeArray{
				&corev1.PersistentVolumeClaimTypeArgs{
					Metadata: &metav1.ObjectMetaArgs{
						Name: pulumi.String("cas"),
					},
					Spec: &corev1.PersistentVolumeClaimSpecArgs{
						StorageClassName: pulumi.String("standard"),
						AccessModes: pulumi.StringArray{
							pulumi.String("ReadWriteOnce"),
						},
						Resources: &corev1.VolumeResourceRequirementsArgs{
							Requests: pulumi.StringMap{
								"storage": pulumi.String("33Gi"),
							},
						},
					},
				},
				&corev1.PersistentVolumeClaimTypeArgs{
					Metadata: &metav1.ObjectMetaArgs{
						Name: pulumi.String("ac"),
					},
					Spec: &corev1.PersistentVolumeClaimSpecArgs{
						StorageClassName: pulumi.String("standard"),
						AccessModes: pulumi.StringArray{
							pulumi.String("ReadWriteOnce"),
						},
						Resources: &corev1.VolumeResourceRequirementsArgs{
							Requests: pulumi.StringMap{
								"storage": pulumi.String("1Gi"),
							},
						},
					},
				},
			},
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, storageStatefulset)

	storageHpa, err := autoscalingv2.NewHorizontalPodAutoscaler(ctx, "storage",
		&autoscalingv2.HorizontalPodAutoscalerArgs{
			ApiVersion: pulumi.String("v2"),
			Kind:       pulumi.String("HorizontalPodAutoscaler"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("storage-hpa"),
				Namespace: pulumi.String("buildbarn"),
			},
			Spec: &autoscalingv2.HorizontalPodAutoscalerSpecArgs{
				MaxReplicas: pulumi.Int(2),
				MinReplicas: pulumi.Int(1),
				ScaleTargetRef: &autoscalingv2.CrossVersionObjectReferenceArgs{
					ApiVersion: pulumi.String("apps/v1"),
					Kind:       pulumi.String("Statefulset"),
					Name:       pulumi.String("storage-hpa"),
				},
				Behavior: &autoscalingv2.HorizontalPodAutoscalerBehaviorArgs{
					ScaleUp: &autoscalingv2.HPAScalingRulesArgs{
						SelectPolicy:               pulumi.String("Max"),
						StabilizationWindowSeconds: pulumi.Int(60),
						Policies: &autoscalingv2.HPAScalingPolicyArray{
							&autoscalingv2.HPAScalingPolicyArgs{
								PeriodSeconds: pulumi.Int(15),
								Type:          pulumi.String("Percent"),
								Value:         pulumi.Int(100),
							},
							&autoscalingv2.HPAScalingPolicyArgs{
								PeriodSeconds: pulumi.Int(15),
								Type:          pulumi.String("Pods"),
								Value:         pulumi.Int(4),
							},
						},
					},
				},
				Metrics: &autoscalingv2.MetricSpecArray{
					&autoscalingv2.MetricSpecArgs{
						Resource: &autoscalingv2.ResourceMetricSourceArgs{
							Name: pulumi.String("cpu"),
							Target: &autoscalingv2.MetricTargetArgs{
								AverageUtilization: pulumi.Int(85),
								Type:               pulumi.String("Utilization"),
							},
						},
						Type: pulumi.String("Resource"),
					},
					&autoscalingv2.MetricSpecArgs{
						Resource: &autoscalingv2.ResourceMetricSourceArgs{
							Name: pulumi.String("memory"),
							Target: &autoscalingv2.MetricTargetArgs{
								AverageUtilization: pulumi.Int(80),
								Type:               pulumi.String("Utilization"),
							},
						},
						Type: pulumi.String("Resource"),
					},
				},
			},
		})
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, storageHpa)

	storageSvc, err := corev1.NewService(ctx, "storage", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("storage"),
			Namespace: ns.Metadata.Name(),
			Annotations: pulumi.StringMap{
				"prometheus.io/port":   pulumi.String("80"),
				"prometheus.io/scrape": pulumi.String("true"),
			},
		},
		Spec: &corev1.ServiceSpecArgs{
			Selector: pulumi.StringMap{"app": pulumi.String("storage")},
			Type:     pulumi.String("ClusterIP"),
			Ports: &corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:     pulumi.String("grpc"),
					Port:     pulumi.Int(8981),
					Protocol: pulumi.String("TCP"),
				},
			},
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, storageSvc)

	// worker-ubuntu22-04 deployment

	workerContainer := &corev1.ContainerArgs{
		Name:            pulumi.String("worker"),
		Image:           pulumi.String("ghcr.io/buildbarn/bb-worker:20240716T044555Z-9850e82"),
		ImagePullPolicy: pulumi.String("Always"),
		Resources: &corev1.ResourceRequirementsArgs{
			Requests: pulumi.StringMap{
				"memory": pulumi.String("80Mi"),
				"cpu":    pulumi.String("20m"),
			},
		},
		Args: pulumi.StringArray{
			pulumi.String("/config/worker-ubuntu22-04.jsonnet"),
		},
		VolumeMounts: &corev1.VolumeMountArray{
			&corev1.VolumeMountArgs{
				MountPath: pulumi.String("/config/"),
				Name:      pulumi.String("configs"),
				ReadOnly:  pulumi.Bool(true),
			},
			&corev1.VolumeMountArgs{
				MountPath: pulumi.String("/worker"),
				Name:      pulumi.String("worker"),
			},
		},
		Env: &corev1.EnvVarArray{
			&corev1.EnvVarArgs{
				Name: pulumi.String("NODE_NAME"),
				ValueFrom: &corev1.EnvVarSourceArgs{
					FieldRef: &corev1.ObjectFieldSelectorArgs{
						FieldPath: pulumi.String("spec.nodeName"),
					},
				},
			},
			&corev1.EnvVarArgs{
				Name: pulumi.String("POD_NAME"),
				ValueFrom: &corev1.EnvVarSourceArgs{
					FieldRef: &corev1.ObjectFieldSelectorArgs{
						FieldPath: pulumi.String("metadata.name"),
					},
				},
			},
		},
	}

	runnerContainer := &corev1.ContainerArgs{
		Name:            pulumi.String("runner"),
		Image:           pulumi.String("ghcr.io/catthehacker/ubuntu:act-22.04@sha256:5f9c35c25db1d51a8ddaae5c0ba8d3c163c5e9a4a6cc97acd409ac7eae239448"),
		ImagePullPolicy: pulumi.String("Always"),
		SecurityContext: &corev1.SecurityContextArgs{
			RunAsUser:                pulumi.Int(65534),
			AllowPrivilegeEscalation: pulumi.Bool(false),
		},
		Resources: &corev1.ResourceRequirementsArgs{
			Requests: pulumi.StringMap{
				"memory": pulumi.String("80Mi"),
				"cpu":    pulumi.String("20m"),
			},
		},
		VolumeMounts: &corev1.VolumeMountArray{
			&corev1.VolumeMountArgs{
				MountPath: pulumi.String("/config/"),
				Name:      pulumi.String("configs"),
				ReadOnly:  pulumi.Bool(true),
			},
			&corev1.VolumeMountArgs{
				MountPath: pulumi.String("/worker"),
				Name:      pulumi.String("worker"),
			},
			&corev1.VolumeMountArgs{
				MountPath: pulumi.String("/bb"),
				Name:      pulumi.String("empty"),
				ReadOnly:  pulumi.Bool(true),
			},
		},
		Command: pulumi.StringArray{
			pulumi.String("/bb/tini"),
			pulumi.String("-v"),
			pulumi.String("--"),
			pulumi.String("/bb/bb_runner"),
			pulumi.String("/config/runner-ubuntu22-04.jsonnet"),
		},
	}

	installerInitContainer := &corev1.ContainerArgs{
		Name:            pulumi.String("bb-runner-installer"),
		Image:           pulumi.String("ghcr.io/buildbarn/bb-runner-installer:20240716T044555Z-9850e82"),
		ImagePullPolicy: pulumi.String("Always"),
		Resources: &corev1.ResourceRequirementsArgs{
			Requests: pulumi.StringMap{
				"memory": pulumi.String("80Mi"),
				"cpu":    pulumi.String("20m"),
			},
		},
		VolumeMounts: &corev1.VolumeMountArray{
			&corev1.VolumeMountArgs{
				MountPath: pulumi.String("/bb/"),
				Name:      pulumi.String("empty"),
			},
		},
	}

	volumeInitContainer := &corev1.ContainerArgs{
		Name:            pulumi.String("volume-init"),
		Image:           pulumi.String("busybox:1.31.1-uclibc"),
		ImagePullPolicy: pulumi.String("Always"),
		Resources: &corev1.ResourceRequirementsArgs{
			Requests: pulumi.StringMap{
				"memory": pulumi.String("80Mi"),
				"cpu":    pulumi.String("20m"),
			},
		},
		Command: pulumi.StringArray{
			pulumi.String("sh"),
			pulumi.String("-c"),
			pulumi.String("mkdir -pm 0777 /worker/build && mkdir -pm 0700 /worker/cache && chmod 0777 /worker"),
		},
		VolumeMounts: &corev1.VolumeMountArray{
			&corev1.VolumeMountArgs{
				MountPath: pulumi.String("/worker"),
				Name:      pulumi.String("worker"),
			},
		},
	}

	workerPodTemplate := &corev1.PodTemplateSpecArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Labels: pulumi.StringMap{"app": pulumi.String("worker")},
		},
		Spec: &corev1.PodSpecArgs{
			NodeSelector: pulumi.StringMap{
				"kubernetes.io/arch": pulumi.String("amd64"),
				"kubernetes.io/os":   pulumi.String("linux"),
				"node-role":          pulumi.String("autoscaled-ondemand")},
			InitContainers: corev1.ContainerArray{installerInitContainer, volumeInitContainer},
			Containers:     corev1.ContainerArray{workerContainer, runnerContainer},
			Tolerations: corev1.TolerationArray{
				&corev1.TolerationArgs{
					Key:      pulumi.String("nativelink/tolerates-spot"),
					Operator: pulumi.String("Exists"),
					Effect:   pulumi.String("NoSchedule"),
				},
			},
			Volumes: &corev1.VolumeArray{
				&corev1.VolumeArgs{
					Name:     pulumi.String("empty"),
					EmptyDir: &corev1.EmptyDirVolumeSourceArgs{},
				},
				&corev1.VolumeArgs{
					Name:     pulumi.String("worker"),
					EmptyDir: &corev1.EmptyDirVolumeSourceArgs{},
				},
				&corev1.VolumeArgs{
					Name: pulumi.String("configs"),
					ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
						Name: pulumi.String("buildbarn-config"),
						Items: &corev1.KeyToPathArray{
							&corev1.KeyToPathArgs{
								Key:  pulumi.String("common.libsonnet"),
								Path: pulumi.String("common.libsonnet"),
							},
							&corev1.KeyToPathArgs{
								Key:  pulumi.String("runner-ubuntu22-04.jsonnet"),
								Path: pulumi.String("runner-ubuntu22-04.jsonnet"),
							},
							&corev1.KeyToPathArgs{
								Key:  pulumi.String("worker-ubuntu22-04.jsonnet"),
								Path: pulumi.String("worker-ubuntu22-04.jsonnet"),
							},
						},
					},
				},
			},
		},
	}
	workerReplicas := 1
	workerDeploy, err := appsv1.NewDeployment(ctx, "worker", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("worker"), Namespace: ns.Metadata.Name()},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(workerReplicas),
			Selector: metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{"app": pulumi.String("worker")}},
			Template: workerPodTemplate,
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, workerDeploy)

	workerHpa, err := autoscalingv2.NewHorizontalPodAutoscaler(ctx, "worker",
		&autoscalingv2.HorizontalPodAutoscalerArgs{
			ApiVersion: pulumi.String("v2"),
			Kind:       pulumi.String("HorizontalPodAutoscaler"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("worker-hpa"),
				Namespace: pulumi.String("buildbarn"),
			},
			Spec: &autoscalingv2.HorizontalPodAutoscalerSpecArgs{
				MaxReplicas: pulumi.Int(8),
				MinReplicas: pulumi.Int(1),
				ScaleTargetRef: &autoscalingv2.CrossVersionObjectReferenceArgs{
					ApiVersion: pulumi.String("apps/v1"),
					Kind:       pulumi.String("Deployment"),
					Name:       pulumi.String("worker-hpa"),
				},
				Behavior: &autoscalingv2.HorizontalPodAutoscalerBehaviorArgs{
					ScaleUp: &autoscalingv2.HPAScalingRulesArgs{
						SelectPolicy:               pulumi.String("Max"),
						StabilizationWindowSeconds: pulumi.Int(60),
						Policies: &autoscalingv2.HPAScalingPolicyArray{
							&autoscalingv2.HPAScalingPolicyArgs{
								PeriodSeconds: pulumi.Int(15),
								Type:          pulumi.String("Percent"),
								Value:         pulumi.Int(100),
							},
							&autoscalingv2.HPAScalingPolicyArgs{
								PeriodSeconds: pulumi.Int(15),
								Type:          pulumi.String("Pods"),
								Value:         pulumi.Int(4),
							},
						},
					},
				},
				Metrics: &autoscalingv2.MetricSpecArray{
					&autoscalingv2.MetricSpecArgs{
						Resource: &autoscalingv2.ResourceMetricSourceArgs{
							Name: pulumi.String("cpu"),
							Target: &autoscalingv2.MetricTargetArgs{
								AverageUtilization: pulumi.Int(85),
								Type:               pulumi.String("Utilization"),
							},
						},
						Type: pulumi.String("Resource"),
					},
					&autoscalingv2.MetricSpecArgs{
						Resource: &autoscalingv2.ResourceMetricSourceArgs{
							Name: pulumi.String("memory"),
							Target: &autoscalingv2.MetricTargetArgs{
								AverageUtilization: pulumi.Int(80),
								Type:               pulumi.String("Utilization"),
							},
						},
						Type: pulumi.String("Resource"),
					},
				},
			},
		})
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, workerHpa)

	return nil

}
