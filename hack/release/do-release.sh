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

# Complete release automation script
# Handles version bump, bundle generation, git operations, and PR workflow
# Images are built by GitHub Actions after release

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Source common functions
source "${SCRIPT_DIR}/common.sh"

# Change to repository root immediately to ensure all git commands
# operate on the correct repository, regardless of where script is invoked from
cd "${ROOT_DIR}"

# Verify we're inside a git repository
if ! git rev-parse --is-inside-work-tree &>/dev/null; then
    echo "Error: Not inside a git repository"
    echo "Expected repository root: ${ROOT_DIR}"
    exit 1
fi

# Disable git pager for better script automation
export GIT_PAGER=cat

usage() {
    cat <<EOF
Usage: $0 <new-version> --fork FORK [options]

⭐ Recommended - Complete end-to-end release automation for trustee-operator.
Prepares version, bundle files, and creates pull request. Images are built by GitHub Actions.

Arguments:
  new-version    The new version to release (e.g., 0.18.0)

Required Options:
  --fork FORK         Your fork URL to push to (e.g., git@github.com:username/trustee-operator.git)

Optional Options:
  --skip-push         Skip pushing to fork and creating PR
  --skip-pr           Skip creating PR (but still push to fork)
  --dry-run           Show what would be done without making changes
  -h, --help          Show this help message

Examples:
  # Complete release preparation (recommended)
  $0 0.18.0 --fork git@github.com:lmilleri/trustee-operator.git

  # Dry run to see what would happen
  $0 0.18.0 --fork git@github.com:lmilleri/trustee-operator.git --dry-run

  # Prepare release without pushing (testing)
  $0 0.18.0 --fork git@github.com:lmilleri/trustee-operator.git --skip-push

Release steps performed:
  1. Run tests
  2. Bump version to new version
  3. Regenerate bundle manifests
  4. Commit changes to branch release-v{VERSION}
  5. Push to fork (unless --skip-push)
  6. Create pull request to main (unless --skip-push or --skip-pr)

After PR is merged:
  - Create GitHub release (tag v{VERSION})
  - GitHub Actions will automatically build and push multi-arch operator image
  - Submit bundle files to community-operators using prepare-community-operators.sh
EOF
}

# Parse arguments
NEW_VERSION=""
FORK_REPO=""
SKIP_PUSH=false
SKIP_PR=false
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --fork)
            if [[ $# -lt 2 ]] || [[ -z "${2:-}" ]]; then
                log_error "Missing value for --fork"
                usage
                exit 1
            fi
            FORK_REPO="$2"
            shift 2
            ;;
        --skip-push)
            SKIP_PUSH=true
            shift
            ;;
        --skip-pr)
            SKIP_PR=true
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
            if [[ -z "${NEW_VERSION}" ]]; then
                NEW_VERSION="$1"
            else
                log_error "Unexpected argument: $1"
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

# Set branch name
BRANCH="release-v${NEW_VERSION}"

# Validate version format
validate_version "${NEW_VERSION}" || exit 1

if [[ "${DRY_RUN}" == "true" ]]; then
    log_warn "DRY RUN MODE - No changes will be made"
    echo ""
fi

log_info "Starting release process for version ${NEW_VERSION}"
log_info "Fork: ${FORK_REPO}"
log_info "Branch: ${BRANCH}"
echo ""

# Check current version to avoid no-op releases
CURRENT_VERSION=$(get_version_from_makefile || echo "")
if [[ -n "${CURRENT_VERSION}" && "${CURRENT_VERSION}" == "${NEW_VERSION}" ]]; then
    log_warn "Current version in Makefile is already ${NEW_VERSION}"
    log_warn "Nothing to do - version is already set to the target version"
    log_warn "If you need to regenerate bundle files, use: make bundle IMG=..."
    exit 0
fi

# Check if git working directory is clean (including untracked files)
if [[ -n "$(git status --porcelain)" ]]; then
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

