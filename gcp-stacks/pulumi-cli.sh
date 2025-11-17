#!/bin/bash

#set -ex

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" > /dev/null && pwd )"

STACK=""
SUB_STACK_DIR=""
ACTION="preview"
PREVIEW="0"
REFRESH_BEFORE_UP="0"
UPDATE_KUBECONFIG="0"

function print_usage() {
  CMD="$1"
  ERROR_MSG="$2"

  if [ "$CMD" == "" ]; then
    CMD="./pulumi-cli.sh"
  fi

  if [ "$ERROR_MSG" != "" ]; then
    echo -e "\nERROR: $ERROR_MSG\n"
  fi

  echo -e "Use this script to provision an GKE cluster for hosting NativeLink claims using Pulumi \n"
  echo -e "Usage: $CMD [ACTION] [OPTIONS] ... where OPTIONS include:\n"
  echo -e "  -s               Stack name; required for all actions except bootstrap"
  echo -e "  -p               Do a preview before running the specified action"
  echo -e "  -r               Do a refresh before running up"
  echo -e "  --sub-stack-dir  Run the specified action for one of the sub-stacks; otherwise the action is applied to all sub-stacks"
  echo -e "                      Supported sub-stacks: infra-gcp, sql, mon-log, infra-kube, nativelink-shared, api"
  echo -e "  --update-kubeconfig Run 'gcloud container clusters ...' before running up to ensure you are authenticated correctly"
  echo -e "\nSupported actions are: bootstrap, init, up, refresh, destroy, rm, mod_tidy, gcloud_auth\n"
  echo -e "\nTo create the Pulumi YAML files for a new stack, run:\n\t$CMD boostrap\n"
  echo -e "\nTo build out a new GKE cluster (after boostrap), run:\n\t$CMD init -s <STACK_NAME>\n"
}

