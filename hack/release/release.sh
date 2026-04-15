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
Usage: $0 <new-version> --fork FORK [options]

Automates the release process for trustee-operator.

Arguments:
  new-version    The new version to release (e.g., 0.18.0)

Required Options:
  --fork FORK         Your fork URL to push to (e.g., git@github.com:username/trustee-operator.git)

Optional Options:
  --branch BRANCH     Branch name to create (default: release-v\${VERSION})
  --skip-tests        Skip running tests before release
  --skip-bundle       Skip bundle regeneration
  --skip-commit       Skip creating git commit
  --skip-push         Skip pushing to fork and creating PR
  --dry-run           Show what would be done without making changes

Examples:
  $0 0.18.0 --fork git@github.com:lmilleri/trustee-operator.git
  $0 0.18.0 --fork git@github.com:lmilleri/trustee-operator.git --dry-run
  $0 0.18.0 --fork git@github.com:lmilleri/trustee-operator.git --skip-tests

Release steps performed:
  1. Run tests (unless --skip-tests)
  2. Bump version to new version
  3. Regenerate bundle manifests (unless --skip-bundle)
  4. Commit changes (unless --skip-commit)
  5. Push to fork (unless --skip-push)
  6. Create pull request to main (unless --skip-push)

Note: Git tags are created via GitHub release UI after PR is merged
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

# Parse arguments
NEW_VERSION=""
FORK_REPO=""
BRANCH=""
SKIP_TESTS=false
SKIP_BUNDLE=false
SKIP_COMMIT=false
SKIP_PUSH=false
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --fork)
            FORK_REPO="$2"
            shift 2
            ;;
        --branch)
            BRANCH="$2"
            shift 2
            ;;
        --skip-tests)
            SKIP_TESTS=true
            shift
            ;;
        --skip-bundle)
            SKIP_BUNDLE=true
            shift
            ;;
        --skip-commit)
            SKIP_COMMIT=true
            shift
            ;;
        --skip-push)
            SKIP_PUSH=true
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
        *)
            if [[ -z "${NEW_VERSION}" ]]; then
                NEW_VERSION="$1"
            else
                log_error "Unknown option: $1"
                usage
                exit 1
            fi
            shift
            ;;
    esac
done

if [[ -z "${NEW_VERSION}" ]]; then
    log_error "Missing required argument: new-version"
    usage
    exit 1
fi

# Validate required --fork parameter
if [[ -z "${FORK_REPO}" ]]; then
    log_error "Missing required parameter: --fork"
    log_error "You must specify your fork URL"
    log_error "Example: $0 0.18.0 --fork git@github.com:yourusername/trustee-operator.git"
    exit 1
fi

# Set default branch name
if [[ -z "${BRANCH}" ]]; then
    BRANCH="release-v${NEW_VERSION}"
fi