# Ensure local main is up-to-date with upstream
if [[ "${DRY_RUN}" == "false" ]]; then
    # Determine which remote to use for upstream sync
    # Prefer 'upstream' remote if it exists (fork-based workflow)
    # Otherwise use 'origin' (direct contributor workflow)
    UPSTREAM_REMOTE=""

    if git remote get-url upstream &>/dev/null; then
        UPSTREAM_REMOTE="upstream"
        log_info "Found 'upstream' remote, will sync with upstream/main"
    elif git remote get-url origin &>/dev/null; then
        # Check if origin appears to be the canonical repo
        ORIGIN_URL=$(git remote get-url origin)
        if [[ "${ORIGIN_URL}" == *"confidential-containers/trustee-operator"* ]] || \
           [[ "${ORIGIN_URL}" == *"github.com/confidential-containers/trustee-operator"* ]]; then
            UPSTREAM_REMOTE="origin"
            log_info "Using 'origin' remote (appears to be canonical repo)"
        else
            log_warn "No 'upstream' remote found and 'origin' appears to be a fork"
            log_warn "Cannot verify if main is up-to-date with canonical repository"
            log_warn "Consider adding upstream remote: git remote add upstream <canonical-repo-url>"
            # Don't exit - allow user to continue with warning
            UPSTREAM_REMOTE=""
        fi
    fi

    if [[ -n "${UPSTREAM_REMOTE}" ]]; then
        log_info "Fetching latest from ${UPSTREAM_REMOTE}..."
        git fetch "${UPSTREAM_REMOTE}" || {
            log_error "Failed to fetch from ${UPSTREAM_REMOTE}"
            exit 1
        }

        # Check if upstream/main (or origin/main) exists
        if ! git rev-parse "${UPSTREAM_REMOTE}/main" &>/dev/null; then
            log_warn "${UPSTREAM_REMOTE}/main not found, skipping upstream check"
        else
            # Get commit hashes
            LOCAL_MAIN=$(git rev-parse main)
            REMOTE_MAIN=$(git rev-parse "${UPSTREAM_REMOTE}/main")

            if [[ "${LOCAL_MAIN}" == "${REMOTE_MAIN}" ]]; then
                log_info "Local main is up-to-date with ${UPSTREAM_REMOTE}/main"
            else
                # Check relationship between local and remote
                # If upstream/main is ancestor of main → local is ahead
                # If main is ancestor of upstream/main → local is behind
                if git merge-base --is-ancestor "${UPSTREAM_REMOTE}/main" main; then
                    log_warn "Local main is ahead of ${UPSTREAM_REMOTE}/main"
                    log_warn "You may want to push your changes before creating a release"
                elif git merge-base --is-ancestor main "${UPSTREAM_REMOTE}/main"; then
                    log_warn "Local main is behind ${UPSTREAM_REMOTE}/main"
                    log_info "Attempting to fast-forward..."

                    git pull --ff-only "${UPSTREAM_REMOTE}" main || {
                        log_error "Failed to fast-forward main branch"
                        log_error "Your local main has diverged from ${UPSTREAM_REMOTE}/main"
                        log_error "Please manually update your main branch before releasing"
                        exit 1
                    }
                    log_info "Successfully updated main to match ${UPSTREAM_REMOTE}/main"
                else
                    log_error "Local main has diverged from ${UPSTREAM_REMOTE}/main"
                    log_error "Please manually resolve this before releasing"
                    exit 1
                fi
            fi
        fi
    fi
else
    log_info "[DRY RUN] Would fetch and verify main is up-to-date with upstream"
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
        log_error "Branch ${BRANCH} already exists locally"
        log_error ""
        log_error "This could be from a previous failed release attempt with unwanted commits."
        log_error "To avoid accidentally including old commits in the release PR:"
        log_error ""
        log_error "  1. Delete the existing branch:"
        log_error "     git branch -D ${BRANCH}"
        log_error ""
        log_error "  2. Re-run this script to create a fresh branch from main"
        log_error ""
        exit 1
    else
        git checkout -b "${BRANCH}" || {
            log_error "Failed to create branch ${BRANCH}"
            exit 1
        }
    fi
else
    log_info "[DRY RUN] Would create and checkout branch: ${BRANCH}"
fi

# Step 1: Run tests
log_step "Step 1: Running tests"
if [[ "${DRY_RUN}" == "true" ]]; then
    log_info "[DRY RUN] Would run: make test"
