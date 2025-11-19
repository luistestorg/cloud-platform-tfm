package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/cloudformation"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/cloudwatch"

	awsiam "github.com/pulumi/pulumi-aws-iam/sdk/go/aws-iam"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/lb"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/route53"
	ec2x "github.com/pulumi/pulumi-awsx/sdk/go/awsx/ec2"
	"github.com/pulumi/pulumi-eks/sdk/go/eks"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	storagev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/storage/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	"tracemachina.com/shared"
)

const (
	GlobalHelmChartPath           = "helm-charts"
	GlobalDashboardPath           = "../config/dashboards"
	GlobalKibanaDashboardPath     = "../config/dashboards/%s-dashboards.ndjson"
	GlobalConfigPath              = "../config"
	GlobalCrossplanePath          = "./crossplane/"
	GlobalTemporalImageRepository = "299166832260.dkr.ecr.us-west-2.amazonaws.com"

	ebsCsiDriverChartVers            = "2.32.0"
	metricsServerChartVers           = "3.12.2"
	karpenterChartVers               = "0.37.0" // helm fetch oci://public.ecr.aws/karpenter/karpenter --version=0.37.0
	ciliumChartVers                  = "1.15.6"
	awsGatewayAPIControllerChartVers = "v1.1.0" // helm fetch oci://public.ecr.aws/aws-application-networking-k8s/aws-gateway-controller-chart --version=v1.1.0
	kubecostChartVers                = "2.4.2-3"

	vpcFlowLogsName = "vpc-flow-logs"
)

type (
	EksConfig struct {
		VpcName                  string
		AwsAccountID             string
		ControlPlaneInstanceType string
		WorkerNodeAmiID          string
		K8sVersion               string
		AutoTags                 map[string]string
		AwsAuthIamAdminRole      string
		AwsAuthIamReadOnlyRole   string
		Env                      string
		ClusterName              string

		EnableLogStack       bool
		ManagedNodeGroupName string

		MongoStorageClass     string
		MongoStorageSnapshot  string
		MongoUseSts           bool
		MongoExistingClaim    string
		MongoRootPassword     pulumi.StringOutput // loaded at runtime from a secret
		MongoDatabasePassword pulumi.StringOutput // loaded at runtime from a secret

		SharedRedisStorageClass string
		LogGroupRetention       int

		ClusterRedisSize         string
		ClusterRedisStorageClass string
		ClusterRedisZone         string

		CreateSharedNativeLinkNamespace  bool
		CreateRunbooksNamespace          bool
		SharedRedisSize                  string
		MtRedisSize                      string
		MtRedisZone                      string
		MongoZone                        string
		MongoSize                        string
		EnableCilium                     bool
		EnableMongoDB                    bool
		EnableNats                       bool
		EnableRedisCluster               bool
		EnableBuildBarn                  bool
		EnableElasticSearch              bool
		EnableActionsRunnerController    bool
		KarpenterSpotToSpotConsolidation bool
		VpcFlowLogsEnabled               bool

		EnableKubeCost             bool
		EnableTekton               bool
		EnableAwsGatewayController bool
		KubecostPvcSnapshot        string
	}
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// see Pulumi-<s>.yaml
		awsConfig := config.New(ctx, "aws")
		region := awsConfig.Require("region")

		cfg := config.New(ctx, "")
		eksCfg := initEKSConfig(cfg)
		tlsCfg := initTLSSharedConfig(cfg)

		ssApiCfg := initSelfServiceApiConfig(cfg)

		ciSupportCfg := initCiSupportConfig(cfg)

		nlSharedCfg := initNLSharedConfig(cfg)

		// add common tag set to all taggable AWS resources
		// from: https://github.com/joeduffy/aws-tags-example
		if len(eksCfg.AutoTags) > 0 {
			_ = RegisterAutoTags(ctx, eksCfg.AutoTags)
		}

		available, err := aws.GetAvailabilityZones(ctx, &aws.GetAvailabilityZonesArgs{
			State: pulumi.StringRef("available"),
		}, nil)
		if err != nil {
			fmt.Printf("Failed to get availability zones due to: %v\n", err)
			return err
		}
		fmt.Printf("Available zones: %s\n", strings.Join(available.Names, ", "))

		// uses the default cidr block: "10.0.0.0/16"
		vpc, err := ec2x.NewVpc(ctx, eksCfg.VpcName, nil)
		if err != nil {
			return err
		}

		ctx.Export("domain", pulumi.String(tlsCfg.Domain))
		ctx.Export("vpcId", vpc.VpcId)
		ctx.Export("privateSubnetIds", vpc.PrivateSubnetIds)
		ctx.Export("publicSubnetIds", vpc.PublicSubnetIds)

		if eksCfg.VpcFlowLogsEnabled {
			if err = CreateFlowLogs(ctx, vpc, eksCfg); err != nil {
				return err
			}
		}

		s, err := CreateCluster(ctx, vpc, eksCfg, region, tlsCfg)
		if err != nil {
			return err
		}
		ctx.Export("cluster", pulumi.String(s.ClusterName))

		// Using these shared stacks for having a single process for GKE and AWS.
		// Other related values are being kept because other services depend on them

		sharedOauthConfig := initOauthConfig(cfg)

		monStackConfig := initMonStack(cfg)

		s.OauthConfig = sharedOauthConfig
		s.GlobalHelmChartPath = GlobalHelmChartPath
		s.GlobalDashboardPath = GlobalDashboardPath
		s.GlobalKibanaDashboardPath = GlobalKibanaDashboardPath
		s.GlobalConfigPath = GlobalConfigPath
		s.CiSupportCfg = ciSupportCfg
		s.GlobalCrossplanePath = GlobalCrossplanePath
		s.GlobalTemporalImageRepository = GlobalTemporalImageRepository

		s.Route53RoleName = s.ClusterScopedResourceName("route53-dns")
		iamRole, err := createRoute53IamRole(ctx, s.Route53RoleName, s, eksCfg)
		if err != nil {
			return err
		}
		s.Route53IamRole = iamRole

		ssApiCfg.GrafanaAdminPassword = cfg.RequireSecret("grafanaAdminPassword")

		//Could be redundant but this works for just using the api stack for both GKE and AWS
		ssApiCfg.EnableMongoDB = nlSharedCfg.EnableMongoDB
		ssApiCfg.MongoDatabasePassword = nlSharedCfg.MongoDatabasePassword
		ssApiCfg.MongoRootPassword = nlSharedCfg.MongoRootPassword
		ssApiCfg.SharedRedisZone = nlSharedCfg.SharedRedisZone
		s.EnableAwsGatewayController = eksCfg.EnableAwsGatewayController
		s.AwsAccountID = eksCfg.AwsAccountID
		s.VpcFlowLogsEnabled = eksCfg.VpcFlowLogsEnabled

		alertWebhookUrl, err := cfg.TrySecret("alertWebhookUrl")
		if err == nil {
			s.AlertWebhookURL = &alertWebhookUrl
		}

		slackWebhookUrl, err := cfg.TrySecret("slackWebhookUrl")
		if err == nil {
			s.SlackWebhookURL = &slackWebhookUrl
		}

		// Using double resource definition, when the configs are migrated to the shared env, only one will be used
		if s.Env == "dev" {
			s.Resources.InitResourcesDev()
		} else {
			if s.Env == "prod" {
				s.Resources.InitResourcesProd()
			}
		}

		if err = ConfigureAwsAuthIamRoles(ctx, s, eksCfg); err != nil {
			return err
		}

		if eksCfg.EnableAwsGatewayController {
			if err = DeployAwsGatewayController(ctx, s); err != nil {
				return err
			}
		}

		fmt.Printf("EKS cluster '%s' created, deploying control plane components ...\n", eksCfg.ClusterName)

		if err = DeployControlPlaneComponents(ctx, s, eksCfg, ssApiCfg, monStackConfig, nlSharedCfg); err != nil {
			return err
		}

		fmt.Printf("CreateSharedNativeLinkNamespace? %t\n", nlSharedCfg.CreateSharedNativeLinkNamespace)
		if nlSharedCfg.CreateSharedNativeLinkNamespace {
			// we need to create the nativelink-shared namespace and wildcard cert ...
			// the API creates the NativeLink claim (since it does that for all other claims)
			if err = nlSharedCfg.DeploySharedNamespace(ctx, s); err != nil {
				return err
			}
		}

		if eksCfg.CreateRunbooksNamespace {
			// Creates runbooks namespace and deploys runbooks app
			if err = s.DeployRunbooks(ctx, ssApiCfg); err != nil {
				return err
			}
		}

		// lastly, are we deploying the self-service API into this cluster?
		if ssApiCfg.APIEnabled {
			fmt.Println("Deploying the self-service app ...")
			if err = ssApiCfg.DeploySelfServiceAPI(ctx, s); err != nil {
				return err
			}
		} else {
			fmt.Println("Not deploying the self-service app.")
		}

		ctx.Export("ingress", pulumi.ToStringArray(s.IngressHosts))

		if eksCfg.VpcFlowLogsEnabled {
			if err = UpdateEksFlowLogRetention(ctx, s); err != nil {
				return err
			}
		}

		return nil
	})
}

func initCiSupportConfig(cfg *config.Config) *shared.CiSupportSharedStack {

	var secCompConfig shared.CiSupportSharedStack
	secCompConfig.EnableActionsRunnerController = cfg.GetBool("enableActionsController")

	if secCompConfig.EnableActionsRunnerController {

		secCompConfig.GithubConfigURL = cfg.Require("githubConfigUrl")
		secCompConfig.GithubActionsAppID = cfg.RequireSecret("github_app_id")
		secCompConfig.GithubActionsAppInstID = cfg.RequireSecret("github_app_installation_id")
		secCompConfig.GithubActionsPrivateKey = cfg.RequireSecret("github_app_private_key")
	}

	secCompConfig.EnableTekton = cfg.GetBool("enableTekton")

	return &secCompConfig

}

func initOauthConfig(cfg *config.Config) *shared.OauthConfig {

	var oauthConfig shared.OauthConfig
	_ = cfg.GetObject("oauthConfig", &oauthConfig)

	if oauthConfig.Oauth2Provider == "" {
		oauthConfig.Oauth2Provider = "oidc"
	}

	if oauthConfig.Oauth2Scope == "" {
		oauthConfig.Oauth2Scope = "openid email"
	}

	oauthConfig.Oauth2ClientSecret = cfg.RequireSecret("oauth2ClientSecret")
	oauthConfig.Oauth2CookieSecret = cfg.RequireSecret("oauth2CookieSecret")

	return &oauthConfig

}

