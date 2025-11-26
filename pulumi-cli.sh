#!/bin/bash

# Master Pulumi CLI script for cloud-platform-tfm
# Orchestrates deployment of all micro-stacks

STACK=""
ACTION="preview"
PREVIEW="0"
REFRESH_BEFORE_UP="0"
CLOUD_PROVIDER=""
STACKS_TO_DEPLOY="all"

function print_usage() {
  CMD="$1"
  ERROR_MSG="$2"

  if [ "$ERROR_MSG" != "" ]; then
    echo -e "\nERROR: $ERROR_MSG\n"
  fi

  echo -e "Master deployment script for cloud-platform-tfm Pulumi micro-stacks\n"
  echo -e "Usage: $CMD [ACTION] [OPTIONS] ... where OPTIONS include:\n"
  echo -e "  -s           Stack name; required"
  echo -e "  -c           Cloud provider: aws|gcp (required)"
  echo -e "  -m           Micro-stacks to deploy: all|infra|kube|api|ci|mon (default: all)"
  echo -e "  -p           Do a preview before running the specified action"
  echo -e "  -r           Do a refresh before running up"
  echo -e "\nSupported actions are: init, up, refresh, destroy, preview\n"
  echo -e "\nExamples:"
  echo -e "  Deploy everything on AWS:"
  echo -e "    $CMD up -s dev-aws -c aws\n"
  echo -e "  Deploy only infrastructure on GCP:"
  echo -e "    $CMD up -s prod-gcp -c gcp -m infra\n"
  echo -e "  Preview full deployment:"
  echo -e "    $CMD preview -s dev-aws -c aws -m all\n"
}

function validate_cloud_provider() {
  if [[ "${CLOUD_PROVIDER}" != "aws" && "${CLOUD_PROVIDER}" != "gcp" ]]; then
    print_usage "$0" "Invalid cloud provider: ${CLOUD_PROVIDER}. Must be 'aws' or 'gcp'"
    exit 1
  fi
}

function get_infra_dir() {
  if [ "${CLOUD_PROVIDER}" == "aws" ]; then
    echo "infra-aws"
  else
    echo "infra-gcp"
  fi
}

function deploy_micro_stack() {
  local stack_dir="$1"
  local stack_name="$2"
  
  echo -e "\n=========================================="
  echo -e "Deploying micro-stack: ${stack_name}"
  echo -e "==========================================\n"
  
  if [ ! -d "${stack_dir}" ]; then
    echo -e "WARNING: Directory ${stack_dir} not found, skipping..."
    return 0
  fi
  
  cd "${stack_dir}"
  
  if [ -f "pulumi-cli.sh" ]; then
    chmod +x pulumi-cli.sh
    
    case "${ACTION}" in
      init)
        ./pulumi-cli.sh init -s "${STACK}"
        ;;
      up)
        if [ "${PREVIEW}" == "1" ]; then
          ./pulumi-cli.sh up -s "${STACK}" -p
        else
          if [ "${REFRESH_BEFORE_UP}" == "1" ]; then
            ./pulumi-cli.sh up -s "${STACK}" -r
          else
            ./pulumi-cli.sh up -s "${STACK}"
          fi
        fi
        ;;
      preview)
        ./pulumi-cli.sh preview -s "${STACK}"
        ;;
      refresh)
        ./pulumi-cli.sh refresh -s "${STACK}"
        ;;
      destroy)
        ./pulumi-cli.sh destroy -s "${STACK}"
        ;;
    esac
    
    local exit_code=$?
    cd - > /dev/null
    
    if [ $exit_code -ne 0 ]; then
      echo -e "\nERROR: Failed to deploy ${stack_name}"
      return $exit_code
    fi
  else
    echo -e "WARNING: No pulumi-cli.sh found in ${stack_dir}, skipping..."
    cd - > /dev/null
  fi
  
  return 0
}

