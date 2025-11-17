package shared

import (
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/container"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	GlobalHelmChartPath           = "../../nativelink-cloud/helm-charts"
	GlobalDashboardPath           = "../../nativelink-cloud/config/dashboards"
	GlobalKibanaDashboardPath     = "../../nativelink-cloud/config/dashboards/%s-dashboards.ndjson"
	GlobalConfigPath              = "../../nativelink-cloud/config"
	GlobalCrossplanePath          = "../../nativelink-cloud/crossplane/"
	GlobalGKEServiceAccount       = "gke-cloud-platform-deployer@native-link-cloud.iam.gserviceaccount.com"
	GlobalWorkloadIdentityPool    = "native-link-cloud.svc.id.goog"
	GlobalClusterIssuer           = "letsencrypt-tls-issuer"
	Platform                      = "gke"
	GlobalPriorityClassName       = "trace-high-priority"
	GlobalPriorityClassValue      = 1000000000
	GlobalTemporalImageRepository = "us-central1-docker.pkg.dev/native-link-cloud/nativelink"
)

func GenerateKubeconfig(clusterEndpoint pulumi.StringOutput, clusterName pulumi.StringOutput,
	clusterMasterAuth container.ClusterMasterAuthOutput) pulumi.StringOutput {
	context := pulumi.Sprintf("%s", clusterName)

	return pulumi.Sprintf(`apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: %s
    server: https://%s
  name: %s
contexts:
- context:
    cluster: %s
    user: %s
  name: %s
current-context: %s
kind: Config
preferences: {}
users:
- name: %s
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: gke-gcloud-auth-plugin
      installHint: Install gke-gcloud-auth-plugin for use with kubectl by following
        https://cloud.google.com/blog/products/containers-kubernetes/kubectl-auth-changes-in-gke
      provideClusterInfo: true
`,
		clusterMasterAuth.ClusterCaCertificate().Elem(),
		clusterEndpoint, context, context, context, context, context, context)
}