func initNLSharedConfig(cfg *config.Config) *shared.NLSharedStack {

	var nlSharedConfig shared.NLSharedStack

	cfg.RequireObject("eks", &nlSharedConfig)

	if nlSharedConfig.EnableMongoDB {
		nlSharedConfig.MongoRootPassword = cfg.RequireSecret("mongoRootPassword")
		nlSharedConfig.MongoDatabasePassword = cfg.RequireSecret("mongoDatabasePassword")
	}

	if nlSharedConfig.SharedRedisStorageClass == "" {
		nlSharedConfig.SharedRedisStorageClass = "standard"
	}
	return &nlSharedConfig

}

func initTLSSharedConfig(cfg *config.Config) *shared.TLSConfig {
	var tlsCfg shared.TLSConfig
	cfg.RequireObject("tls", &tlsCfg)
	if tlsCfg.AcmeServer == "" {
		// fallback to using the let's encrypt staging server
		tlsCfg.AcmeServer = "https://acme-staging-v02.api.letsencrypt.org/directory"
	}
	if tlsCfg.Email == "" {
		tlsCfg.Email = "tim@tracemachina.com"
	}
	return &tlsCfg
}

func initMonStack(cfg *config.Config) *shared.MonSharedStack {
	var monCfg shared.MonSharedStack
	cfg.RequireObject("mon", &monCfg)
	monCfg.GrafanaAdminPassword = cfg.RequireSecret("grafanaAdminPassword")

	monCfg.KibanaPassword = cfg.RequireSecret("kibanaPassword")

	if monCfg.PrometheusStorage == "" {
		monCfg.PrometheusStorage = "50Gi"
	}
	if monCfg.PrometheusStorageClass == "" {
		monCfg.PrometheusStorageClass = "gp3-8k-iops"
	}
	if monCfg.PrometheusMemoryRequests == "" {
		monCfg.PrometheusMemoryRequests = "20Gi"
	}
	if monCfg.PrometheusCPURequests == "" {
		monCfg.PrometheusCPURequests = "3"
	}
	if monCfg.PrometheusReplicas <= 0 {
		monCfg.PrometheusReplicas = 1
	}

	if monCfg.GrafanaStorageClass == "" {
		monCfg.GrafanaStorageClass = "gp3-enc"
	}

	if monCfg.GrafanaStorage == "" {
		monCfg.GrafanaStorage = "20Gi"
	}

	if monCfg.ElasticSearchStorageSize == "" {
		monCfg.ElasticSearchStorageSize = "30Gi"
	}
	if monCfg.KibanaStorageSize == "" {
		monCfg.KibanaStorageSize = "30Gi"
	}

	monCfg.ElasticSearchPassword = cfg.RequireSecret("elasticSearchPassword")

	return &monCfg
}

// initialize the EKS configuration, installing defaults as needed
func initEKSConfig(cfg *config.Config) *EksConfig {
	var eksCfg EksConfig
	cfg.RequireObject("eks", &eksCfg)
	if eksCfg.ControlPlaneInstanceType == "" {
		eksCfg.ControlPlaneInstanceType = "m6i.large"
	}
	if eksCfg.K8sVersion == "" {
		eksCfg.K8sVersion = "1.28"
	}

	if eksCfg.EnableMongoDB {
		eksCfg.MongoRootPassword = cfg.RequireSecret("mongoRootPassword")
		eksCfg.MongoDatabasePassword = cfg.RequireSecret("mongoDatabasePassword")
	}

	if eksCfg.SharedRedisStorageClass == "" {
		eksCfg.SharedRedisStorageClass = "gp3-8k-iops-enc"
	}

	if eksCfg.LogGroupRetention == 0 {
		eksCfg.LogGroupRetention = 7
	}

	return &eksCfg
}

func initSelfServiceApiConfig(cfg *config.Config) *shared.SelfServiceAPIConfig {
	var ssApiCfg shared.SelfServiceAPIConfig
	_ = cfg.GetObject("selfServiceApi", &ssApiCfg)

	if ssApiCfg.APIEnabled {
		ssApiCfg.DbPassword = cfg.RequireSecret("dbPassword")
		ssApiCfg.PgPassword = cfg.RequireSecret("pgPassword")
		ssApiCfg.CachePassword = cfg.RequireSecret("cachePassword")
		ssApiCfg.SharedCachePassword = cfg.RequireSecret("sharedCachePassword")
		ssApiCfg.BootstrapAdminPassword = cfg.RequireSecret("bootstrapAdminPassword")

		if ssApiCfg.RdsInstanceType == "" {
			ssApiCfg.RdsInstanceType = "db.t4g.small"
		}

		if ssApiCfg.SubDomain == "" {
			ssApiCfg.SubDomain = "api"
		}
	}

	if ssApiCfg.SegmentAPIEnabled {
		ssApiCfg.SegmentAPIWriteKey = cfg.RequireSecret("segmentApiWriteKey")
	}

	if ssApiCfg.DbOffsiteBackupBucket != "" {
		ssApiCfg.DbOffsiteBackupGCSKeyFile = cfg.RequireSecret("dbOffsiteBackupGCSKey")
	}

	if ssApiCfg.APIEnabled && ssApiCfg.NativeLinkDbEnabled {
		ssApiCfg.NativelinkDbPassword = cfg.RequireSecret("nativelinkDbPassword")
	}

	ssApiCfg.GithubAuthToken = cfg.RequireSecret("githubAuthToken")

	return &ssApiCfg
}

func CreateCluster(ctx *pulumi.Context, vpc *ec2x.Vpc, eksCfg *EksConfig, awsRegion string, tlsCfg *shared.TLSConfig) (*shared.Stack, error) {
	instanceRoleName := fmt.Sprintf("%s-instance-role", eksCfg.ClusterName)
	instanceRole, err := createEksInstanceRole(ctx, instanceRoleName)
	if err != nil {
		return nil, err
	}

	minSize := pulumi.Int(2)
	maxSize := pulumi.Int(3)
	if eksCfg.ManagedNodeGroupName != "" {
		minSize = pulumi.Int(0)
		maxSize = pulumi.Int(0)
	}

	/*
		todo: logs disabled until we can fix crossplane:

				EnabledClusterLogTypes: pulumi.StringArray{
				pulumi.String("api"),
				pulumi.String("audit"),
				pulumi.String("authenticator"),
			},

	*/

	clusterArgs := &eks.ClusterArgs{
		Name:                         pulumi.String(eksCfg.ClusterName),
		Version:                      pulumi.String(eksCfg.K8sVersion),
		CreateOidcProvider:           pulumi.BoolPtr(true),
		InstanceType:                 pulumi.String(eksCfg.ControlPlaneInstanceType),
		MinSize:                      minSize,
		MaxSize:                      maxSize,
		DesiredCapacity:              minSize,
		VpcId:                        vpc.VpcId,
		PublicSubnetIds:              vpc.PublicSubnetIds,
		PrivateSubnetIds:             vpc.PrivateSubnetIds,
		NodeAssociatePublicIpAddress: pulumi.BoolRef(false),
		InstanceRole:                 instanceRole,
	}

	// Setting this prevents build-out from working properly :(
	// Keeping for legacy clusters only
	if eksCfg.AwsAuthIamAdminRole != "" && (eksCfg.ClusterName == "build-faster" || eksCfg.ClusterName == "dev-usw2") {
		authRoleArn := pulumi.Sprintf("arn:aws:iam::%s:role/%s", eksCfg.AwsAccountID, eksCfg.AwsAuthIamAdminRole)
		clusterArgs.ProviderCredentialOpts = eks.KubeconfigOptionsArgs{RoleArn: authRoleArn}
	}

	if eksCfg.WorkerNodeAmiID != "" {
		clusterArgs.NodeAmiId = pulumi.String(eksCfg.WorkerNodeAmiID)
	}

	cluster, err := eks.NewCluster(ctx, eksCfg.ClusterName, clusterArgs)
	if err != nil {
		return nil, err
	}
	dependsOn := []pulumi.Resource{cluster}

	var managedNodeGroup *eks.ManagedNodeGroup
	if eksCfg.ManagedNodeGroupName != "" {
		fmt.Printf("ManagedNodeGroupName: %s\n", eksCfg.ManagedNodeGroupName)
		nodeGroupArgs := &eks.ManagedNodeGroupArgs{
			Cluster:       cluster,
			ClusterName:   cluster.EksCluster.Name(),
			NodeRoleArn:   instanceRole.Arn,
			SubnetIds:     vpc.PrivateSubnetIds,
			InstanceTypes: pulumi.StringArray{pulumi.String(eksCfg.ControlPlaneInstanceType)},
			DiskSize:      pulumi.Int(50),
		}
		mng, err := eks.NewManagedNodeGroup(ctx, eksCfg.ManagedNodeGroupName, nodeGroupArgs, pulumi.DependsOn([]pulumi.Resource{cluster}))
		if err != nil {
			return nil, err
		}
		managedNodeGroup = mng
		dependsOn = append(dependsOn, mng)
	}

	k8sProvider, err := kubernetes.NewProvider(ctx, "k8s-provider", &kubernetes.ProviderArgs{Kubeconfig: cluster.KubeconfigJson})
	if err != nil {
		return nil, err
	}
	ctx.Export("kubeconfig", cluster.Kubeconfig)

	// we need the OIDC ID to create the IRSA for NativeLinkClaim deployments (via Crossplane)
	eksOIDC := cluster.EksCluster.Endpoint().ApplyT(func(ep string) string {
		clusterUnique := strings.Split(strings.TrimPrefix(ep, "https://"), ".")[0]
		oidcID := fmt.Sprintf("oidc.eks.%s.amazonaws.com/id/%s", awsRegion, clusterUnique)
		return oidcID
	}).(pulumi.StringOutput)
	ctx.Export("eksOIDC", eksOIDC)

	// Apply the v1.18.1 AWS VPC CNI upgrade (see: https://github.com/aws/amazon-vpc-cni-k8s/releases/tag/v1.18.2)
	awsNodeConfigMap, err := corev1.NewConfigMap(ctx, "aws-node", &corev1.ConfigMapArgs{
		ApiVersion: pulumi.String("v1"),
		Data:       pulumi.StringMap{"CLUSTER_NAME": pulumi.String(eksCfg.ClusterName), "VPC_ID": vpc.VpcId},
		Kind:       pulumi.String("ConfigMap"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("aws-node"), Namespace: pulumi.String("kube-system")},
	}, pulumi.Provider(k8sProvider), pulumi.DependsOn(dependsOn))
	if err != nil {
		return nil, err
	}
	dependsOn = append(dependsOn, awsNodeConfigMap)

	awsVpcCniConfigYaml, err :=
		yaml.NewConfigFile(ctx, "aws-k8s-cni-yaml", &yaml.ConfigFileArgs{File: "./config/aws-k8s-cni.yaml"},
			pulumi.Provider(k8sProvider), pulumi.DependsOn(dependsOn))
	if err != nil {
		return nil, err
	}
	dependsOn = append(dependsOn, awsVpcCniConfigYaml)

	// holds all the stack state during cluster bootstrapping
	s := &shared.Stack{
		ClusterName:       eksCfg.ClusterName, // a bit redundant, but used in many places ;)
		Region:            awsRegion,
		Eks:               cluster,
		K8sProvider:       k8sProvider,
		InstanceRoleName:  instanceRoleName,
		InstanceRole:      instanceRole,
		TLSCfg:            tlsCfg,
		DependsOn:         dependsOn,
		Vpc:               vpc,
		OidcID:            eksOIDC,
		LogGroupRetention: eksCfg.LogGroupRetention,
		Env:               eksCfg.Env,
		Platform:          "aws",
		AwsAccountID:      eksCfg.AwsAccountID,
	}

	if managedNodeGroup != nil {
		s.DependsOn = append(s.DependsOn, managedNodeGroup)
	}

	return s, nil
}
func createEksInstanceRole(ctx *pulumi.Context, roleName string) (*iam.Role, error) {
	arns := pulumi.ToStringArray([]string{
		"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
		"arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
		"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
	})
	assumeRolePolicy := pulumi.String(`{
		    "Version": "2012-10-17",
		    "Statement": [{
		        "Sid": "",
		        "Effect": "Allow",
		        "Principal": {
		            "Service": "ec2.amazonaws.com"
		        },
		        "Action": "sts:AssumeRole"
		    }]
		}`)

	return iam.NewRole(ctx, roleName, &iam.RoleArgs{Name: pulumi.String(roleName), AssumeRolePolicy: assumeRolePolicy, ManagedPolicyArns: arns})
}