function deploy_all_stacks() {
  local base_dir=$(pwd)
  local infra_dir=$(get_infra_dir)
  
  # Order matters! Infrastructure first, then Kubernetes components, then applications
  local deployment_order=(
    "${infra_dir}:Infrastructure"
    "infra-kube:Kubernetes Infrastructure"
    "api:Self-Service API"
    "ci-support:CI/CD Support"
    "mon-log:Monitoring & Logging"
  )
  
  for stack_spec in "${deployment_order[@]}"; do
    IFS=':' read -r stack_dir stack_name <<< "$stack_spec"
    deploy_micro_stack "${base_dir}/${stack_dir}" "${stack_name}"
    
    if [ $? -ne 0 ]; then
      echo -e "\n❌ Deployment failed at: ${stack_name}"
      exit 1
    fi
  done
  
  echo -e "\n✅ All micro-stacks deployed successfully!"
}

function deploy_selected_stacks() {
  local base_dir=$(pwd)
  
  case "${STACKS_TO_DEPLOY}" in
    infra)
      deploy_micro_stack "${base_dir}/$(get_infra_dir)" "Infrastructure"
      ;;
    kube)
      deploy_micro_stack "${base_dir}/infra-kube" "Kubernetes Infrastructure"
      ;;
    api)
      deploy_micro_stack "${base_dir}/api" "Self-Service API"
      ;;
    ci)
      deploy_micro_stack "${base_dir}/ci-support" "CI/CD Support"
      ;;
    mon)
      deploy_micro_stack "${base_dir}/mon-log" "Monitoring & Logging"
      ;;
    all)
      deploy_all_stacks
      ;;
    *)
      print_usage "$0" "Invalid micro-stack selection: ${STACKS_TO_DEPLOY}"
      exit 1
      ;;
  esac
}

# Parse command line arguments
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
    -c)
      if [[ -z "$2" || "${2:0:1}" == "-" ]]; then
        print_usage "$0" "Missing value for the -c parameter!"
        exit 1
      fi
      CLOUD_PROVIDER="$2"
      shift 2
      ;;
    -m)
      if [[ -z "$2" || "${2:0:1}" == "-" ]]; then
        print_usage "$0" "Missing value for the -m parameter!"
        exit 1
      fi
      STACKS_TO_DEPLOY="$2"
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
        print_usage "$0" "Unrecognized or misplaced argument: $1!"
        exit 1
      else
        break
      fi
      ;;
  esac
done

# Validate required parameters
if [ -z "${STACK}" ]; then
  print_usage "$0" "Must provide the Pulumi stack name using the -s option!"
  exit 1
fi

if [ -z "${CLOUD_PROVIDER}" ]; then
  print_usage "$0" "Must provide the cloud provider using the -c option!"
  exit 1
fi

validate_cloud_provider

# Check prerequisites
for cmd in pulumi kubectl; do
  if ! command -v $cmd &> /dev/null; then
    echo -e "\nERROR: '$cmd' is required but not installed!"
    exit 1
  fi
done

# Check cloud-specific CLI
if [ "${CLOUD_PROVIDER}" == "aws" ]; then
  if ! command -v aws &> /dev/null; then
    echo -e "\nERROR: 'aws' CLI is required for AWS deployments!"
    exit 1
  fi
elif [ "${CLOUD_PROVIDER}" == "gcp" ]; then
  if ! command -v gcloud &> /dev/null; then
    echo -e "\nERROR: 'gcloud' CLI is required for GCP deployments!"
    exit 1
  fi
fi

echo -e "\n=========================================="
echo -e "Cloud Platform TFM - Pulumi Deployment"
echo -e "=========================================="
echo -e "Stack:          ${STACK}"
echo -e "Cloud:          ${CLOUD_PROVIDER}"
echo -e "Action:         ${ACTION}"
echo -e "Micro-stacks:   ${STACKS_TO_DEPLOY}"
echo -e "==========================================\n"

set -e

# Execute deployment
deploy_selected_stacks

exit 0
