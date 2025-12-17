package shared

import (
	"fmt"
	"net/url"
	"strings"

	awsiam "github.com/pulumi/pulumi-aws-iam/sdk/go/aws-iam"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/rds"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/s3"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/serviceaccount"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	DragonflyChartVers  = "v1.16.0"
	TemporalChartVers   = "0.32.0-5"
	CrossplaneChartVers = "v1.19.0"

	crossplaneGcpProviderVers = "v1.12.0"

	crossplaneKubernetesProviderVers = "v0.17.0"
	crossplaneAwsProviderVers        = "v0.51.5"
	crossplaneHelmProviderVers       = "v0.20.4"

	claimsPriorityClassName  = "tfm-high-priority"
	claimsPriorityClassValue = 1000000000
)

type (
	SelfServiceAPIConfig struct {
		APIEnabled                    bool
		DbPassword                    pulumi.StringOutput
		DbName                        pulumi.StringOutput
		CachePassword                 pulumi.StringOutput
		SharedCachePassword           pulumi.StringOutput
		PgPassword                    pulumi.StringOutput
		PgUsername                    string
		SQLPublicIPAddress            pulumi.StringOutput
		SQLPrivateIPAddress           pulumi.StringOutput
		SubDomain                     string
		RdsStorage                    int
		RdsInstanceType               string
		BootstrapAdminPassword        pulumi.StringOutput
		APIImage                      string
		EnableTemporal                bool
		SendWelcomeEmail              bool
		DbOffsiteBackupProject        string
		DbOffsiteBackupBucket         string
		DbOffsiteBackupServiceAccount string
		DbOffsiteBackupGCSKeyFile     pulumi.StringOutput
		SegmentAPIWriteKey            pulumi.StringOutput
		SegmentAPIEnabled             bool
		TfmImage                      string
		SharedRedisZone               string
		SQLZone                       string
		dependsOn                     []pulumi.Resource
		EcrRegion                     string
		AwsRegion                     string
		AwsAccessKeyID                pulumi.StringOutput
		AwsSecretAccessKey            pulumi.StringOutput
		GithubAuthToken               pulumi.StringOutput
		TfmDbEnabled                  bool
		TfmDbPassword                 pulumi.StringOutput
		ClusterID                     string
		ChangelogTip                  string
		EnableSlackNotifications      bool
		EnableMongoDB                 bool
		MongoRootPassword             pulumi.StringOutput
		MongoDatabasePassword         pulumi.StringOutput
		GrafanaAdminPassword          pulumi.StringOutput

		//GCP Specific
		Project       string
		Profile       string
		GCPAWSRoleArn string
		GCPAWSSub     string
	}
)

