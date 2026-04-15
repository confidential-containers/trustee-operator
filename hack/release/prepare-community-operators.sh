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

# Disable git pager for better script automation
export GIT_PAGER=cat

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

usage() {
    cat <<EOF
Usage: $0 [version] --fork FORK [options]

Prepares trustee-operator bundle for submission to community-operators catalog.
Clones the upstream community-operators repo, adds your fork as remote, copies bundle, and prepares for PR.

Arguments:
  version            Release version (e.g., 0.18.0) - defaults to VERSION from Makefile

Required Options:
  --fork FORK        Your fork URL to push to (e.g., git@github.com:username/community-operators.git)

Optional Options:
  --upstream REPO    Upstream repository URL (default: git@github.com:k8s-operatorhub/community-operators.git)
  --branch BRANCH    Branch name to create (default: trustee-operator-v\${VERSION})
  --catalog TYPE     Catalog type: community or upstream (default: community)
  --skip-clone       Skip cloning, use existing repo in --work-dir
  --work-dir DIR     Working directory (default: /tmp/community-operators-\${VERSION})
  --skip-commit      Don't create git commit
  --dry-run          Show what would be done without making changes
  -h, --help         Show this help message

Examples:
  # Prepare bundle with your fork
  $0 0.18.0 --fork git@github.com:lmilleri/community-operators.git

  # Use different fork
  $0 0.18.0 --fork git@github.com:myuser/community-operators.git

  # Use existing cloned directory
  $0 0.18.0 --fork git@github.com:myuser/community-operators.git \\
    --skip-clone --work-dir /tmp/my-operators

  # Dry run to see what would happen
  $0 0.18.0 --fork git@github.com:myuser/community-operators.git --dry-run

After running this script:
  1. Review the changes in the working directory
  2. Test the operator bundle
  3. Push the branch: cd <work-dir> && git push fork <branch>
  4. Create a PR from your fork to upstream on GitHub
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

# Default values
VERSION=""
FORK_REPO=""  # REQUIRED - must be provided via --fork
UPSTREAM_REPO="git@github.com:k8s-operatorhub/community-operators.git"
BRANCH=""
CATALOG="community"
WORK_DIR=""
SKIP_CLONE=false
SKIP_COMMIT=false
DRY_RUN=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --fork)
            FORK_REPO="$2"
            shift 2
            ;;
        --upstream)
            UPSTREAM_REPO="$2"
            shift 2
            ;;
        --branch)
            BRANCH="$2"
            shift 2
            ;;
        --catalog)
            CATALOG="$2"
            if [[ "${CATALOG}" != "community" && "${CATALOG}" != "upstream" ]]; then
                log_error "Invalid catalog type: ${CATALOG}. Must be 'community' or 'upstream'"
                exit 1
            fi
            shift 2
            ;;
        --work-dir)
            WORK_DIR="$2"
            shift 2
            ;;
        --skip-clone)
            SKIP_CLONE=true
            shift
            ;;
        --skip-commit)
            SKIP_COMMIT=true
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

# Validate required --fork parameter
if [[ -z "${FORK_REPO}" ]]; then
    log_error "Missing required parameter: --fork"
    log_error "You must specify your fork URL"
    log_error "Example: $0 0.18.0 --fork git@github.com:yourusername/community-operators.git"
    exit 1
fi

# Set defaults
if [[ -z "${BRANCH}" ]]; then
    BRANCH="trustee-operator-v${VERSION}"
fi

if [[ -z "${WORK_DIR}" ]]; then
    WORK_DIR="/tmp/community-operators-${VERSION}"
fi

# Set catalog path based on type
if [[ "${CATALOG}" == "community" ]]; then
    CATALOG_PATH="operators"
else
    CATALOG_PATH="operators"  # Same for now, but can be different for upstream
fi

TARGET_RELEASE="${VERSION}"
DEST_DIR="${WORK_DIR}/${CATALOG_PATH}/trustee-operator/${TARGET_RELEASE}"

if [[ "${DRY_RUN}" == "true" ]]; then
    log_warn "DRY RUN MODE - No changes will be made"
    echo ""
fi

