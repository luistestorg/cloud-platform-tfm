package shared

import (
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/container"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	// Paths relativos al proyecto
	GlobalHelmChartPath       = "../../helm-charts"
	GlobalDashboardPath       = "../../config/dashboards"
	GlobalKibanaDashboardPath = "../../config/dashboards/%s-dashboards.ndjson"
	GlobalConfigPath          = "../../config"
	GlobalCrossplanePath      = "../../crossplane/"

	// Configuración de GCP para el proyecto TFM
	GlobalGKEServiceAccount    = "gke-cloud-platform-deployer@cloud-platform-tfm.iam.gserviceaccount.com"
	GlobalWorkloadIdentityPool = "cloud-platform-tfm.svc.id.goog"

	// Configuración de certificados y plataforma
	GlobalClusterIssuer = "letsencrypt-tls-issuer"
	Platform            = "gke"

	// Priority classes
	GlobalPriorityClassName  = "tfm-high-priority"
	GlobalPriorityClassValue = 1000000000

	// Repositorio de imágenes Temporal
	GlobalTemporalImageRepository = "us-central1-docker.pkg.dev/cloud-platform-tfm/tfm"

	// Constantes adicionales para el proyecto TFM
	ProjectID   = "cloud-platform-tfm"
	ProjectName = "cloud-platform-tfm"
)

// GenerateKubeconfig genera la configuración de kubeconfig para conectarse al cluster GKE
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
