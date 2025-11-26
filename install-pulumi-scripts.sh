#!/bin/bash

# Script to install pulumi-cli.sh scripts in the correct locations
# for the cloud-platform-tfm project

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="${SCRIPT_DIR}"

echo "========================================"
echo "Installing Pulumi CLI Scripts"
echo "========================================"
echo ""

function install_script() {
  local src="$1"
  local dest="$2"
  local desc="$3"
  
  echo "Installing: $desc"
  echo "  Source: $src"
  echo "  Dest:   $dest"
  
  if [ ! -f "$src" ]; then
    echo "  ❌ ERROR: Source file not found!"
    return 1
  fi
  
  # Create destination directory if needed
  dest_dir=$(dirname "$dest")
  mkdir -p "$dest_dir"
  
  # Copy script
  cp "$src" "$dest"
  chmod +x "$dest"
  
  echo "  ✅ Installed successfully"
  echo ""
  
  return 0
}

# Check if we're in the right directory
if [ ! -d "eks-stacks" ] || [ ! -d "gcp-stacks" ]; then
  echo "ERROR: This script must be run from the cloud-platform-tfm root directory!"
  echo "Current directory: $(pwd)"
  echo "Expected directories: eks-stacks/ and gcp-stacks/"
  exit 1
fi

echo "Project root: $PROJECT_ROOT"
echo ""

# Install master script
install_script \
  "${PROJECT_ROOT}/pulumi-cli-root.sh" \
  "${PROJECT_ROOT}/pulumi-cli.sh" \
  "Master orchestration script"

# Note: AWS and GCP stacks already have their own pulumi-cli.sh scripts
# We'll create wrapper scripts for the micro-stacks inside each

# Install generic script for AWS micro-stacks
AWS_MICRO_STACKS=("infra-aws" "infra-kube" "api" "ci-support" "mon-log")

for stack in "${AWS_MICRO_STACKS[@]}"; do
  if [ -d "${PROJECT_ROOT}/eks-stacks/${stack}" ]; then
    install_script \
      "${PROJECT_ROOT}/pulumi-cli-generic.sh" \
      "${PROJECT_ROOT}/eks-stacks/${stack}/pulumi-cli.sh" \
      "EKS ${stack} micro-stack"
  else
    echo "⚠️  WARNING: Directory eks-stacks/${stack} not found, skipping..."
    echo ""
  fi
done

# Install generic script for GCP micro-stacks
GCP_MICRO_STACKS=("infra-gcp" "infra-kube" "api" "ci-support" "mon-log")

for stack in "${GCP_MICRO_STACKS[@]}"; do
  if [ -d "${PROJECT_ROOT}/gcp-stacks/${stack}" ]; then
    install_script \
      "${PROJECT_ROOT}/pulumi-cli-generic.sh" \
      "${PROJECT_ROOT}/gcp-stacks/${stack}/pulumi-cli.sh" \
      "GCP ${stack} micro-stack"
  else
    echo "⚠️  WARNING: Directory gcp-stacks/${stack} not found, skipping..."
    echo ""
  fi
done

# Copy README
if [ -f "${PROJECT_ROOT}/PULUMI_CLI_SCRIPTS_README.md" ]; then
  cp "${PROJECT_ROOT}/PULUMI_CLI_SCRIPTS_README.md" \
     "${PROJECT_ROOT}/docs/PULUMI_CLI_SCRIPTS.md"
  echo "📚 Documentation copied to docs/PULUMI_CLI_SCRIPTS.md"
  echo ""
fi

echo "========================================"
echo "✅ Installation Complete!"
echo "========================================"
echo ""
echo "Scripts installed at:"
echo "  • ./pulumi-cli.sh (master)"
echo "  • ./eks-stacks/pulumi-cli.sh (existing, not modified)"
echo "  • ./gcp-stacks/pulumi-cli.sh (existing, not modified)"

for stack in "${AWS_MICRO_STACKS[@]}"; do
  if [ -d "${PROJECT_ROOT}/eks-stacks/${stack}" ]; then
    echo "  • ./eks-stacks/${stack}/pulumi-cli.sh"
  fi
done

for stack in "${GCP_MICRO_STACKS[@]}"; do
  if [ -d "${PROJECT_ROOT}/gcp-stacks/${stack}" ]; then
    echo "  • ./gcp-stacks/${stack}/pulumi-cli.sh"
  fi
done

echo ""
echo "Usage examples:"
echo "  # Deploy everything on AWS:"
echo "  ./pulumi-cli.sh up -s dev-aws -c aws"
echo ""
echo "  # Deploy only Kubernetes infrastructure:"
echo "  ./pulumi-cli.sh up -s dev-aws -c aws -m kube"
echo ""
echo "  # Deploy a specific micro-stack:"
echo "  cd eks-stacks/mon-log && ./pulumi-cli.sh up -s dev-aws"
echo ""
echo "  # Use existing EKS script (unchanged):"
echo "  cd eks-stacks && ./pulumi-cli.sh up -s dev-aws"
echo ""
echo "See docs/PULUMI_CLI_SCRIPTS.md for complete documentation."
echo ""

exit 0