else
    log_info "Running tests..."
    make test || {
        log_error "Tests failed. Aborting release."
        exit 1
    }
fi
echo ""

# Step 2: Bump version
log_step "Step 2: Bumping version to ${NEW_VERSION}"
if [[ "${DRY_RUN}" == "true" ]]; then
    log_info "[DRY RUN] Would run: ${SCRIPT_DIR}/bump-version.sh ${NEW_VERSION}"
else
    # Run bump-version.sh with auto-yes via stdin redirection
    bash "${SCRIPT_DIR}/bump-version.sh" "${NEW_VERSION}" <<< "y" || {
        log_error "Version bump failed. Aborting release."
        exit 1
    }
fi
echo ""

# Step 3: Regenerate bundle
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
        exit 1
    }

    # Clean up duplicate image entry in kustomization.yaml that kustomize adds
    remove_duplicate_kustomization_images "${ROOT_DIR}"

    log_info "Bundle generated successfully with image: ${IMG}"
fi
echo ""

# Step 4: Commit changes
log_step "Step 4: Creating git commit"
if [[ "${DRY_RUN}" == "true" ]]; then
    log_info "[DRY RUN] Would run: git add -A && git commit -m 'Release v${NEW_VERSION}'"
    git diff --stat
else
    git add -A
    git commit -m "Release v${NEW_VERSION}

- Bump version to ${NEW_VERSION}
- Regenerate bundle manifests" || {
        log_error "Git commit failed. Aborting release."
        exit 1
    }
    git log -1 --oneline
fi
echo ""