func DeployAwsGatewayController(ctx *pulumi.Context, s *shared.Stack) error {
	/*
	   PREFIX_LIST_ID=$(aws ec2 describe-managed-prefix-lists --query "PrefixLists[?PrefixListName=="\'com.amazonaws.$AWS_REGION.vpc-lattice\'"].PrefixListId" | jq -r '.[]')
	   aws ec2 authorize-security-group-ingress --group-id $CLUSTER_SG --ip-permissions "PrefixListIds=[{PrefixListId=${PREFIX_LIST_ID}}],IpProtocol=-1"
	   PREFIX_LIST_ID_IPV6=$(aws ec2 describe-managed-prefix-lists --query "PrefixLists[?PrefixListName=="\'com.amazonaws.$AWS_REGION.ipv6.vpc-lattice\'"].PrefixListId" | jq -r '.[]')
	   aws ec2 authorize-security-group-ingress --group-id $CLUSTER_SG --ip-permissions "PrefixListIds=[{PrefixListId=${PREFIX_LIST_ID_IPV6}}],IpProtocol=-1"
	*/
	filter := ec2.GetManagedPrefixListsFilter{Name: "prefix-list-name", Values: []string{fmt.Sprintf("com.amazonaws.%s.vpc-lattice", s.Region)}}
	lists, err := ec2.GetManagedPrefixLists(ctx, &ec2.GetManagedPrefixListsArgs{Filters: []ec2.GetManagedPrefixListsFilter{filter}})
	if err != nil {
		fmt.Printf("\nERROR: describe-managed-prefix-lists failed due to: %v\n", err)
		return err
	}

	prefixID := ""
	if lists != nil && len(lists.Ids) > 0 {
		prefixID = lists.Ids[0]
	}
	if prefixID == "" {
		fmt.Printf("\nWARN: describe-managed-prefix-lists returned empty results\n")
		return nil
	}

	_, err = ec2.NewSecurityGroupRule(ctx, "sg-ingress-vpclattice", &ec2.SecurityGroupRuleArgs{
		SecurityGroupId: s.Eks.EksCluster.VpcConfig().ClusterSecurityGroupId().Elem(),
		FromPort:        pulumi.Int(-1),
		ToPort:          pulumi.Int(-1),
		Protocol:        pulumi.String("-1"),
		PrefixListIds:   pulumi.ToStringArray([]string{prefixID}),
		Type:            pulumi.String("ingress"),
	})
	if err != nil {
		fmt.Printf("\nERROR: authorize-security-group-ingress failed due to: %v \n", err)
		return err
	}

	nsName := "aws-application-networking-system"
	ns, err := s.CreateNamespace(ctx, nsName)
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, ns)

	// IRSA for the controller
	saAndNs := fmt.Sprintf("%s:gateway-api-controller", nsName)
	roleName := s.ClusterScopedResourceName("aws-gateway-api-irsa")
	serviceAccount := awsiam.EKSServiceAccountArgs{Name: s.Eks.EksCluster.Name(), ServiceAccounts: pulumi.ToStringArray([]string{saAndNs})}
	eksRole, err := awsiam.NewEKSRole(ctx, roleName, &awsiam.EKSRoleArgs{
		Role:                   awsiam.RoleArgs{Name: pulumi.String(roleName)},
		ClusterServiceAccounts: awsiam.EKSServiceAccountArray([]awsiam.EKSServiceAccountInput{serviceAccount}),
	}, pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	policyName := s.ClusterScopedResourceName("aws-gateway-api-irsa-policy")
	policyJSON := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Action": [
					"vpc-lattice:*",
					"ec2:DescribeVpcs",
					"ec2:DescribeSubnets",
					"ec2:DescribeTags",
					"ec2:DescribeSecurityGroups",
					"logs:CreateLogDelivery",
					"logs:GetLogDelivery",
					"logs:DescribeLogGroups",
					"logs:PutResourcePolicy",
					"logs:DescribeResourcePolicies",
					"logs:UpdateLogDelivery",
					"logs:DeleteLogDelivery",
					"logs:ListLogDeliveries",
					"tag:GetResources",
					"firehose:TagDeliveryStream",
					"s3:GetBucketPolicy",
					"s3:PutBucketPolicy"
				],
				"Resource": "*"
			},
			{
				"Effect" : "Allow",
				"Action" : "iam:CreateServiceLinkedRole",
				"Resource" : "arn:aws:iam::*:role/aws-service-role/vpc-lattice.amazonaws.com/AWSServiceRoleForVpcLattice",
				"Condition" : {
					"StringLike" : {
						"iam:AWSServiceName" : "vpc-lattice.amazonaws.com"
					}
				}
			},
			{
				"Effect" : "Allow",
				"Action" : "iam:CreateServiceLinkedRole",
				"Resource" : "arn:aws:iam::*:role/aws-service-role/delivery.logs.amazonaws.com/AWSServiceRoleForLogDelivery",
				"Condition" : {
					"StringLike" : {
						"iam:AWSServiceName" : "delivery.logs.amazonaws.com"
					}
				}
			}
		]
	}`
	rolePolicy, err := iam.NewRolePolicy(ctx, policyName,
		&iam.RolePolicyArgs{Name: pulumi.String(policyName), Role: eksRole.Name, Policy: pulumi.String(policyJSON)}, pulumi.DependsOn([]pulumi.Resource{eksRole}))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, rolePolicy)

	customValues := pulumi.Map{
		"fullnameOverride": pulumi.String("gateway-api"),
		"deployment": pulumi.Map{
			"replicas": pulumi.Int(1),
		},
		"serviceAccount": pulumi.Map{
			"annotations": pulumi.StringMap{"eks.amazonaws.com/role-arn": eksRole.Arn},
		},
		"defaultServiceNetwork": pulumi.String("test-grpc-gateway"),
		"resources":             s.Resources.GatewayApi,
	}
	if _, err = s.DeployHelmRelease(ctx, ns, "aws-gateway-controller-chart", awsGatewayAPIControllerChartVers, "", "", customValues); err != nil {
		return err
	}

	configGroup, err := yaml.NewConfigGroup(ctx, "gateway-api-crds", &yaml.ConfigGroupArgs{
		Files: []string{"https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/experimental-install.yaml"},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, configGroup)

	gatewayClassSpec, err := shared.JSONToMap(`{ "spec": { "controllerName": "application-networking.k8s.aws/gateway-api-controller" }}`)
	if err != nil {
		return err
	}
	gatewayClass, err := apiextensions.NewCustomResource(ctx, "aws-vpc-lattice-gateway", &apiextensions.CustomResourceArgs{
		ApiVersion:  pulumi.String("gateway.networking.k8s.io/v1beta1"),
		Kind:        pulumi.String("GatewayClass"),
		Metadata:    metav1.ObjectMetaArgs{Name: pulumi.String("amazon-vpc-lattice"), Namespace: ns.Metadata.Name()},
		OtherFields: gatewayClassSpec,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, gatewayClass)

	return nil
}

func DeployControlPlaneComponents(ctx *pulumi.Context, s *shared.Stack, ekscfg *EksConfig, ssAPICfg *shared.SelfServiceAPIConfig, monStackCfg *shared.MonSharedStack, nlSharedCfg *shared.NLSharedStack) error {
	var err error

	if ekscfg.EnableCilium {
		if _, err = deployCilium(ctx, s); err != nil {
			return err
		}
	}

	if _, err = deployMetricsServer(ctx, s); err != nil {
		return err
	}

	if _, err = deployEbsCsiDriver(ctx, s); err != nil {
		return err
	}

	if _, err = DeployKarpenter(ctx, s, ekscfg); err != nil {
		return err
	}

	if err = deployKarpenterNodePool(ctx, s); err != nil {
		return err
	}

	//Using monSharedStack for the monitoring functions and values, we can use a single stack for migrating all the configs but this is the first step

	monStackCfg.ClusterIssuer = shared.GlobalClusterIssuer
	monStackCfg.RdsZone = ssAPICfg.SQLZone
	monStackCfg.BootstrapAdminPassword = ssAPICfg.BootstrapAdminPassword

	if _, err = monStackCfg.DeployMonitoringComponents(ctx, s); err != nil {
		return err
	}
	if err = monStackCfg.DeployCustomDashboards(ctx, s); err != nil {
		return err
	}

	// There are seemingly *many* nginx ingress controllers out there, we're using the one from:
	// https://github.com/kubernetes/ingress-nginx
	if _, err = s.DeployIngressNginxController(ctx); err != nil {
		return err
	}

	if _, err = s.DeployCertManager(ctx); err != nil {
		return err
	}

	if _, err = s.CreateTLSCertIssuer(ctx); err != nil {
		return err
	}

	// now we need to wait until the NLB has been provisioned in AWS before we can create Ingress with TLS certs
	nlb, err := waitForLoadBalancer(ctx, s.ClusterName, 15*time.Minute)
	if err != nil {
		return err
	}
	ctx.Export("nlbDns", pulumi.String(nlb.DnsName))

	// Map our Domain to our NLB using a Route53 A Record
	if err = createRoute53RecordForNLB(ctx, nlb, s); err != nil {
		return err
	}

	// expose Grafana over https
	annotationMap := map[string]string{"nginx.ingress.kubernetes.io/affinity-mode": "persistent"}
	host, err := s.CreateIngress(ctx, "grafana", "monitoring", "oauth2-grafana", 80, annotationMap)
	if err != nil {
		return err
	}
	fmt.Printf("Deployed ingress for host: %s\n", host)

	if _, err = ssAPICfg.DeployCrossplane(ctx, s); err != nil {
		return err
	}

	if err = ssAPICfg.DeployNativeLinkCrossplane(ctx, s); err != nil {
		return err
	}

	if monStackCfg.EnableLogStack {
		if err = monStackCfg.DeployLoggingComponents(ctx, s); err != nil {
			return err
		}

		annotationMap := map[string]string{"nginx.ingress.kubernetes.io/affinity-mode": "persistent"}
		host, err := s.CreateIngress(ctx, "log-analytics", "log-system", "kibana", 5601, annotationMap)
		if err != nil {
			return err
		}

		fmt.Printf("Deployed ingress for host: %s\n", host)

		if monStackCfg.EnableOTEL {
			if err := monStackCfg.DeployOTELCollector(ctx, s); err != nil {
				return err
			}
		}

	}

	if ekscfg.EnableMongoDB {
		if _, err = nlSharedCfg.DeployMongoDB(ctx, s); err != nil {
			return err
		}
	}

	if ekscfg.EnableNats {
		if _, err = nlSharedCfg.DeployNats(ctx, s); err != nil {
			return err
		}
	}

	if s.CiSupportCfg.EnableActionsRunnerController {
		if err = s.CiSupportCfg.DeployActionsRunnerController(ctx, s); err != nil {
			return err
		}
	}

	if ekscfg.EnableKubeCost {
		if _, err = deployKubeCost(ctx, s, ekscfg); err != nil {
			return err
		}
		annotationMap := map[string]string{"nginx.ingress.kubernetes.io/affinity-mode": "persistent"}
		host, err := s.CreateIngress(ctx, "kubecost", "kubecost-analyzer", "kubecost", 9090, annotationMap)
		if err != nil {
			return err
		}
		fmt.Printf("Deployed ingress for host: %s\n", host)
		if err = monStackCfg.DeployKubeCostDashboards(ctx, s); err != nil {
			return err
		}
	}

	if s.CiSupportCfg.EnableTekton {
		if err = s.CiSupportCfg.DeployTekton(ctx, s); err != nil {
			return err
		}
	}

	//if _, err = s.deployKubescape(ctx); err != nil {
	//	return err
	//}

	return nil
}

func deployKubeCost(ctx *pulumi.Context, s *shared.Stack, ekscfg *EksConfig) (*helmv3.Release, error) {
	name := "kubecost"
	ns, err := s.CreateNamespace(ctx, "kubecost-analyzer")
	if err != nil {
		return nil, err
	}
	redirectURI := fmt.Sprintf("https://kubecost.%s/oauth2/callback", s.TLSCfg.Domain)
	opConfigMap, err := s.CreateOAuth2ProxyConfig(ctx, ns, "kubecost", redirectURI, 9090, "http")
	if err != nil {
		return nil, err
	}
	s.DependsOn = append(s.DependsOn, opConfigMap)

	// create the PVC here as kubecost gets into a bad state if you update the chart :(
	pvcSpec := &corev1.PersistentVolumeClaimSpecArgs{
		AccessModes: pulumi.StringArray{
			pulumi.String("ReadWriteOnce"),
		},
		Resources: &corev1.VolumeResourceRequirementsArgs{
			Requests: pulumi.StringMap{
				"storage": pulumi.String("32Gi"),
			},
		},
		StorageClassName: pulumi.String("gp3-enc-imm"),
	}
	if ekscfg.KubecostPvcSnapshot != "" {
		pvcSpec.DataSource = &corev1.TypedLocalObjectReferenceArgs{
			Name:     pulumi.String(ekscfg.KubecostPvcSnapshot),
			Kind:     pulumi.String("VolumeSnapshot"),
			ApiGroup: pulumi.String("snapshot.storage.k8s.io"),
		}
	}

	pvc, err := corev1.NewPersistentVolumeClaim(ctx, "kubecost-pvc", &corev1.PersistentVolumeClaimArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("kubecost-pvc"), Namespace: ns.Metadata.Name()},
		Spec:     pvcSpec,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return nil, err
	}
	s.DependsOn = append(s.DependsOn, pvc)

	customValues := pulumi.Map{
		"nameOverride":     pulumi.String(name),
		"fullnameOverride": pulumi.String(name),
		"prometheus": pulumi.Map{
			"server": pulumi.Map{
				"global": pulumi.Map{
					"external_labels": pulumi.Map{
						"cluster_id": pulumi.String(s.ClusterName),
					},
				},
			},
		},
		"persistentVolume": pulumi.Map{
			"existingClaim": pvc.Metadata.Name(),
		},
		"networkCosts": pulumi.Map{
			"resources": s.Resources.KubecostNetworkCosts,
		},
		"kubecostModel": pulumi.Map{
			"resources": s.Resources.KubecostModel,
		},
		"kubecostAggregator": pulumi.Map{
			"resources": s.Resources.KubecostAggregator,
			"cloudCost": pulumi.Map{
				"resources": &s.Resources.KubecostCloudCost,
			},
		},
		"kubecostFrontend": pulumi.Map{
			"resources": s.Resources.KubecostFrontend,
		},
		"forecasting": pulumi.Map{
			"resources": s.Resources.KubecostForecasting,
		},
		"oauthContainer": pulumi.Map{
			"resources": s.Resources.OauthProxy,
		},
	}

	helmRelease, err := s.DeployHelmRelease(ctx, ns, "cost-analyzer", kubecostChartVers, "", "cost-analyzer.yaml", customValues)
	if err != nil {
		return nil, err
	}
	return helmRelease, nil
}

func deployMetricsServer(ctx *pulumi.Context, s *shared.Stack) (*helmv3.Release, error) {
	return s.DeployHelmRelease(ctx, nil, "metrics-server", metricsServerChartVers, "", "", nil)
}

func isTaggable(t string) bool {
	for _, trt := range taggableResourceTypes {
		if t == trt {
			return true
		}
	}
	return false
}

func UpdateEksFlowLogRetention(ctx *pulumi.Context, s *shared.Stack) error {
	eksFlowLog, err := cloudwatch.LookupLogGroup(ctx, &cloudwatch.LookupLogGroupArgs{
		Name: fmt.Sprintf("/aws/eks/%v/cluster", s.ClusterName),
	}, nil)

	if err != nil {
		return err
	}
	eksFlowLog.RetentionInDays = s.LogGroupRetention

	return nil

}

func ConfigureAwsAuthIamRoles(ctx *pulumi.Context, s *shared.Stack, eksCfg *EksConfig) error {
	// add the readonly RBAC cluster-role and binding ... keeping it in YAML b/c it's easier to edit RBAC there than with Go code IMHO
	cf, err := yaml.NewConfigFile(ctx, "readonly-rbac-yaml", &yaml.ConfigFileArgs{
		File: "config/readonly-rbac.yaml",
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, cf)

	cm, err := corev1.GetConfigMap(ctx, "aws-auth", pulumi.ID("kube-system/aws-auth"), nil, pulumi.Provider(s.K8sProvider))
	if err != nil {
		return err
	}

	// this is super horrific, literally having to append YAML to the ConfigMap data :(
	roleMapping := fmt.Sprintf(`