# Validate version format
if ! [[ "${NEW_VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    log_error "Invalid version format: ${NEW_VERSION}"
    log_error "Version must be in format X.Y.Z (e.g., 0.18.0)"
    exit 1
fi

if [[ "${DRY_RUN}" == "true" ]]; then
    log_warn "DRY RUN MODE - No changes will be made"
    echo ""
fi

log_info "Starting release process for version ${NEW_VERSION}"
log_info "Fork: ${FORK_REPO}"
log_info "Branch: ${BRANCH}"
echo ""

# Check if git working directory is clean
if ! git diff-index --quiet HEAD -- 2>/dev/null; then
    log_error "Git working directory is not clean. Please commit or stash your changes first."
    git status --short
    exit 1
fi

# Ensure we're on main branch to start
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [[ "${CURRENT_BRANCH}" != "main" ]]; then
    log_warn "Not on main branch (current: ${CURRENT_BRANCH})"
    if [[ "${DRY_RUN}" == "false" ]]; then
        read -p "Switch to main branch? [y/N] " -n 1 -r
        echo ""
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            git checkout main || {
                log_error "Failed to switch to main branch"
                exit 1
            }
            CURRENT_BRANCH="main"
        else
            log_info "Aborted by user"
            exit 0
        fi
    fi
fi

# Add fork as remote if not already added
if [[ "${DRY_RUN}" == "false" ]]; then
    if git remote get-url fork &>/dev/null; then
        log_info "Fork remote already exists, updating URL..."
        git remote set-url fork "${FORK_REPO}"
    else
        log_info "Adding fork as remote..."
        git remote add fork "${FORK_REPO}"
    fi
fi

# Create and checkout release branch
if [[ "${DRY_RUN}" == "false" ]]; then
    log_info "Creating branch: ${BRANCH}"
    if git show-ref --verify --quiet refs/heads/"${BRANCH}"; then
        log_warn "Branch ${BRANCH} already exists locally, checking it out"
        git checkout "${BRANCH}"
    else
        git checkout -b "${BRANCH}" || {
            log_error "Failed to create branch ${BRANCH}"
            exit 1
        }
    fi
else
    log_info "[DRY RUN] Would create and checkout branch: ${BRANCH}"
fi

cd "${ROOT_DIR}"

# Step 1: Run tests
if [[ "${SKIP_TESTS}" == "false" ]]; then
    log_step "Step 1: Running tests"
    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY RUN] Would run: GOTOOLCHAIN=local make test"
    else
        # Use local Go toolchain instead of downloading version from go.mod
        log_info "Running tests with local Go toolchain..."
        export GOTOOLCHAIN=local

        make test || {
            log_error "Tests failed. Aborting release."
            exit 1
        }
    fi
    echo ""
else
    log_warn "Skipping tests (--skip-tests)"
    echo ""
fi

# Step 2: Bump version
log_step "Step 2: Bumping version to ${NEW_VERSION}"
if [[ "${DRY_RUN}" == "true" ]]; then
    log_info "[DRY RUN] Would run: ${SCRIPT_DIR}/bump-version.sh ${NEW_VERSION}"
else
    # Run bump-version.sh with auto-yes
    export REPLY="y"
    bash "${SCRIPT_DIR}/bump-version.sh" "${NEW_VERSION}" <<< "y" || {
        log_error "Version bump failed. Aborting release."
        exit 1
    }
fi
echo ""

# Step 3: Regenerate bundle
if [[ "${SKIP_BUNDLE}" == "false" ]]; then
    log_step "Step 3: Regenerating bundle manifests"

    # Set IMG to the versioned image
    REGISTRY="quay.io/confidential-containers"
    IMAGE_TAG_BASE="${REGISTRY}/trustee-operator"
    IMG="${IMAGE_TAG_BASE}:v${NEW_VERSION}"

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY RUN] Would run: make bundle IMG=${IMG}"
    else
        make bundle IMG="${IMG}" || {
            log_error "Bundle generation failed. Aborting release."
            log_info "You can rollback using: bash ${SCRIPT_DIR}/rollback-version.sh"
            exit 1
        }

        # Clean up duplicate image entry in kustomization.yaml that kustomize adds
        if grep -q "name: quay.io/confidential-containers/trustee-operator" "${ROOT_DIR}/config/manager/kustomization.yaml" 2>/dev/null; then
            log_info "Removing duplicate image entry from config/manager/kustomization.yaml"
            sed -i '/^- name: quay\.io\/confidential-containers\/trustee-operator$/,/^  newTag:/d' "${ROOT_DIR}/config/manager/kustomization.yaml"
        fi

        log_info "Bundle generated successfully with image: ${IMG}"
    fi
    echo ""
else
    log_warn "Skipping bundle regeneration (--skip-bundle)"
    echo ""
fi

# Step 4: Commit changes
if [[ "${SKIP_COMMIT}" == "false" ]]; then
    log_step "Step 4: Creating git commit"
    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY RUN] Would run: git add -A && git commit -m 'Release v${NEW_VERSION}'"
        git diff --stat
    else
        git add -A
        git commit -m "Release v${NEW_VERSION}

