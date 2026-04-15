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

# Disable git pager for better script automation
export GIT_PAGER=cat

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
  --skip-push        Skip pushing to fork and creating PR
  --skip-pr          Skip creating PR (but still push to fork)
  --dry-run          Show what would be done without making changes
  -h, --help         Show this help message

Examples:
  # Complete submission with automatic PR creation
  $0 0.18.0 --fork git@github.com:lmilleri/community-operators.git

  # Dry run to see what would happen
  $0 0.18.0 --fork git@github.com:myuser/community-operators.git --dry-run

  # Push to fork but skip PR creation
  $0 0.18.0 --fork git@github.com:myuser/community-operators.git --skip-pr

  # Prepare files but don't push/PR (for review)
  $0 0.18.0 --fork git@github.com:myuser/community-operators.git --skip-push

What this script does:
  1. Clones k8s-operatorhub/community-operators to temp directory
  2. Adds your fork as remote
  3. Creates branch trustee-operator-v{VERSION}
  4. Copies bundle files to operators/trustee-operator/{VERSION}/
  5. Creates commit
  6. Pushes to fork and creates PR (unless --skip-push or --skip-pr)
EOF
}

# Default values
VERSION=""
FORK_REPO=""  # REQUIRED - must be provided via --fork
SKIP_PUSH=false
SKIP_PR=false
DRY_RUN=false

# Parse arguments
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
    VERSION=$(get_version_from_makefile) || exit 1
    log_info "Using version from Makefile: ${VERSION}"
fi

# Validate version format
validate_version "${VERSION}" || exit 1

# Validate required --fork parameter
if [[ -z "${FORK_REPO}" ]]; then
    log_error "Missing required parameter: --fork"
    log_error "You must specify your fork URL"
    log_error "Example: $0 0.18.0 --fork git@github.com:yourusername/community-operators.git"
    exit 1
fi

# Set fixed values
UPSTREAM_REPO="git@github.com:k8s-operatorhub/community-operators.git"
BRANCH="trustee-operator-v${VERSION}"
CATALOG="community"
WORK_DIR="${TMPDIR:-/tmp}/community-operators-${VERSION}"

# Validate WORK_DIR for safety before any rm -rf operations

# Strip trailing slashes to prevent symlink bypass (e.g., /tmp/link/)
WORK_DIR="${WORK_DIR%/}"

# Find a working realpath command (GNU realpath preferred for -m support)
REALPATH_CMD=""
if command -v grealpath &>/dev/null; then
    # macOS with coreutils installed (brew install coreutils)
    REALPATH_CMD="grealpath"
elif command -v realpath &>/dev/null; then
    # Linux or macOS with realpath available
    REALPATH_CMD="realpath"
else
    # realpath not found - check if we're in dry-run mode
    if [[ "${DRY_RUN}" == "true" ]]; then
        log_warn "realpath command not found - skipping WORK_DIR validation in dry-run mode"
        log_warn "On macOS: brew install coreutils (for actual operations)"
        log_warn "On Linux: install coreutils package (for actual operations)"
        REALPATH_CMD=""
    else
        log_error "realpath command not found - cannot safely validate WORK_DIR"
        log_error "On macOS: brew install coreutils"
        log_error "On Linux: install coreutils package"
        log_error "This is required to prevent symlink-based path traversal attacks"
        log_error ""
        log_error "Without realpath, the script cannot guarantee that WORK_DIR is safely under temp/"
        log_error "and subsequent 'rm -rf' operations could accidentally delete files outside temp/"
        exit 1
    fi
fi

# Determine the system temp directory
# Use TMPDIR if set (macOS often sets this), otherwise /tmp
SYSTEM_TMPDIR="${TMPDIR:-/tmp}"
SYSTEM_TMPDIR="${SYSTEM_TMPDIR%/}"  # Strip trailing slash

