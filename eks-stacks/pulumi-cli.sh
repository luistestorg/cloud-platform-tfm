#!/bin/bash

STACK=""
ACTION="preview"
PREVIEW="0"
REFRESH_BEFORE_UP="0"

function print_usage() {
  CMD="$1"
  ERROR_MSG="$2"

  if [ "$ERROR_MSG" != "" ]; then
    echo -e "\nERROR: $ERROR_MSG\n"
  fi

  echo -e "Use this script to provision an EKS cluster for hosting NativeLink claims using Pulumi \n"
  echo -e "Usage: $CMD [ACTION] [OPTIONS] ... where OPTIONS include:\n"
  echo -e "  -s           Stack name; required"
  echo -e "  -p           Do a preview before running the specified action"
  echo -e "  -r           Do a refresh before running up"
  echo -e "\nSupported actions are: init, up, refresh, destroy, rm\n"
  echo -e "\nTo build out a new EKS cluster, run:\n\t$CMD init -s <STACK_NAME>\n"
}

function check_kubectl_context() {
  # ensure kubectl is pointing at the right cluster, else we'd risk affecting resources on the wrong cluster
  clusterName=$(kubectl config current-context | cut -d '/' -f 2)
  if [ -z "$clusterName" ]; then
    echo -e "\nERROR: failed to determine the 'clusterName' for this stack!\n"
    exit 1
  fi
  region=$(pulumi config get "aws:region")
  if [ -z "$region" ]; then
    echo -e "\nERROR: failed to determine the 'aws:region' for this stack!\n"
    exit 1
  fi  
  if [[ ${STACK} == "${clusterName}" ]]; then
    echo -e "Using kubeconfig: $STACK"
  else
    echo -e "ERROR: Make sure your current kubeconfig '${clusterName}' is pointing at the EKS '${STACK}' cluster in ${region}!\n"
    exit 1
  fi
}

if [ $# -gt 0 ]; then
  while true; do
    case "$1" in
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
        -p)
            PREVIEW="1"
            shift
        ;;
        -r)
            REFRESH_BEFORE_UP="1"
            shift
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

if [ "${STACK}" == "" ]; then
  print_usage "$SCRIPT_CMD" "Must provide the Pulumi stack name using the -s option!"
  exit 1
fi

hash kubectl
has_prereq=$?
if [ $has_prereq == 1 ]; then
  echo -e "\nERROR: Must install 'kubectl' before proceeding with this script!"
  exit 1
fi

hash pulumi
has_prereq=$?
if [ $has_prereq == 1 ]; then
  echo -e "\nERROR: Must install 'pulumi' before proceeding with this script!"
  exit 1
fi