function init_sub_stack() {
  date -u
  stackName="$1"
  subStackDir="$2"
  set +e
  pulumi stack select "${STACK}" -C "${subStackDir}" &> select_output
  select_out=$(<select_output)
  rm select_output
  regex="^error: no stack named '.*' found$"
  # if the stack doesn't exist, then we init it and set secrets
  initStack=0
  if [[ $select_out =~ $regex ]]; then
    echo -e "\nInitializing sub-stack in: ${subStackDir}"
    pulumi stack select "${stackName}" --create -C "${subStackDir}"
    initStack=1
  else
    echo -e "\nSub-stack in '${subStackDir}' already init'd, re-running up ..."
  fi
  set -e

  if [[ "${subStackDir}" == "sql" && "${initStack}" == "1" ]]; then
  # random passwords that get saved into K8s secrets
    if [[ "$OSTYPE" == "linux-gnu" ]]; then
      dbPassword=$(date -u | md5sum | cut -d ' ' -f 1)
      pgPassword=$(date -u | md5sum | cut -d ' ' -f 1)
    else
      dbPassword=$(date -u | md5)
      pgPassword=$(date -u | md5)
    fi
    pulumi config set -s "${stackName}" --secret sql:dbPassword "${dbPassword}" -C "${subStackDir}"
    pulumi config set -s "${stackName}" --secret sql:pgPassword "${pgPassword}" -C "${subStackDir}"
  fi

  # additional random passwords
  if [[ "${subStackDir}" == "mon-log" && "${initStack}" == "1" ]]; then
    if [[ "$OSTYPE" == "linux-gnu" ]]; then
      grafanaAdminPassword=$(date -u | md5sum | cut -d ' ' -f 1)
      oauth2CookieSecret=$(date -u | md5sum | cut -d ' ' -f 1) # must be 32 chars
      bootstrapAdminPassword=$(date -u | md5sum | cut -d ' ' -f 1)
      elasticSearchPassword=$(date -u | md5sum | cut -d ' ' -f 1)
      kibanaPassword=$(date -u | md5sum | cut -d ' ' -f 1)
    else
      grafanaAdminPassword=$(date -u | md5)
      oauth2CookieSecret=$(date -u | md5) # must be 32 chars
      bootstrapAdminPassword=$(date -u | md5)
      elasticSearchPassword=$(date -u | md5)
      kibanaPassword=$(date -u | md5)
    fi

    pulumi config set -s "${stackName}" --secret mon-log:oauth2ClientSecret "${OAUTH2_CLIENT_SECRET}" -C "${subStackDir}"
    pulumi config set -s "${stackName}" --secret mon-log:slackWebhookUrl "${SLACK_WEBHOOK_URL}" -C "${subStackDir}"
    pulumi config set -s "${stackName}" --secret mon-log:oauth2CookieSecret "${oauth2CookieSecret}" -C "${subStackDir}"
    pulumi config set -s "${stackName}" --secret mon-log:grafanaAdminPassword "${grafanaAdminPassword}" -C "${subStackDir}"
    pulumi config set -s "${stackName}" --secret mon-log:bootstrapAdminPassword "${bootstrapAdminPassword}" -C "${subStackDir}"    
    pulumi config set -s "${stackName}" --secret mon-log:elasticSearchPassword "${elasticSearchPassword}" -C "${subStackDir}"
    pulumi config set -s "${stackName}" --secret mon-log:kibanaPassword "${kibanaPassword}" -C "${subStackDir}"
  fi

  if [[ "${subStackDir}" == "nativelink-shared" && "${initStack}" == "1" ]]; then
    if [[ "$OSTYPE" == "linux-gnu" ]]; then
      mongoRootPassword=$(date -u | md5sum | cut -d ' ' -f 1)
      mongoDatabasePassword=$(date -u | md5sum | cut -d ' ' -f 1)
    else
      mongoRootPassword=$(date -u | md5)
      mongoDatabasePassword=$(date -u | md5)
    fi
    pulumi config set -s "${stackName}" --secret nativelink-shared:mongoRootPassword "${mongoRootPassword}" -C "${subStackDir}"
    pulumi config set -s "${stackName}" --secret nativelink-shared:mongoDatabasePassword "${mongoDatabasePassword}" -C "${subStackDir}"
  fi

  if [[ "${subStackDir}" == "api" && "${initStack}" == "1" ]]; then
    if [[ "$OSTYPE" == "linux-gnu" ]]; then
      oauth2CookieSecret=`date -u | md5sum | cut -d ' ' -f 1` # must be 32 chars
      cachePassword=`date -u | md5sum | cut -d ' ' -f 1`
      sharedCachePassword=`date -u | md5sum | cut -d ' ' -f 1`
    else
      oauth2CookieSecret=`date -u | md5` # must be 32 chars
      cachePassword=`date -u | md5`
      sharedCachePassword=`date -u | md5`
    fi

    pulumi config set -s "${stackName}" --secret api:oauth2CookieSecret "${oauth2CookieSecret}" -C "${subStackDir}"
    pulumi config set -s "${stackName}" --secret api:cachePassword "${cachePassword}" -C "${subStackDir}"
    pulumi config set -s "${stackName}" --secret api:sharedCachePassword "${sharedCachePassword}" -C "${subStackDir}"
    #Get this from 1password https://start.1password.com/open/i?a=NSXIQQENTNHTLJSLS7Y5SIUZNY&v=wymdcm3xv76bewdn6lqytamazi&i=prhu7meamucd4zksjfohcabr6a&h=tracemachina.1password.com
    #Or using the CloudEngSA Access Keys and executing `aws --profile --region us-east-2 cloudeng-access ecr get-login-password`
    pulumi config set -s "${stackName}" --secret api:awsAccessKeyId "${AWS_ACCESS_KEY_ID}" -C "${subStackDir}"
    pulumi config set -s "${stackName}" --secret api:awsSecretAccessKey "${AWS_SECRET_ACCESS_KEY}" -C "${subStackDir}"
    # TODO: not sure if this is needed ~ pulumi config set -s "${stackName}" aws:region us-west-2 -C "${subStackDir}"
  fi

  if [[ "${subStackDir}" == "ci-support" && "${initStack}" == "1" ]]; then

    #Get this values from the github app in the cloud-platform repository
    pulumi config set -s "${stackName}" --secret ci-support:github_app_id "${GITHUB_APP_ID}" -C "${subStackDir}"
    pulumi config set -s "${stackName}" --secret ci-support:github_app_installation_id "${GITHUB_APP_INSTALLATION_ID}" -C "${subStackDir}"
    #Another option for this would be provide the PRIVATE_KEY file location and use cat for loading the value:
    #cat PRIVATE_KEY_FILE  | pulumi config set --secret ci-support:github_app_private_key -C "${subStackDir}"
    pulumi config set -s "${stackName}" --secret ci-support:github_app_private_key "${GITHUB_APP_PRIVATE_KEY}" -C "${subStackDir}"
    
  fi


  set +e # give a chance to retry
  PULUMI_K8S_ENABLE_PATCH_FORCE="true" pulumi up -s "${stackName}" --yes --skip-preview -C "${subStackDir}"
  pulumi refresh -s "${stackName}" --yes --skip-preview -C "${subStackDir}"
  set -e
  date -u
}

