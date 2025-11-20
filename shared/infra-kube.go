package shared

import (
	"fmt"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	CertManagerChartVers = "v1.15.1"
	IngressChartVers     = "4.11.1"
	NvidiaChartVers      = "0.14.4"
)

func (s *Stack) DeployIngressNginxController(ctx *pulumi.Context) (*helmv3.Release, error) {

	ns, err := s.CreateNamespace(ctx, "ingress-nginx")
	if err != nil {
		return nil, err
	}

	s.DependsOn = append(s.DependsOn, ns)

	customIngressNginxValues := pulumi.Map{
		"controller": pulumi.Map{
			"resources": s.Resources.IngressNginx,
			"service": pulumi.Map{
				"externalTrafficPolicy": pulumi.String("Local"),
				"annotations":           ingressNginxAnnotationsByPlatform(s.Platform),
			},
		},
	}

	return s.DeployHelmRelease(ctx, ns, "ingress-nginx", IngressChartVers, "", "ingress-nginx-values.yaml", customIngressNginxValues)
}

func (s *Stack) DeployCertManager(ctx *pulumi.Context) (*helmv3.Release, error) {

	ns, err := s.CreateNamespace(ctx, "cert-manager")
	if err != nil {
		return nil, err
	}
	s.DependsOn = append(s.DependsOn, ns)
	customValues := pulumi.Map{
		"resources": s.Resources.CertManagerController,
		"cainjector": pulumi.Map{
			"resources": s.Resources.CertManagerCAInjector,
		},
		"webhook": pulumi.Map{
			"resources": s.Resources.CertManagerWebhook,
		},
		"global": pulumi.Map{
			"priorityClassName": priorityClassByPlatformAndWorkloadType("deployment"),
		},
	}
	if s.Platform == "aws" {

		if s.EnableAwsGatewayController {
			customValues["config"] = pulumi.Map{
				"apiVersion":       pulumi.String("controller.config.cert-manager.io/v1alpha1"),
				"kind":             pulumi.String("ControllerConfiguration"),
				"enableGatewayAPI": pulumi.Bool(true),
			}
		}
	}

	return s.DeployHelmRelease(ctx, ns, "cert-manager", CertManagerChartVers, "", "cert-manager-values.yaml", customValues)
}