# Path validation (skipped in dry-run mode when realpath is not available)
if [[ -n "${REALPATH_CMD}" ]]; then
    # Canonicalize the temp directory itself to handle /tmp -> /private/tmp on macOS
    # The temp directory should exist, so we can use realpath without -m
    if [[ ! -d "${SYSTEM_TMPDIR}" ]]; then
        log_error "System temp directory does not exist: ${SYSTEM_TMPDIR}"
        log_error "This is unexpected and may indicate a system configuration issue"
        exit 1
    fi

    TMPDIR_CANONICAL=$($REALPATH_CMD "${SYSTEM_TMPDIR}" 2>/dev/null)
    if [[ -z "${TMPDIR_CANONICAL}" ]]; then
        log_error "Failed to canonicalize system temp directory: ${SYSTEM_TMPDIR}"
        log_error "This may indicate a system configuration issue"
        exit 1
    fi

    # Also canonicalize /tmp itself (may differ from SYSTEM_TMPDIR on macOS)
    # This handles the case where TMPDIR is set but WORK_DIR defaults to /tmp/community-operators-*
    TMP_CANONICAL=""
    if [[ -d "/tmp" ]]; then
        TMP_CANONICAL=$($REALPATH_CMD "/tmp" 2>/dev/null)
    fi
else
    # Dry-run mode without realpath - use literal paths without validation
    TMPDIR_CANONICAL="${SYSTEM_TMPDIR}"
    TMP_CANONICAL="/tmp"
fi

# Early validation before attempting any directory creation
# Reject paths with .. (path traversal) or that don't start with / (relative paths)
if [[ "${WORK_DIR}" == *".."* ]]; then
    log_error "WORK_DIR cannot contain '..' (path traversal): ${WORK_DIR}"
    log_error "This is a security restriction to prevent directory creation outside temp"
    exit 1
fi