- rolearn: arn:aws:iam::%s:role/%s
  username: admin:{{SessionName}}
  groups:
    - system:masters
- rolearn: arn:aws:iam::%s:role/%s
  username: readonly:{{SessionName}}
  groups:
    - readonly
`, eksCfg.AwsAccountID, eksCfg.AwsAuthIamAdminRole, eksCfg.AwsAccountID, eksCfg.AwsAuthIamReadOnlyRole)

	addRole := pulumi.All(cm.Data).ApplyT(func(all []interface{}) (string, error) {
		existingData := all[0].(map[string]string)
		if _, ok := existingData["mapRoles"]; ok {
			// make sure the role is not already mapped for this aws-auth configmap, if so, leave data as-is
			toJSON, _ := json.Marshal(existingData)
			lookFor := fmt.Sprintf("%s:role/%s", eksCfg.AwsAccountID, eksCfg.AwsAuthIamAdminRole)
			if !strings.Contains(string(toJSON), lookFor) {
				existingData["mapRoles"] += roleMapping
			} else {
				fmt.Printf("IAM role '%s' already mapped in 'aws-auth' configmap\n", eksCfg.AwsAuthIamAdminRole)
			}
		} else {
			existingData["mapRoles"] = roleMapping
		}

		_, err = corev1.NewConfigMap(ctx, "aws-auth-edited", &corev1.ConfigMapArgs{
			Metadata: metav1.ObjectMetaArgs{
				Annotations: pulumi.StringMap{"pulumi.com/patchForce": pulumi.String("true")},
				Name:        pulumi.String("aws-auth"),
				Namespace:   pulumi.String("kube-system"),
			},
			Data: pulumi.ToStringMap(existingData),
		}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{cm}))
		if err != nil {
			_ = ctx.Log.Error("Update 'aws-auth' configmap failed due to: "+err.Error(), nil)
		}
		return eksCfg.AwsAuthIamAdminRole, err
	})
	ctx.Export("awsAuthIamAdminRole", addRole) // this fails if an error is returned

	return nil
}

func deployEbsCsiDriver(ctx *pulumi.Context, s *shared.Stack) (*helmv3.Release, error) {
	policyArns := []string{"arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy"}
	roleName := s.ClusterScopedResourceName("ebs-csi")
	serviceAccount := awsiam.EKSServiceAccountArgs{Name: s.Eks.EksCluster.Name(), ServiceAccounts: pulumi.ToStringArray([]string{"kube-system:ebs-csi-controller-sa"})}
	eksRole, err := awsiam.NewEKSRole(ctx, roleName, &awsiam.EKSRoleArgs{
		Role:                   awsiam.RoleArgs{Name: pulumi.String(roleName)},
		ClusterServiceAccounts: awsiam.EKSServiceAccountArray([]awsiam.EKSServiceAccountInput{serviceAccount}),
		RolePolicyArns:         pulumi.ToStringArray(policyArns),
	}, pulumi.DependsOn([]pulumi.Resource{s.Eks}))
	if err != nil {
		return nil, err
	}
	s.DependsOn = append(s.DependsOn, eksRole)

	policyJSON := `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "kms:Decrypt", 
        "kms:GenerateDataKeyWithoutPlaintext",
        "kms:CreateGrant"
      ],
      "Resource": "*"
    }
  ]
}`
	policyName := s.ClusterScopedResourceName("ebs-csi-kms-policy")
	_, err = iam.NewRolePolicy(ctx, policyName,
		&iam.RolePolicyArgs{Name: pulumi.String(policyName), Role: eksRole.Name, Policy: pulumi.String(policyJSON)}, pulumi.DependsOn([]pulumi.Resource{eksRole}))
	if err != nil {
		return nil, err
	}

	// snapshotter CRDs
	configGroup, err := yaml.NewConfigGroup(ctx, "snapshotter-crds", &yaml.ConfigGroupArgs{
		Files: []string{
			"https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/master/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml",
			"https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/master/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml",
			"https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/master/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml",
		},
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return nil, err
	}
	s.DependsOn = append(s.DependsOn, configGroup)

	cf, err := yaml.NewConfigFile(ctx, "csi-snapshot-controller", &yaml.ConfigFileArgs{
		File: "config/csi-snapshot-controller.yaml",
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return nil, err
	}
	s.DependsOn = append(s.DependsOn, cf)

	customValues := pulumi.Map{
		"controller": pulumi.Map{
			"replicaCount": pulumi.Int(2),
			"serviceAccount": pulumi.Map{
				"annotations": pulumi.Map{
					"eks.amazonaws.com/role-arn": eksRole.Arn,
				},
			},
		},
		"sidecars": pulumi.Map{
			"snapshotter": pulumi.Map{
				"forceEnable": pulumi.Bool(true),
			},
		},
		"volumeSnapshotClasses": pulumi.Array{
			pulumi.Map{
				"name":           pulumi.String("gp3-snapshotclass"),
				"deletionPolicy": pulumi.String("Delete"),
			},
		},
	}
	helmRel, err := s.DeployHelmRelease(ctx, nil, "aws-ebs-csi-driver", ebsCsiDriverChartVers, "", "", customValues)
	if err != nil {
		return nil, err
	}

	// For each subnet ID associated with the EKS cluster, retrieve the availability zone.
	s.Vpc.Subnets.ApplyT(func(applyTo interface{}) string {
		subnetZones := pulumi.StringArray{}
		subnets := applyTo.([]*ec2.Subnet)
		for _, next := range subnets {
			subnetZones = append(subnetZones, next.AvailabilityZone)
		}
		subnetZones.ToStringArrayOutput().ApplyT(func(inner interface{}) string {
			zones := inner.([]string)
			var zoneSet []string
			for _, z := range zones {
				hasZone := false
				for _, zz := range zoneSet {
					if z == zz {
						hasZone = true
						break
					}
				}
				if !hasZone {
					zoneSet = append(zoneSet, z)
				}
			}

			_, err = storagev1.NewStorageClass(ctx, "gp3-8k-iops", &storagev1.StorageClassArgs{
				Metadata:    &metav1.ObjectMetaArgs{Name: pulumi.String("gp3-8k-iops")},
				Provisioner: pulumi.String("ebs.csi.aws.com"),
				Parameters: pulumi.StringMap{
					"type":                      pulumi.String("gp3"),
					"csi.storage.k8s.io/fstype": pulumi.String("ext4"),
					"iops":                      pulumi.String("8000"),
					"throughput":                pulumi.String("1000"),
				},
				ReclaimPolicy:        pulumi.String("Retain"),
				AllowVolumeExpansion: pulumi.Bool(true),
				VolumeBindingMode:    pulumi.String("WaitForFirstConsumer"),
				AllowedTopologies: corev1.TopologySelectorTermArray{
					&corev1.TopologySelectorTermArgs{
						MatchLabelExpressions: corev1.TopologySelectorLabelRequirementArray{
							&corev1.TopologySelectorLabelRequirementArgs{
								Key:    pulumi.String("topology.ebs.csi.aws.com/zone"),
								Values: pulumi.ToStringArray(zoneSet),
							},
						},
					},
				},
			}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{helmRel}))
			if err != nil {
				fmt.Printf("Failed to create new StorageClass 'gp3-8k-iops' due to: %s\n", err.Error())
			}

			_, err = storagev1.NewStorageClass(ctx, "gp3", &storagev1.StorageClassArgs{
				Metadata:    &metav1.ObjectMetaArgs{Name: pulumi.String("gp3")},
				Provisioner: pulumi.String("ebs.csi.aws.com"),
				Parameters: pulumi.StringMap{
					"type":                      pulumi.String("gp3"),
					"csi.storage.k8s.io/fstype": pulumi.String("ext4"),
				},
				ReclaimPolicy:        pulumi.String("Retain"),
				AllowVolumeExpansion: pulumi.Bool(true),
				VolumeBindingMode:    pulumi.String("WaitForFirstConsumer"),
				AllowedTopologies: corev1.TopologySelectorTermArray{
					&corev1.TopologySelectorTermArgs{
						MatchLabelExpressions: corev1.TopologySelectorLabelRequirementArray{
							&corev1.TopologySelectorLabelRequirementArgs{
								Key:    pulumi.String("topology.ebs.csi.aws.com/zone"),
								Values: pulumi.ToStringArray(zoneSet),
							},
						},
					},
				},
			}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{helmRel}))
			if err != nil {
				fmt.Printf("Failed to create new StorageClass 'gp3' due to: %s\n", err.Error())
			}

			_, err = storagev1.NewStorageClass(ctx, "gp3-8k-iops-enc", &storagev1.StorageClassArgs{
				Metadata:    &metav1.ObjectMetaArgs{Name: pulumi.String("gp3-8k-iops-enc")},
				Provisioner: pulumi.String("ebs.csi.aws.com"),
				Parameters: pulumi.StringMap{
					"type":                      pulumi.String("gp3"),
					"csi.storage.k8s.io/fstype": pulumi.String("ext4"),
					"iops":                      pulumi.String("8000"),
					"throughput":                pulumi.String("1000"),
					"encrypted":                 pulumi.String("true"),
				},
				ReclaimPolicy:        pulumi.String("Retain"),
				AllowVolumeExpansion: pulumi.Bool(true),
				VolumeBindingMode:    pulumi.String("WaitForFirstConsumer"),
				AllowedTopologies: corev1.TopologySelectorTermArray{
					&corev1.TopologySelectorTermArgs{
						MatchLabelExpressions: corev1.TopologySelectorLabelRequirementArray{
							&corev1.TopologySelectorLabelRequirementArgs{
								Key:    pulumi.String("topology.ebs.csi.aws.com/zone"),
								Values: pulumi.ToStringArray(zoneSet),
							},
						},
					},
				},
			}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{helmRel}))
			if err != nil {
				fmt.Printf("Failed to create new StorageClass 'gp3-8k-iops-enc' due to: %s\n", err.Error())
			}

			_, err = storagev1.NewStorageClass(ctx, "gp3-enc", &storagev1.StorageClassArgs{
				Metadata:    &metav1.ObjectMetaArgs{Name: pulumi.String("gp3-enc")},
				Provisioner: pulumi.String("ebs.csi.aws.com"),
				Parameters: pulumi.StringMap{
					"type":                      pulumi.String("gp3"),
					"csi.storage.k8s.io/fstype": pulumi.String("ext4"),
					"encrypted":                 pulumi.String("true"),
				},
				ReclaimPolicy:        pulumi.String("Delete"),
				AllowVolumeExpansion: pulumi.Bool(true),
				VolumeBindingMode:    pulumi.String("WaitForFirstConsumer"),
				AllowedTopologies: corev1.TopologySelectorTermArray{
					&corev1.TopologySelectorTermArgs{
						MatchLabelExpressions: corev1.TopologySelectorLabelRequirementArray{
							&corev1.TopologySelectorLabelRequirementArgs{
								Key:    pulumi.String("topology.ebs.csi.aws.com/zone"),
								Values: pulumi.ToStringArray(zoneSet),
							},
						},
					},
				},
			}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{helmRel}))
			if err != nil {
				fmt.Printf("Failed to create new StorageClass 'gp3-enc' due to: %s\n", err.Error())
			}

			_, err = storagev1.NewStorageClass(ctx, "gp3-enc-imm", &storagev1.StorageClassArgs{
				Metadata:    &metav1.ObjectMetaArgs{Name: pulumi.String("gp3-enc-imm")},
				Provisioner: pulumi.String("ebs.csi.aws.com"),
				Parameters: pulumi.StringMap{
					"type":                      pulumi.String("gp3"),
					"csi.storage.k8s.io/fstype": pulumi.String("ext4"),
					"encrypted":                 pulumi.String("true"),
				},
				ReclaimPolicy:        pulumi.String("Delete"),
				AllowVolumeExpansion: pulumi.Bool(true),
				VolumeBindingMode:    pulumi.String("Immediate"),
				AllowedTopologies: corev1.TopologySelectorTermArray{
					&corev1.TopologySelectorTermArgs{
						MatchLabelExpressions: corev1.TopologySelectorLabelRequirementArray{
							&corev1.TopologySelectorLabelRequirementArgs{
								Key:    pulumi.String("topology.ebs.csi.aws.com/zone"),
								Values: pulumi.ToStringArray(zoneSet),
							},
						},
					},
				},
			}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn([]pulumi.Resource{helmRel}))
			if err != nil {
				fmt.Printf("Failed to create new StorageClass 'gp3-enc-imm' due to: %s\n", err.Error())
			}

			return ""
		})
		return ""
	})

	return helmRel, nil
}

func createRoute53IamRole(ctx *pulumi.Context, roleName string, s *shared.Stack, eksCfg *EksConfig) (*iam.Role, error) {
	AwsAccountID := eksCfg.AwsAccountID

	assumeRolePolicy := pulumi.Sprintf(`{
	  "Version": "2012-10-17",
	  "Statement": [
		{
		  "Effect": "Allow",
		  "Principal": {
			"AWS": "arn:aws:iam::%s:role/%s"
		  },
		  "Action": "sts:AssumeRole"
		}
	  ]
	}`, AwsAccountID, s.InstanceRoleName)

	policyJSON := pulumi.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Action": "route53:GetChange",
				"Resource": "arn:aws:route53:::change/*"
			},
			{
				"Effect": "Allow",
				"Action": [
					"route53:ChangeResourceRecordSets",
					"route53:ListResourceRecordSets"
				],
				"Resource": "arn:aws:route53:::hostedzone/%s"
			}
		]
	}`, s.TLSCfg.Route53ZoneID)

	iamRole, err := iam.NewRole(ctx, "Route53", &iam.RoleArgs{Name: pulumi.String(roleName), AssumeRolePolicy: assumeRolePolicy})
	if err != nil {
		return nil, err
	}

	policyName := s.ClusterScopedResourceName("Route53Policy")
	_, err = iam.NewRolePolicy(ctx, "Route53Policy",
		&iam.RolePolicyArgs{Name: pulumi.String(policyName), Role: iamRole.Name, Policy: policyJSON}, pulumi.DependsOn([]pulumi.Resource{iamRole}))

	return iamRole, err
}