# Step 5: Push to fork and create PR
if [[ "${SKIP_PUSH}" == "false" ]]; then
    if [[ "${SKIP_PR}" == "false" ]]; then
        log_step "Step 5: Pushing to fork and creating PR"
    else
        log_step "Step 5: Pushing to fork"
    fi

    # Extract fork owner from fork URL (needed for pushing to fork)
    FORK_OWNER=$(echo "${FORK_REPO}" | sed -E 's|.*github\.com[:/]([^/]+)/.*|\1|')

    # Validate FORK_OWNER extraction
    # Should be a GitHub username/org (no slashes, no URL artifacts)
    # If extraction failed, sed returns the original string, so check for that too
    if [[ -z "${FORK_OWNER}" ]] || \
       [[ "${FORK_OWNER}" == "${FORK_REPO}" ]] || \
       [[ "${FORK_OWNER}" == *"/"* ]] || \
       [[ "${FORK_OWNER}" == *":"* ]] || \
       [[ "${FORK_OWNER}" == *"@"* ]] || \
       [[ "${FORK_OWNER}" == *"://"* ]] || \
       [[ "${FORK_OWNER}" == *".git"* ]] || \
       [[ "${FORK_OWNER}" == *"github.com"* ]] || \
       [[ "${FORK_OWNER}" == *"."* ]]; then
        log_error "Failed to extract fork owner from: ${FORK_REPO}"
        log_error "Expected a GitHub URL like: git@github.com:username/trustee-operator.git"
        log_error "Extracted owner: '${FORK_OWNER}' (invalid)"
        exit 1
    fi

    # Determine upstream repo for PR target (only needed if creating PR)
    if [[ "${SKIP_PR}" == "false" ]]; then
        # Use same logic as sync check: prefer 'upstream' remote, fallback to 'origin' if canonical
        UPSTREAM_REPO=""
        if git remote get-url upstream &>/dev/null; then
            UPSTREAM_URL=$(git remote get-url upstream)
            UPSTREAM_REPO=$(echo "${UPSTREAM_URL}" | sed -E 's|.*github\.com[:/]([^/]+/[^/]+)(\.git)?$|\1|' | sed 's/\.git$//')
            if [[ "${DRY_RUN}" == "false" ]]; then
                log_info "Using 'upstream' remote for PR target: ${UPSTREAM_REPO}"
            fi
        elif git remote get-url origin &>/dev/null; then
            ORIGIN_URL=$(git remote get-url origin)
            # Check if origin appears to be the canonical repo
            if [[ "${ORIGIN_URL}" == *"confidential-containers/trustee-operator"* ]] || \
               [[ "${ORIGIN_URL}" == *"github.com/confidential-containers/trustee-operator"* ]]; then
                UPSTREAM_REPO=$(echo "${ORIGIN_URL}" | sed -E 's|.*github\.com[:/]([^/]+/[^/]+)(\.git)?$|\1|' | sed 's/\.git$//')
                if [[ "${DRY_RUN}" == "false" ]]; then
                    log_info "Using 'origin' remote for PR target (canonical repo): ${UPSTREAM_REPO}"
                fi
            else
                log_error "No 'upstream' remote found and 'origin' appears to be a fork"
                log_error "Cannot determine canonical repository for PR target"
                log_error "Please add upstream remote: git remote add upstream <canonical-repo-url>"
                exit 1
            fi
        else
            log_error "No git remotes found"
            exit 1
        fi

        # Validate UPSTREAM_REPO extraction
        # Should be exactly "owner/repo" format (one slash, no URL artifacts)
        if [[ -z "${UPSTREAM_REPO}" ]] || \
           [[ "${UPSTREAM_REPO}" != *"/"* ]] || \
           [[ "${UPSTREAM_REPO}" == *":"* ]] || \
           [[ "${UPSTREAM_REPO}" == *"@"* ]] || \
           [[ "${UPSTREAM_REPO}" == *"://"* ]] || \
           [[ "${UPSTREAM_REPO}" == *".git"* ]] || \
           [[ "${UPSTREAM_REPO}" == *"github.com"* ]] || \
           [[ $(echo "${UPSTREAM_REPO}" | grep -o "/" | wc -l) -ne 1 ]]; then
            log_error "Failed to extract upstream repo"
            log_error "Expected a GitHub URL like: git@github.com:org/repo.git"
            log_error "Extracted repo: '${UPSTREAM_REPO}' (invalid)"
            log_error "You can create the PR manually at GitHub"
            exit 1
        fi
    fi

    # Check GitHub CLI availability and authentication if we're going to create a PR
    # Skip these checks in dry-run mode since gh won't actually be invoked
    if [[ "${SKIP_PR}" == "false" && "${DRY_RUN}" == "false" ]]; then
        # Check if gh command is available
        if ! command -v gh &>/dev/null; then
            log_error "GitHub CLI (gh) is not installed - cannot create pull request"
            log_error ""
            log_error "Install instructions:"
            log_error "  macOS:   brew install gh"
            log_error "  Linux:   https://github.com/cli/cli#installation"
            log_error "  Windows: https://github.com/cli/cli#installation"
            log_error ""
            log_error "After installation, authenticate with: gh auth login"
            log_error ""
            log_error "Alternatively, use --skip-pr to skip PR creation and create it manually"
            exit 1
        fi

        # Check if gh is authenticated
        if ! gh auth status &>/dev/null; then
            log_error "GitHub CLI is not authenticated - cannot create pull request"
            log_error ""
            log_error "Run: gh auth login"
            log_error ""
            log_error "This will authenticate gh with your GitHub account and allow PR creation"
            log_error ""
            log_error "Alternatively, use --skip-pr to skip PR creation and create it manually"
            exit 1
        fi

        log_info "GitHub CLI authenticated and ready for PR creation"
    fi

    # Validate that we're creating PR to canonical repo (only if creating PR)
    if [[ "${SKIP_PR}" == "false" ]]; then
        # It's OK if fork == upstream (same-repo PR for direct contributors)
        # But ensure upstream is the canonical repo
        if [[ "${UPSTREAM_REPO}" != "confidential-containers/trustee-operator" ]]; then
            FORK_REPO_PATH=$(echo "${FORK_REPO}" | sed -E 's|.*github\.com[:/]([^/]+/[^/]+)(\.git)?$|\1|' | sed 's/\.git$//')
            log_warn "PR will be created to: ${UPSTREAM_REPO}"
            log_warn "This does not appear to be the canonical repository"
            log_warn "Expected: confidential-containers/trustee-operator"
            log_warn "Got: ${UPSTREAM_REPO}"

            # If fork and upstream are the same and it's not canonical, that's definitely wrong
            if [[ "${UPSTREAM_REPO}" == "${FORK_REPO_PATH}" ]]; then
                log_error "Both fork and upstream point to non-canonical repo: ${UPSTREAM_REPO}"
                log_error "Please configure 'upstream' remote to point to canonical repository"
                log_error "  git remote add upstream git@github.com:confidential-containers/trustee-operator.git"
                exit 1
            fi
        fi
    fi

    # Now do the actual push and PR creation (or show dry-run output)
    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY RUN] Would run: git push -u fork ${BRANCH}"
        if [[ "${SKIP_PR}" == "false" ]]; then
            log_info "[DRY RUN] Would run: gh pr create --repo \"${UPSTREAM_REPO}\" --base main --head \"${FORK_OWNER}:${BRANCH}\" --title \"Release v${NEW_VERSION}\""
        fi
    else
        log_info "Pushing branch to fork..."
        git push -u fork "${BRANCH}" || {
            log_error "Git push to fork failed."
            exit 1
        }
        log_info "Branch ${BRANCH} pushed to fork"

        if [[ "${SKIP_PR}" == "false" ]]; then
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
        else
            log_warn "Skipping PR creation (--skip-pr)"
            log_warn "You can create PR manually:"
            log_warn "  gh pr create --base main"
        fi
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
log_info "Release Preparation Complete!"
log_info "================================================================"
log_info ""
log_info "Version: ${NEW_VERSION}"
log_info ""

