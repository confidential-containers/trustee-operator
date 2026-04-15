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
# Builds for linux/amd64 and linux/arm64 by default

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Get VERSION from Makefile or argument
VERSION="${1:-$(grep -oP '^VERSION \?= \K.*' "${ROOT_DIR}/Makefile")}"

# Registry and image names
REGISTRY="${REGISTRY:-quay.io/confidential-containers}"
IMAGE_TAG_BASE="${IMAGE_TAG_BASE:-${REGISTRY}/trustee-operator}"
IMG="${IMG:-${IMAGE_TAG_BASE}:v${VERSION}}"
PLATFORMS="${PLATFORMS:-linux/amd64,linux/arm64,linux/s390x}"

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
IMG="${IMG}" make manifests bundle

# Clean up duplicate image entry in kustomization.yaml that kustomize adds
# Keep only the 'controller' image entry, remove duplicates
if grep -q "name: quay.io/confidential-containers/trustee-operator" config/manager/kustomization.yaml; then
    echo "Removing duplicate image entry from config/manager/kustomization.yaml..."
    # Remove the duplicate entry (lines with the full image name)
    sed -i '/^- name: quay\.io\/confidential-containers\/trustee-operator$/,/^  newTag:/d' config/manager/kustomization.yaml
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