func createKarpenterControllerIRSA(ctx *pulumi.Context, s *shared.Stack, ekscfg *EksConfig) (*awsiam.EKSRole, error) {
	// create the IAM role for service account needed by the Karpenter controller pod,
	//    see: https://docs.aws.amazon.com/eks/latest/userguide/associate-service-account-role.html
	// also see: https://www.pulumi.com/registry/packages/aws-iam/api-docs/eksrole/
	roleName := s.ClusterScopedResourceName("karpenter-controller")
	serviceAccount := awsiam.EKSServiceAccountArgs{Name: s.Eks.EksCluster.Name(), ServiceAccounts: pulumi.ToStringArray([]string{"karpenter:karpenter"})}
	eksRole, err := awsiam.NewEKSRole(ctx, roleName, &awsiam.EKSRoleArgs{
		Role:                   awsiam.RoleArgs{Name: pulumi.String(roleName)},
		ClusterServiceAccounts: awsiam.EKSServiceAccountArray([]awsiam.EKSServiceAccountInput{serviceAccount}),
	}, pulumi.DependsOn([]pulumi.Resource{s.Eks}))
	if err != nil {
		return nil, err
	}

	policyJSON := createKarpenterControllerPolicyDocJSON(s, ekscfg)
	policyName := s.ClusterScopedResourceName("karpenter-controller-policy")
	_, err = iam.NewRolePolicy(ctx, policyName,
		&iam.RolePolicyArgs{Name: pulumi.String(policyName), Role: eksRole.Name, Policy: pulumi.String(policyJSON)}, pulumi.DependsOn([]pulumi.Resource{eksRole}))

	return eksRole, err
}