if [[ "${DRY_RUN}" == "false" ]]; then
    log_info "What was done:"
    log_info "  ✓ Tests passed"
    log_info "  ✓ Version bumped to ${NEW_VERSION}"
    log_info "  ✓ Bundle files generated in: bundle/"
    log_info "    - manifests/"
    log_info "    - metadata/"
    log_info "    - tests/"
    log_info "  ✓ Changes committed to git branch: ${BRANCH}"
    if [[ "${SKIP_PUSH}" == "false" ]]; then
        log_info "  ✓ Branch pushed to fork"
        if [[ "${SKIP_PR}" == "false" ]]; then
            log_info "  ✓ Pull request created"
        fi
    fi
    echo ""
    log_info "Next steps:"
    if [[ "${SKIP_PUSH}" == "false" && "${SKIP_PR}" == "false" ]]; then
        log_info "  1. Review and merge the pull request"
        log_info ""
        log_info "  2. After PR is merged, create GitHub release:"
        log_info "     https://github.com/confidential-containers/trustee-operator/releases/new"
        log_info "     - Tag: v${NEW_VERSION}"
        log_info "     - GitHub Actions will build and push multi-arch operator image"
        log_info "       (linux/amd64, linux/arm64, linux/s390x)"
        log_info ""
        log_info "  3. Wait for GitHub Actions to complete the image build"
        log_info "     Check: https://github.com/confidential-containers/trustee-operator/actions"
        log_info ""
        log_info "  4. Verify operator image in registry:"
        log_info "     quay.io/confidential-containers/trustee-operator:v${NEW_VERSION}"
        log_info ""
        log_info "  5. Submit bundle files to community-operators:"
        log_info "     ${SCRIPT_DIR}/prepare-community-operators.sh ${NEW_VERSION} --fork git@github.com:USERNAME/community-operators.git"
    elif [[ "${SKIP_PUSH}" == "false" && "${SKIP_PR}" == "true" ]]; then
        log_info "  1. Create pull request: gh pr create --base main"
        log_info "  2. After PR is merged, create GitHub release:"
        log_info "     https://github.com/confidential-containers/trustee-operator/releases/new"
        log_info "     - Tag: v${NEW_VERSION}"
        log_info "     - GitHub Actions will build and push multi-arch operator image"
        log_info "  3. Submit bundle files to community-operators"
    else
        log_info "  1. Push branch to fork: git push -u fork ${BRANCH}"
        log_info "  2. Create pull request: gh pr create --base main"
        log_info "  3. After PR is merged, create GitHub release"
        log_info "  4. Submit bundle files to community-operators"
    fi
else
    log_info "This was a dry run. No changes were made."
    log_info "Run without --dry-run to perform the actual release preparation."
fi

echo ""
