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

# Source common functions
source "${SCRIPT_DIR}/common.sh"

usage() {
    cat <<EOF
Usage: $0 [version] [options]

Builds and pushes multi-platform trustee-operator images using docker buildx.

Arguments:
  version            Version to build (optional, defaults to VERSION from Makefile)

Options:
  --registry REGISTRY        Container registry (default: quay.io/confidential-containers)
  --skip-docker-push         Skip pushing docker image (single-platform build only)
  --dry-run                  Show what would be done without making changes
  -h, --help                 Show this help message

Examples:
  # Build and push using version from Makefile
  $0

  # Build and push specific version
  $0 0.18.0

  # Build and push to different registry
  $0 0.18.0 --registry myregistry.io/myorg

  # Dry run to see what would happen
  $0 0.18.0 --dry-run

  # Build single-platform locally without pushing (loads to local Docker)
  $0 0.18.0 --skip-docker-push

Steps performed:
  1. Run tests
  2. Generate manifests (make manifests)
  3. Generate bundle files (make bundle)
  4. Build multi-platform operator image (linux/amd64, linux/arm64, linux/s390x)
  5. Push operator image to registry

Note:
  - Multi-platform builds require push to registry (cannot load locally)
  - Use --skip-docker-push for single-platform local builds (linux/amd64 only)
  - Bundle/catalog images are not built (community-operators uses bundle files only)
EOF
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
REGISTRY="quay.io/confidential-containers"
VERSION=""
SKIP_DOCKER_PUSH=false
PLATFORMS="linux/amd64,linux/arm64,linux/s390x"
DRY_RUN=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --registry)
            if [[ $# -lt 2 ]] || [[ -z "${2:-}" ]]; then
                log_error "Missing value for --registry"
                usage
                exit 1
            fi
            REGISTRY="$2"
            shift 2
            ;;
        --skip-docker-push)
            SKIP_DOCKER_PUSH=true
            shift
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
    VERSION=$(get_version_from_makefile) || exit 1
    log_info "Using version from Makefile: ${VERSION}"
fi

# Validate version format
validate_version "${VERSION}" || exit 1

# Set up image variables
IMAGE_TAG_BASE="${REGISTRY}/trustee-operator"
IMG="${IMAGE_TAG_BASE}:v${VERSION}"

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
if [[ "${SKIP_DOCKER_PUSH}" == "true" ]]; then
    log_info "Build method:    docker buildx (single-platform: linux/amd64, local only)"
else
    log_info "Build method:    docker buildx (multi-platform: ${PLATFORMS})"
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

# Step 1: Run tests
log_step "Step 1: Running tests"
if [[ "${DRY_RUN}" == "true" ]]; then
    log_info "[DRY RUN] Would run: make test"
else
    log_info "Running tests..."
    make test || {
        log_error "Tests failed"
        exit 1
    }
    log_info "Tests passed"
fi
echo ""

# Step 2: Generate manifests
log_step "Step 2: Generating manifests"
run_cmd "Running make manifests" make manifests
echo ""

# Step 3: Generate bundle
log_step "Step 3: Generating bundle"
run_cmd "Running make bundle" make bundle VERSION="${VERSION}" IMG="${IMG}"

# Clean up duplicate image entry in kustomization.yaml that kustomize adds
if [[ "${DRY_RUN}" == "false" ]]; then
    remove_duplicate_kustomization_images "${ROOT_DIR}"
fi
echo ""

# Step 4: Build docker image
log_step "Step 4: Building operator image"

if [[ "${DRY_RUN}" == "false" ]]; then
    if [[ "${SKIP_DOCKER_PUSH}" == "true" ]]; then
        # Single platform buildx without push - use --load to load into local docker
        log_info "Building single-platform image with docker buildx (--load, no push)..."
        log_info "Platform: linux/amd64"

        # Use unique builder name to avoid conflicts with user's existing builders
        BUILDER_NAME="trustee-operator-builder-$$"
        CREATED_BUILDER=false

        # Create buildx builder (use unique name to avoid conflicts)
        if docker buildx create --name "${BUILDER_NAME}" 2>/dev/null; then
            CREATED_BUILDER=true
            log_info "Created temporary builder: ${BUILDER_NAME}"
        else
            # Create failed - verify the builder actually exists before proceeding
            if docker buildx inspect "${BUILDER_NAME}" &>/dev/null; then
                log_warn "Builder ${BUILDER_NAME} already exists, reusing it"
            else
                log_error "Failed to create buildx builder: ${BUILDER_NAME}"
                log_error ""
                log_error "This may indicate:"
                log_error "  - Docker is not running"
                log_error "  - docker buildx is not available (update Docker to a recent version)"
                log_error "  - Insufficient permissions"
                log_error ""
                log_error "Verify with: docker buildx version"
                exit 1
            fi
        fi

        # Build with --load instead of --push (single platform only)
        docker buildx build --builder "${BUILDER_NAME}" --load --platform="linux/amd64" --tag "${IMG}" . || {
            log_error "Docker buildx failed"
            # Only remove builder if we created it
            if [[ "${CREATED_BUILDER}" == "true" ]]; then
                docker buildx rm "${BUILDER_NAME}" 2>/dev/null || true
            fi
            exit 1
        }

        # Clean up builder only if we created it
        if [[ "${CREATED_BUILDER}" == "true" ]]; then
            docker buildx rm "${BUILDER_NAME}" 2>/dev/null || true
            log_info "Removed temporary builder: ${BUILDER_NAME}"
        fi

        log_info "Image loaded into local Docker (not pushed to registry)"
    else
        # Build and push with buildx (via Makefile) - multi-platform
        log_info "Building and pushing multi-platform images with docker buildx..."
        make docker-buildx IMG="${IMG}" PLATFORMS="${PLATFORMS}" || {
            log_error "Docker buildx failed"
            exit 1
        }
    fi
else
    if [[ "${SKIP_DOCKER_PUSH}" == "true" ]]; then
        log_info "[DRY RUN] Would run: docker buildx build --builder <temp-builder> --load --platform=linux/amd64 --tag ${IMG} ."
    else
        log_info "[DRY RUN] Would run: make docker-buildx IMG=${IMG} PLATFORMS=${PLATFORMS}"
    fi
fi
echo ""

# Step 5: Push docker image
if [[ "${SKIP_DOCKER_PUSH}" == "false" ]]; then
    log_step "Step 5: Pushing operator image"
    if [[ "${DRY_RUN}" == "false" ]]; then
        # Buildx with multiple platforms already pushed in step 4
        log_info "Multi-platform images already pushed by docker-buildx in step 4"
    else
        log_info "[DRY RUN] Images would be pushed by docker-buildx in step 4"
    fi
    echo ""
else
    log_step "Step 5: Push docker image"
    log_warn "Skipping docker push (--skip-docker-push)"
    if [[ "${DRY_RUN}" == "false" ]]; then
        log_info "Single-platform image built with buildx --load (loaded into local Docker only)"
    fi
    echo ""
fi

# Summary
log_info "================================================================"
log_info "Build and Push Complete!"
log_info "================================================================"
log_info ""

if [[ "${DRY_RUN}" == "false" ]]; then
    if [[ "${SKIP_DOCKER_PUSH}" == "false" ]]; then
        log_info "Multi-platform images built and pushed:"
        log_info "  ✓ ${IMG}"
        log_info "  Platforms: ${PLATFORMS}"
    else
        log_info "Single-platform image built (not pushed):"
        log_info "    ${IMG}"
        log_info "  Platform: linux/amd64 (loaded to local Docker)"
    fi
    echo ""
    log_info "Bundle files generated in: bundle/"
    log_info "  - manifests/"
    log_info "  - metadata/"
    log_info "  - tests/"
    echo ""
    if [[ "${SKIP_DOCKER_PUSH}" == "false" ]]; then
        log_info "Next steps:"
        log_info "  1. Verify operator image in registry: ${IMG}"
        log_info "  2. Test the operator deployment"
        log_info "  3. Create GitHub release"
        log_info "  4. Submit bundle files to community-operators"
    else
        log_info "Next steps:"
        log_info "  1. Test the operator locally: docker run ${IMG}"
        log_info "  2. Push to registry when ready: docker push ${IMG}"
    fi
else
    log_info "This was a dry run. No changes were made."
    log_info "Run without --dry-run to perform the actual build and push."
fi

echo ""