// Reads a JSON template to create the permissions for the Karpenter controller role, which requires some cluster metadata
func createKarpenterControllerPolicyDocJSON(s *shared.Stack, ekscfg *EksConfig) string {
	policyDocTmpl := shared.ReadFile(pulumi.NewFileAsset("./config/karpenter-iam-permissions.json").Path())
	policyDocTmpl = strings.ReplaceAll(policyDocTmpl, "${Region}", s.Region)
	policyDocTmpl = strings.ReplaceAll(policyDocTmpl, "${ClusterName}", s.ClusterName)
	policyDocTmpl = strings.ReplaceAll(policyDocTmpl, "${AccountId}", ekscfg.AwsAccountID)
	policyDocTmpl = strings.ReplaceAll(policyDocTmpl, "${NodeRole}", s.InstanceRoleName)
	return policyDocTmpl
}

func DeployKarpenter(ctx *pulumi.Context, s *shared.Stack, ekscfg *EksConfig) (*helmv3.Release, error) {
	ns, err := s.CreateNamespace(ctx, "karpenter")
	if err != nil {
		return nil, err
	}

	// controller IRSA
	eksRole, err := createKarpenterControllerIRSA(ctx, s, ekscfg)
	if err != nil {
		return nil, err
	}
	s.DependsOn = append(s.DependsOn, eksRole)

	// deploy karpenter cloudformation for sqs
	cfYaml := shared.ReadFile(pulumi.NewFileAsset("./config/karpenter-sqs-cloudformation.yaml").Path())
	_, err = cloudformation.NewStack(ctx, "karpenter-sqs", &cloudformation.StackArgs{
		TemplateBody: pulumi.String(cfYaml),
		Parameters: pulumi.StringMap{
			"ClusterName": pulumi.String(s.ClusterName),
		},
	})
	if err != nil {
		return nil, err
	}

	// Karpenter helm chart requires some cluster specific runtime metadata
	customValues := pulumi.Map{
		"serviceAccount": pulumi.Map{
			"annotations": pulumi.Map{
				"eks.amazonaws.com/role-arn": eksRole.Arn,
			},
		},
		"settings": pulumi.Map{
			"clusterName":       pulumi.String(s.ClusterName),
			"batchMaxDuration":  pulumi.String("30s"),
			"interruptionQueue": pulumi.String(s.ClusterName),
			"featureGates": pulumi.Map{
				"drift":                   pulumi.Bool(true),
				"spotToSpotConsolidation": pulumi.Bool(ekscfg.KarpenterSpotToSpotConsolidation),
			},
		},
		"controller": pulumi.Map{
			"resources": s.Resources.Karpenter,
		},
	}

	/*
		customCrdValues := pulumi.Map{}
		_, err = s.deployHelmRelease(ctx, ns, "karpenter-crd", karpenterChartVers, "", "", customCrdValues)
		if err != nil {
			return nil, err
		}
	*/

	return s.DeployHelmRelease(ctx, ns, "karpenter", karpenterChartVers, "", "karpenter-values.yaml", customValues)
}

