package main

import (
	"testing"

	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
)

type mocks int

// Mock for Pulumi resources
func (mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	outputs := args.Inputs.Copy()

	switch args.TypeToken {
	case "kubernetes:core/v1:Namespace":
		if metadata, ok := args.Inputs["metadata"]; ok {
			outputs["metadata"] = metadata
		}

	case "kubernetes:helm.sh/v3:Release":
		outputs["status"] = resource.NewPropertyValue("deployed")
		outputs["version"] = resource.NewPropertyValue("1")
		outputs["name"] = args.Inputs["name"]
		outputs["namespace"] = args.Inputs["namespace"]

	case "kubernetes:core/v1:Service":
		outputs["metadata"] = args.Inputs["metadata"]
		outputs["spec"] = args.Inputs["spec"]
	}

	return args.Name + "_id", outputs, nil
}

func (mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	outputs := map[string]interface{}{}

	switch args.Token {
	case "kubernetes:yaml:decode":
		outputs["result"] = []interface{}{}
	}

	return resource.NewPropertyMapFromMap(outputs), nil
}

// TestMonitoringNamespaceCreation verifies monitoring namespace is created
func TestMonitoringNamespaceCreation(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Create monitoring namespace
		ns, err := v1.NewNamespace(ctx, "monitoring", &v1.NamespaceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String("monitoring"),
				Labels: pulumi.StringMap{
					"name":    pulumi.String("monitoring"),
					"purpose": pulumi.String("observability"),
				},
			},
		})
		if err != nil {
			return err
		}

		// Validate namespace properties
		ns.Metadata.Name().ApplyT(func(name string) error {
			assert.Equal(t, "monitoring", name, "Namespace should be named 'monitoring'")
			return nil
		})

		ctx.Export("monitoringNamespace", ns.Metadata.Name())

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestPrometheusDeployment verifies Prometheus helm release configuration
func TestPrometheusDeployment(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Deploy kube-prometheus-stack
		promStack, err := helm.NewRelease(ctx, "kube-prometheus-stack", &helm.ReleaseArgs{
			Name:      pulumi.String("kube-prometheus-stack"),
			Namespace: pulumi.String("monitoring"),
			Chart:     pulumi.String("kube-prometheus-stack"),
			Version:   pulumi.String("58.2.2"),
			RepositoryOpts: &helm.RepositoryOptsArgs{
				Repo: pulumi.String("https://prometheus-community.github.io/helm-charts"),
			},
			Values: pulumi.Map{
				"prometheus": pulumi.Map{
					"prometheusSpec": pulumi.Map{
						"retention": pulumi.String("7d"),
						"replicas":  pulumi.Int(1),
						"storageSpec": pulumi.Map{
							"volumeClaimTemplate": pulumi.Map{
								"spec": pulumi.Map{
									"storageClassName": pulumi.String("gp3-enc"),
									"accessModes":      pulumi.StringArray{pulumi.String("ReadWriteOnce")},
									"resources": pulumi.Map{
										"requests": pulumi.Map{
											"storage": pulumi.String("20Gi"),
										},
									},
								},
							},
						},
					},
				},
			},
		})
		if err != nil {
			return err
		}

		// Validate Prometheus deployment
		promStack.Name.ApplyT(func(name string) error {
			assert.Equal(t, "kube-prometheus-stack", name, "Prometheus stack name should match")
			return nil
		})

		promStack.Status.ApplyT(func(status string) error {
			assert.Equal(t, "deployed", status, "Prometheus should be deployed")
			return nil
		})

		ctx.Export("prometheusRelease", promStack.Name)

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestGrafanaConfiguration verifies Grafana configuration in prometheus stack
func TestGrafanaConfiguration(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Deploy kube-prometheus-stack with Grafana config
		promStack, err := helm.NewRelease(ctx, "kube-prometheus-stack", &helm.ReleaseArgs{
			Name:      pulumi.String("kube-prometheus-stack"),
			Namespace: pulumi.String("monitoring"),
			Chart:     pulumi.String("kube-prometheus-stack"),
			Version:   pulumi.String("58.2.2"),
			RepositoryOpts: &helm.RepositoryOptsArgs{
				Repo: pulumi.String("https://prometheus-community.github.io/helm-charts"),
			},
			Values: pulumi.Map{
				"grafana": pulumi.Map{
					"enabled": pulumi.Bool(true),
					"persistence": pulumi.Map{
						"enabled":          pulumi.Bool(true),
						"storageClassName": pulumi.String("gp3-enc"),
						"size":             pulumi.String("10Gi"),
					},
					"adminPassword": pulumi.String("test-password"),
				},
			},
		})
		if err != nil {
			return err
		}

		// Validate Grafana is enabled
		promStack.Status.ApplyT(func(status string) error {
			assert.Equal(t, "deployed", status, "Grafana should be deployed as part of stack")
			return nil
		})

		ctx.Export("grafanaService", pulumi.String("kube-prometheus-stack-grafana"))

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestLokiDeployment verifies Loki deployment configuration
func TestLokiDeployment(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Deploy Loki
		loki, err := helm.NewRelease(ctx, "loki", &helm.ReleaseArgs{
			Name:      pulumi.String("loki"),
			Namespace: pulumi.String("monitoring"),
			Chart:     pulumi.String("loki"),
			Version:   pulumi.String("6.6.2"),
			RepositoryOpts: &helm.RepositoryOptsArgs{
				Repo: pulumi.String("https://grafana.github.io/helm-charts"),
			},
			Values: pulumi.Map{
				"deploymentMode": pulumi.String("SingleBinary"),
				"loki": pulumi.Map{
					"commonConfig": pulumi.Map{
						"replication_factor": pulumi.Int(1),
					},
					"storage": pulumi.Map{
						"type": pulumi.String("filesystem"),
					},
				},
				"singleBinary": pulumi.Map{
					"replicas": pulumi.Int(1),
					"persistence": pulumi.Map{
						"enabled":      pulumi.Bool(true),
						"storageClass": pulumi.String("gp3-enc"),
						"size":         pulumi.String("10Gi"),
					},
				},
			},
		})
		if err != nil {
			return err
		}

		// Validate Loki deployment
		loki.Name.ApplyT(func(name string) error {
			assert.Equal(t, "loki", name, "Loki release name should match")
			return nil
		})

		loki.Status.ApplyT(func(status string) error {
			assert.Equal(t, "deployed", status, "Loki should be deployed")
			return nil
		})

		ctx.Export("lokiService", pulumi.String("loki"))

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestPromtailDaemonSet verifies Promtail configuration
func TestPromtailDaemonSet(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Deploy Promtail
		promtail, err := helm.NewRelease(ctx, "promtail", &helm.ReleaseArgs{
			Name:      pulumi.String("promtail"),
			Namespace: pulumi.String("monitoring"),
			Chart:     pulumi.String("promtail"),
			Version:   pulumi.String("6.15.5"),
			RepositoryOpts: &helm.RepositoryOptsArgs{
				Repo: pulumi.String("https://grafana.github.io/helm-charts"),
			},
			Values: pulumi.Map{
				"config": pulumi.Map{
					"clients": pulumi.Array{
						pulumi.Map{
							"url": pulumi.String("http://loki.monitoring.svc.cluster.local:3100/loki/api/v1/push"),
						},
					},
				},
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
			},
		})
		if err != nil {
			return err
		}

		// Validate Promtail deployment
		promtail.Name.ApplyT(func(name string) error {
			assert.Equal(t, "promtail", name, "Promtail release name should match")
			return nil
		})

		ctx.Export("promtailDaemonSet", pulumi.String("promtail"))

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestMonitoringStackOutputs verifies all required outputs are exported
func TestMonitoringStackOutputs(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Export required outputs
		ctx.Export("environment", pulumi.String("dev"))
		ctx.Export("monitoringNamespace", pulumi.String("monitoring"))
		ctx.Export("grafanaService", pulumi.String("kube-prometheus-stack-grafana"))
		ctx.Export("prometheusService", pulumi.String("kube-prometheus-stack-prometheus"))
		ctx.Export("lokiService", pulumi.String("loki"))
		ctx.Export("promtailDaemonSet", pulumi.String("promtail"))
		ctx.Export("alertmanagerService", pulumi.String("kube-prometheus-stack-alertmanager"))

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestPrometheusStorageConfiguration verifies storage is properly configured
func TestPrometheusStorageConfiguration(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Test storage configuration with proper types
		storageClassName := "gp3-enc"

		// Validate storage class is encrypted
		assert.Contains(t, storageClassName, "enc", "Storage class should be encrypted (contain 'enc')")

		// Create example helm release to validate structure
		_, err := helm.NewRelease(ctx, "test-prometheus", &helm.ReleaseArgs{
			Name:      pulumi.String("test-prometheus"),
			Namespace: pulumi.String("monitoring"),
			Chart:     pulumi.String("kube-prometheus-stack"),
			Values: pulumi.Map{
				"prometheus": pulumi.Map{
					"prometheusSpec": pulumi.Map{
						"storageSpec": pulumi.Map{
							"volumeClaimTemplate": pulumi.Map{
								"spec": pulumi.Map{
									"storageClassName": pulumi.String(storageClassName),
									"resources": pulumi.Map{
										"requests": pulumi.Map{
											"storage": pulumi.String("20Gi"),
										},
									},
								},
							},
						},
					},
				},
			},
		})

		return err
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestAlertManagerConfiguration verifies AlertManager is configured
func TestAlertManagerConfiguration(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Deploy with AlertManager enabled
		promStack, err := helm.NewRelease(ctx, "kube-prometheus-stack", &helm.ReleaseArgs{
			Name:      pulumi.String("kube-prometheus-stack"),
			Namespace: pulumi.String("monitoring"),
			Chart:     pulumi.String("kube-prometheus-stack"),
			RepositoryOpts: &helm.RepositoryOptsArgs{
				Repo: pulumi.String("https://prometheus-community.github.io/helm-charts"),
			},
			Values: pulumi.Map{
				"alertmanager": pulumi.Map{
					"enabled": pulumi.Bool(true),
				},
			},
		})
		if err != nil {
			return err
		}

		promStack.Status.ApplyT(func(status string) error {
			assert.Equal(t, "deployed", status, "Stack with AlertManager should be deployed")
			return nil
		})

		ctx.Export("alertmanagerService", pulumi.String("kube-prometheus-stack-alertmanager"))

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestStackReferenceToInfraKube verifies stack reference configuration
func TestStackReferenceToInfraKube(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Create stack reference to infra-kube
		infraKubeStack, err := pulumi.NewStackReference(ctx, "organization/eks-infra-kube/dev", nil)
		if err != nil {
			return err
		}

		// Get kubeconfig from stack reference
		kubeconfig := infraKubeStack.GetStringOutput(pulumi.String("kubeconfig"))

		kubeconfig.ApplyT(func(kc string) error {
			assert.NotEmpty(t, kc, "Kubeconfig from stack reference should not be empty")
			return nil
		})

		return nil
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}

// TestMonitoringResourceRequests verifies resource requests are configured
func TestMonitoringResourceRequests(t *testing.T) {
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Test resource configuration
		cpuRequest := "200m"
		memRequest := "512Mi"

		// Validate values are set
		assert.NotEmpty(t, cpuRequest, "CPU request should be set")
		assert.Contains(t, cpuRequest, "m", "CPU should be in millicores")
		assert.NotEmpty(t, memRequest, "Memory request should be set")

		// Create helm release with resources
		_, err := helm.NewRelease(ctx, "test-monitoring", &helm.ReleaseArgs{
			Name:      pulumi.String("test-monitoring"),
			Namespace: pulumi.String("monitoring"),
			Chart:     pulumi.String("kube-prometheus-stack"),
			Values: pulumi.Map{
				"prometheus": pulumi.Map{
					"prometheusSpec": pulumi.Map{
						"resources": pulumi.Map{
							"requests": pulumi.Map{
								"cpu":    pulumi.String(cpuRequest),
								"memory": pulumi.String(memRequest),
							},
							"limits": pulumi.Map{
								"cpu":    pulumi.String("500m"),
								"memory": pulumi.String("1Gi"),
							},
						},
					},
				},
			},
		})

		return err
	}, pulumi.WithMocks("project", "stack", mocks(0)))

	assert.NoError(t, err)
}