if [[ "${WORK_DIR}" != /* ]]; then
    log_error "WORK_DIR must be an absolute path starting with '/': ${WORK_DIR}"
    log_error "Relative paths are not allowed for safety"
    exit 1
fi

# Basic prefix check before attempting mkdir (prevents creating dirs outside temp)
# This is a preliminary check; the canonical path check below is the authoritative validation
# Check against both literal paths and their canonical forms to allow /private/tmp on macOS
PASSES_PREFIX_CHECK=false

if [[ "${WORK_DIR}/" == "${SYSTEM_TMPDIR}/"* ]] || \
   [[ "${WORK_DIR}/" == "/tmp/"* ]] || \
   [[ "${WORK_DIR}/" == "${TMPDIR_CANONICAL}/"* ]] || \
   [[ -n "${TMP_CANONICAL}" && "${WORK_DIR}/" == "${TMP_CANONICAL}/"* ]]; then
    PASSES_PREFIX_CHECK=true
fi

if [[ "${PASSES_PREFIX_CHECK}" == "false" ]]; then
    log_error "WORK_DIR must start with system temp directory"
    log_error "  Computed: ${WORK_DIR}"
    log_error "  Allowed prefixes:"
    log_error "    - ${SYSTEM_TMPDIR}/"
    if [[ "${TMPDIR_CANONICAL}" != "${SYSTEM_TMPDIR}" ]]; then
        log_error "    - ${TMPDIR_CANONICAL}/ (canonical)"
    fi
    log_error "    - /tmp/"
    if [[ -n "${TMP_CANONICAL}" && "${TMP_CANONICAL}" != "/tmp" ]]; then
        log_error "    - ${TMP_CANONICAL}/ (canonical)"
    fi
    log_error ""
    log_error "This check prevents creating directories outside temp before canonicalization"
    exit 1
fi

# Canonicalize WORK_DIR to resolve symlinks and path traversal
# Skip validation in dry-run mode when realpath is not available
if [[ -n "${REALPATH_CMD}" ]]; then
    # Try to use realpath -m to canonicalize even if path doesn't exist
    WORK_DIR_CANONICAL=""
    if WORK_DIR_CANONICAL=$($REALPATH_CMD -m "${WORK_DIR}" 2>/dev/null); then
        # GNU realpath with -m support
        :
    else
        # BSD realpath or -m not supported - create the directory temporarily
        # Safe to do now because we've validated the path doesn't contain .. and has correct prefix
        WORK_DIR_CREATED=false
        if [[ ! -e "${WORK_DIR}" ]]; then
            mkdir -p "${WORK_DIR}" || {
                log_error "Failed to create temporary directory for validation: ${WORK_DIR}"
                exit 1
            }
            WORK_DIR_CREATED=true
        fi

        # Now canonicalize the existing path
        WORK_DIR_CANONICAL=$($REALPATH_CMD "${WORK_DIR}" 2>/dev/null)

        # Remove the directory if we created it temporarily
        if [[ "${WORK_DIR_CREATED}" == "true" ]]; then
            rmdir "${WORK_DIR}" 2>/dev/null || true
        fi
    fi

    if [[ -z "${WORK_DIR_CANONICAL}" ]]; then
        log_error "Failed to canonicalize path: ${WORK_DIR}"
        log_error "This may indicate an invalid path or system issue"
        exit 1
    fi

    # CRITICAL SAFETY CHECK: Reject WORK_DIR if it equals a temp root directory
    # This prevents "rm -rf /tmp" or "rm -rf $TMPDIR" which would wipe the entire temp
    # WORK_DIR must be a subdirectory under temp, not the temp directory itself
    if [[ "${WORK_DIR_CANONICAL}" == "${TMPDIR_CANONICAL}" ]] || \
       [[ -n "${TMP_CANONICAL}" && "${WORK_DIR_CANONICAL}" == "${TMP_CANONICAL}" ]] || \
       [[ "${WORK_DIR_CANONICAL}" == "/tmp" ]]; then
        log_error "WORK_DIR cannot be the temp root directory itself"
        log_error "  Computed:    ${WORK_DIR}"
        log_error "  Resolved to: ${WORK_DIR_CANONICAL}"
        log_error ""
        log_error "This would cause 'rm -rf ${WORK_DIR_CANONICAL}' which would wipe the entire temp directory!"
        log_error ""
        log_error "WORK_DIR must be a subdirectory under temp, for example:"
        log_error "  ${TMPDIR_CANONICAL}/community-operators-${VERSION}"
        log_error ""
        log_error "This error suggests an invalid TMPDIR environment variable."
        log_error "Expected: ${TMPDIR:-/tmp}/community-operators-${VERSION}"
        exit 1
    fi

    # Ensure the canonical WORK_DIR is under an allowed temp directory
    # Accept either TMPDIR_CANONICAL (system temp) or TMP_CANONICAL (/tmp canonical)
    # This handles both Linux (/tmp) and macOS where /tmp→/private/tmp and TMPDIR→/var/folders/
    # Use a slash suffix to ensure we match directory prefix, not just string prefix
    UNDER_TMPDIR=false
    UNDER_TMP=false

    if [[ "${WORK_DIR_CANONICAL}/" == "${TMPDIR_CANONICAL}/"* ]]; then
        UNDER_TMPDIR=true
    fi

    if [[ -n "${TMP_CANONICAL}" ]] && [[ "${WORK_DIR_CANONICAL}/" == "${TMP_CANONICAL}/"* ]]; then
        UNDER_TMP=true
    fi

    if [[ "${UNDER_TMPDIR}" == "false" ]] && [[ "${UNDER_TMP}" == "false" ]]; then
        log_error "WORK_DIR must be under system temp directory for safety"
        log_error "  Computed:      ${WORK_DIR}"
        log_error "  Resolved:      ${WORK_DIR_CANONICAL}"
        log_error "  System temp:   ${SYSTEM_TMPDIR} → ${TMPDIR_CANONICAL}"
        if [[ -n "${TMP_CANONICAL}" ]] && [[ "${TMP_CANONICAL}" != "${TMPDIR_CANONICAL}" ]]; then
            log_error "  /tmp resolved: ${TMP_CANONICAL}"
        fi
        log_error ""
        log_error "Path traversal (../) or symlinks pointing outside temp are not allowed"
        log_error "This prevents accidental deletion of files outside the temp directory"
        exit 1
    fi

    # Use canonical path for all operations
    WORK_DIR="${WORK_DIR_CANONICAL}"
else
    # Dry-run mode without realpath - skip validation, use literal path
    log_warn "Skipping WORK_DIR path validation (dry-run mode without realpath)"
fi

# Check GitHub CLI availability early (before cloning/copying) if we're going to create a PR
# Skip these checks in dry-run mode since gh won't actually be invoked
if [[ "${SKIP_PUSH}" == "false" && "${SKIP_PR}" == "false" && "${DRY_RUN}" == "false" ]]; then
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
if [[ -z "$(find "${ROOT_DIR}/bundle/manifests" -mindepth 1 -print -quit 2>/dev/null)" ]]; then
    log_error "Bundle manifests directory is empty: ${ROOT_DIR}/bundle/manifests"
    log_error "Please run 'make bundle' first"
    exit 1
fi

# Step 1: Clone repository
log_step "Step 1: Cloning community-operators repository"

# Remove existing work directory to ensure fresh clone
if [[ -d "${WORK_DIR}" ]]; then
    if [[ "${DRY_RUN}" == "false" ]]; then
        log_info "Removing existing work directory: ${WORK_DIR}"
        rm -rf "${WORK_DIR}"
    else
        log_info "[DRY RUN] Would remove: ${WORK_DIR}"
    fi
fi

if [[ "${DRY_RUN}" == "false" ]]; then
    log_info "Cloning upstream repository (shallow clone): ${UPSTREAM_REPO}..."
    # Use shallow clone (--depth 1 --single-branch) to speed up cloning
    # We only need current state to add files and create PR, not full history
    git clone --depth 1 --single-branch "${UPSTREAM_REPO}" "${WORK_DIR}" || {
        log_error "Failed to clone repository"
        exit 1
    }

    cd "${WORK_DIR}"

    # Add fork as remote
    if git remote get-url fork &>/dev/null; then
        log_info "Fork remote already exists, updating URL..."
        git remote set-url fork "${FORK_REPO}"
    else
        log_info "Adding fork as remote: ${FORK_REPO}"
        git remote add fork "${FORK_REPO}"
    fi

    # Create new branch from main/master
    log_info "Creating branch: ${BRANCH}"
    if git show-ref --verify --quiet "refs/heads/${BRANCH}"; then
        log_warn "Branch already exists, checking it out"
        git checkout "${BRANCH}"
    else
        git checkout -b "${BRANCH}"
    fi
else
    log_info "[DRY RUN] Would clone (shallow): ${UPSTREAM_REPO} to ${WORK_DIR}"
    log_info "[DRY RUN]   (using --depth 1 --single-branch for faster cloning)"
    log_info "[DRY RUN] Would add fork remote: ${FORK_REPO}"
    log_info "[DRY RUN] Would create branch: ${BRANCH}"
fi
echo ""

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
    # Use parameter expansion to strip WORK_DIR prefix (safer than sed with regex metacharacters)
    find "${DEST_DIR}" -type f | while read -r file; do echo "${file#${WORK_DIR}/}"; done | sort
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

# Step 3: Create git commit
HAS_CHANGES=true

log_step "Step 3: Creating git commit"

if [[ "${DRY_RUN}" == "false" ]]; then
    cd "${WORK_DIR}"

    # Stage the changes first
    log_info "Staging changes..."
    git add "${CATALOG_PATH}/trustee-operator/${TARGET_RELEASE}"

    # Check if there are any staged changes
    if git diff --cached --quiet; then
        log_warn "No changes to commit"
        log_warn "Bundle for version ${TARGET_RELEASE} already exists in upstream repository"
        HAS_CHANGES=false
    else
        log_info "Creating commit..."
        COMMIT_MSG="operator trustee-operator (${TARGET_RELEASE})

Add trustee-operator version ${TARGET_RELEASE}"

        git commit -m "${COMMIT_MSG}" || {
            log_error "Failed to create commit"
            exit 1
        }

        log_info "Commit created successfully"
        git log -1 --oneline
    fi
else
    log_info "[DRY RUN] Would create commit with bundle changes"
fi
echo ""

# Step 4: Push to fork and create PR
if [[ "${SKIP_PUSH}" == "false" && "${HAS_CHANGES}" == "true" ]]; then
    if [[ "${SKIP_PR}" == "false" ]]; then
        log_step "Step 4: Pushing to fork and creating PR"
    else
        log_step "Step 4: Pushing to fork"
    fi

    # Extract and validate fork owner and upstream repo for both dry-run and actual execution
    # This ensures dry-run output matches actual command
    if [[ "${SKIP_PR}" == "false" ]]; then
        # Extract fork owner from fork URL
        FORK_OWNER=$(echo "${FORK_REPO}" | sed -E 's|.*github\.com[:/]([^/]+)/.*|\1|')

        # Get upstream repo (should be k8s-operatorhub/community-operators)
        UPSTREAM_REPO_NORMALIZED=$(echo "${UPSTREAM_REPO}" | sed -E 's|.*github\.com[:/]([^/]+/[^/]+)(\.git)?$|\1|' | sed 's/\.git$//')

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
            log_error "Expected a GitHub URL like: git@github.com:username/community-operators.git"
            log_error "Extracted owner: '${FORK_OWNER}' (invalid)"
            log_error "You can create the PR manually at GitHub"
            exit 1
        fi

        # Validate UPSTREAM_REPO_NORMALIZED extraction
        # Should be exactly "owner/repo" format (one slash, no URL artifacts)
        if [[ -z "${UPSTREAM_REPO_NORMALIZED}" ]] || \
           [[ "${UPSTREAM_REPO_NORMALIZED}" != *"/"* ]] || \
           [[ "${UPSTREAM_REPO_NORMALIZED}" == *":"* ]] || \
           [[ "${UPSTREAM_REPO_NORMALIZED}" == *"@"* ]] || \
           [[ "${UPSTREAM_REPO_NORMALIZED}" == *"://"* ]] || \
           [[ "${UPSTREAM_REPO_NORMALIZED}" == *".git"* ]] || \
           [[ "${UPSTREAM_REPO_NORMALIZED}" == *"github.com"* ]] || \
           [[ $(echo "${UPSTREAM_REPO_NORMALIZED}" | grep -o "/" | wc -l) -ne 1 ]]; then
            log_error "Failed to extract upstream repo from: ${UPSTREAM_REPO}"
            log_error "Expected a GitHub URL like: git@github.com:k8s-operatorhub/community-operators.git"
            log_error "Extracted repo: '${UPSTREAM_REPO_NORMALIZED}' (invalid)"
            log_error "You can create the PR manually at GitHub"
            exit 1
        fi
    fi

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY RUN] Would run: cd ${WORK_DIR} && git push -u fork ${BRANCH}"
        if [[ "${SKIP_PR}" == "false" ]]; then
            log_info "[DRY RUN] Would run: gh pr create --repo \"${UPSTREAM_REPO_NORMALIZED}\" --base main --head \"${FORK_OWNER}:${BRANCH}\" --title \"operator trustee-operator (${TARGET_RELEASE})\" --body '...'"
            log_info "[DRY RUN]   (PR body includes operator information, testing details, and checklist)"
        fi
    else
        cd "${WORK_DIR}"

        log_info "Pushing branch to fork..."
        git push -u fork "${BRANCH}" || {
            log_error "Git push to fork failed."
            exit 1
        }
        log_info "Branch ${BRANCH} pushed to fork"

        if [[ "${SKIP_PR}" == "false" ]]; then
            log_info "Creating pull request..."
            gh pr create \
                --repo "${UPSTREAM_REPO_NORMALIZED}" \
                --base main \
                --head "${FORK_OWNER}:${BRANCH}" \
                --title "operator trustee-operator (${TARGET_RELEASE})" \
                --body "$(cat <<EOF
# New Operator Submission

## Operator Information
- **Operator Name**: trustee-operator
- **Version**: ${TARGET_RELEASE}
- **Catalog**: ${CATALOG}

## Description
Trustee Operator manages the deployment and lifecycle of Trustee components for Confidential Containers.

## Testing
- Bundle validation: \`operator-sdk bundle validate operators/trustee-operator/${TARGET_RELEASE}\`
- Scorecard tests included in bundle

## Checklist
- [x] Bundle files follow community-operators guidelines
- [x] Operator metadata is complete
- [x] Version follows semantic versioning
- [x] ClusterServiceVersion replaces field points to previous version

🤖 Generated with automated release scripts
EOF
)" || {
                log_error "Failed to create pull request"
                log_error "You can create it manually at:"
                log_error "  https://github.com/${UPSTREAM_REPO_NORMALIZED}/compare/main...${FORK_OWNER}:${BRANCH}"
                exit 1
            }
            log_info "Pull request created successfully"
        else
            log_warn "Skipping PR creation (--skip-pr)"
            FORK_OWNER=$(echo "${FORK_REPO}" | sed -E 's|.*github\.com[:/]([^/]+)/.*|\1|')
            UPSTREAM_REPO_NORMALIZED=$(echo "${UPSTREAM_REPO}" | sed -E 's|.*github\.com[:/]([^/]+/[^/]+)(\.git)?$|\1|' | sed 's/\.git$//')
            log_warn "You can create PR manually at:"
            log_warn "  https://github.com/${UPSTREAM_REPO_NORMALIZED}/compare/main...${FORK_OWNER}:${BRANCH}"
        fi
    fi
    echo ""
elif [[ "${SKIP_PUSH}" == "false" && "${HAS_CHANGES}" == "false" ]]; then
    log_warn "Skipping push to fork and PR creation - no changes to submit"
    log_info "Version ${TARGET_RELEASE} already exists in upstream repository"
    echo ""
else
    log_warn "Skipping push to fork and PR creation (--skip-push)"
    log_warn "You can push and create PR manually:"
    log_warn "  cd ${WORK_DIR}"
    log_warn "  git push fork ${BRANCH}"
    log_warn "  gh pr create --base main"
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
    if [[ "${HAS_CHANGES}" == "false" ]]; then
        log_info "Version ${TARGET_RELEASE} already exists in upstream community-operators"
        log_info "No submission needed - this version has already been published to OperatorHub"
    elif [[ "${SKIP_PUSH}" == "false" ]]; then
        if [[ "${SKIP_PR}" == "false" ]]; then
            log_info "✓ Pull request created to community-operators"
            echo ""
            log_info "Next steps:"
            log_info "  1. Monitor the PR and respond to reviews"
            log_info "  2. Wait for CI checks to pass"
            log_info "  3. Follow up with community-operators maintainers"
            log_info ""
            log_info "Testing (optional):"
            log_info "  cd ${WORK_DIR}"
            log_info "  operator-sdk bundle validate ${DEST_DIR}"
        else
            log_info "✓ Branch pushed to fork: ${BRANCH}"
            echo ""
            log_info "Next steps:"
            log_info "  1. Test the bundle (optional):"
            log_info "     cd ${WORK_DIR}"
            log_info "     operator-sdk bundle validate ${DEST_DIR}"
            log_info ""
            log_info "  2. Create pull request: gh pr create --base main"
        fi
    else
        log_info "Next steps:"
        log_info "  1. Review the changes:"
        log_info "     cd ${WORK_DIR}"
        log_info "     git status"
        log_info ""
        log_info "  2. Test the operator bundle (optional):"
        log_info "     operator-sdk bundle validate ${DEST_DIR}"
        log_info ""
        log_info "  3. Push the branch to your fork:"
        log_info "     cd ${WORK_DIR}"
        log_info "     git push fork ${BRANCH}"
        log_info ""
        log_info "  4. Create a Pull Request:"
        FORK_USER=$(echo "${FORK_REPO}" | sed -E 's|.*github\.com[:/]([^/]+)/.*|\1|')
        UPSTREAM_REPO_NORMALIZED=$(echo "${UPSTREAM_REPO}" | sed -E 's|.*github\.com[:/]([^/]+/[^/]+)(\.git)?$|\1|' | sed 's/\.git$//')
        log_info "     Go to: https://github.com/${UPSTREAM_REPO_NORMALIZED}/compare"
        log_info "     Base: ${UPSTREAM_REPO_NORMALIZED}:main"
        log_info "     Compare: ${FORK_USER}:${BRANCH}"
        log_info ""
        log_info "  5. Follow the PR template and community-operators guidelines"
    fi
else
    log_info "This was a dry run. No changes were made."
    log_info "Run without --dry-run to perform the actual operation."
fi

echo ""
