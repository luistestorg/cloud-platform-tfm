#!/bin/bash

# Pulumi CLI script for GCP Infrastructure
# Deploys all GCP micro-stacks

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

  echo -e "GCP Infrastructure deployment script\n"
  echo -e "Usage: $CMD [ACTION] [OPTIONS] ... where OPTIONS include:\n"
  echo -e "  -s           Stack name; required"
  echo -e "  -p           Do a preview before running the specified action"
  echo -e "  -r           Do a refresh before running up"
  echo -e "\nSupported actions are: init, up, refresh, destroy, preview\n"
  echo -e "\nExample: $CMD up -s dev-gcp\n"
}

function deploy_gcp_stack() {
  local stack_dir="$1"
  local stack_name="$2"
  
  echo -e "\n=========================================="
  echo -e "Deploying: ${stack_name}"
  echo -e "==========================================\n"
  
  if [ ! -d "${stack_dir}" ]; then
    echo -e "WARNING: ${stack_dir} not found, skipping..."
    return 0
  fi
  
  cd "${stack_dir}"
  
  pulumi stack select "${STACK}" 2>/dev/null || pulumi stack init "${STACK}"
  
  case "${ACTION}" in
    init)
      echo "Stack initialized: ${STACK}"
      ;;
    up)
      if [ "${REFRESH_BEFORE_UP}" == "1" ]; then
        pulumi refresh --yes --skip-preview
      fi
      
      if [ "${PREVIEW}" == "0" ]; then
        pulumi up --yes --skip-preview
      else
        pulumi up
      fi
      ;;
    preview)
      pulumi preview
      ;;
    refresh)
      pulumi refresh --yes --skip-preview
      ;;
    destroy)
      pulumi destroy --yes
      ;;
  esac
  
  local exit_code=$?
  cd - > /dev/null
  return $exit_code
}

# Parse arguments
if [ $# -eq 0 ]; then
  print_usage "$0"
  exit 1
fi

while true; do
  case "$1" in
    init|up|refresh|preview|destroy)
      ACTION="$1"
      shift
      ;;
    -s)
      if [[ -z "$2" || "${2:0:1}" == "-" ]]; then
        print_usage "$0" "Missing value for the -s parameter!"
        exit 1
      fi
      STACK="$2"
      shift 2
      ;;
    -p)
      PREVIEW="1"
      shift
      ;;
    -r)
      REFRESH_BEFORE_UP="1"
      shift
      ;;
    -help|-usage|--help|--usage)
      print_usage "$0"
      exit 0
      ;;
    --)
      shift
      break
      ;;
    *)
      if [ "$1" != "" ]; then
        print_usage "$0" "Unrecognized argument: $1!"
        exit 1
      else
        break
      fi
      ;;
  esac
done

if [ -z "${STACK}" ]; then
  print_usage "$0" "Must provide the Pulumi stack name using the -s option!"
  exit 1
fi

# Check prerequisites
if ! command -v pulumi &> /dev/null; then
  echo -e "\nERROR: 'pulumi' is required but not installed!"
  exit 1
fi

if ! command -v gcloud &> /dev/null; then
  echo -e "\nERROR: 'gcloud' CLI is required but not installed!"
  exit 1
fi

BASE_DIR=$(pwd)

echo -e "\n=========================================="
echo -e "GCP Infrastructure Deployment"
echo -e "=========================================="
echo -e "Stack:  ${STACK}"
echo -e "Action: ${ACTION}"
echo -e "==========================================\n"

set -e

# Deploy GCP stacks in order
GCP_STACKS=(
  "gke-network-stack:GKE Network"
  "gke-infra-stack:GKE Infrastructure"
  "gke-infra-kube-stack:GKE Kubernetes Infrastructure"
  "gke-mon-log-stack:GKE Monitoring & Logging"
  "gke-ci-support-stack:GKE CI/CD Support"
  "gke-api-stack:GKE Self-Service API"
  "gke-tfm-shared-stack:GKE TFM Shared Resources"
)

for stack_spec in "${GCP_STACKS[@]}"; do
  IFS=':' read -r stack_dir stack_name <<< "$stack_spec"
  deploy_gcp_stack "${BASE_DIR}/${stack_dir}" "${stack_name}"
  
  if [ $? -ne 0 ]; then
    echo -e "\n❌ Deployment failed at: ${stack_name}"
    exit 1
  fi
done

echo -e "\n✅ All GCP stacks deployed successfully!"

exit 0