func (ssApiCfg *SelfServiceAPIConfig) DeploySelfServiceAPI(ctx *pulumi.Context, s *Stack) error {
	ns, err := s.CreateNamespace(ctx, "tfm-api")
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, ns)

	//Creating priorityClass to be used by the claims
	_, err = s.createPriorityClass(ctx, claimsPriorityClassName, claimsPriorityClassValue)
	if err != nil {
		return err
	}

	if err = ssApiCfg.deployDragonflyHelm(ctx, "dragonfly", ns, s); err != nil {
		return err
	}

	passwordData := pulumi.StringMap{"postgres-password": ssApiCfg.PgPassword, "password": ssApiCfg.DbPassword}
	if ssApiCfg.TfmDbEnabled {
		passwordData["nativelink-password"] = ssApiCfg.TfmDbPassword
	}

	pgSecret, err := corev1.NewSecret(ctx, "postgres", &corev1.SecretArgs{
		ApiVersion: pulumi.String("v1"),
		Kind:       pulumi.String("Secret"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("postgres"), Namespace: ns.Metadata.Name()},
		StringData: passwordData,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, pgSecret)

	if s.Platform == "aws" {

		// deploy RDS
		rdsSg, err := ec2.NewSecurityGroup(ctx, s.ClusterScopedResourceName("rds-sg"), &ec2.SecurityGroupArgs{
			VpcId: s.Vpc.VpcId,
			Ingress: ec2.SecurityGroupIngressArray{
				&ec2.SecurityGroupIngressArgs{
					Description: pulumi.String("PostgreSQL"),
					FromPort:    pulumi.Int(5432),
					ToPort:      pulumi.Int(5432),
					Protocol:    pulumi.String("tcp"),
					CidrBlocks:  pulumi.ToStringArrayOutput([]pulumi.StringOutput{s.Vpc.Vpc.CidrBlock()}),
				},
			},
		})
		if err != nil {
			return err
		}

		subnetGroup, err := rds.NewSubnetGroup(ctx, s.ClusterScopedResourceName("rds-subnets"), &rds.SubnetGroupArgs{SubnetIds: s.Vpc.PrivateSubnetIds})
		if err != nil {
			return err
		}

		rdsInst, err := rds.NewInstance(ctx, s.ClusterScopedResourceName("rds-api"), &rds.InstanceArgs{
			AvailabilityZone:             pulumi.String(ssApiCfg.SQLZone),
			AllocatedStorage:             pulumi.Int(ssApiCfg.RdsStorage),
			AllowMajorVersionUpgrade:     pulumi.BoolPtr(false),
			AutoMinorVersionUpgrade:      pulumi.BoolPtr(false),
			BackupRetentionPeriod:        pulumi.Int(6),
			Engine:                       pulumi.String("postgres"),
			EngineVersion:                pulumi.String("16.3"),
			InstanceClass:                pulumi.String(ssApiCfg.RdsInstanceType),
			Name:                         pulumi.String("nlssapidb"),
			Username:                     pulumi.String("nlssapidb"),
			Password:                     ssApiCfg.DbPassword,
			ApplyImmediately:             pulumi.Bool(true),
			StorageEncrypted:             pulumi.Bool(true),
			StorageType:                  pulumi.String("gp3"),
			VpcSecurityGroupIds:          pulumi.StringArray{rdsSg.ID()},
			DbSubnetGroupName:            subnetGroup.Name,
			SkipFinalSnapshot:            pulumi.BoolPtr(true),
			EnabledCloudwatchLogsExports: pulumi.ToStringArray([]string{"postgresql", "upgrade"}),
		}, pulumi.DependsOn([]pulumi.Resource{rdsSg, subnetGroup, pgSecret}))
		if err != nil {
			return err
		}
		s.DependsOn = append(s.DependsOn, rdsInst)

		/*
			rdsInst2, err := rds.NewInstance(ctx, s.ClusterScopedResourceName("rds-api-"), &rds.InstanceArgs{
				AvailabilityZone:             pulumi.String(ssApiCfg.SQLZone),
				AllocatedStorage:             pulumi.Int(1500),
				AllowMajorVersionUpgrade:     pulumi.BoolPtr(false),
				AutoMinorVersionUpgrade:      pulumi.BoolPtr(false),
				BackupRetentionPeriod:        pulumi.Int(6),
				Engine:                       pulumi.String("postgres"),
				EngineVersion:                pulumi.String("16.8"),
				InstanceClass:                pulumi.String("db.t4g.xlarge"),
				Name:                         pulumi.String("nlssapidb"),
				Username:                     pulumi.String("nlssapidb"),
				Password:                     ssApiCfg.DbPassword,
				ApplyImmediately:             pulumi.Bool(true),
				StorageEncrypted:             pulumi.Bool(true),
				StorageType:                  pulumi.String("gp3"),
				VpcSecurityGroupIds:          pulumi.StringArray{rdsSg.ID()},
				DbSubnetGroupName:            subnetGroup.Name,
				SkipFinalSnapshot:            pulumi.BoolPtr(true),
				EnabledCloudwatchLogsExports: pulumi.ToStringArray([]string{"postgresql", "upgrade"}),
			}, pulumi.DependsOn([]pulumi.Resource{rdsSg, subnetGroup, pgSecret}))
			if err != nil {
				return err
			}
			s.DependsOn = append(s.DependsOn, rdsInst2)

			backupEnvMap := pulumi.StringMap{
				"PGHOST":     rdsInst2.Address,
				"PGUSER":     pulumi.String("nlssapidb"),
				"PGDATABASE": rdsInst2.DbName,
			}
			_, err = corev1.NewSecret(ctx, "restore-db-env", &corev1.SecretArgs{
				ApiVersion: pulumi.String("v1"),
				Kind:       pulumi.String("Secret"),
				Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("restore-db-env"), Namespace: ns.Metadata.Name()},
				StringData: backupEnvMap,
			}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{ns}))
			if err != nil {
				return err
			}
			ctx.Export("rds2Address", rdsInst2.Address)
			ctx.Export("rds2DbName", rdsInst2.DbName)
		*/

		ssApiCfg.SQLPrivateIPAddress = rdsInst.Address
		ssApiCfg.DbName = rdsInst.DbName
		ssApiCfg.PgUsername = "nlssapidb"

		ctx.Export("rdsAddress", ssApiCfg.SQLPrivateIPAddress)
		ctx.Export("rdsDbName", ssApiCfg.DbName)

		if s.VpcFlowLogsEnabled {
			rdsFlowLogGroups, err := cloudwatch.GetLogGroups(ctx, &cloudwatch.GetLogGroupsArgs{
				LogGroupNamePrefix: pulumi.StringRef(fmt.Sprintf("/aws/rds/instance/%v", s.ClusterScopedResourceName("rds-api"))),
			}, nil)
			if err != nil {
				return err
			}
			rdsFlowLog, err := cloudwatch.LookupLogGroup(ctx, &cloudwatch.LookupLogGroupArgs{
				Name: fmt.Sprint(rdsFlowLogGroups.LogGroupNames[0]),
			}, nil)
			if err != nil {
				return err
			}
			rdsFlowLog.RetentionInDays = s.LogGroupRetention
		}
	}

	// Deploy Temporal Helm Chart
	if ssApiCfg.EnableTemporal {
		temporalRel, helmErr := ssApiCfg.deployTemporal(ctx, ns, pgSecret, s)
		if helmErr != nil {
			fmt.Printf("deploy temporal failed due to: %v\n", helmErr)
			return helmErr
		}
		s.DependsOn = append(s.DependsOn, temporalRel)
	}
	s.IngressHosts = append(s.IngressHosts, fmt.Sprintf("https://workflows.%s", s.TLSCfg.Domain))

	apiSubDomain := "api"

	// OAuth2 proxy config
	redirectURI := fmt.Sprintf("https://%s.%s/oauth2/callback", apiSubDomain, s.TLSCfg.Domain)
	opConfigMap, err := s.CreateOAuth2ProxyConfig(ctx, ns, "api", redirectURI, 8000, "http")
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, opConfigMap)

	oauth2ProxyContainer := s.CreateOAuth2ProxyContainerArgs(opConfigMap.Metadata.Name(), "quay.io/oauth2-proxy/oauth2-proxy:v7.5.1-arm64")

	adminEmails := []string{s.TLSCfg.Email}
	adminUsers := strings.Join(adminEmails, ",")

	// IRSA for API (for SES, S3 bucket clean-up, etc)
	var irsaArn pulumi.StringInput
	saName := "api"
	if s.Platform == "aws" {
		irsaArn, err = ssApiCfg.createAPIIRSA(ctx, saName, s)
		if err != nil {
			return err
		}
	}

	validateURL, err := url.Parse(s.OauthConfig.Oauth2ValidateURL)
	if err != nil {
		return err
	}

	s3Bucket := &s3.Bucket{}

	if s.Platform == "aws" {

		// note ~ this bucket is not used
		_, err = s3.NewBucket(ctx, "log_bucket", &s3.BucketArgs{
			BucketPrefix: pulumi.Sprintf("log-bucket-%s", s.ClusterName),
		})
		if err != nil {
			return err
		}

		s3Bucket, err = s3.NewBucket(ctx, "shared-cas-s3-bucket", &s3.BucketArgs{
			BucketPrefix: pulumi.Sprintf("shared-cas-%s", s.ClusterName),
			Versioning:   s3.BucketVersioningArgs{Enabled: pulumi.BoolPtr(false)},
			ServerSideEncryptionConfiguration: &s3.BucketServerSideEncryptionConfigurationArgs{
				Rule: &s3.BucketServerSideEncryptionConfigurationRuleArgs{
					ApplyServerSideEncryptionByDefault: &s3.BucketServerSideEncryptionConfigurationRuleApplyServerSideEncryptionByDefaultArgs{
						SseAlgorithm: pulumi.String("aws:kms"),
					},
				},
			},
			LifecycleRules: &s3.BucketLifecycleRuleArray{
				&s3.BucketLifecycleRuleArgs{
					Id:         pulumi.Sprintf("shared-bucket-lifecycle-%s", s.ClusterName),
					Expiration: &s3.BucketLifecycleRuleExpirationArgs{Days: pulumi.Int(30)},
					Enabled:    pulumi.Bool(true),
				},
			},
		})
		if err != nil {
			return err
		}
		s.DependsOn = append(s.DependsOn, s3Bucket)

	}

	imageID := ssApiCfg.TfmImage
	apiEnvDataMap := pulumi.StringMap{
		"APP_BASE_URL":         pulumi.String("http://localhost:8000"),
		"TZ":                   pulumi.String("UTC"),
		"APP_ENVIRONMENT":      pulumi.String(s.Env),
		"CACHE_ENABLED":        pulumi.String("true"),
		"CACHE_HOSTNAME":       pulumi.String("dragonfly"),
		"CACHE_DB":             pulumi.String("0"),
		"DB_HOSTNAME":          ssApiCfg.SQLPrivateIPAddress,
		"DB_NAME":              ssApiCfg.DbName,
		"DB_USER":              pulumi.String(ssApiCfg.PgUsername),
		"APP_INIT_ADMIN_USERS": pulumi.String(adminUsers),
		"K8S_ENABLED":          pulumi.String("true"),
		"K8S_DOMAIN":           pulumi.String(s.TLSCfg.Domain),
		"K8S_REGION":           pulumi.String(s.Region),
		"K8S_TLS_ISSUER":       pulumi.String(GlobalClusterIssuer),
		"K8S_DEFAULT_IMAGE":    pulumi.String(imageID),
		"OAUTH2_CLIENT_ID":     pulumi.String(s.OauthConfig.Oauth2ClientID),
		"OAUTH2_DNS_NAME":      pulumi.String(validateURL.Host),
		"OAUTH2_ISSUER":        pulumi.String(s.OauthConfig.OidcIssuerURL),
		"GRAFANA_ENABLED":      pulumi.String("true"),
	}

	if ssApiCfg.EnableMongoDB {
		apiEnvDataMap["MONGODB_ENABLED"] = pulumi.String("true")
		apiEnvDataMap["MONGODB_URI"] = pulumi.String("mongodb://mdb.mongodb.svc.cluster.local:27017")
		apiEnvDataMap["MONGODB_USER"] = pulumi.String("root")
		apiEnvDataMap["MONGODB_DATABASE"] = pulumi.String("bep")
	} else {
		apiEnvDataMap["MONGODB_ENABLED"] = pulumi.String("false")
	}

	if s.Platform == "aws" {
		apiEnvDataMap["K8S_ACCOUNT_ID"] = pulumi.String(s.AwsAccountID)
		apiEnvDataMap["SHARED_S3_BUCKET"] = s3Bucket.Bucket
		apiEnvDataMap["SHARED_S3_BUCKET_ARN"] = s3Bucket.Arn
		apiEnvDataMap["AWS_REGION"] = pulumi.String(s.Region)
		apiEnvDataMap["GITHUB_CLUSTER_ID"] = pulumi.String(ssApiCfg.ChangelogTip) // used for changelog tracking
		apiEnvDataMap["K8S_TYPE"] = pulumi.String("eks")
		apiEnvDataMap["K8S_OIDC"] = s.OidcID
		apiEnvDataMap["WORKER_IMAGE_REPO"] = pulumi.String("299166832260.dkr.ecr.us-east-2.amazonaws.com")

	} else {
		apiEnvDataMap["AWS_LOCAL_PROFILE"] = pulumi.String(ssApiCfg.Profile)
		apiEnvDataMap["AWS_ROLE_ARN_GCP"] = pulumi.String(ssApiCfg.GCPAWSRoleArn)
		apiEnvDataMap["AWS_AUDIENCE_GCP"] = pulumi.String(ssApiCfg.Profile)
		apiEnvDataMap["AWS_SUB_GCP"] = pulumi.String(ssApiCfg.GCPAWSSub)
		apiEnvDataMap["AWS_REGION"] = pulumi.String(ssApiCfg.AwsRegion)
		apiEnvDataMap["GITHUB_CLUSTER_ID"] = pulumi.String(ssApiCfg.ClusterID) // used for changelog tracking
		apiEnvDataMap["K8S_TYPE"] = pulumi.String("gke")
		apiEnvDataMap["WORKER_IMAGE_REPO"] = pulumi.String("us-docker.pkg.dev/cloud-platform-tfm/aws-remote")
	}
	apiEnvDataMap["SHARED_CAS_ZONE"] = pulumi.String(ssApiCfg.SharedRedisZone)

	if ssApiCfg.EnableTemporal {
		apiEnvDataMap["TEMPORAL_ENABLED"] = pulumi.String("true")
	}

	if ssApiCfg.SendWelcomeEmail {
		apiEnvDataMap["SEND_WELCOME_EMAIL"] = pulumi.String("true")
	}

	apiEnvConfig, err := corev1.NewConfigMap(ctx, "api-env", &corev1.ConfigMapArgs{
		ApiVersion: pulumi.String("v1"),
		Data:       apiEnvDataMap,
		Kind:       pulumi.String("ConfigMap"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("api-env"), Namespace: ns.Metadata.Name()},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, apiEnvConfig)

	apiCredsMap := pulumi.StringMap{
		"boostrap-admin-pass":  ssApiCfg.BootstrapAdminPassword,
		"oauth2-client-secret": s.OauthConfig.Oauth2ClientSecret,
		"github-auth-token":    ssApiCfg.GithubAuthToken,
	}

	if ssApiCfg.EnableSlackNotifications {
		apiCredsMap["slack-webhook-url"] = s.SlackWebhookURL
	}

	if ssApiCfg.EnableMongoDB {
		apiCredsMap["mongodb-password"] = ssApiCfg.MongoRootPassword
	}

	if s.AlertWebhookURL != nil {
		apiCredsMap["alert-webhook-url"] = *s.AlertWebhookURL
	}
	if ssApiCfg.SegmentAPIEnabled {
		apiCredsMap["segment-write-key"] = ssApiCfg.SegmentAPIWriteKey
	}

	apiCreds, err := corev1.NewSecret(ctx, "api-creds-secret", &corev1.SecretArgs{
		ApiVersion: pulumi.String("v1"),
		Kind:       pulumi.String("Secret"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("api-creds"), Namespace: ns.Metadata.Name()},
		StringData: apiCredsMap,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{apiEnvConfig}))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, apiCreds)

	grafanaAuthSecret, err := corev1.NewSecret(ctx, "grafana-auth", &corev1.SecretArgs{
		ApiVersion: pulumi.String("v1"),
		Kind:       pulumi.String("Secret"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("grafana-auth"), Namespace: ns.Metadata.Name()},
		StringData: pulumi.StringMap{
			"admin-user":     pulumi.String("admin"),
			"admin-password": ssApiCfg.GrafanaAdminPassword,
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{ns}))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, grafanaAuthSecret)

	// service account

	saMetadata := &metav1.ObjectMetaArgs{}

	if s.Platform == "gke" {
		saMetadata = &metav1.ObjectMetaArgs{
			Name:        pulumi.String(saName),
			Namespace:   ns.Metadata.Name(),
			Annotations: pulumi.StringMap{"iam.gke.io/gcp-service-account": pulumi.String(s.GlobalGKEServiceAccount)},
		}

	} else {

		saMetadata = &metav1.ObjectMetaArgs{
			Name:        pulumi.String(saName),
			Namespace:   ns.Metadata.Name(),
			Annotations: pulumi.StringMap{"eks.amazonaws.com/role-arn": irsaArn},
		}
	}

	sa, err := corev1.NewServiceAccount(ctx, "api-sa", &corev1.ServiceAccountArgs{
		Metadata: saMetadata,
	}, pulumi.Provider(s.K8sProvider))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, sa)

	crb, err := rbacv1.NewClusterRoleBinding(ctx, "api-admin-binding", &rbacv1.ClusterRoleBindingArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String("api-admin-binding"),
		},
		RoleRef: &rbacv1.RoleRefArgs{
			ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
			Kind:     pulumi.String("ClusterRole"),
			Name:     pulumi.String("cluster-admin"),
		},
		Subjects: rbacv1.SubjectArray{
			&rbacv1.SubjectArgs{
				Kind:      pulumi.String("ServiceAccount"),
				Name:      pulumi.String("api"),
				Namespace: ns.Metadata.Name(),
			},
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{sa}))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, crb)

	envVars := corev1.EnvVarArray{
		&corev1.EnvVarArgs{
			Name: pulumi.String("POD_NAME"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				FieldRef: &corev1.ObjectFieldSelectorArgs{
					FieldPath: pulumi.String("metadata.name"),
				},
			},
		},
		&corev1.EnvVarArgs{
			Name: pulumi.String("DB_PASSWORD"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				SecretKeyRef: &corev1.SecretKeySelectorArgs{
					Name: pgSecret.Metadata.Name(),
					Key:  pulumi.String("password"),
				},
			},
		},
		&corev1.EnvVarArgs{
			Name: pulumi.String("SLACK_WEBHOOK_URL"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				SecretKeyRef: &corev1.SecretKeySelectorArgs{
					Name:     apiCreds.Metadata.Name(),
					Key:      pulumi.String("slack-webhook-url"),
					Optional: pulumi.Bool(true),
				},
			},
		},
		&corev1.EnvVarArgs{
			Name: pulumi.String("BOOTSTRAP_ADMIN_PASS"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				SecretKeyRef: &corev1.SecretKeySelectorArgs{
					Name: apiCreds.Metadata.Name(),
					Key:  pulumi.String("boostrap-admin-pass"),
				},
			},
		},
		&corev1.EnvVarArgs{
			Name: pulumi.String("CACHE_PASSWORD"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				SecretKeyRef: &corev1.SecretKeySelectorArgs{
					Name: pulumi.String("dragonfly"),
					Key:  pulumi.String("dfly_password"),
				},
			},
		},
		&corev1.EnvVarArgs{
			Name: pulumi.String("OAUTH2_CLIENT_SECRET"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				SecretKeyRef: &corev1.SecretKeySelectorArgs{
					Name: apiCreds.Metadata.Name(),
					Key:  pulumi.String("oauth2-client-secret"),
				},
			},
		},
		&corev1.EnvVarArgs{
			Name: pulumi.String("MONGODB_PASS"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				SecretKeyRef: &corev1.SecretKeySelectorArgs{
					Name:     apiCreds.Metadata.Name(),
					Key:      pulumi.String("mongodb-password"),
					Optional: pulumi.Bool(true),
				},
			},
		},
		&corev1.EnvVarArgs{
			Name: pulumi.String("SEGMENT_WRITE_KEY"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				SecretKeyRef: &corev1.SecretKeySelectorArgs{
					Name:     apiCreds.Metadata.Name(),
					Key:      pulumi.String("segment-write-key"),
					Optional: pulumi.Bool(true),
				},
			},
		},
		&corev1.EnvVarArgs{
			Name: pulumi.String("GITHUB_AUTH_TOKEN"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				SecretKeyRef: &corev1.SecretKeySelectorArgs{
					Name:     apiCreds.Metadata.Name(),
					Key:      pulumi.String("github-auth-token"),
					Optional: pulumi.Bool(true),
				},
			},
		},
		&corev1.EnvVarArgs{
			Name: pulumi.String("GRAFANA_USER"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				SecretKeyRef: &corev1.SecretKeySelectorArgs{
					Name:     grafanaAuthSecret.Metadata.Name(),
					Key:      pulumi.String("admin-user"),
					Optional: pulumi.Bool(true),
				},
			},
		},
		&corev1.EnvVarArgs{
			Name: pulumi.String("GRAFANA_PASS"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				SecretKeyRef: &corev1.SecretKeySelectorArgs{
					Name:     grafanaAuthSecret.Metadata.Name(),
					Key:      pulumi.String("admin-password"),
					Optional: pulumi.Bool(true),
				},
			},
		},
	}
	if s.AlertWebhookURL != nil {
		envVars = append(envVars, &corev1.EnvVarArgs{
			Name: pulumi.String("ALERT_WEBHOOK_URL"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				SecretKeyRef: &corev1.SecretKeySelectorArgs{
					Name:     apiCreds.Metadata.Name(),
					Key:      pulumi.String("alert-webhook-url"),
					Optional: pulumi.Bool(true),
				},
			},
		})
	}

	apiContainer := &corev1.ContainerArgs{
		Name:            pulumi.String("api"),
		Image:           pulumi.String(ssApiCfg.APIImage),
		ImagePullPolicy: pulumi.String("Always"),
		EnvFrom: &corev1.EnvFromSourceArray{
			corev1.EnvFromSourceArgs{
				ConfigMapRef: corev1.ConfigMapEnvSourceArgs{
					Name: pulumi.String("api-env"),
				},
			},
			corev1.EnvFromSourceArgs{
				SecretRef: corev1.SecretEnvSourceArgs{
					Name:     pulumi.String("nldb-env"),
					Optional: pulumi.Bool(true),
				},
			},
		},
		Env: envVars,
		Ports: corev1.ContainerPortArray{
			&corev1.ContainerPortArgs{
				Name:          pulumi.String("api"),
				Protocol:      pulumi.String("TCP"),
				ContainerPort: pulumi.Int(8000),
			},
		},
		Resources: s.Resources.Api,
		LivenessProbe: &corev1.ProbeArgs{
			HttpGet: &corev1.HTTPGetActionArgs{
				Path: pulumi.String("/live"),
				Port: pulumi.Int(8000),
			},
			FailureThreshold:    pulumi.Int(3),
			InitialDelaySeconds: pulumi.Int(2),
			PeriodSeconds:       pulumi.Int(15),
		},
		ReadinessProbe: &corev1.ProbeArgs{
			HttpGet: &corev1.HTTPGetActionArgs{
				Path: pulumi.String("/ready"),
				Port: pulumi.Int(8000),
			},
			FailureThreshold:    pulumi.Int(3),
			InitialDelaySeconds: pulumi.Int(2),
			PeriodSeconds:       pulumi.Int(15),
		},
	}

	var apiPodAnnotations pulumi.StringMap
	var pullSecrets corev1.LocalObjectReferenceArray

	if s.Platform == "gke" {
		apiPodAnnotations = pulumi.StringMap{"kubectl.kubernetes.io/default-container": pulumi.String("api")}
	} else {
		apiPodAnnotations = pulumi.StringMap{"karpenter.sh/do-not-disrupt": pulumi.String("true"), "kubectl.kubernetes.io/default-container": pulumi.String("api")}
	}

	pullSecrets = corev1.LocalObjectReferenceArray{}

	podTemplate := &corev1.PodTemplateSpecArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Labels:      pulumi.StringMap{"app": pulumi.String("api")},
			Annotations: apiPodAnnotations,
		},
		Spec: &corev1.PodSpecArgs{
			Affinity: &corev1.AffinityArgs{
				NodeAffinity: &corev1.NodeAffinityArgs{
					PreferredDuringSchedulingIgnoredDuringExecution: &corev1.PreferredSchedulingTermArray{
						&corev1.PreferredSchedulingTermArgs{
							Weight: pulumi.Int(100),
							Preference: &corev1.NodeSelectorTermArgs{
								MatchExpressions: corev1.NodeSelectorRequirementArray{
									&corev1.NodeSelectorRequirementArgs{
										Key:      pulumi.String("topology.kubernetes.io/zone"),
										Operator: pulumi.String("In"),
										Values:   pulumi.StringArray{pulumi.String(ssApiCfg.SQLZone)}, // same zone as our RDS instance
									},
								},
							},
						},
					},
				},
			},
			ServiceAccountName: sa.Metadata.Name(),
			NodeSelector:       pulumi.StringMap{"kubernetes.io/arch": pulumi.String("arm64"), "node-role": pulumi.String("not-disruptable")},
			Containers:         corev1.ContainerArray{oauth2ProxyContainer, apiContainer},
			ImagePullSecrets:   pullSecrets,
			Tolerations: corev1.TolerationArray{
				corev1.TolerationArgs{
					Key:      pulumi.String("tfm/not-disruptable"),
					Operator: pulumi.String("Exists"),
					Effect:   pulumi.String("NoSchedule"),
				},
			},
		},
	}

	replicas := 2
	apiDeploy, err := appsv1.NewDeployment(ctx, "api", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("api"), Namespace: ns.Metadata.Name()},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(replicas),
			Selector: metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{"app": pulumi.String("api")}},
			Template: podTemplate,
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, apiDeploy)

	apiSvc, err := corev1.NewService(ctx, "api", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("api"), Namespace: ns.Metadata.Name()},
		Spec: &corev1.ServiceSpecArgs{
			Selector: pulumi.StringMap{"app": pulumi.String("api")},
			Ports: &corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("proxy"),
					Port:       pulumi.Int(8888),
					Protocol:   pulumi.String("TCP"),
					TargetPort: pulumi.String("proxy"),
				},
				&corev1.ServicePortArgs{
					Name:       pulumi.String("api"),
					Port:       pulumi.Int(8000), // by-pass OAuth2 proxy (for api-key auth)
					Protocol:   pulumi.String("TCP"),
					TargetPort: pulumi.String("api"),
				},
			},
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, apiSvc)

	apiPdb, err := CreatePdb(ctx, ns, "api-pdb", "api")
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, apiPdb)

	ingressAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-read-timeout": "3600",
		"nginx.ingress.kubernetes.io/proxy-send-timeout": "3600",
		"nginx.org/websocket-services":                   "api",
	}

	host, err := s.CreateIngress(ctx, apiSubDomain, "tfm-api", "api", 8888, ingressAnnotations)
	if err != nil {
		return err
	}
	fmt.Printf("Deployed ingress for host: %s\n", host)

	if ssApiCfg.DbOffsiteBackupBucket != "" {
		if err = ssApiCfg.deployOffsiteBackupCronJob(ctx, ns, s); err != nil {
			return err
		}
	}

	return nil
}