log_info "================================================================"
log_info "Community Operators Bundle Preparation"
log_info "================================================================"
log_info "Version:        ${VERSION}"
log_info "Upstream:       ${UPSTREAM_REPO}"
log_info "Fork:           ${FORK_REPO}"
log_info "Branch:         ${BRANCH}"
log_info "Catalog:        ${CATALOG}"
log_info "Work Directory: ${WORK_DIR}"
log_info "Target Path:    ${CATALOG_PATH}/trustee-operator/${TARGET_RELEASE}"
log_info "================================================================"
echo ""

# Check if bundle directory exists
if [[ ! -d "${ROOT_DIR}/bundle" ]]; then
    log_error "Bundle directory not found: ${ROOT_DIR}/bundle"
    log_error "Please run 'make bundle' first"
    exit 1
fi

# Check if bundle has files
if [[ -z "$(ls -A ${ROOT_DIR}/bundle/manifests 2>/dev/null)" ]]; then
    log_error "Bundle manifests directory is empty: ${ROOT_DIR}/bundle/manifests"
    log_error "Please run 'make bundle' first"
    exit 1
fi

# Step 1: Clone repository (unless skipped)
if [[ "${SKIP_CLONE}" == "false" ]]; then
    log_step "Step 1: Cloning community-operators repository"

    if [[ -d "${WORK_DIR}" ]]; then
        log_warn "Work directory already exists: ${WORK_DIR}"
        read -p "Do you want to remove it and re-clone? [y/N] " -n 1 -r
        echo ""
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            if [[ "${DRY_RUN}" == "false" ]]; then
                rm -rf "${WORK_DIR}"
                log_info "Removed existing directory"
            else
                log_info "[DRY RUN] Would remove: ${WORK_DIR}"
            fi
        else
            log_info "Using existing directory"
            SKIP_CLONE=true
        fi
    fi

    if [[ "${SKIP_CLONE}" == "false" ]]; then
        if [[ "${DRY_RUN}" == "false" ]]; then
            log_info "Cloning upstream repository: ${UPSTREAM_REPO}..."
            git clone "${UPSTREAM_REPO}" "${WORK_DIR}" || {
                log_error "Failed to clone repository"
                exit 1
            }

            cd "${WORK_DIR}"

            # Add fork as remote
            log_info "Adding fork as remote: ${FORK_REPO}"
            git remote add fork "${FORK_REPO}" || log_warn "Fork remote already exists"

            # Create new branch from main/master
            log_info "Creating branch: ${BRANCH}"
            git checkout -b "${BRANCH}" || {
                log_warn "Branch already exists, checking it out"
                git checkout "${BRANCH}"
            }
        else
            log_info "[DRY RUN] Would clone: ${UPSTREAM_REPO} to ${WORK_DIR}"
            log_info "[DRY RUN] Would add fork remote: ${FORK_REPO}"
            log_info "[DRY RUN] Would create branch: ${BRANCH}"
        fi
    fi
    echo ""
else
    log_warn "Skipping clone (--skip-clone)"
    if [[ ! -d "${WORK_DIR}" ]]; then
        log_error "Work directory does not exist: ${WORK_DIR}"
        log_error "Either remove --skip-clone or provide existing directory with --work-dir"
        exit 1
    fi
    echo ""
fi

# Step 2: Copy bundle
log_step "Step 2: Copying bundle to ${CATALOG_PATH}/trustee-operator/${TARGET_RELEASE}"

if [[ "${DRY_RUN}" == "false" ]]; then
    # Remove existing directory if it exists
    if [[ -d "${DEST_DIR}" ]]; then
        log_info "Removing existing directory: ${DEST_DIR}"
        rm -rf "${DEST_DIR}"
    fi

    # Create target directory
    log_info "Creating directory: ${DEST_DIR}"
    mkdir -p "${DEST_DIR}"

    # Copy bundle contents
    log_info "Copying bundle files..."
    cp -r "${ROOT_DIR}/bundle/manifests" "${DEST_DIR}/"
    cp -r "${ROOT_DIR}/bundle/metadata" "${DEST_DIR}/"

    # Copy tests directory if it exists (scorecard tests)
    if [[ -d "${ROOT_DIR}/bundle/tests" ]]; then
        cp -r "${ROOT_DIR}/bundle/tests" "${DEST_DIR}/"
        log_info "Copied tests directory"
    else
        log_warn "No tests directory found in bundle/"
    fi

    # Note: bundle.Dockerfile is typically NOT included in community-operators submissions
    # as it's auto-generated by the community-operators CI/CD pipeline.
    # Uncomment the following lines if your submission requires it:
    #
    # if [[ -f "${ROOT_DIR}/bundle.Dockerfile" ]]; then
    #     cp "${ROOT_DIR}/bundle.Dockerfile" "${DEST_DIR}/"
    #     log_info "Copied bundle.Dockerfile"
    # fi

    log_info "Bundle copied successfully"

    # Show what was copied
    log_info "Copied files:"
    find "${DEST_DIR}" -type f | sed "s|${WORK_DIR}/||" | sort