func (s *Stack) CreateTLSCertIssuer(ctx *pulumi.Context) (*apiextensions.CustomResource, error) {

	var deps []pulumi.Resource

	issuerSpec, err := s.buildTLSCertIssuerSpec()
	if err != nil {
		return nil, err
	}

	// create the Role that allows changing Route53
	if s.Platform == "aws" {
		deps = []pulumi.Resource{s.Route53IamRole}
		deps = append(deps, s.DependsOn...)
	}

	issuer, err := apiextensions.NewCustomResource(ctx, GlobalClusterIssuer, &apiextensions.CustomResourceArgs{
		ApiVersion:  pulumi.String("cert-manager.io/v1"),
		Kind:        pulumi.String("ClusterIssuer"),
		Metadata:    metav1.ObjectMetaArgs{Name: pulumi.String(GlobalClusterIssuer)},
		OtherFields: issuerSpec,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return nil, err
	}
	s.DependsOn = append(s.DependsOn, issuer)
	ctx.Export("clusterIssuer", issuer.Metadata.Name())

	selfSignedSpec, _ := JSONToMap(`{
			"spec": {
			  "selfSigned": {}
			}
		  }`)
	selfSignedIssuerName := "self-signed-issuer"
	issuer, err = apiextensions.NewCustomResource(ctx, selfSignedIssuerName, &apiextensions.CustomResourceArgs{
		ApiVersion:  pulumi.String("cert-manager.io/v1"),
		Kind:        pulumi.String("ClusterIssuer"),
		Metadata:    metav1.ObjectMetaArgs{Name: pulumi.String(selfSignedIssuerName)},
		OtherFields: selfSignedSpec,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(deps))
	if err != nil {
		return nil, err
	}
	s.DependsOn = append(s.DependsOn, issuer)

	return issuer, nil
}

func (s *Stack) buildTLSCertIssuerSpec() (map[string]interface{}, error) {

	if s.Platform == "gke" {

		return JSONToMap(fmt.Sprintf(`{
    "spec": {
        "acme": {
            "server": "%s",
            "email": "%s",
            "privateKeySecretRef": {
                "name": "letsencrypt-issuer-key"
            },
            "solvers": [
                {
                    "dns01": {
                        "cloudDNS": {
                            "project": "%s"
                        }
                    },
                    "selector": {
                        "dnsZones": [
                            "%s"
                        ]
                    }
                }
            ]
        }
    }
}`, s.TLSCfg.AcmeServer, s.TLSCfg.Email, s.Project, s.TLSCfg.Domain))
	}
	route53RoleArn := fmt.Sprintf("arn:aws:iam::%s:role/%s", s.AwsAccountID, s.Route53RoleName)
	return JSONToMap(fmt.Sprintf(`{
	  "spec": {
		"acme": {
		  "server": "%s",
		  "email": "%s",
		  "privateKeySecretRef": {
			"name": "letsencrypt-issuer-key"
		  },
		  "solvers": [
			{
			  "dns01": {
				"route53": {
				  "region": "%s",
				  "hostedZoneID": "%s",
				  "role": "%s"
				}
			  },
			  "selector": {
				"dnsZones": ["%s"]
			  }
			}
		  ]
		}
	  }
	}`, s.TLSCfg.AcmeServer, s.TLSCfg.Email, s.Region, s.TLSCfg.Route53ZoneID, route53RoleArn, s.TLSCfg.Domain))

}

func (s *Stack) CreateWildCardCert(ctx *pulumi.Context, ns *corev1.Namespace) error {
	certName := "wildcard-tls"

	certSpec, err := JSONToMap(fmt.Sprintf(`{
    "spec": {
        "dnsNames": ["*.%s"],
        "issuerRef": {
            "group": "cert-manager.io",
            "kind": "ClusterIssuer",
            "name": "%s"
        },
        "secretName": "%s"
    }}`, s.TLSCfg.Domain, GlobalClusterIssuer, certName))
	if err != nil {
		return err
	}

	wildcardCert, err := apiextensions.NewCustomResource(ctx, certName, &apiextensions.CustomResourceArgs{
		ApiVersion:  pulumi.String("cert-manager.io/v1"),
		Kind:        pulumi.String("Certificate"),
		Metadata:    metav1.ObjectMetaArgs{Name: pulumi.String(certName), Namespace: ns.Metadata.Name()},
		OtherFields: certSpec,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, wildcardCert)
	return nil
}

func (s *Stack) DeployGpuPlugin(ctx *pulumi.Context) error {
	nvidiaConfig := `version: v1
sharing:
  timeSlicing:
    resources:
      - name: nvidia.com/gpu
        replicas: 10
`
	cm, err := corev1.NewConfigMap(ctx, "nvidia-device-plugin", &corev1.ConfigMapArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("nvidia-device-plugin"), Namespace: pulumi.String("kube-system")},
		Data:     pulumi.StringMap{"default": pulumi.String(nvidiaConfig)},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, cm)

	customValues := pulumi.Map{
		"nameOverride":      pulumi.String("nvidia"),
		"fullnameOverride":  pulumi.String("nvidia"),
		"namespaceOverride": pulumi.String("kube-system"),
		"failOnInitError":   pulumi.BoolPtr(true),
		"config": pulumi.Map{
			"name": pulumi.String("nvidia-device-plugin"),
		},
		"nodeSelector": pulumi.Map{
			"nvidia.com/gpu": pulumi.String("present"),
		},
	}
	fmt.Printf("%v", customValues)
	helmRel, err := s.DeployHelmRelease(ctx, nil, "nvidia-device-plugin", NvidiaChartVers, "", "", customValues)
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, helmRel)

	return nil
}

// Required for auth cert-manager with clouddns
func (s *Stack) LinkCertManagerSa(ctx *pulumi.Context, gkeServiceAccount string) error {

	/* kubectl annotate serviceaccount --namespace=cert-manager cert-manager  "iam.gke.io/gcp-service-account=gke-cloud-platform-deployer@cloud-platform-tfm.iam.gserviceaccount.com"*/

	cmSaPatch, err := corev1.NewServiceAccountPatch(ctx, "cert-manager-annotation", &corev1.ServiceAccountPatchArgs{
		Metadata: &metav1.ObjectMetaPatchArgs{
			Name: pulumi.String("cert-manager"),
			Annotations: pulumi.StringMap{
				"iam.gke.io/gcp-service-account": pulumi.String(gkeServiceAccount),
			},
			Namespace: pulumi.String("cert-manager"),
		},
	}, pulumi.DependsOn(s.DependsOn))

	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, cmSaPatch)

	return nil
}

func ingressNginxAnnotationsByPlatform(platform string) pulumi.Map {
	if platform == "gke" {
		return pulumi.Map{
			"ingressclass.kubernetes.io/is-default-class": pulumi.String("true"),
			"kubernetes.io/ingress.class":                 pulumi.String("nginx"),
			"nginx.ingress.kubernetes.io/ssl-redirect":    pulumi.String("true"),
		}
	}
	return pulumi.Map{
		"service.beta.kubernetes.io/aws-load-balancer-backend-protocol":                  pulumi.String("tcp"),
		"service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout":           pulumi.String("60"),
		"service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled": pulumi.String("false"),
		"service.beta.kubernetes.io/aws-load-balancer-type":                              pulumi.String("nlb"),
		"service.beta.kubernetes.io/aws-load-balancer-attributes":                        pulumi.String("load_balancing.cross_zone.enabled=false"),
	}

}