func (ssApiCfg *SelfServiceAPIConfig) deployDragonflyHelm(ctx *pulumi.Context, uniqueName string, ns *corev1.Namespace, s *Stack) error {
	name := "dragonfly"
	secret, err := corev1.NewSecret(ctx, uniqueName, &corev1.SecretArgs{
		ApiVersion: pulumi.String("v1"),
		Kind:       pulumi.String("Secret"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String(name), Namespace: ns.Metadata.Name()},
		StringData: pulumi.StringMap{"dfly_password": ssApiCfg.CachePassword},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, secret)

	customValues := pulumi.Map{
		"replicaCount":     pulumi.Int(1),
		"extraArgs":        pulumi.ToStringArray([]string{"--maxmemory=2Gi", "--proactor_threads=8"}),
		"nameOverride":     pulumi.String(name),
		"fullnameOverride": pulumi.String(name),
		"passwordFromSecret": pulumi.Map{
			"enable": pulumi.BoolPtr(true),
			"existingSecret": pulumi.Map{
				"name": pulumi.String(name),
				"key":  pulumi.String("dfly_password"),
			},
		},
		"podAnnotations":    annotationsByPlatform(s.Platform),
		"tolerations":       nodeTolerationsByPlatformAndDistruptionType(s.Platform, false),
		"nodeSelector":      getNodeSelector(false, "", "", ""),
		"priorityClassName": priorityClassByPlatformAndWorkloadType("deployment"),
		"resources":         s.Resources.DragonFly,
	}

	if _, err = s.DeployHelmRelease(ctx, ns, uniqueName, DragonflyChartVers, "dragonfly", "", customValues); err != nil {
		return err
	}
	return nil
}
func (ssApiCfg *SelfServiceAPIConfig) deployOffsiteBackupCronJob(ctx *pulumi.Context, ns *corev1.Namespace, s *Stack) error {
	secret, err := corev1.NewSecret(ctx, "gcloud-sa-key-file", &corev1.SecretArgs{
		ApiVersion: pulumi.String("v1"),
		Kind:       pulumi.String("Secret"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("gcloud-sa-key-file"), Namespace: ns.Metadata.Name()},
		StringData: pulumi.StringMap{"key.json": ssApiCfg.DbOffsiteBackupGCSKeyFile},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{ns}))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, secret)

	backupEnvMap := pulumi.StringMap{
		"PGHOST":       ssApiCfg.SQLPrivateIPAddress,
		"PGUSER":       pulumi.String(ssApiCfg.PgUsername),
		"PGDATABASE":   ssApiCfg.DbName,
		"GCP_SA_EMAIL": pulumi.String(ssApiCfg.DbOffsiteBackupServiceAccount),
		"GCP_PROJECT":  pulumi.String(ssApiCfg.DbOffsiteBackupProject),
		"GCS_BUCKET":   pulumi.String(ssApiCfg.DbOffsiteBackupBucket),
	}
	envVarsSecret, err := corev1.NewSecret(ctx, "offsite-backup-env", &corev1.SecretArgs{
		ApiVersion: pulumi.String("v1"),
		Kind:       pulumi.String("Secret"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("offsite-backup-env"), Namespace: ns.Metadata.Name()},
		StringData: backupEnvMap,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{ns}))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, envVarsSecret)

	cronJob, err := yaml.NewConfigFile(ctx, "offsite-backup-cronjob", &yaml.ConfigFileArgs{
		File: "config/offsite-backup-cronjob.yaml",
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, cronJob)

	return nil
}

