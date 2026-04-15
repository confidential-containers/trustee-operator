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

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

usage() {
    cat <<EOF
Usage: $0 [version] [options]

Builds and pushes trustee-operator image.

Arguments:
  version            Version to build (optional, defaults to VERSION from Makefile)

Options:
  --registry REGISTRY        Container registry (default: quay.io/confidential-containers)
  --skip-manifests           Skip generating manifests
  --skip-bundle              Skip generating bundle files
  --skip-docker-build        Skip building docker image
  --skip-docker-push         Skip pushing docker image
  --docker-buildx            Use docker buildx for multi-platform builds
  --platforms PLATFORMS      Platforms for buildx (default: linux/amd64,linux/arm64,linux/s390x)
  --dry-run                  Show what would be done without making changes
  -h, --help                 Show this help message

Examples:
  # Build and push using version from Makefile
  $0

  # Build and push specific version
  $0 0.18.0

  # Build and push to different registry
  $0 0.18.0 --registry myregistry.io/myorg

  # Build with multi-platform support
  $0 0.18.0 --docker-buildx --platforms linux/amd64,linux/arm64,linux/s390x

  # Dry run to see what would happen
  $0 0.18.0 --dry-run

  # Skip pushing (build only)
  $0 0.18.0 --skip-docker-push

Environment Variables:
  VERSION             Version to build (overridden by positional argument)
  REGISTRY            Container registry
  IMAGE_TAG_BASE      Base image name (default: \${REGISTRY}/trustee-operator)
  IMG                 Operator image (default: \${IMAGE_TAG_BASE}:v\${VERSION})

Steps performed:
  1. Generate manifests (make manifests)
  2. Generate bundle files (make bundle) - creates files for community-operators
  3. Build operator image (make docker-build or docker-buildx)
  4. Push operator image (make docker-push)

Note: Bundle/catalog images are not built. Community-operators uses bundle files only.
EOF
}

log_info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

log_step() {
    echo -e "${BLUE}==>${NC} $*"
}

run_cmd() {
    local description="$1"
    shift

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY RUN] Would run: $*"
    else
        log_info "${description}"
        "$@" || {
            log_error "Command failed: $*"
            return 1
        }
    fi
}

# Default values
REGISTRY="${REGISTRY:-quay.io/confidential-containers}"
VERSION=""
SKIP_MANIFESTS=false
SKIP_BUNDLE=false
SKIP_DOCKER_BUILD=false
SKIP_DOCKER_PUSH=false
USE_BUILDX=false
PLATFORMS="linux/amd64,linux/arm64,linux/s390x"
DRY_RUN=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --registry)
            REGISTRY="$2"
            shift 2
            ;;
        --skip-manifests)
            SKIP_MANIFESTS=true
            shift
            ;;
        --skip-bundle)
            SKIP_BUNDLE=true
            shift
            ;;
        --skip-docker-build)
            SKIP_DOCKER_BUILD=true
            shift
            ;;
        --skip-docker-push)
            SKIP_DOCKER_PUSH=true
            shift
            ;;
        --docker-buildx)
            USE_BUILDX=true
            shift
            ;;
        --platforms)
            PLATFORMS="$2"
            shift 2
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        -*)
            log_error "Unknown option: $1"
            usage
            exit 1
            ;;
        *)
            if [[ -z "${VERSION}" ]]; then
                VERSION="$1"
            else
                log_error "Unexpected argument: $1"
                usage
                exit 1
            fi
            shift
            ;;
    esac
done

cd "${ROOT_DIR}"

# Get version from Makefile if not provided
if [[ -z "${VERSION}" ]]; then
    VERSION=$(grep -oP '^VERSION \?= \K.*' "${ROOT_DIR}/Makefile" || echo "")
    if [[ -z "${VERSION}" ]]; then
        log_error "Could not determine version. Please provide version as argument or set VERSION in Makefile"
        exit 1
    fi
    log_info "Using version from Makefile: ${VERSION}"
fi