function update_stack() {
  stackName="$1"
  stackDir="$2"

  echo -e "\nUpdating stack ${stackName} in dir: ${stackDir}\n"

  if [ "${REFRESH_BEFORE_UP}" == "1" ]; then
    pulumi refresh -s "${stackName}" --yes --skip-preview -C "${stackDir}"
  fi
  if [ "${PREVIEW}" == "0" ]; then
    pulumi up -s "${stackName}" --yes --skip-preview -C "${stackDir}"
  else
    pulumi up -s "${stackName}" -C "${stackDir}"
  fi
}

function check_kubectl_context() {
  # ensure kubectl is pointing at the right cluster, else we'd risk affecting resources on the wrong cluster
  clusterName=$(pulumi config get "gcp-stack:clusterName")
  if [ -z "$clusterName" ]; then
    echo -e "\nERROR: failed to determine the 'clusterName' for this stack!\n"
    exit 1
  fi
  region=$(pulumi config get "gcp-stack:region")
  if [ -z "$region" ]; then
    echo -e "\nERROR: failed to determine the 'gcp-stack:region' for this stack!\n"
    exit 1
  fi
  current=$(kubectl config current-context)
  if [[ $current = *"${clusterName}"* ]]; then
    echo -e "Using kubeconfig: $current"
  else
    echo -e "ERROR: Make sure your current kubeconfig '${current}' is pointing at the GKE '${clusterName}' cluster in ${region}!\n"
    exit 1
  fi
}

if [ $# -gt 0 ]; then
  while true; do
    case "$1" in
        mod_tidy)
            ACTION="mod_tidy"
            shift
        ;;
        init)
            ACTION="init"
            shift
        ;;
        up)
            ACTION="up"
            shift
        ;;
        refresh)
            ACTION="refresh"
            shift
        ;;
        preview)
            ACTION="preview"
            shift
        ;;
        destroy)
            ACTION="destroy"
            shift
        ;;
        rm)
            ACTION="rm"
            shift
        ;;
        bootstrap)
            ACTION="bootstrap"
            shift
        ;;
        gcloud_auth)
            ACTION="gcloud_auth"
            shift
        ;;
        -p)
            PREVIEW="1"
            shift
        ;;
        -r)
            REFRESH_BEFORE_UP="1"
            shift
        ;;
        --update-kubeconfig)
            UPDATE_KUBECONFIG="1"
            shift
        ;;
        --sub-stack-dir)
            if [[ -z "$2" || "${2:0:1}" == "-" ]]; then
              print_usage "$SCRIPT_CMD" "Missing value for the --sub-stack-dir parameter!"
              exit 1
            fi
            SUB_STACK_DIR="$2"
            shift 2
        ;;
        -s)
            if [[ -z "$2" || "${2:0:1}" == "-" ]]; then
              print_usage "$SCRIPT_CMD" "Missing value for the -s parameter!"
              exit 1
            fi
            STACK="$2"
            shift 2
        ;;
        -help|-usage|--help|--usage)
            print_usage "$SCRIPT_CMD"
            exit 0
        ;;
        --)
            shift
            break
        ;;
        *)
            if [ "$1" != "" ]; then
              print_usage "$SCRIPT_CMD" "Unrecognized or misplaced argument: $1!"
              exit 1
            else
              break # out-of-args, stop looping
            fi
        ;;
    esac
  done