func (ssApiCfg *SelfServiceAPIConfig) deployTemporal(ctx *pulumi.Context, ns *corev1.Namespace, pgSecret *corev1.Secret, s *Stack) (*helmv3.Release, error) {
	redirectURI := fmt.Sprintf("https://workflows.%s/oauth2/callback", s.TLSCfg.Domain)
	opConfigMap, err := s.CreateOAuth2ProxyConfig(ctx, ns, "workflows", redirectURI, 8080, "http")
	if err != nil {
		return nil, err
	}
	s.DependsOn = append(s.DependsOn, opConfigMap)

	hostnames := []string{fmt.Sprintf("workflows.%s", s.TLSCfg.Domain)}

	customValues := pulumi.Map{
		"web": pulumi.Map{
			"ingress": pulumi.Map{
				"enabled":   pulumi.BoolPtr(true),
				"className": pulumi.String("nginx"),
				"annotations": pulumi.Map{
					"cert-manager.io/cluster-issuer": pulumi.String(GlobalClusterIssuer),
				},
				"hosts": pulumi.ToStringArray(hostnames),
				"tls": pulumi.Array{
					pulumi.Map{
						"hosts":      pulumi.ToStringArray(hostnames),
						"secretName": pulumi.String("temporal-tls"),
					},
				},
			},
			"resources": s.Resources.TemporalWeb,
			"oauthProxy": pulumi.Map{
				"resources": s.Resources.OauthProxy,
			},
		},
		"server": pulumi.Map{
			"podAnnotations": pulumi.Map{
				"karpenter.sh/do-not-disrupt": pulumi.String("true"),
			},
			"tolerations":  nodeTolerationsByPlatformAndDistruptionType(s.Platform, false),
			"nodeSelector": getNodeSelector(false, "", "", ""),
			"config": pulumi.Map{
				"persistence": pulumi.Map{
					"default": pulumi.Map{
						"sql": pulumi.Map{
							"host":           ssApiCfg.SQLPrivateIPAddress,
							"user":           pulumi.String(ssApiCfg.PgUsername),
							"existingSecret": pgSecret.Metadata.Name(),
							"secretKey":      pulumi.String("password"),
						},
					},
					"visibility": pulumi.Map{
						"sql": pulumi.Map{
							"host":           ssApiCfg.SQLPrivateIPAddress,
							"user":           pulumi.String(ssApiCfg.PgUsername),
							"existingSecret": pgSecret.Metadata.Name(),
							"secretKey":      pulumi.String("password"),
						},
					},
				},
			},
			"frontend": pulumi.Map{
				"resources": s.Resources.TemporalFrontend,
			},
			"history": pulumi.Map{
				"resources": s.Resources.TemporalHistory,
			},
			"matching": pulumi.Map{
				"resources": s.Resources.TemporalMatching,
			},
			"worker": pulumi.Map{
				"resources": s.Resources.TemporalWorker,
			},
		},
		//This will only be executed during temporal installation, following upgrades won´t execute it
		"initJob": pulumi.Map{
			"enabled":     pulumi.Bool(true),
			"image":       pulumi.String(fmt.Sprintf("%s/temporal_auto:0.0.7", s.GlobalTemporalImageRepository)),
			"pgHost":      ssApiCfg.SQLPrivateIPAddress,
			"pgPort":      pulumi.String("5432"),
			"pgUser":      pulumi.String(ssApiCfg.PgUsername),
			"pgSecret":    pgSecret.Metadata.Name(),
			"pgSecretKey": pulumi.String("password"),
		},
	}
	return s.DeployHelmRelease(ctx, ns, "temporal", TemporalChartVers, "", "temporal-values.yaml", customValues)
}