- Bump version to ${NEW_VERSION}
- Regenerate bundle manifests

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>" || {
            log_error "Git commit failed. Aborting release."
            exit 1
        }
        git log -1 --oneline
    fi
    echo ""
else
    log_warn "Skipping git commit (--skip-commit)"
    echo ""
fi

# Step 5: Push to fork and create PR
if [[ "${SKIP_PUSH}" == "false" ]]; then
    log_step "Step 5: Pushing to fork and creating PR"

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY RUN] Would run: git push -u fork ${BRANCH}"
        log_info "[DRY RUN] Would run: gh pr create --base main --head ${BRANCH}"
    else
        log_info "Pushing branch to fork..."
        git push -u fork "${BRANCH}" || {
            log_error "Git push to fork failed."
            exit 1
        }
        log_info "Branch ${BRANCH} pushed to fork"

        # Extract fork owner from fork URL
        FORK_OWNER=$(echo "${FORK_REPO}" | sed -E 's|.*github\.com[:/]([^/]+)/.*|\1|')

        # Get upstream repo (origin remote)
        UPSTREAM_REPO=$(git remote get-url origin | sed -E 's|.*github\.com[:/]([^/]+/[^/]+)(\.git)?$|\1|' | sed 's/\.git$//')

        log_info "Creating pull request..."
        gh pr create \
            --repo "${UPSTREAM_REPO}" \
            --base main \
            --head "${FORK_OWNER}:${BRANCH}" \
            --title "Release v${NEW_VERSION}" \
            --body "$(cat <<EOF
## Release v${NEW_VERSION}

### Changes
- Bump version to ${NEW_VERSION}
- Regenerate bundle manifests for community-operators submission

### Next Steps
After merging this PR:
1. Create GitHub release at https://github.com/confidential-containers/trustee-operator/releases/new
   - Tag: v${NEW_VERSION}
   - GitHub Actions will build and push multi-arch operator image
2. Submit bundle files to community-operators

🤖 Generated with automated release scripts
EOF
)" || {
            log_error "Failed to create pull request"
            log_error "You can create it manually at:"
            log_error "  https://github.com/${UPSTREAM_REPO}/compare/main...${FORK_OWNER}:${BRANCH}"
            exit 1
        }
        log_info "Pull request created successfully"
    fi
    echo ""
else
    log_warn "Skipping push to fork and PR creation (--skip-push)"
    log_warn "You can push and create PR manually:"
    log_warn "  git push -u fork ${BRANCH}"
    log_warn "  gh pr create --base main"
    echo ""
fi

# Summary
log_info "================================================================"
log_info "Release preparation complete!"
log_info "================================================================"
log_info ""
log_info "Version: ${NEW_VERSION}"
log_info "Branch: ${BRANCH}"
log_info ""

if [[ "${DRY_RUN}" == "false" ]]; then
    if [[ "${SKIP_PUSH}" == "false" ]]; then
        log_info "✓ Pull request created for v${NEW_VERSION}"
        echo ""
        log_info "Next steps:"
        log_info "  1. Review and merge the pull request"
        log_info "  2. After merge, create GitHub release:"
        log_info "     https://github.com/confidential-containers/trustee-operator/releases/new"
        log_info "     - Tag: v${NEW_VERSION}"
        log_info "     - GitHub Actions will build and push multi-arch operator image"
        log_info "  3. Submit bundle files to community-operators"
    else
        log_info "Next steps:"
        log_info "  1. Push branch to fork: git push -u fork ${BRANCH}"
        log_info "  2. Create pull request: gh pr create --base main"
        log_info "  3. After PR is merged, create GitHub release"
    fi
else
    log_info "This was a dry run. No changes were made."
    log_info "Run without --dry-run to perform the actual release preparation."
fi

echo ""