func deployKarpenterNodePool(ctx *pulumi.Context, s *shared.Stack) error {
	_ = pulumi.All(s.Eks.NodeSecurityGroup.Arn(), s.Vpc.PrivateSubnetIds).ApplyT(func(all []interface{}) string {
		nodeSgArn := all[0].(string)
		// sigh: parse the ID from the Arn :(
		findAt := strings.Index(nodeSgArn, ":security-group/")
		nodeSgID := nodeSgArn[findAt+len(":security-group/"):]

		sgTerms := shared.BuildIDTerms([]string{nodeSgID})
		snTerms := shared.BuildIDTerms(all[1].([]string))
		spec, _ := shared.JSONToMap(fmt.Sprintf(`{
		  "spec": {
			"amiFamily": "AL2",
			"role": "%s",
			"detailedMonitoring": false,
			"metadataOptions": {
			  "httpEndpoint": "enabled",
			  "httpProtocolIPv6": "disabled",
			  "httpPutResponseHopLimit": 2,
			  "httpTokens": "required"
			},
			"blockDeviceMappings": [
			  {
				"deviceName": "/dev/xvda",
				"ebs": {
				  "deleteOnTermination": true,
				  "volumeSize": "50Gi",
				  "volumeType": "gp3",
                  "encrypted": true
				}
			  }
			],
            "securityGroupSelectorTerms": %s,
            "subnetSelectorTerms": %s
		  }
		}`, s.InstanceRoleName, sgTerms, snTerms))

		ec2NodeClassName := "default-ec2-nodeclass"
		ec2NodeClass, err := apiextensions.NewCustomResource(ctx, ec2NodeClassName, &apiextensions.CustomResourceArgs{
			ApiVersion:  pulumi.String("karpenter.k8s.aws/v1beta1"),
			Kind:        pulumi.String("EC2NodeClass"),
			Metadata:    metav1.ObjectMetaArgs{Name: pulumi.String(ec2NodeClassName)},
			OtherFields: spec,
		}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
		if err != nil {
			return err.Error()
		}
		s.DependsOn = append(s.DependsOn, ec2NodeClass)

		// Node class that uses local NVMe for ephemeral storage (RAID0)
		isvSpec, _ := shared.JSONToMap(fmt.Sprintf(`{
		  "spec": {
			"amiFamily": "AL2",
			"role": "%s",
			"detailedMonitoring": false,
			"metadataOptions": {
			  "httpEndpoint": "enabled",
			  "httpProtocolIPv6": "disabled",
			  "httpPutResponseHopLimit": 2,
			  "httpTokens": "required"
			},
            "instanceStorePolicy": "RAID0",
			"blockDeviceMappings": [
			  {
				"deviceName": "/dev/xvda",
				"ebs": {
				  "deleteOnTermination": true,
				  "volumeSize": "100Gi",
				  "volumeType": "gp3",
                  "encrypted": true
				}
			  }
            ],
            "securityGroupSelectorTerms": %s,
            "subnetSelectorTerms": %s
		  }
		}`, s.InstanceRoleName, sgTerms, snTerms))

		isvNodeClassName := "inst-store-vol-nodeclass"
		isvNodeClass, err := apiextensions.NewCustomResource(ctx, isvNodeClassName, &apiextensions.CustomResourceArgs{
			ApiVersion:  pulumi.String("karpenter.k8s.aws/v1beta1"),
			Kind:        pulumi.String("EC2NodeClass"),
			Metadata:    metav1.ObjectMetaArgs{Name: pulumi.String(isvNodeClassName)},
			OtherFields: isvSpec,
		}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
		if err != nil {
			return err.Error()
		}
		s.DependsOn = append(s.DependsOn, isvNodeClass)

		// Node class that uses a 50GB EBS GP3 vol
		ebs50GBSpec, _ := shared.JSONToMap(fmt.Sprintf(`{
		  "spec": {
			"amiFamily": "AL2",
			"role": "%s",
			"detailedMonitoring": false,
			"metadataOptions": {
			  "httpEndpoint": "enabled",
			  "httpProtocolIPv6": "disabled",
			  "httpPutResponseHopLimit": 2,
			  "httpTokens": "required"
			},
			"blockDeviceMappings": [
			  {
				"deviceName": "/dev/xvda",
				"ebs": {
				  "deleteOnTermination": true,
				  "volumeSize": "50Gi",
				  "volumeType": "gp3",
				  "encrypted": true
				}
			  }
			],
            "securityGroupSelectorTerms": %s,
            "subnetSelectorTerms": %s
		  }
		}`, s.InstanceRoleName, sgTerms, snTerms))

		ebs50GBNodeClassName := "ebs-50g-gp3-nodeclass"
		ebs50GBNodeClass, err := apiextensions.NewCustomResource(ctx, ebs50GBNodeClassName, &apiextensions.CustomResourceArgs{
			ApiVersion:  pulumi.String("karpenter.k8s.aws/v1beta1"),
			Kind:        pulumi.String("EC2NodeClass"),
			Metadata:    metav1.ObjectMetaArgs{Name: pulumi.String(ebs50GBNodeClassName)},
			OtherFields: ebs50GBSpec,
		}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
		if err != nil {
			return err.Error()
		}
		s.DependsOn = append(s.DependsOn, ebs50GBNodeClass)

		nodePools := []string{"karpenter", "isv", "ebs-50g-gp3", "spot", "isv-spot", "not-disruptable", "not-disruptable-50g-gp3", "isv-spot-demo-builds"}
		for _, np := range nodePools {
			if err = deployNodePool(ctx, np, s); err != nil {
				return err.Error()
			}
		}

		err = deployGpuNodePool(ctx, s)
		if err != nil {
			return err.Error()
		}

		return ""
	}).(pulumi.StringOutput)

	return nil
}

func deployGpuNodePool(ctx *pulumi.Context, s *shared.Stack) error {
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
	helmRel, err := s.DeployHelmRelease(ctx, nil, "nvidia-device-plugin", shared.NvidiaChartVers, "", "", customValues)
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, helmRel)

	// deploy a GPU NodePool
	return deployNodePool(ctx, "gpu", s)
}

func deployNodePool(ctx *pulumi.Context, key string, s *shared.Stack) error {
	nodePoolName := fmt.Sprintf("%s-nodepool-%s", key, s.ClusterName)
	nodePoolSpec := shared.ReadFile(pulumi.NewFileAsset(fmt.Sprintf("./config/nodepools/%s-nodepool-spec.json", key)).Path())
	nodePoolSpecMap, _ := shared.JSONToMap(nodePoolSpec)
	nodePool, err := apiextensions.NewCustomResource(ctx, nodePoolName, &apiextensions.CustomResourceArgs{
		ApiVersion:  pulumi.String("karpenter.sh/v1beta1"),
		Kind:        pulumi.String("NodePool"),
		Metadata:    metav1.ObjectMetaArgs{Name: pulumi.String(nodePoolName)},
		OtherFields: nodePoolSpecMap,
	}, pulumi.Provider(s.K8sProvider), pulumi.DependsOn(s.DependsOn))
	if err != nil {
		return err
	}
	s.DependsOn = append(s.DependsOn, nodePool)
	return nil
}

func deployCilium(ctx *pulumi.Context, s *shared.Stack) (*helmv3.Release, error) {
	customValues := pulumi.Map{
		"debug": pulumi.Map{
			"enabled": pulumi.Bool(true),
		},
		"cni": pulumi.Map{
			"chainingMode": pulumi.String("aws-cni"),
			"exclusive":    pulumi.Bool(false),
		},
		"enableIPv4Masquerade": pulumi.Bool(false),
		"routingMode":          pulumi.String("native"),
		"endpointRoutes": pulumi.Map{
			"enabled": pulumi.Bool(true),
		},
		"hubble": pulumi.Map{
			"relay": pulumi.Map{
				"enabled": pulumi.Bool(false),
			},
			"ui": pulumi.Map{
				"enabled": pulumi.Bool(false),
			},
		},
		"nodePort": pulumi.Map{
			"enabled": pulumi.Bool(true), // TODO: investigate using kubeProxyReplacement=true
		},
		"gatewayAPI": pulumi.Map{
			"enabled": pulumi.Bool(true),
		},
		"encryption": pulumi.Map{
			"enabled":        pulumi.Bool(false), // DO NOT ENABLE ~ causes probe failures with CAS under load :(
			"type":           pulumi.String("wireguard"),
			"nodeEncryption": pulumi.Bool(false),
		},
		"resources": pulumi.Map{
			"requests": pulumi.Map{
				"cpu":    pulumi.String("30m"),
				"memory": pulumi.String("215Mi"),
			},
			"limits": pulumi.Map{
				"cpu":    pulumi.String("100m"),
				"memory": pulumi.String("300Mi"),
			},
		},
	}
	return s.DeployHelmRelease(ctx, nil, "cilium", ciliumChartVers, "", "", customValues)
}

func createRoute53RecordForNLB(ctx *pulumi.Context, nlb *lb.LookupLoadBalancerResult, s *shared.Stack) error {
	recordName := s.ClusterScopedResourceName("route53-nlb")
	aliasToNlb := &route53.RecordAliasArgs{Name: pulumi.String(nlb.DnsName), ZoneId: pulumi.String(nlb.ZoneId), EvaluateTargetHealth: pulumi.Bool(true)}
	_, err := route53.NewRecord(ctx, recordName, &route53.RecordArgs{
		ZoneId:  pulumi.String(s.TLSCfg.Route53ZoneID),
		Name:    pulumi.Sprintf("*.%s", s.TLSCfg.Domain), // wildcard mapping to all subdomains
		Type:    pulumi.String("A"),
		Aliases: route53.RecordAliasArray{aliasToNlb},
	})
	return err
}

func waitForLoadBalancer(ctx *pulumi.Context, clusterName string, maxWait time.Duration) (*lb.LookupLoadBalancerResult, error) {
	// block the foreground thread until we have our LB
	startAt := time.Now()
	timeoutAt := startAt.Add(maxWait)
	attempts := 0
	matchClusterTag := fmt.Sprintf("kubernetes.io/cluster/%s", clusterName)
	for time.Now().Before(timeoutAt) {
		attempts++
		result, err := lb.GetLbs(ctx, &lb.GetLbsArgs{Tags: map[string]string{matchClusterTag: "owned"}}, nil)
		if err != nil {
			fmt.Printf("GetLbs call failed: %v\n", err)
		}

		if result != nil && len(result.Arns) > 0 {
			fmt.Printf("Found NLB: %s\n", strings.Join(result.Arns, ", "))
			foundNLB, err := lb.LookupLoadBalancer(ctx, &lb.LookupLoadBalancerArgs{
				Arn: pulumi.StringRef(result.Arns[0]),
			}, nil)
			if err != nil {
				fmt.Printf("LookupLoadBalancer(%s) call failed: %v\n", result.Arns[0], err)
			}
			if foundNLB != nil {
				fmt.Printf("Took %s to get NLB: %s\n", time.Since(startAt), foundNLB.DnsName)
				return foundNLB, nil
			}
		}
		fmt.Printf("NLB not found after %d attempts, will retry in 30 secs ...\n", attempts)
		time.Sleep(30 * time.Second)
	}
	return nil, fmt.Errorf("failed to get NLB with tag '%s' within the timeout period %s", matchClusterTag, maxWait.String())
}

func CreateFlowLogs(ctx *pulumi.Context, vpc *ec2x.Vpc, eksCfg *EksConfig) error {

	// Gets already created Role.
	flowLogsRole, err := iam.LookupRole(ctx, &iam.LookupRoleArgs{
		Name: vpcFlowLogsName,
	}, nil)

	if err != nil {
		return err
	}

	logGroup, err := cloudwatch.NewLogGroup(ctx, vpcFlowLogsName, &cloudwatch.LogGroupArgs{
		Name:            pulumi.String(vpcFlowLogsName),
		RetentionInDays: pulumi.Int(eksCfg.LogGroupRetention),
		Tags: pulumi.StringMap{
			"VpcName":     pulumi.String(eksCfg.VpcName),
			"ClusterName": pulumi.String(eksCfg.ClusterName),
		},
	})
	if err != nil {
		return err
	}

	_, err = ec2.NewFlowLog(ctx, vpcFlowLogsName, &ec2.FlowLogArgs{
		IamRoleArn:     pulumi.String(flowLogsRole.Arn),
		LogDestination: logGroup.Arn,
		TrafficType:    pulumi.String("ALL"),
		VpcId:          vpc.VpcId,
	})

	if err != nil {
		return err
	}

	// Export necessary information.
	ctx.Export("flowLogGroupId", logGroup.Name)
	ctx.Export("flowLogRoleArn", pulumi.String(flowLogsRole.Arn))

	return nil

}

func RegisterAutoTags(ctx *pulumi.Context, autoTags map[string]string) error {
	return ctx.RegisterStackTransformation(
		func(args *pulumi.ResourceTransformationArgs) *pulumi.ResourceTransformationResult {
			if args.Props != nil && isTaggable(args.Type) {
				ptr := reflect.ValueOf(args.Props)
				if !ptr.IsZero() {
					val := ptr.Elem()
					if !val.IsZero() {
						fmt.Printf("Applying auto-tags to (%s) %s\n", args.Type, args.Name)

						tags := val.FieldByName("Tags")

						var tagsMap pulumi.StringMap
						if !tags.IsZero() {
							tagsMap = tags.Interface().(pulumi.StringMap)
						} else {
							tagsMap = pulumi.StringMap{}
						}
						for k, v := range autoTags {
							tagsMap[k] = pulumi.String(v)
						}
						tags.Set(reflect.ValueOf(tagsMap))

						return &pulumi.ResourceTransformationResult{Props: args.Props, Opts: args.Opts}
					}
				}
			}
			return nil
		},
	)
}

// taggableResourceTypes is a list of known AWS type tokens that are taggable.
var taggableResourceTypes = []string{
	"aws:accessanalyzer/analyzer:Analyzer",
	"aws:acm/certificate:Certificate",
	"aws:acmpca/certificateAuthority:CertificateAuthority",
	"aws:alb/loadBalancer:LoadBalancer",
	"aws:alb/targetGroup:TargetGroup",
	"aws:apigateway/apiKey:ApiKey",
	"aws:apigateway/clientCertificate:ClientCertificate",
	"aws:apigateway/domainName:DomainName",
	"aws:apigateway/restApi:RestApi",
	"aws:apigateway/stage:Stage",
	"aws:apigateway/usagePlan:UsagePlan",
	"aws:apigateway/vpcLink:VpcLink",
	"aws:applicationloadbalancing/loadBalancer:LoadBalancer",
	"aws:applicationloadbalancing/targetGroup:TargetGroup",
	"aws:appmesh/mesh:Mesh",
	"aws:appmesh/route:Route",
	"aws:appmesh/virtualNode:VirtualNode",
	"aws:appmesh/virtualRouter:VirtualRouter",
	"aws:appmesh/virtualService:VirtualService",
	"aws:appsync/graphQLApi:GraphQLApi",
	"aws:athena/workgroup:Workgroup",
	"aws:autoscaling/group:Group",
	"aws:backup/plan:Plan",
	"aws:backup/vault:Vault",
	"aws:cfg/aggregateAuthorization:AggregateAuthorization",
	"aws:cfg/configurationAggregator:ConfigurationAggregator",
	"aws:cfg/rule:Rule",
	"aws:cloudformation/stack:Stack",
	"aws:cloudformation/stackSet:StackSet",
	"aws:cloudfront/distribution:Distribution",
	"aws:cloudhsmv2/cluster:Cluster",
	"aws:cloudtrail/trail:Trail",
	"aws:cloudwatch/eventRule:EventRule",
	"aws:cloudwatch/logGroup:LogGroup",
	"aws:cloudwatch/metricAlarm:MetricAlarm",
	"aws:codebuild/project:Project",
	"aws:codecommit/repository:Repository",
	"aws:codepipeline/pipeline:Pipeline",
	"aws:codepipeline/webhook:Webhook",
	"aws:codestarnotifications/notificationRule:NotificationRule",
	"aws:cognito/identityPool:IdentityPool",
	"aws:cognito/userPool:UserPool",
	"aws:datapipeline/pipeline:Pipeline",
	"aws:datasync/agent:Agent",
	"aws:datasync/efsLocation:EfsLocation",
	"aws:datasync/locationSmb:LocationSmb",
	"aws:datasync/nfsLocation:NfsLocation",
	"aws:datasync/s3Location:S3Location",
	"aws:datasync/task:Task",
	"aws:dax/cluster:Cluster",
	"aws:directconnect/connection:Connection",
	"aws:directconnect/hostedPrivateVirtualInterfaceAccepter:HostedPrivateVirtualInterfaceAccepter",
	"aws:directconnect/hostedPublicVirtualInterfaceAccepter:HostedPublicVirtualInterfaceAccepter",
	"aws:directconnect/hostedTransitVirtualInterfaceAcceptor:HostedTransitVirtualInterfaceAcceptor",
	"aws:directconnect/linkAggregationGroup:LinkAggregationGroup",
	"aws:directconnect/privateVirtualInterface:PrivateVirtualInterface",
	"aws:directconnect/publicVirtualInterface:PublicVirtualInterface",
	"aws:directconnect/transitVirtualInterface:TransitVirtualInterface",
	"aws:directoryservice/directory:Directory",
	"aws:dlm/lifecyclePolicy:LifecyclePolicy",
	"aws:dms/endpoint:Endpoint",
	"aws:dms/replicationInstance:ReplicationInstance",
	"aws:dms/replicationSubnetGroup:ReplicationSubnetGroup",
	"aws:dms/replicationTask:ReplicationTask",
	"aws:docdb/cluster:Cluster",
	"aws:docdb/clusterInstance:ClusterInstance",
	"aws:docdb/clusterParameterGroup:ClusterParameterGroup",
	"aws:docdb/subnetGroup:SubnetGroup",
	"aws:dynamodb/table:Table",
	"aws:ebs/snapshot:Snapshot",
	"aws:ebs/snapshotCopy:SnapshotCopy",
	"aws:ebs/volume:Volume",
	"aws:ec2/ami:Ami",
	"aws:ec2/amiCopy:AmiCopy",
	"aws:ec2/amiFromInstance:AmiFromInstance",
	"aws:ec2/capacityReservation:CapacityReservation",
	"aws:ec2/customerGateway:CustomerGateway",
	"aws:ec2/defaultNetworkAcl:DefaultNetworkAcl",
	"aws:ec2/defaultRouteTable:DefaultRouteTable",
	"aws:ec2/defaultSecurityGroup:DefaultSecurityGroup",
	"aws:ec2/defaultSubnet:DefaultSubnet",
	"aws:ec2/defaultVpc:DefaultVpc",
	"aws:ec2/defaultVpcDhcpOptions:DefaultVpcDhcpOptions",
	"aws:ec2/eip:Eip",
	"aws:ec2/fleet:Fleet",
	"aws:ec2/instance:Instance",
	"aws:ec2/internetGateway:InternetGateway",
	"aws:ec2/keyPair:KeyPair",
	"aws:ec2/launchTemplate:LaunchTemplate",
	"aws:ec2/natGateway:NatGateway",
	"aws:ec2/networkAcl:NetworkAcl",
	"aws:ec2/networkInterface:NetworkInterface",
	"aws:ec2/placementGroup:PlacementGroup",
	"aws:ec2/routeTable:RouteTable",
	"aws:ec2/securityGroup:SecurityGroup",
	"aws:ec2/spotInstanceRequest:SpotInstanceRequest",
	"aws:ec2/subnet:Subnet",
	"aws:ec2/vpc:Vpc",
	"aws:ec2/vpcDhcpOptions:VpcDhcpOptions",
	"aws:ec2/vpcEndpoint:VpcEndpoint",
	"aws:ec2/vpcEndpointService:VpcEndpointService",
	"aws:ec2/vpcPeeringConnection:VpcPeeringConnection",
	"aws:ec2/vpcPeeringConnectionAccepter:VpcPeeringConnectionAccepter",
	"aws:ec2/vpnConnection:VpnConnection",
	"aws:ec2/vpnGateway:VpnGateway",
	"aws:ec2clientvpn/endpoint:Endpoint",
	"aws:ec2transitgateway/routeTable:RouteTable",
	"aws:ec2transitgateway/transitGateway:TransitGateway",
	"aws:ec2transitgateway/vpcAttachment:VpcAttachment",
	"aws:ec2transitgateway/vpcAttachmentAccepter:VpcAttachmentAccepter",
	"aws:ecr/repository:Repository",
	"aws:ecs/capacityProvider:CapacityProvider",
	"aws:ecs/cluster:Cluster",
	"aws:ecs/service:Service",
	"aws:ecs/taskDefinition:TaskDefinition",
	"aws:efs/fileSystem:FileSystem",
	"aws:eks/cluster:Cluster",
	"aws:eks/fargateProfile:FargateProfile",
	"aws:eks/nodeGroup:NodeGroup",
	"aws:elasticache/cluster:Cluster",
	"aws:elasticache/replicationGroup:ReplicationGroup",
	"aws:elasticbeanstalk/application:Application",
	"aws:elasticbeanstalk/applicationVersion:ApplicationVersion",
	"aws:elasticbeanstalk/environment:Environment",
	"aws:elasticloadbalancing/loadBalancer:LoadBalancer",
	"aws:elasticloadbalancingv2/loadBalancer:LoadBalancer",
	"aws:elasticloadbalancingv2/targetGroup:TargetGroup",
	"aws:elasticsearch/domain:Domain",
	"aws:elb/loadBalancer:LoadBalancer",
	"aws:emr/cluster:Cluster",
	"aws:fsx/lustreFileSystem:LustreFileSystem",
	"aws:fsx/windowsFileSystem:WindowsFileSystem",
	"aws:gamelift/alias:Alias",
	"aws:gamelift/build:Build",
	"aws:gamelift/fleet:Fleet",
	"aws:gamelift/gameSessionQueue:GameSessionQueue",
	"aws:glacier/vault:Vault",
	"aws:glue/crawler:Crawler",
	"aws:glue/job:Job",
	"aws:glue/trigger:Trigger",
	"aws:iam/role:Role",
	"aws:iam/user:User",
	"aws:inspector/resourceGroup:ResourceGroup",
	"aws:kinesis/analyticsApplication:AnalyticsApplication",
	"aws:kinesis/firehoseDeliveryStream:FirehoseDeliveryStream",
	"aws:kinesis/stream:Stream",
	"aws:kms/externalKey:ExternalKey",
	"aws:kms/key:Key",
	"aws:lambda/function:Function",
	"aws:lb/loadBalancer:LoadBalancer",
	"aws:lb/targetGroup:TargetGroup",
	"aws:licensemanager/licenseConfiguration:LicenseConfiguration",
	"aws:lightsail/instance:Instance",
	"aws:mediaconvert/queue:Queue",
	"aws:mediapackage/channel:Channel",
	"aws:mediastore/container:Container",
	"aws:mq/broker:Broker",
	"aws:mq/configuration:Configuration",
	"aws:msk/cluster:Cluster",
	"aws:neptune/cluster:Cluster",
	"aws:neptune/clusterInstance:ClusterInstance",
	"aws:neptune/clusterParameterGroup:ClusterParameterGroup",
	"aws:neptune/eventSubscription:EventSubscription",
	"aws:neptune/parameterGroup:ParameterGroup",
	"aws:neptune/subnetGroup:SubnetGroup",
	"aws:opsworks/stack:Stack",
	"aws:organizations/account:Account",
	"aws:pinpoint/app:App",
	"aws:qldb/ledger:Ledger",
	"aws:ram/resourceShare:ResourceShare",
	"aws:rds/cluster:Cluster",
	"aws:rds/clusterEndpoint:ClusterEndpoint",
	"aws:rds/clusterInstance:ClusterInstance",
	"aws:rds/clusterParameterGroup:ClusterParameterGroup",
	"aws:rds/clusterSnapshot:ClusterSnapshot",
	"aws:rds/eventSubscription:EventSubscription",
	"aws:rds/instance:Instance",
	"aws:rds/optionGroup:OptionGroup",
	"aws:rds/parameterGroup:ParameterGroup",
	"aws:rds/securityGroup:SecurityGroup",
	"aws:rds/snapshot:Snapshot",
	"aws:rds/subnetGroup:SubnetGroup",
	"aws:redshift/cluster:Cluster",
	"aws:redshift/eventSubscription:EventSubscription",
	"aws:redshift/parameterGroup:ParameterGroup",
	"aws:redshift/snapshotCopyGrant:SnapshotCopyGrant",
	"aws:redshift/snapshotSchedule:SnapshotSchedule",
	"aws:redshift/subnetGroup:SubnetGroup",
	"aws:resourcegroups/group:Group",
	"aws:route53/healthCheck:HealthCheck",
	"aws:route53/resolverEndpoint:ResolverEndpoint",
	"aws:route53/resolverRule:ResolverRule",
	"aws:route53/zone:Zone",
	"aws:s3/bucket:Bucket",
	"aws:s3/bucketObject:BucketObject",
	"aws:sagemaker/endpoint:Endpoint",
	"aws:sagemaker/endpointConfiguration:EndpointConfiguration",
	"aws:sagemaker/model:Model",
	"aws:sagemaker/notebookInstance:NotebookInstance",
	"aws:secretsmanager/secret:Secret",
	"aws:servicecatalog/portfolio:Portfolio",
	"aws:sfn/activity:Activity",
	"aws:sfn/stateMachine:StateMachine",
	"aws:sns/topic:Topic",
	"aws:sqs/queue:Queue",
	"aws:ssm/activation:Activation",
	"aws:ssm/document:Document",
	"aws:ssm/maintenanceWindow:MaintenanceWindow",
	"aws:ssm/parameter:Parameter",
	"aws:ssm/patchBaseline:PatchBaseline",
	"aws:storagegateway/cachesIscsiVolume:CachesIscsiVolume",
	"aws:storagegateway/gateway:Gateway",
	"aws:storagegateway/nfsFileShare:NfsFileShare",
	"aws:storagegateway/smbFileShare:SmbFileShare",
	"aws:swf/domain:Domain",
	"aws:transfer/server:Server",
	"aws:transfer/user:User",
	"aws:waf/rateBasedRule:RateBasedRule",
	"aws:waf/rule:Rule",
	"aws:waf/ruleGroup:RuleGroup",
	"aws:waf/webAcl:WebAcl",
	"aws:wafregional/rateBasedRule:RateBasedRule",
	"aws:wafregional/rule:Rule",
	"aws:wafregional/ruleGroup:RuleGroup",
	"aws:wafregional/webAcl:WebAcl",
	"aws:workspaces/directory:Directory",
	"aws:workspaces/ipGroup:IpGroup",
}