hash aws
has_prereq=$?
if [ $has_prereq == 1 ]; then
  echo -e "\nERROR: Must install 'aws' cli before proceeding with this script!"
  exit 1
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

  echo -e "\nWill build out a new EKS cluster using stack name: ${STACK}"

  if [ -z "${OAUTH2_CLIENT_SECRET}" ]; then
    echo -e "\nERROR: Must set the OAUTH2_CLIENT_SECRET env var before building the ${STACK} stack.\n"
    exit 1
  fi

  # random passwords that get saved into K8s secrets

  if [[ "$OSTYPE" == "linux-gnu" ]]; then
    grafanaAdminPassword=`date -u | md5sum | cut -d ' ' -f 1`
    mongoRootPassword=`date -u | md5sum | cut -d ' ' -f 1`
    mongoDatabasePassword=`date -u | md5sum | cut -d ' ' -f 1`
    oauth2CookieSecret=`date -u | md5sum | cut -d ' ' -f 1` # must be 32 chars
    dbPassword=`date -u | md5sum | cut -d ' ' -f 1`
    pgPassword=`date -u | md5sum | cut -d ' ' -f 1`
    cachePassword=`date -u | md5sum | cut -d ' ' -f 1`
    bootstrapAdminPassword=`date -u | md5sum | cut -d ' ' -f 1`
    sharedCachePassword=`date -u | md5sum | cut -d ' ' -f 1`
    elasticSearchPassword=`date -u | md5sum | cut -d ' ' -f 1`
    kibanaPassword=`date -u | md5sum | cut -d ' ' -f 1`
  else
    grafanaAdminPassword=`date -u | md5`
    mongoRootPassword=`date -u | md5`
    mongoDatabasePassword=`date -u | md5`
    oauth2CookieSecret=`date -u | md5` # must be 32 chars
    dbPassword=`date -u | md5`
    pgPassword=`date -u | md5`
    cachePassword=`date -u | md5`
    bootstrapAdminPassword=`date -u | md5`
    sharedCachePassword=`date -u | md5`
    elasticSearchPassword=`date -u | md5`
    kibanaPassword=`date -u | md5`
  fi

  pulumi stack init "${STACK}"
  pulumi stack select "${STACK}"

  pulumi config set --secret nativelink-cloud:oauth2ClientSecret "${OAUTH2_CLIENT_SECRET}"
  pulumi config set --secret nativelink-cloud:oauth2CookieSecret "${oauth2CookieSecret}"
  pulumi config set --secret nativelink-cloud:mongoRootPassword "${mongoRootPassword}"
  pulumi config set --secret nativelink-cloud:mongoDatabasePassword "${mongoDatabasePassword}"
  pulumi config set --secret nativelink-cloud:grafanaAdminPassword "${grafanaAdminPassword}"
  pulumi config set --secret nativelink-cloud:dbPassword "${dbPassword}"
  pulumi config set --secret nativelink-cloud:pgPassword "${pgPassword}"
  pulumi config set --secret nativelink-cloud:cachePassword "${cachePassword}"
  pulumi config set --secret nativelink-cloud:sharedCachePassword "${sharedCachePassword}"
  pulumi config set --secret nativelink-cloud:bootstrapAdminPassword "${bootstrapAdminPassword}"
  pulumi config set --secret nativelink-cloud:elasticSearchPassword "${elasticSearchPassword}"
  pulumi config set --secret nativelink-cloud:kibanaPassword "${kibanaPassword}"

  echo -e "\nProvisioning new EKS cluster, this can take up to 30 minutes to complete ...\n"

  set +e # give a chance to retry
  PULUMI_K8S_ENABLE_PATCH_FORCE="true" pulumi up --yes --skip-preview
  pulumi refresh --yes --skip-preview
  set -e
  PULUMI_K8S_ENABLE_PATCH_FORCE="true" pulumi up --yes --skip-preview

  clusterName=$(pulumi config get "nativelink-cloud:eks" | jq -r .clusterName)
  region=$(pulumi config get "aws:region")

  echo -e "\nCluster is ready, running command to add this cluster to your ~/.kube/config:\n\n"
  echo -e "\taws eks --profile eks-access --region $region update-kubeconfig --name $clusterName\n"

  if [[ "${clusterName}" != "" && "${region}" != "" ]]; then
    aws eks --profile eks-access --region $region update-kubeconfig --name $clusterName
    current=$(kubectl config current-context)
    echo -e "\nUsing kubeconfig: $current\n"
  fi

  exit 0
fi

pulumi stack select "${STACK}"

#Checks if the stack name matches the current context cluster 
check_kubectl_context

if [ "${ACTION}" == "up" ]; then

  if [ "${REFRESH_BEFORE_UP}" == "1" ]; then
    pulumi refresh --yes --skip-preview
  fi

  if [ "${PREVIEW}" == "0" ]; then
    pulumi up --yes --skip-preview
  else
    pulumi up
  fi
fi

if [ "${ACTION}" == "preview" ]; then
  pulumi preview
fi

if [ "${ACTION}" == "refresh" ]; then
  pulumi refresh --yes --skip-preview
fi

