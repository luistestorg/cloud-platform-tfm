#!/bin/bash

# Generic Pulumi CLI script for micro-stacks
# Can be used for: infra-kube, api, ci-support, mon-log

STACK=""
ACTION="preview"
PREVIEW="0"
REFRESH_BEFORE_UP="0"

# Detect micro-stack name from directory
MICROSTACK_NAME=$(basename "$(pwd)")

function print_usage() {
  CMD="$1"
  ERROR_MSG="$2"

  if [ "$ERROR_MSG" != "" ]; then
    echo -e "\nERROR: $ERROR_MSG\n"
  fi

  echo -e "Pulumi deployment script for ${MICROSTACK_NAME} micro-stack\n"
  echo -e "Usage: $CMD [ACTION] [OPTIONS] ... where OPTIONS include:\n"
  echo -e "  -s           Stack name; required"
  echo -e "  -p           Do a preview before running the specified action"
  echo -e "  -r           Do a refresh before running up"
  echo -e "\nSupported actions are: init, up, refresh, destroy, preview\n"
  echo -e "\nExample: $CMD up -s dev-aws\n"
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

# Verify stack configuration file exists
STACK_CONFIG_FILE="Pulumi.${STACK}.yaml"
if [ "${ACTION}" != "init" ] && [ ! -f "${STACK_CONFIG_FILE}" ]; then
  echo -e "\nERROR: Stack config file '${STACK_CONFIG_FILE}' not found!"
  echo -e "Run with 'init' action first or check your -s argument.\n"
  exit 1
fi

echo -e "\n=========================================="
echo -e "Micro-stack: ${MICROSTACK_NAME}"
echo -e "=========================================="
echo -e "Stack:  ${STACK}"
echo -e "Action: ${ACTION}"
echo -e "==========================================\n"

set -e

case "${ACTION}" in
  init)
    pulumi stack init "${STACK}" 2>/dev/null || pulumi stack select "${STACK}"
    echo -e "\n✅ Stack '${STACK}' initialized for ${MICROSTACK_NAME}"
    
    # Prompt for required configuration
    echo -e "\nPlease configure the stack using:"
    echo -e "  pulumi config set <key> <value>"
    ;;
    
  up)
    pulumi stack select "${STACK}"
    
    if [ "${REFRESH_BEFORE_UP}" == "1" ]; then
      echo "Refreshing stack state..."
      pulumi refresh --yes --skip-preview
    fi
    
    if [ "${PREVIEW}" == "0" ]; then
      pulumi up --yes --skip-preview
    else
      pulumi up
    fi
    
    echo -e "\n✅ Deployment completed for ${MICROSTACK_NAME}"
    ;;
    
  preview)
    pulumi stack select "${STACK}"
    pulumi preview
    ;;
    
  refresh)
    pulumi stack select "${STACK}"
    pulumi refresh --yes --skip-preview
    echo -e "\n✅ Refresh completed for ${MICROSTACK_NAME}"
    ;;
    
  destroy)
    pulumi stack select "${STACK}"
    
    echo -e "\n⚠️  WARNING: This will destroy all resources in ${MICROSTACK_NAME}!"
    echo -e "Type '${STACK}' to confirm:"
    read -r confirmation
    
    if [ "${confirmation}" != "${STACK}" ]; then
      echo -e "\nDestroy operation cancelled."
      exit 0
    fi
    
    pulumi destroy --yes
    echo -e "\n✅ Resources destroyed for ${MICROSTACK_NAME}"
    ;;
    
  *)
    print_usage "$0" "Unknown action: ${ACTION}"
    exit 1
    ;;
esac

exit 0