fi

subStacks=( "infra-gcp" "sql" "mon-log" "infra-kube" "nativelink-shared" "api" "ci-support")

if [ "${ACTION}" == "mod_tidy" ]; then
  go mod tidy
  for sub in "${subStacks[@]}"
  do
    cd "${sub}"
    echo "Running 'go mod tidy' in ${sub}"
    go mod tidy
    cd ..
  done
  exit 0
fi

gcloud --version > /dev/null 2<&1
has_prereq=$?
if [ $has_prereq == 1 ]; then
  echo -e "\nERROR: Must install 'gcloud' command line tools! See https://cloud.google.com/sdk/docs/quickstarts"
  exit 1
fi

who_am_i=$(gcloud auth list --filter=status:ACTIVE --format="value(account)")
if [ "$who_am_i" == "" ]; then
  echo -e "\nERROR: GCP user unknown, please use: 'gcloud auth login <account>' before proceeding with this script!"
  exit 1
fi
echo -e "\nGCP user: $who_am_i"

gcp_project=$(gcloud config get-value project)
if [ -z "${gcp_project}" ]; then
  echo -e "\nERROR: GCP project not set! Please run:"
  echo -e "\t gcloud config set project <PROJECT>"
  echo -e "where <PROJECT> is your GCP project, such as: native-link-cloud"
  exit 1
fi
echo -e "\nGCP project: $gcp_project"
projectId="native-link-cloud"
if [ "${gcp_project}" != "${projectId}" ]; then
  echo -e "\nERROR: Please update your default GCP project to: $projectId\ngcloud config set project $projectId\n"
  exit 1
fi

region=$(gcloud config get-value compute/region)
echo -e "\nGCP region: $region"

zone=$(gcloud config get-value compute/zone)
echo -e "\nGCP zone: $zone"

