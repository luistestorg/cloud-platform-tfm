#!/bin/bash

# Pulumi CLI script for AWS Infrastructure
# Delegates to eks-stacks/pulumi-cli.sh

STACK=""
ACTION="preview"
PREVIEW_FLAG=""
REFRESH_FLAG=""

function print_usage() {
  CMD="$1"
  ERROR_MSG="$2"

  if [ "$ERROR_MSG" != "" ]; then
    echo -e "\nERROR: $ERROR_MSG\n"
  fi

  echo -e "AWS Infrastructure deployment script\n"
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
    init|up|refresh|preview|destroy|rm)
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
      PREVIEW_FLAG="-p"
      shift
      ;;
    -r)
      REFRESH_FLAG="-r"
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

# Check if eks-stacks directory exists
if [ ! -d "eks-stacks" ]; then
  echo -e "\nERROR: eks-stacks directory not found!"
  exit 1
fi

cd eks-stacks

if [ ! -f "pulumi-cli.sh" ]; then
  echo -e "\nERROR: eks-stacks/pulumi-cli.sh not found!"
  exit 1
fi

chmod +x pulumi-cli.sh

echo -e "Delegating to eks-stacks deployment...\n"

# Build command with flags
CMD="./pulumi-cli.sh ${ACTION} -s ${STACK}"
[ -n "${PREVIEW_FLAG}" ] && CMD="${CMD} ${PREVIEW_FLAG}"
[ -n "${REFRESH_FLAG}" ] && CMD="${CMD} ${REFRESH_FLAG}"

eval $CMD

exit $?