else
    log_info "[DRY RUN] Would remove: ${DEST_DIR}"
    log_info "[DRY RUN] Would create: ${DEST_DIR}"
    log_info "[DRY RUN] Would copy:"
    log_info "[DRY RUN]   - ${ROOT_DIR}/bundle/manifests → ${DEST_DIR}/manifests"
    log_info "[DRY RUN]   - ${ROOT_DIR}/bundle/metadata → ${DEST_DIR}/metadata"
    if [[ -d "${ROOT_DIR}/bundle/tests" ]]; then
        log_info "[DRY RUN]   - ${ROOT_DIR}/bundle/tests → ${DEST_DIR}/tests"
    fi
    log_info "[DRY RUN]   NOTE: bundle.Dockerfile is not copied (auto-generated by community-operators)"
fi

echo ""

# Step 3: Create git commit (unless skipped)
if [[ "${SKIP_COMMIT}" == "false" && "${DRY_RUN}" == "false" ]]; then
    log_step "Step 3: Creating git commit"

    cd "${WORK_DIR}"

    # Check if there are changes
    if git diff --quiet && git diff --cached --quiet; then
        log_warn "No changes to commit"
    else
        log_info "Staging changes..."
        git add "${CATALOG_PATH}/trustee-operator/${TARGET_RELEASE}"

        log_info "Creating commit..."
        COMMIT_MSG="operator trustee-operator (${TARGET_RELEASE})

Add trustee-operator version ${TARGET_RELEASE}

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"

        git commit -m "${COMMIT_MSG}" || {
            log_error "Failed to create commit"
            exit 1
        }

        log_info "Commit created successfully"
        git log -1 --oneline
    fi
    echo ""
else
    if [[ "${SKIP_COMMIT}" == "true" ]]; then
        log_warn "Skipping commit (--skip-commit)"
    else
        log_info "[DRY RUN] Would create commit with bundle changes"
    fi
    echo ""
fi

# Summary
log_info "================================================================"
log_info "Bundle Preparation Complete!"
log_info "================================================================"
log_info ""
log_info "Working directory: ${WORK_DIR}"
log_info "Branch:            ${BRANCH}"
log_info "Bundle location:   ${CATALOG_PATH}/trustee-operator/${TARGET_RELEASE}"
log_info ""

if [[ "${DRY_RUN}" == "false" ]]; then
    log_info "Next steps:"
    log_info "  1. Review the changes:"
    log_info "     cd ${WORK_DIR}"
    log_info "     git status"
    log_info "     git diff"
    log_info ""
    log_info "  2. Test the operator bundle (optional):"
    log_info "     operator-sdk bundle validate ${DEST_DIR}"
    log_info ""
    log_info "  3. Push the branch to your fork:"
    log_info "     cd ${WORK_DIR}"
    log_info "     git push fork ${BRANCH}"
    log_info ""
    log_info "  4. Create a Pull Request:"
    log_info "     Go to: https://github.com/k8s-operatorhub/community-operators/compare"
    log_info "     Base: k8s-operatorhub/community-operators:main"

    # Extract username from fork URL
    FORK_USER=$(echo "${FORK_REPO}" | sed -E 's|.*github\.com[:/]([^/]+)/.*|\1|')
    log_info "     Compare: ${FORK_USER}:${BRANCH}"
    log_info ""
    log_info "  5. Follow the PR template and community-operators guidelines"
else
    log_info "This was a dry run. No changes were made."
    log_info "Run without --dry-run to perform the actual operation."
fi

echo ""