if [ "${ACTION}" == "bootstrap" ]; then
  if ! command -v gomplate &> /dev/null; then
    echo -e "\nERROR: Must install 'gomplate' before proceeding with this script!\n(hint: brew install gomplate or see: https://docs.gomplate.ca/installing/)\n"
    exit 1
  fi

  echo -e "\nLet's bootstrap the Pulumi YAML files for a new GKE stack in the '$projectId' project."
  echo -e "\nEnter a unique cluster name (this will also be the Pulumi stack name): "
  read -r clusterIn
  if [ -z "$clusterIn" ]; then
    echo -e "\nERROR: Cluster name is required!"
    exit 1
  fi

  stackRegex="^[a-z][a-z0-9-]{2,11}$"
  if [[ ! $clusterIn =~ $stackRegex ]]; then
    echo -e "\nERROR: Cluster name must match regex: $stackRegex"
    exit 1
  fi

  echo -e "\nEnter the GKE version: "
  read -r gkeVersIn
  if [ -z "$gkeVersIn" ]; then
    echo -e "\nERROR: GKE version is required (and which version changes often,\n    see: https://cloud.google.com/kubernetes-engine/docs/release-notes\n"
    exit 1
  fi

  defaultRegion="$region"
  if [ -z "$defaultRegion" ]; then
    defaultRegion="us-central1"
  fi
  defaultZone="$zone"
  if [ -z "$defaultZone" ]; then
    defaultZone="$defaultRegion-a"
  fi

  echo -e "\nEnter the GCP region (default: $defaultRegion): "
  read -r regionIn
  if [ -z "$regionIn" ]; then
    regionIn="$defaultRegion"
  fi

  echo -e "\nEnter the primary zone in the '$regionIn' region (default: $defaultZone): "
  read -r zoneIn
  if [ -z "$zoneIn" ]; then
    zoneIn="$defaultZone"
  fi

  echo -e "\nEnter a secondary zone in the '$regionIn' region (must be different from primary): "
  read -r secondaryZoneIn
  if [ -z "$secondaryZoneIn" ]; then
    echo -e "\nERROR: secondary zone is required\n"
    exit 1
  fi
  if [ "$secondaryZoneIn" == "$zoneIn" ]; then
    echo -e "\nERROR: secondary zone must be different than the primary '$zoneIn'\n"
    exit 1
  fi

  echo -e "\nCluster type (one of: dev | staging | prod, default: dev): "
  read -r envIn
  if [ -z "$envIn" ]; then
    envIn="dev"
  fi

  echo -e "\nEnter the cluster domain (default: $clusterIn.scdev.nativelink.net): "
  read -r domainIn
  if [ -z "$domainIn" ]; then
    domainIn="$clusterIn.scdev.nativelink.net"
  fi
  domainRegex="^([a-z0-9\-]+)\.scdev\.nativelink\.net$"
  if [[ ! $domainIn =~ $domainRegex ]]; then
    echo -e "\nERROR: Invalid cluster domain '$domainIn' must match regex: $domainRegex"
    exit 1
  fi

  echo -e "\nEnter the DB instance type (default: db-f1-micro): "
  read -r dbTypeIn
  if [ -z "$dbTypeIn" ]; then
    dbTypeIn="db-f1-micro"
  fi

  echo "GcpProject: $projectId" > bootstrap.yaml
  echo "GcpRegion: $regionIn" >> bootstrap.yaml
  echo "GcpZone: $zoneIn" >> bootstrap.yaml
  echo "GcpZone2: $secondaryZoneIn" >> bootstrap.yaml
  echo "StackName: $clusterIn" >> bootstrap.yaml
  echo "Env: $envIn" >> bootstrap.yaml
  echo "ClusterDomain: $domainIn" >> bootstrap.yaml
  echo "DbInstanceType: $dbTypeIn" >> bootstrap.yaml
  echo "GcpUser: $who_am_i" >> bootstrap.yaml
  echo "GkeVersion: $gkeVersIn" >> bootstrap.yaml

  echo -e "\nSettings to render the YAML templates:\n"
  cat bootstrap.yaml
  echo -e "\nDo the boostrap settings shown above look correct? Y/n"
  read -r confirmNs
  if [[ "${confirmNs}" != "" && "${confirmNs}" != "y" && "${confirmNs}" != "Y" ]]; then
    echo -e "\nSettings not confirmed, try another time. Bye bye\n"
    exit 0
  fi

  set -e
  gomplate -d cfg=./bootstrap.yaml -f Pulumi.stack-yaml.gotmpl --out "Pulumi.$clusterIn.yaml"
  gomplate -d cfg=./bootstrap.yaml -f infra-gcp/Pulumi.stack-yaml.gotmpl --out "infra-gcp/Pulumi.$clusterIn.yaml"
  gomplate -d cfg=./bootstrap.yaml -f sql/Pulumi.stack-yaml.gotmpl --out "sql/Pulumi.$clusterIn.yaml"
  gomplate -d cfg=./bootstrap.yaml -f mon-log/Pulumi.stack-yaml.gotmpl --out "mon-log/Pulumi.$clusterIn.yaml"
  gomplate -d cfg=./bootstrap.yaml -f infra-kube/Pulumi.stack-yaml.gotmpl --out "infra-kube/Pulumi.$clusterIn.yaml"
  gomplate -d cfg=./bootstrap.yaml -f nativelink-shared/Pulumi.stack-yaml.gotmpl --out "nativelink-shared/Pulumi.$clusterIn.yaml"
  gomplate -d cfg=./bootstrap.yaml -f api/Pulumi.stack-yaml.gotmpl --out "api/Pulumi.$clusterIn.yaml"
  gomplate -d cfg=./bootstrap.yaml -f ci-support/Pulumi.stack-yaml.gotmpl --out "ci-support/Pulumi.$clusterIn.yaml"

  rm bootstrap.yaml

  echo -e "Successfully rendered the Pulumi YAML files for the $clusterIn stack, review the generated files:"
  cat "Pulumi.$clusterIn.yaml"
  for sub in "${subStacks[@]}"
  do
    stackYaml="${sub}/Pulumi.${clusterIn}.yaml"
    echo -e "\n$stackYaml:\n"
    cat $stackYaml
    echo ""
  done

  echo -e "To build the cluster run:"
  echo -e "./pulumi-cli.sh init -s $clusterIn\n\n"

  exit 0