func (ssApiCfg *SelfServiceAPIConfig) DeployCrossplane(ctx *pulumi.Context, s *Stack) (*helmv3.Release, error) {
	ns, err := s.CreateNamespace(ctx, "crossplane-system")
	if err != nil {
		return nil, err
	}
	s.DependsOn = []pulumi.Resource{ns}

	customCrossplaneValues := pulumi.Map{
		"resourcesCrossplane": s.Resources.Crossplane,
		"tolerations":         nodeTolerationsByPlatformAndDistruptionType(s.Platform, false),
		"priorityClassName":   priorityClassByPlatformAndWorkloadType("deployment"),
	}

	helmRelease, err := s.DeployHelmRelease(ctx, ns, "crossplane", CrossplaneChartVers, "", "crossplane-values.yaml", customCrossplaneValues)
	if err != nil {
		return nil, err
	}

	if err = ssApiCfg.createCrossplaneKubernetesProvider(ctx, helmRelease, s); err != nil {
		return nil, err
	}

	if s.Platform == "gke" {
		if err = ssApiCfg.createCrossplaneGCPProvider(ctx, helmRelease, s); err != nil {
			return nil, err
		}
	} else {
		if err = ssApiCfg.CreateCrossplaneAWSProvider(ctx, helmRelease, s); err != nil {
			return nil, err
		}
	}
	if err = ssApiCfg.createCrossplaneHelmProvider(ctx, helmRelease, s); err != nil {
		return nil, err
	}

	return helmRelease, nil
}