if [ "${ACTION}" == "destroy" ]; then
  echo -e "\nDestroy operation cannot be undone!\nPlease confirm that this is what you'd like to do by typing '${STACK}':"
  read stackIn

  if [ "${stackIn}" == "build-faster" ]; then
    echo -e "\nDestroying 'build-faster' stack not allowed!\n"
    exit 1
  fi

  if [ "${stackIn}" != "${STACK}" ]; then
    echo -e "\nDestroy request not confirmed."
    exit 0
  fi

  # ensure kubectl is pointing at the right cluster, else we'd risk deleting / draining resources on the wrong cluster
  clusterName=$(pulumi config get "nativelink-cloud:eks" | jq -r .clusterName)
  if [ -z $clusterName ]; then
    echo -e "\nERROR: failed to determine the 'clusterName' for this stack!\n"
    exit 1
  fi
  region=$(pulumi config get "aws:region")
  if [ -z $region ]; then
    echo -e "\nERROR: failed to determine the 'aws:region' for this stack!\n"
    exit 1
  fi
  echo "updating kubectl to use cluster '$clusterName' in region '$region'"
  #aws eks --profile tm --region $region update-kubeconfig --name $clusterName
  current=$(kubectl config current-context)
  if [[ $current = *"${clusterName}"* ]]; then
    echo -e "Using kubeconfig: $current"
  else
    echo -e "ERROR: Make sure your current kubeconfig '${current}' is pointing at the '${clusterName}' cluster in ${region}!\n"
    exit 1
  fi

  echo -e "\nDestroying the '${STACK}' stack, this may take several minutes to complete ..."

  set +e
  echo -e "\nRemoving finalizers on Karpenter/Crossplane resources that block / slow the cluster teardown process ..."
  kubectl delete buckets --all --all-namespaces --ignore-not-found=true # the S3 buckets still exist
  kubectl delete nativelink --all --all-namespaces --ignore-not-found=true
  kubectl delete nativelinkclaims --all --all-namespaces --ignore-not-found=true
  kubectl patch providers provider-aws --type json --patch='[ { "op": "remove", "path": "/metadata/finalizers" } ]'
  kubectl patch providers provider-kubernetes --type json --patch='[ { "op": "remove", "path": "/metadata/finalizers" } ]'
  kubectl patch providerconfig provider-config-aws --type json --patch='[ { "op": "remove", "path": "/metadata/finalizers" } ]'
  kubectl patch providerconfig.kubernetes.crossplane.io provider-config-kubernetes --type json --patch='[ { "op": "remove", "path": "/metadata/finalizers" } ]'
  kubectl delete providerconfig provider-config-aws --force --ignore-not-found=true
  kubectl delete providerconfig.kubernetes.crossplane.io provider-config-kubernetes --force --ignore-not-found=true
  kubectl delete providers provider-aws --force --ignore-not-found=true
  kubectl delete providers provider-kubernetes --force --ignore-not-found=true
  kubectl get nodepool --no-headers | tr -s " " | cut -d' ' -f1 - | xargs -I {} kubectl patch nodepool/{} --type json --patch='[ { "op": "remove", "path": "/metadata/finalizers" } ]'
  kubectl get ec2nodeclass --no-headers | tr -s " " | cut -d' ' -f1 - | xargs -I {} kubectl patch ec2nodeclass/{} --type json --patch='[ { "op": "remove", "path": "/metadata/finalizers" } ]'
  kubectl get nodes -l karpenter.sh/initialized=true --no-headers | tr -s " " | cut -d' ' -f1 - | xargs -I {} kubectl patch node/{} --type json --patch='[ { "op": "remove", "path": "/metadata/finalizers" } ]'
  kubectl get crds --no-headers | grep snapshot | tr -s " " | cut -d' ' -f1 - | xargs -I {} kubectl patch crd/{} --type json --patch='[ { "op": "remove", "path": "/metadata/finalizers" } ]'
  kubectl get ns --no-headers | tr -s " " | cut -d' ' -f1 - | xargs -I {} kubectl patch ns/{} --type json --patch='[ { "op": "remove", "path": "/spec/finalizers" } ]'

  pulumi refresh --yes --skip-preview
  export PULUMI_K8S_DELETE_UNREACHABLE="true"

  if [ "${PREVIEW}" == "0" ]; then
    PULUMI_K8S_DELETE_UNREACHABLE="true" pulumi destroy --yes --skip-preview --continue-on-error
  else
    pulumi destroy
  fi
  exit 0
fi

if [ "${ACTION}" == "rm" ]; then
  cp "${STACK_CONFIG_FILE}" "Pulumi.${STACK}.bak"
  echo -e "Backed up stack config YAML to: Pulumi.${STACK}.bak\n"
  pulumi stack rm "${STACK}" --yes --force
fi