fi

if [ "${STACK}" == "" ]; then
  print_usage "$SCRIPT_CMD" "Must provide the Pulumi stack name using the -s option!"
  exit 1
fi

if ! command -v kubectl &> /dev/null; then
  echo -e "\nERROR: Must install 'kubectl' before proceeding with this script!"
  exit 1
fi

if ! command -v pulumi &> /dev/null; then
  echo -e "\nERROR: Must install 'pulumi' before proceeding with this script!"
  exit 1
fi

if [ "${ACTION}" == "gcloud_auth" ]; then
  gcloud auth application-default login
  gcloud container clusters get-credentials "${STACK}" --region "${region}" --project "${gcp_project}"
  if command -v kubectx &> /dev/null; then
    kubectx gke-${STACK}=.
    kubens nativelink-api
  fi
  exit 0
fi

set -e

STACK_CONFIG_FILE="Pulumi.${STACK}.yaml"

if ! test -f "${STACK_CONFIG_FILE}"; then
  echo -e "\nERROR: Stack config file '${STACK_CONFIG_FILE}' not found! Check your -s arg!\n"
  exit 1
fi

if [ "${ACTION}" == "init" ]; then

  if [ "${STACK}" == "build-faster" ]; then
    echo -e "\nERROR: Stack ${STACK} is already initialized in the 'us-east-2' region!\nFailing this script out of an abundance of caution to not impact an existing cluster! Pick another stack name.\n"
    exit 1
  fi

  # verify all the sub-stack YAML are in-place before starting the init process
  for sub in "${subStacks[@]}"
  do
    subStackYaml="${sub}/Pulumi.${STACK}.yaml"
    if ! test -f "${subStackYaml}"; then
      echo -e "\nERROR: Required file '${subStackYaml}' not found! You need to create this file before continuing with init."
    fi
  done

  if [ -z "${OAUTH2_CLIENT_SECRET}" ]; then
    echo -e "\nERROR: Must export the OAUTH2_CLIENT_SECRET env var before building the 'mon-log' sub-stack.\n"
    exit 1
  fi

  if [ -z "${SLACK_WEBHOOK_URL}" ]; then
    echo -e "\nERROR: Must export the SLACK_WEBHOOK_URL env var before building the 'mon-log' sub-stack.\n"
    exit 1
  fi

  if [ -z "${AWS_ACCESS_KEY_ID}" ]; then
    echo -e "\nERROR: Must export the AWS_ACCESS_KEY_ID env var before building the 'api' sub-stack.\n"
    exit 1
  fi

  if [ -z "${AWS_SECRET_ACCESS_KEY}" ]; then
    echo -e "\nERROR: Must export the AWS_SECRET_ACCESS_KEY env var before building the 'api' sub-stack.\n"
    exit 1
  fi

  if [ -z "${GITHUB_APP_ID}" ]; then
    echo -e "\nERROR: Must export the GITHUB_APP_ID env var before building the 'ci-support' sub-stack.\n"
    exit 1
  fi

  if [ -z "${GITHUB_APP_INSTALLATION_ID}" ]; then
    echo -e "\nERROR: Must export the GITHUB_APP_INSTALLATION_ID env var before building the 'ci-support' sub-stack.\n"
    exit 1
  fi

  if [ -z "${GITHUB_APP_PRIVATE_KEY}" ]; then
    echo -e "\nERROR: Must export the GITHUB_APP_PRIVATE_KEY env var before building the 'ci-support' sub-stack.\n"
    exit 1
  fi

  echo -e "\nWill build out a new GKE cluster using stack name: ${STACK}, including sub-stacks:"
  echo -e "\t infra-gcp"
  echo -e "\t sql"
  echo -e "\t mon-log"
  echo -e "\t infra-kube"
  echo -e "\t nativelink-shared"
  echo -e "\t api\n"
  echo -e "\t ci-support\n"

  set -e
  pulumi stack select "${STACK}" --create

  echo -e "\nProvisioning new GKE cluster, this can take up to 10 minutes to complete ...\n"

  set +e # give a chance to retry
  PULUMI_K8S_ENABLE_PATCH_FORCE="true" pulumi up --yes --skip-preview
  pulumi refresh --yes --skip-preview
  set -e
  PULUMI_K8S_ENABLE_PATCH_FORCE="true" pulumi up --yes --skip-preview

  clusterName=$(pulumi config get "gcp-stack:clusterName")
  region=$(pulumi config get "gcp-stack:region")
  project=$(pulumi config get "gcp-stack:project")

  echo -e "\nCluster is ready, running command to add this cluster to your ~/.kube/config:\n\n"
  echo -e "\t gcloud container clusters get-credentials $clusterName --region $region --project $project\n"

  if [[ "${clusterName}" != "" && "${region}" != "" && "${project}" != "" ]]; then
    gcloud container clusters get-credentials "$clusterName" --region "$region" --project "$project"
    current=$(kubectl config current-context)
    echo -e "\nUsing kubeconfig: $current\n"
  fi

  for sub in "${subStacks[@]}"
  do
    init_sub_stack "${STACK}" "${sub}"
  done

  exit 0