func (ssApiCfg *SelfServiceAPIConfig) createAPIIRSA(ctx *pulumi.Context, saName string, s *Stack) (pulumi.StringInput, error) {
	policyJSON := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Action": [
					"ses:SendEmail",
					"ses:SendRawEmail"
				],
				"Resource": "*"
			},
            {
				"Effect": "Allow",
				"Action": [
					"s3:*", 
					"s3-object-lambda:*"
				],
				"Resource": ["*"]
            },
            {
				"Effect": "Allow",
				"Action": [
					"cognito-idp:ListUsers",
                    "cognito-idp:AdminGetUser",
                    "cognito-idp:AdminUpdateUserAttributes",
					"cognito-idp:ListUserPoolClients",
					"cognito-idp:DescribeUserPoolClient",
					"cognito-idp:UpdateUserPoolClient"

				],
				"Resource": ["*"]
            }
		]
	}`

	// IRSA

	serviceAccount := awsiam.EKSServiceAccountArgs{
		Name:            pulumi.String(s.ClusterName),
		ServiceAccounts: pulumi.ToStringArray([]string{fmt.Sprintf("tfm-api:%s", saName)}),
	}

	roleName := s.ClusterScopedResourceName("cloud-api-irsa")
	eksRole, err := awsiam.NewEKSRole(ctx, roleName, &awsiam.EKSRoleArgs{
		Role:                   awsiam.RoleArgs{Name: pulumi.String(roleName)},
		ClusterServiceAccounts: awsiam.EKSServiceAccountArray([]awsiam.EKSServiceAccountInput{serviceAccount}),
	}, pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return pulumi.String(""), err
	}
	policyName := s.ClusterScopedResourceName("cloud-api-irsa-policy")
	_, err = iam.NewRolePolicy(ctx, policyName,
		&iam.RolePolicyArgs{Name: pulumi.String(policyName), Role: eksRole.Name, Policy: pulumi.String(policyJSON)}, pulumi.DependsOn([]pulumi.Resource{eksRole}))
	if err != nil {
		return pulumi.String(""), err
	}

	s.DependsOn = append(s.DependsOn, eksRole)

	return eksRole.Arn, nil
}

func (apiConfig *SelfServiceAPIConfig) createCrossplaneKubernetesProvider(ctx *pulumi.Context, crossplaneRelease *helmv3.Release, s *Stack) error {
	sa, err := corev1.NewServiceAccount(ctx, "provider-kubernetes", &corev1.ServiceAccountArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("provider-kubernetes"), Namespace: pulumi.String("crossplane-system")},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{crossplaneRelease}))
	if err != nil {
		return err
	}

	s.DependsOn = append(s.DependsOn, sa)

	// needs cluster-admin to deploy K8s objects
	_, err = rbacv1.NewClusterRoleBinding(ctx, "provider-kubernetes-admin-binding", &rbacv1.ClusterRoleBindingArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String("provider-kubernetes-admin-binding"),
		},
		RoleRef: &rbacv1.RoleRefArgs{
			ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
			Kind:     pulumi.String("ClusterRole"),
			Name:     pulumi.String("cluster-admin"),
		},
		Subjects: rbacv1.SubjectArray{
			&rbacv1.SubjectArgs{
				Kind:      pulumi.String("ServiceAccount"),
				Name:      pulumi.String("provider-kubernetes"),
				Namespace: pulumi.String("crossplane-system"),
			},
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{sa}))
	if err != nil {
		return err
	}

	var k8sControllerCfgSpec map[string]interface{}

	if s.Platform == "gke" {

		k8sControllerCfgSpec, _ = JSONToMap(`{
	  "spec": {
        "args": ["--debug","--poll=30s","--max-reconcile-rate=20"],
        "serviceAccountName": "provider-kubernetes",
		"resources": {
          "requests": {
            "cpu": "2000m",
            "memory": "1100Mi"
          }
        }
	  }	  
	}`)
	} else {
		k8sControllerCfgSpec, _ = JSONToMap(`{
			"spec": {
			  "args": ["--debug"],
			  "serviceAccountName": "provider-kubernetes",
			  "nodeSelector": {
				"node-role": "not-disruptable"        
			  },
			  "tolerations": [
				{
				  "key": "tfm/not-disruptable",
				  "operator": "Exists",
				  "effect": "NoSchedule"
				}
			  ],
			  "resources": {
				"requests": {
				  "cpu": "2000m",
				  "memory": "1100Mi"
				}
			  }
			}
		  }`)
	}
	k8sControllerCfg, err := apiextensions.NewCustomResource(ctx, "controller-config-k8s", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("pkg.crossplane.io/v1alpha1"),
		Kind:       pulumi.String("ControllerConfig"),
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.String("controller-config-k8s"),
		},
		OtherFields: k8sControllerCfgSpec,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{sa}))
	if err != nil {
		return err
	}

	apiConfig.dependsOn = append(apiConfig.dependsOn, k8sControllerCfg)

	providerK8sSpec, _ := JSONToMap(fmt.Sprintf(`{
	  "spec": {
		"package": "xpkg.upbound.io/crossplane-contrib/provider-kubernetes:%s",
		"controllerConfigRef": {
			"name": "controller-config-k8s"
		}
	  }
	}`, crossplaneKubernetesProviderVers))
	providerK8s, err := apiextensions.NewCustomResource(ctx, "provider-kubernetes", &apiextensions.CustomResourceArgs{
		ApiVersion:  pulumi.String("pkg.crossplane.io/v1"),
		Kind:        pulumi.String("Provider"),
		Metadata:    metav1.ObjectMetaArgs{Name: pulumi.String("provider-kubernetes")},
		OtherFields: providerK8sSpec,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{k8sControllerCfg}))
	if err != nil {
		return err
	}

	k8sProviderConfigSpec, _ := JSONToMap(`{
	  "spec": {
		"credentials": {
			"source": "InjectedIdentity"
		}
	  }
	}`)
	_, err = apiextensions.NewCustomResource(ctx, "provider-config-kubernetes", &apiextensions.CustomResourceArgs{
		ApiVersion:  pulumi.String("kubernetes.crossplane.io/v1alpha1"),
		Kind:        pulumi.String("ProviderConfig"),
		Metadata:    metav1.ObjectMetaArgs{Name: pulumi.String("provider-config-kubernetes")},
		OtherFields: k8sProviderConfigSpec,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{providerK8s}))

	return err
}

func (apiConfig *SelfServiceAPIConfig) createCrossplaneHelmProvider(ctx *pulumi.Context, crossplaneRelease *helmv3.Release, s *Stack) error {
	providerK8sSpec, _ := JSONToMap(fmt.Sprintf(`{
	  "spec": {
		"package": "xpkg.upbound.io/crossplane-contrib/provider-helm:%s",
		"controllerConfigRef": {
			"name": "controller-config-k8s"
		}
	  }
	}`, crossplaneHelmProviderVers))
	providerHelm, err := apiextensions.NewCustomResource(ctx, "provider-helm", &apiextensions.CustomResourceArgs{
		ApiVersion:  pulumi.String("pkg.crossplane.io/v1"),
		Kind:        pulumi.String("Provider"),
		Metadata:    metav1.ObjectMetaArgs{Name: pulumi.String("provider-helm")},
		OtherFields: providerK8sSpec,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{crossplaneRelease}))
	if err != nil {
		return err
	}

	apiConfig.dependsOn = append(apiConfig.dependsOn, providerHelm)

	helmProviderConfigSpec, _ := JSONToMap(`{
	  "spec": {
		"credentials": {
			"source": "InjectedIdentity"
		}
	  }
	}`)
	_, err = apiextensions.NewCustomResource(ctx, "provider-config-helm", &apiextensions.CustomResourceArgs{
		ApiVersion:  pulumi.String("helm.crossplane.io/v1beta1"),
		Kind:        pulumi.String("ProviderConfig"),
		Metadata:    metav1.ObjectMetaArgs{Name: pulumi.String("provider-config-helm")},
		OtherFields: helmProviderConfigSpec,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{providerHelm}))

	return err
}

// Create the Crossplane AWS Provider using an IAM role for the Service Account to do AWS things
func (apiConfig *SelfServiceAPIConfig) CreateCrossplaneAWSProvider(ctx *pulumi.Context, crossplaneRelease *helmv3.Release, s *Stack) error {
	// see: https://github.com/crossplane-contrib/provider-aws/blob/master/AUTHENTICATION.md
	clusterName := s.ClusterName
	roleName := fmt.Sprintf("crossplane-aws-%s", clusterName)

	// TODO: this is broad access to the Account, restrict as needed
	policyArns := []string{"arn:aws:iam::aws:policy/AdministratorAccess"}

	// EksRole
	serviceAccount := awsiam.EKSServiceAccountArgs{
		Name:            pulumi.String(s.ClusterName),
		ServiceAccounts: pulumi.ToStringArray([]string{"crossplane-system:provider-aws"}),
	}

	eksRole, err := awsiam.NewEKSRole(ctx, roleName, &awsiam.EKSRoleArgs{
		Role:                   awsiam.RoleArgs{Name: pulumi.String(roleName)},
		ClusterServiceAccounts: awsiam.EKSServiceAccountArray([]awsiam.EKSServiceAccountInput{serviceAccount}),
		RolePolicyArns:         pulumi.ToStringArray(policyArns),
	}, pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}

	// The Crossplane Helm chart creates the SA, but we need to annotate it with the ARN of our IRSA

	// also see https://docs.upbound.io/providers/provider-aws/authentication/#webidentity
	// we're just using crossplane for now vs. upbound

	// TODO: the ControllerConfig CRD is deprecated, replace with DeploymentRuntimeConfig from pkg.crossplane.io/v1beta1
	awsConfigSpec, _ := JSONToMap(`{
	  "spec": {
        "serviceAccountName": "provider-aws",
        "args": ["--debug"],
		"podSecurityContext": {
			"fsGroup": 2000
		}
	  }
	}`)
	awsConfig, err := apiextensions.NewCustomResource(ctx, "controller-config-aws", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("pkg.crossplane.io/v1alpha1"),
		Kind:       pulumi.String("ControllerConfig"),
		Metadata: metav1.ObjectMetaArgs{
			Name:        pulumi.String("controller-config-aws"),
			Annotations: pulumi.StringMap{"eks.amazonaws.com/role-arn": eksRole.Arn},
		},
		OtherFields: awsConfigSpec,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{crossplaneRelease, eksRole}))
	if err != nil {
		return err
	}

	providerSpec, _ := JSONToMap(fmt.Sprintf(`{
	  "spec": {
		"package": "xpkg.upbound.io/crossplane-contrib/provider-aws:%s",
		"controllerConfigRef": {
			"name": "controller-config-aws"
		}
	  }
	}`, crossplaneAwsProviderVers))
	provider, err := apiextensions.NewCustomResource(ctx, "provider-aws", &apiextensions.CustomResourceArgs{
		ApiVersion:  pulumi.String("pkg.crossplane.io/v1"),
		Kind:        pulumi.String("Provider"),
		Metadata:    metav1.ObjectMetaArgs{Name: pulumi.String("provider-aws")},
		OtherFields: providerSpec,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{awsConfig}))
	if err != nil {
		return err
	}

	// Provider Config
	providerConfigSpec, _ := JSONToMap(`{
	  "spec": {
		"credentials": {
			"source": "InjectedIdentity"
		}
	  }
	}`)
	_, err = apiextensions.NewCustomResource(ctx, "provider-config-aws", &apiextensions.CustomResourceArgs{
		ApiVersion:  pulumi.String("aws.crossplane.io/v1beta1"),
		Kind:        pulumi.String("ProviderConfig"),
		Metadata:    metav1.ObjectMetaArgs{Name: pulumi.String("provider-config-aws")},
		OtherFields: providerConfigSpec,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{provider}))

	return err
}

// Create the Crossplane GCP Provider using an IAM role for the Service Account to do GCP things
func (apiConfig *SelfServiceAPIConfig) createCrossplaneGCPProvider(ctx *pulumi.Context, crossplaneRelease *helmv3.Release, s *Stack) error {
	// see: https://marketplace.upbound.io/providers/upbound/provider-family-gcp/v0.41.0/docs

	// kube SA

	saEnt, err := serviceaccount.LookupAccount(ctx, &serviceaccount.LookupAccountArgs{
		AccountId: s.GlobalGKEServiceAccount,
		Project:   &s.Project,
	})
	if err != nil {
		return err
	}

	cmSaPatch, err := corev1.NewServiceAccountPatch(ctx, "crossplane-annotation", &corev1.ServiceAccountPatchArgs{
		Metadata: &metav1.ObjectMetaPatchArgs{
			Name: pulumi.String("crossplane"),
			Annotations: pulumi.StringMap{
				"iam.gke.io/gcp-service-account": pulumi.String(s.GlobalGKEServiceAccount),
			},
			Namespace: pulumi.String("crossplane-system"),
		},
	}, pulumi.DependsOn(s.DependsOn))

	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, cmSaPatch)

	// The Crossplane Helm chart creates the SA, but we need to annotate it with the ARN of our IRSA

	gcpConfigSpec, _ := JSONToMap(`{
	  "spec": {
        "serviceAccountName": "provider-gcp",
        "args": ["--debug"],
		"podSecurityContext": {
			"fsGroup": 2000
		}
	  }
	}`)
	gcpConfig, err := apiextensions.NewCustomResource(ctx, "controller-config-gcp", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("pkg.crossplane.io/v1alpha1"),
		Kind:       pulumi.String("ControllerConfig"),
		Metadata: metav1.ObjectMetaArgs{
			Name:        pulumi.String("controller-config-gcp"),
			Annotations: pulumi.StringMap{"iam.gke.io/gcp-service-account": pulumi.String(saEnt.Email)},
		},
		OtherFields: gcpConfigSpec,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{crossplaneRelease, cmSaPatch}))
	if err != nil {
		return err
	}

	providerSpec, _ := JSONToMap(fmt.Sprintf(`{
	  "spec": {
		"package": "xpkg.upbound.io/upbound/provider-gcp-storage:%s",
		"controllerConfigRef": {
			"name": "controller-config-gcp"
		}
	  }
	}`, crossplaneGcpProviderVers))
	gcpProvider, err := apiextensions.NewCustomResource(ctx, "provider-gcp", &apiextensions.CustomResourceArgs{
		ApiVersion:  pulumi.String("pkg.crossplane.io/v1"),
		Kind:        pulumi.String("Provider"),
		Metadata:    metav1.ObjectMetaArgs{Name: pulumi.String("provider-gcp-storage")},
		OtherFields: providerSpec,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{gcpConfig}))
	if err != nil {
		return err
	}

	apiConfig.dependsOn = append(apiConfig.dependsOn, gcpProvider)

	// Provider Config
	providerConfigSpec, _ := JSONToMap(`{
	  "spec": {
	  	"projectID" :  "cloud-platform-tfm",
		"credentials": {
			"source": "InjectedIdentity"
		}
	  }
	}`)
	gcpProviderConfig, err := apiextensions.NewCustomResource(ctx, "provider-config-gcp", &apiextensions.CustomResourceArgs{
		ApiVersion:  pulumi.String("gcp.upbound.io/v1beta1"),
		Kind:        pulumi.String("ProviderConfig"),
		Metadata:    metav1.ObjectMetaArgs{Name: pulumi.String("provider-config-gcp")},
		OtherFields: providerConfigSpec,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{gcpProvider}))

	apiConfig.dependsOn = append(apiConfig.dependsOn, gcpProviderConfig)
	return err
}
