#!/usr/bin/env bash

# Copyright Confidential Containers Contributors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Simple wrapper script for building and pushing multi-platform images
# Builds for linux/amd64, linux/arm64, and linux/s390x by default

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Portable in-place sed that works on both GNU and BSD
# Uses extended regex mode (-E) for better portability
sed_inplace() {
    local pattern="$1"
    local file="$2"
    local tmpfile="${file}.tmp.$$"

    # Verify source file exists and is readable
    if [[ ! -f "$file" ]] || [[ ! -r "$file" ]]; then
        echo "Error: Cannot read file: $file" >&2
        return 1
    fi

    # Run sed and explicitly check exit status
    if ! sed -E "$pattern" "$file" > "$tmpfile"; then
        # sed failed - clean up temp file and abort
        rm -f "$tmpfile"
        echo "Error: sed failed on file: $file" >&2
        return 1
    fi

    # Move temp file to replace original
    if ! mv "$tmpfile" "$file"; then
        # mv failed - clean up temp file and abort
        rm -f "$tmpfile"
        echo "Error: Failed to replace file: $file" >&2
        return 1
    fi

    return 0
}

usage() {
    cat <<EOF
Usage: $0 [version] [options]

Builds and pushes multi-platform operator image.

Arguments:
  version            Version to build (optional, defaults to VERSION from Makefile)

Options:
  --registry REGISTRY    Container registry (default: quay.io/confidential-containers)
  --platforms PLATFORMS  Platforms to build (default: linux/amd64,linux/arm64,linux/s390x)
  -h, --help             Show this help message

Examples:
  # Build using version from Makefile
  $0

  # Build specific version
  $0 0.18.0

  # Build with custom registry
  $0 0.18.0 --registry myregistry.io/myorg

  # Build for specific platforms
  $0 0.18.0 --platforms linux/amd64,linux/arm64
EOF
}

# Default values
VERSION=""
REGISTRY="quay.io/confidential-containers"
PLATFORMS="linux/amd64,linux/arm64,linux/s390x"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --registry)
            if [[ $# -lt 2 ]] || [[ -z "${2:-}" ]]; then
                echo "Error: Missing value for --registry"
                usage
                exit 1
            fi
            REGISTRY="$2"
            shift 2
            ;;
        --platforms)
            if [[ $# -lt 2 ]] || [[ -z "${2:-}" ]]; then
                echo "Error: Missing value for --platforms"
                usage
                exit 1
            fi
            PLATFORMS="$2"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        -*)
            echo "Error: Unknown option: $1"
            usage
            exit 1
            ;;
        *)
            if [[ -z "${VERSION}" ]]; then
                VERSION="$1"
            else
                echo "Error: Unexpected argument: $1"
                usage
                exit 1
            fi
            shift
            ;;
    esac
done

# Get version from Makefile if not provided
if [[ -z "${VERSION}" ]]; then
    VERSION=$(awk '/^VERSION \?=/ {print $3}' "${ROOT_DIR}/Makefile" || echo "")
    if [[ -z "${VERSION}" ]]; then
        echo "Error: Could not determine version. Please provide version as argument or set VERSION in Makefile"
        exit 1
    fi
    echo "Using version from Makefile: ${VERSION}"
fi

# Validate version format
if ! [[ "${VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: Invalid version format: ${VERSION}"
    echo "Version must be in format X.Y.Z (e.g., 0.18.0)"
    exit 1
fi

# Construct image names
IMAGE_TAG_BASE="${REGISTRY}/trustee-operator"
IMG="${IMAGE_TAG_BASE}:v${VERSION}"

# Escape IMAGE_TAG_BASE for use in regex patterns (escape . and /)
ESCAPED_IMAGE_TAG_BASE=$(echo "${IMAGE_TAG_BASE}" | sed 's|[./]|\\&|g')

echo "Building and pushing multi-platform operator image for version ${VERSION}"
echo "  IMG:        ${IMG}"
echo "  Platforms:  ${PLATFORMS}"
echo ""

cd "${ROOT_DIR}"

# Use local Go toolchain to prevent version mismatch issues
echo "Using local Go toolchain..."
export GOTOOLCHAIN=local
echo ""

# Build and push operator image with bundle
set -x

# Generate manifests and bundle
# Pass both VERSION and IMG to ensure bundle metadata matches the image tag
VERSION="${VERSION}" IMG="${IMG}" make manifests bundle

# Clean up duplicate image entry in kustomization.yaml that kustomize adds
# Keep only the 'controller' image entry, remove duplicates
# Use grep -F for fixed-string matching to avoid treating dots in registry as regex wildcards
if grep -Fq "name: ${IMAGE_TAG_BASE}" config/manager/kustomization.yaml 2>/dev/null; then
    echo "Removing duplicate image entry from config/manager/kustomization.yaml..."
    # Remove the duplicate entry (lines with the full image name)
    # Use [[:space:]]* to match leading whitespace (YAML is indented)
    sed_inplace "/^[[:space:]]*- name: ${ESCAPED_IMAGE_TAG_BASE}\$/,/^[[:space:]]*newTag:/d" config/manager/kustomization.yaml
fi

# Run tests
echo "Running tests..."
make test || {
    echo "ERROR: Tests failed"
    exit 1
}
echo "Tests passed"

# Build and push multi-platform docker images
echo "Building multi-platform operator image..."
make docker-buildx IMG="${IMG}" PLATFORMS="${PLATFORMS}" || {
    echo "ERROR: Docker buildx failed"
    exit 1
}

echo ""
echo "Build and push complete!"
echo "  Operator: ${IMG} (${PLATFORMS})"
echo ""
echo "Note: Bundle/catalog images are not needed for community-operators submission."
echo "      Only bundle files (in bundle/ directory) are submitted."