fi

stackDir="${SCRIPT_DIR}"
if [ "${SUB_STACK_DIR}" != "" ]; then
  stackDir="${SUB_STACK_DIR}"
  if ! test -d "${stackDir}"; then
    echo -e "\nERROR: Invalid sub-stack dir: ${stackDir}! Check your --sub-stack arg"
    exit 1
  fi
fi
pulumi stack select "${STACK}" -C "${stackDir}"

if [ "${ACTION}" == "up" ]; then
  if [ "${UPDATE_KUBECONFIG}" == "1" ]; then
    gcloud auth application-default login
    gcloud container clusters get-credentials "${STACK}" --region "${region}" --project "${gcp_project}"
  fi

  check_kubectl_context

  update_stack "${STACK}" "${stackDir}"

  if [ "${SUB_STACK_DIR}" == "" ]; then
    # update all sub-stacks too
    for sub in "${subStacks[@]}"
    do
      update_stack "${STACK}" "${sub}"
    done
  fi

fi

if [ "${ACTION}" == "preview" ]; then
  echo -e "\nRunning preview for stack '${STACK}' from: ${stackDir}"
  pulumi preview -s "${STACK}" -C "${stackDir}"
fi

if [ "${ACTION}" == "refresh" ]; then
  echo -e "\nRunning preview for stack '${STACK}' from: ${stackDir}"
  pulumi refresh -s "${STACK}" --yes --skip-preview -C "${stackDir}"
fi