# Validate version format
if ! [[ "${VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    log_error "Invalid version format: ${VERSION}"
    log_error "Version must be in format X.Y.Z (e.g., 0.18.0)"
    exit 1
fi

# Set up image variables
IMAGE_TAG_BASE="${IMAGE_TAG_BASE:-${REGISTRY}/trustee-operator}"
IMG="${IMG:-${IMAGE_TAG_BASE}:v${VERSION}}"

if [[ "${DRY_RUN}" == "true" ]]; then
    log_warn "DRY RUN MODE - No changes will be made"
    echo ""
fi

log_info "================================================================"
log_info "Build and Push Configuration"
log_info "================================================================"
log_info "Version:         ${VERSION}"
log_info "Registry:        ${REGISTRY}"
log_info "Image Tag Base:  ${IMAGE_TAG_BASE}"
log_info ""
log_info "Image to build:"
log_info "  Operator:      ${IMG}"
log_info ""
if [[ "${USE_BUILDX}" == "true" ]]; then
    log_info "Build method:    docker buildx (platforms: ${PLATFORMS})"
else
    log_info "Build method:    docker build"
fi
log_info "================================================================"
echo ""

# Confirm with user unless dry run
if [[ "${DRY_RUN}" == "false" ]]; then
    read -p "Do you want to proceed? [y/N] " -n 1 -r
    echo ""
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "Aborted by user"
        exit 0
    fi
    echo ""
fi

# Use local Go toolchain to prevent version mismatch issues
log_info "Using local Go toolchain to prevent version mismatch issues..."
export GOTOOLCHAIN=local
echo ""

# Step 1: Generate manifests
if [[ "${SKIP_MANIFESTS}" == "false" ]]; then
    log_step "Step 1: Generating manifests"
    run_cmd "Running make manifests" make manifests
    echo ""
else
    log_warn "Skipping manifests generation (--skip-manifests)"
    echo ""
fi

# Step 2: Generate bundle
if [[ "${SKIP_BUNDLE}" == "false" ]]; then
    log_step "Step 2: Generating bundle"
    run_cmd "Running make bundle" make bundle IMG="${IMG}"

    # Clean up duplicate image entry in kustomization.yaml that kustomize adds
    if [[ "${DRY_RUN}" == "false" ]]; then
        if grep -q "name: quay.io/confidential-containers/trustee-operator" "${ROOT_DIR}/config/manager/kustomization.yaml" 2>/dev/null; then
            log_info "Removing duplicate image entry from config/manager/kustomization.yaml"
            sed -i '/^- name: quay\.io\/confidential-containers\/trustee-operator$/,/^  newTag:/d' "${ROOT_DIR}/config/manager/kustomization.yaml"
        fi
    fi
    echo ""
else
    log_warn "Skipping bundle generation (--skip-bundle)"
    echo ""
fi

# Step 3: Build docker image
if [[ "${SKIP_DOCKER_BUILD}" == "false" ]]; then
    log_step "Step 3: Building operator image"

    if [[ "${DRY_RUN}" == "false" ]]; then
        # Run tests before docker build
        log_info "Running tests before docker build..."
        make test || {
            log_error "Tests failed"
            exit 1
        }
        log_info "Tests passed"

        # Build docker image
        if [[ "${USE_BUILDX}" == "true" ]]; then
            log_info "Building with docker buildx..."
            make docker-buildx IMG="${IMG}" PLATFORMS="${PLATFORMS}" || {
                log_error "Docker buildx failed"
                exit 1
            }
        else
            log_info "Building docker image..."
            docker build -t "${IMG}" . || {
                log_error "Docker build failed"
                exit 1
            }
        fi
    else
        if [[ "${USE_BUILDX}" == "true" ]]; then
            log_info "[DRY RUN] Would run: make docker-buildx IMG=${IMG} PLATFORMS=${PLATFORMS}"
        else
            log_info "[DRY RUN] Would run: docker build -t ${IMG} ."
        fi
    fi
    echo ""
else
    log_warn "Skipping docker build (--skip-docker-build)"
    echo ""
fi

# Step 4: Push docker image
if [[ "${SKIP_DOCKER_PUSH}" == "false" ]]; then
    log_step "Step 4: Pushing operator image"
    if [[ "${DRY_RUN}" == "false" ]]; then
        if [[ "${USE_BUILDX}" == "true" ]]; then
            log_info "Image already pushed by docker-buildx"
        else
            log_info "Pushing docker image..."
            docker push "${IMG}" || {
                log_error "Docker push failed"
                exit 1
            }
        fi
    else
        if [[ "${USE_BUILDX}" == "true" ]]; then
            log_info "[DRY RUN] Image would be pushed by docker-buildx"
        else
            log_info "[DRY RUN] Would run: docker push ${IMG}"
        fi
    fi
    echo ""
else
    log_warn "Skipping docker push (--skip-docker-push)"
    echo ""
fi

# Summary
log_info "================================================================"
log_info "Build and Push Complete!"
log_info "================================================================"
log_info ""

if [[ "${DRY_RUN}" == "false" ]]; then
    if [[ "${SKIP_DOCKER_PUSH}" == "false" ]]; then
        log_info "Image built and pushed:"
        log_info "  ✓ ${IMG}"
    else
        log_info "Image built (not pushed):"
        log_info "    ${IMG}"
    fi
    echo ""
    log_info "Bundle files generated in: bundle/"
    log_info "  - manifests/"
    log_info "  - metadata/"
    log_info "  - tests/"
    echo ""
    log_info "Next steps:"
    log_info "  1. Verify operator image in registry: ${IMG}"
    log_info "  2. Test the operator deployment"
    log_info "  3. Create GitHub release"
    log_info "  4. Submit bundle files to community-operators"
else
    log_info "This was a dry run. No changes were made."
    log_info "Run without --dry-run to perform the actual build and push."
fi

echo ""
