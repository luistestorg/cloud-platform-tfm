# Native Link EKS Cluster Bootstrap

## Prerequisites

Install the AWS cli and establish AWS creds, see:
https://www.pulumi.com/docs/clouds/aws/get-started/begin/

On a Mac, do:
```
brew install awscli
```

For working with Pulumi, you'll need an AWS profile named: `tm`:
```
[tm]
region = us-east-2
output = json
```

Verify:
```
aws sts get-caller-identity --profile tm
```

If that doesn't work, then make sure you have the AWS access key and secret added to `~/.aws/credentials`, such as:
```
[tm]
aws_access_key_id = ???
aws_secret_access_key = ???
```

Install Pulumi / Go 1.22 if needed
```
brew update && brew upgrade pulumi
brew install go@1.22
```

## EKS Clusters

Currently, we have two long-lived clusters:
* [build-faster](https://api.build-faster.nativelink.net) (Ohio: us-east-2): Production, hosts customer deployments
* [dev-usw2](https://api.dev-usw2.nativelink.net) (Oregon: us-west-2): Staging / development

Access to the Cloud Orchestrator API and Web console for these clusters is managed by AWS Cognito in the specific region.

### Get kubectl access to dev-usw2 cluster

You need to follow a very specific pattern for accessing our clusters using `kubectl`.

First, go install `kubectx` and `kubens`; on a Mac:

```
brew install kubectx
curl -sS https://webi.sh/kubens | sh
```

We also recommend [kube-ps1](https://github.com/jonmosco/kube-ps1), so you can see which cluster you're currently connected to in your shell prompt.

Next, configure the `eks-access` profile in `~/.aws/config`:
```
[profile eks-access]
source_profile=tm
role_arn = arn:aws:iam::299166832260:role/eks-kubectl-access
role_session_name = YOUR_NAME_HERE
```
_Note: the `source_profile` **MUST** be `tm` as that is the configured profile in our Pulumi stacks. The `role_session_name` helps differentiate you in the Kube API server audit logs since we're all coming through the same IAM role._

Authenticate to the `dev-usw2` staging cluster:
```
aws eks --profile eks-access --region us-west-2 update-kubeconfig --name dev-usw2
kubectx dev-usw2=.
```

This gives you **read-only** access (get / list) to all objects in the cluster.

Verify you do not have full `cluster-admin` access (the following command should output `no`):
```
kubectl auth can-i "*" "*"
```

If you don't have access to the cluster, make sure your user ARN is added to the `eks-kubectl-access` IAM role trust relationships list (via AWS console).

#### Break Glass Operation

If you need to make manual changes to the cluster, you'll need to configure a "break glass" kubeconfig. 
This process is intended to prevent you from accidentally updating the wrong cluster, such as changing prod when you thought you were pointing to your dev kind cluster.

First, add your IAM user to the `eks-kubectl-dev-access` role using the AWS console (it may already be there).

Verify you can assume that role:
```
aws sts assume-role --profile tm --role-arn arn:aws:iam::299166832260:role/eks-kubectl-dev-access --role-session-name kbg-test
```
_Note: you need to pass the `--profile tm` here since that profile resolves the AWS API access key and secret in the `~/.aws/credentials` file_

Next, define another AWS profile named `eks-break-glass-dev` that assumes the `eks-kubectl-dev-access` IAM role with `source_profile=tm`:
```
[profile eks-break-glass-dev]
source_profile=tm
role_arn = arn:aws:iam::299166832260:role/eks-kubectl-dev-access
role_session_name = kbg-YOUR_NAME_HERE
```
_Replace `YOUR_NAME_HERE` with your name, so we can differentiate users in the audit logs, such as `role_session_name = kbg-tim`_

Next, you need to create a **kubeconfig**, but instead of writing it out to your `~/.kube/config` file, we're going to save it to a separate file that our `kbg` script references directly.
```
aws eks --profile eks-break-glass-dev --region us-west-2 update-kubeconfig --name dev-usw2 --kubeconfig=$HOME/.kube/breakglass
```
_Tip: Never use the `eks-break-glass-dev` profile to configure access to clusters added to your default `$HOME/.kube/config` configuration._

To verify you have full `cluster-admin` access, run the following (should report `yes`):
```
KBG_CONTEXT=dev-usw2 hack/kbg auth can-i "*" "*"
```
Whenever you need to "break glass" to manually work on the cluster, use the `hack/kbg` script instead of `kubectl`.

This is similar to using `sudo`, so please think before you type when using break glass mode.

For PROD, the process is similar, except you use the following profile:
```
[profile eks-break-glass-prod]
source_profile=tm
role_arn = arn:aws:iam::299166832260:role/eks-kubectl-prod-access
role_session_name = kbg-YOUR_NAME_HERE
```
Then add this cluster to your break glass kubeconfig
```
aws eks --profile eks-break-glass-prod --region us-east-2 update-kubeconfig --name build-faster --kubeconfig=$HOME/.kube/breakglass
```
Test:
```
KBG_CONTEXT=build-faster hack/kbg auth can-i "*" "*"
```

#### Namespace Admin for NativeLink Claims

Use the [dev app](dev.nativelink.com) (or [Swagger UI](https://api.dev-usw2.nativelink.net/swagger/index.html#)) to deploy a NativeLink claim on the `dev-usw2` cluster if you don't already have one.
This will create the `nativelink-<claimId>` namespace on the cluster.

With [Kubernetes RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#kubectl-create-rolebinding), you can scope a ClusterRole, e.g. `cluster-admin`, to a specific namespace using a `RoleBinding` (instead of a `ClusterRoleBinding`). Thus, you have two basic options:
1. Create a `RoleBinding` for yourself and other users if needed, or
2. Create a `RoleBinding` for a group of users based on IAM role, e.g. `readonly`

For option 1, create a `RoleBinding` in a specific namespace to grant one or more users `cluster-admin` (a Role) access (in that namespace only) where NativeLink is deployed:
```
cat <<EOF | KBG_CONTEXT=dev-usw2 hack/kbg create -n NAMESPACE -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: nativelink-claim-admin
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- apiGroup: rbac.authorization.k8s.io
  kind: User
  name: USERNAME
EOF
```
* _Notice we're using the `kbg` script to create the rolebinding_
* _Replace `NAMESPACE` in the command with the actual namespace you want to create the role binding in_
* _Replace `USERNAME` with your role session name (from `~/.aws/config`) prefixed by `readonly:`, e.g. `readonly:tim-test`_

You can get your username by doing:
```
kubectl auth whoami | grep -i username | xargs | cut -d' ' -f2 -
```

Your username will be `readonly:<ROLE_SESSION_NAME>` based on how we mapped the `eks-kubectl-access` IAM Role in the `aws-auth` ConfigMap, i.e.
```
    - rolearn: arn:aws:iam::299166832260:role/eks-kubectl-access
      username: readonly:{{SessionName}}
      groups:
        - readonly
```

For option 2, create a `RoleBinding` in a specific namespace to grant users in the `readonly` IAM role `cluster-admin` (a Role) access (in that namespace only) where NativeLink is deployed:

```
cat <<EOF | KBG_CONTEXT=dev-usw2 hack/kbg create -n NAMESPACE -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: nativelink-claim-admin
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- apiGroup: rbac.authorization.k8s.io
  kind: Group
  name: readonly
EOF
```
_Replace `NAMESPACE` in the command with the actual namespace you want to create the role binding in_
_Note: this grants anyone in the `readonly` IAM role full access to your namespace (good for collaboration)_

## Building a New Cluster

### Prerequisites
Choose a stack name, region, and domain name, configure in the `Pulumi.<stack>.yaml` (see Pulumi.example.yaml)

Most of the configuration parameters are self-explanatory. Be sure to update the following cluster/region/zone specific settings:
```
config:
  aws:region: us-west-2
  nativelink-cloud:eks:
    awsAccountId: "299166832260"
    clusterName: build-test3
    vpcName: build-test3
  nativelink-cloud:selfServiceApi:
    rdsZone: "us-west-2a"
  nativelink-cloud:tls:
    domain: build-test3.nativelink.net
```

Set up a domain name in Route53 if not using `nativelink.net`.

Configure the OAuth2 settings in the stack YAML, e.g.
```
  nativelink-cloud:selfServiceApi:
    apiEnabled: true
    oauth2ClientId: "1mkfc64irkbhvaann4o701f87"
    oauth2ValidateUrl: "https://nativelink.auth.us-east-2.amazoncognito.com/oauth2/userInfo"
    oidcIssuerUrl: "https://cognito-idp.us-east-2.amazonaws.com/us-east-2_EuxT2H2ua"
```
_See Pulumi.build-faster.yaml for a complete example of OAuth2 settings_

### Build the Cluster

Login to Pulumi:
```
pulumi login
```
This should prompt you for an access token (get from Tim for now).

Configure an OIDC Provider for the Self-Service API, such as AWS Cognito; you can get the Cognito OAuth2 secret from the AWS console for now.

You'll need to set the OAuth2 client secret in an env var for the new stack using:
```
export OAUTH2_CLIENT_SECRET="???"
```

Once your stack config YAML is ready, run:
```
./pulumi-cli.sh init -s <STACK>
```
It can take up to 15 minutes to build out the cluster and dependencies.

__Note: running `pulumi preview` for a yet to be created stack doesn't work because we wait to see the NLB to be provisioned, just run `./pulumi-cli.sh init -s <STACK>` and then you do `preview` or `refresh` after it completes.__

After running `init` the first time, run `./pulumi-cli.sh up -s <STACK>` to apply any changes.

Also, due to how Pulumi works, if the `init` fails, then you'll need to run `up` after correcting any errors, i.e. `init` is a one-time-shot kind of thing :(

If you configured your stack with `apiEnabled: true`, be sure to update the redirect URL in the provider config (such as in AWS Cognito):
```
https://api.<YOUR_DOMAIN>/oauth2/callback
```

To connect to the EKS cluster using kubectl:
```
aws eks --profile <PROFILE> --region <REGION> update-kubeconfig --name <CLUSTER>
```
_Tip: the `./pulumi-cli.sh init -s <STACK>` script will do this for you if the provisioning process works correctly.

### Alertmanager Integration with BetterStack

If you're building a production cluster that needs on-call support, then you'll want to configure the BetterStack Webhook URL secret by doing:
```
pulumi config set --secret nativelink-cloud:alertWebhookUrl "https://uptime.betterstack.com/api/v1/prometheus/webhook/???"
```
_Get the value for `???` from the BetterStack Web console_

### AWS Cognito Details

For more information about the OAuth2 config for AWS Cognito for the `build-faster` stack:
```
{
  "authorization_endpoint": "https://nativelink.auth.us-east-2.amazoncognito.com/oauth2/authorize",
  "id_token_signing_alg_values_supported": [
    "RS256"
  ],
  "issuer": "https://cognito-idp.us-east-2.amazonaws.com/us-east-2_EuxT2H2ua",
  "jwks_uri": "https://cognito-idp.us-east-2.amazonaws.com/us-east-2_EuxT2H2ua/.well-known/jwks.json",
  "response_types_supported": [
    "code",
    "token"
  ],
  "scopes_supported": [
    "openid",
    "email",
    "phone",
    "profile"
  ],
  "subject_types_supported": [
    "public"
  ],
  "token_endpoint": "https://nativelink.auth.us-east-2.amazoncognito.com/oauth2/token",
  "token_endpoint_auth_methods_supported": [
    "client_secret_basic",
    "client_secret_post"
  ],
  "userinfo_endpoint": "https://nativelink.auth.us-east-2.amazoncognito.com/oauth2/userInfo"
}
```