if [ "${ACTION}" == "destroy" ]; then
  echo -e "\nDestroy operation cannot be undone!\nPlease confirm that this is what you'd like to do by typing '${STACK}':"
  read -r stackIn

  if [ "${stackIn}" != "${STACK}" ]; then
    echo -e "\nDestroy request not confirmed."
    exit 0
  fi

  # ensure kubectl is pointing at the right cluster, else we'd risk deleting / draining resources on the wrong cluster
  clusterName=$(pulumi config get "gcp-stack:clusterName")
  if [ -z "$clusterName" ]; then
    echo -e "\nERROR: failed to determine the 'clusterName' for this stack!\n"
    exit 1
  fi
  region=$(pulumi config get "gcp-stack:region")
  if [ -z "$region" ]; then
    echo -e "\nERROR: failed to determine the 'gcp-stack:region' for this stack!\n"
    exit 1
  fi
  project=$(pulumi config get "gcp-stack:project")
  if [ -z "$project" ]; then
    echo -e "\nERROR: failed to determine the 'gcp-stack:project' for this stack!\n"
    exit 1
  fi
  echo "updating kubectl to use cluster '$clusterName' in region '$region'"
  gcloud container clusters get-credentials "$clusterName" --region "$region" --project "$project"
  current=$(kubectl config current-context)
  if [[ $current = *"${clusterName}"* ]]; then
    echo -e "Using kubeconfig: $current"
  else
    echo -e "ERROR: Make sure your current kubeconfig '${current}' is pointing at the '${clusterName}' cluster in ${region}!\n"
    exit 1
  fi

  echo -e "\nDestroying the '${STACK}' stack (and all sub-stacks), this may take several minutes to complete ..."

  set +e
  kubectl delete nativelink --all --all-namespaces --ignore-not-found=true
  kubectl delete nativelinkclaims --all --all-namespaces --ignore-not-found=true
  kubectl patch providers provider-gcp --type json --patch='[ { "op": "remove", "path": "/metadata/finalizers" } ]'
  kubectl patch providers provider-kubernetes --type json --patch='[ { "op": "remove", "path": "/metadata/finalizers" } ]'
  kubectl patch providerconfig provider-config-gcp --type json --patch='[ { "op": "remove", "path": "/metadata/finalizers" } ]'
  kubectl patch providerconfig.kubernetes.crossplane.io provider-config-kubernetes --type json --patch='[ { "op": "remove", "path": "/metadata/finalizers" } ]'
  kubectl delete providerconfig provider-config-gcp --force --ignore-not-found=true
  kubectl delete providerconfig.kubernetes.crossplane.io provider-config-kubernetes --force --ignore-not-found=true
  kubectl delete providers provider-gcp --force --ignore-not-found=true
  kubectl delete providers provider-kubernetes --force --ignore-not-found=true

  revSubStacks=("ci-support" "api" "nativelink-shared" "infra-kube" "mon-log" "sql" "infra-gcp" )

  for sub in "${revSubStacks[@]}"
  do
    echo -e "\nDestroying sub-stack in: ${sub}"
    if [ "${REFRESH_BEFORE_UP}" == "1" ]; then
      pulumi refresh -s "${STACK}" --yes --skip-preview -C "${sub}"
    fi

    if [ "${PREVIEW}" == "0" ]; then
      pulumi destroy -s "${STACK}" --yes --skip-preview -C "${sub}"
    else
      pulumi destroy -s "${STACK}" -C "${sub}"
    fi
  done

  if [ "${REFRESH_BEFORE_UP}" == "1" ]; then
    pulumi refresh -s "${STACK}" --yes --skip-preview
  fi

  if [ "${PREVIEW}" == "0" ]; then
    pulumi destroy -s "${STACK}" --yes --skip-preview
  else
    pulumi destroy -s "${STACK}"
  fi
  exit 0
fi

if [ "${ACTION}" == "rm" ]; then
  for sub in "${subStacks[@]}"
  do
    cp "${sub}/${STACK_CONFIG_FILE}" "${sub}/Pulumi.${STACK}.bak"
    echo -e "Backed up stack config YAML to: ${sub}/Pulumi.${STACK}.bak\n"
    pulumi stack rm "${STACK}" --yes --force -C "${sub}"
  done

  cp "${STACK_CONFIG_FILE}" "Pulumi.${STACK}.bak"
  echo -e "Backed up stack config YAML to: Pulumi.${STACK}.bak\n"
  pulumi stack rm "${STACK}" --yes --force
fi
