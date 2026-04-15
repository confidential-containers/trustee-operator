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

# Complete end-to-end release script
# Combines version bumping, git operations, and image building

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
Prepares version, bundle files, and creates pull request. Images are built by GitHub Actions.

Arguments:
  new-version    The new version to release (e.g., 0.18.0)

Required Options:
  --fork FORK           Your fork URL to push to (e.g., git@github.com:username/trustee-operator.git)

Optional Options:
  --branch BRANCH       Branch name to create (default: release-v\${VERSION})
  --skip-tests          Skip running tests
  --skip-push           Skip git push and PR creation (for testing)
  --dry-run             Show what would be done without making changes

Examples:
  # Full release preparation
  $0 0.18.0 --fork git@github.com:lmilleri/trustee-operator.git

  # Dry run to see what would happen
  $0 0.18.0 --fork git@github.com:lmilleri/trustee-operator.git --dry-run

  # Prepare release without pushing (testing)
  $0 0.18.0 --fork git@github.com:lmilleri/trustee-operator.git --skip-push

This script performs the following steps:
  1. Run tests
  2. Bump version
  3. Regenerate bundle files (for community-operators submission)
  4. Commit changes
  5. Push to fork
  6. Create pull request to main

After PR is merged:
  - Create GitHub release (tag v{VERSION})
  - GitHub Actions will automatically build and push multi-arch operator image
  - Submit bundle files to community-operators using prepare-community-operators.sh
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

if [[ "${DRY_RUN}" == "true" ]]; then
    log_warn "DRY RUN MODE - No changes will be made"
    echo ""
fi

log_info "================================================================"
log_info "Release Preparation for v${NEW_VERSION}"
log_info "================================================================"
if [[ "${SKIP_PUSH}" == "true" ]]; then
    log_warn "Git push disabled (--skip-push)"
fi
echo ""

RELEASE_OPTS="--fork ${FORK_REPO}"
[[ -n "${BRANCH}" ]] && RELEASE_OPTS="${RELEASE_OPTS} --branch ${BRANCH}"
[[ "${SKIP_TESTS}" == "true" ]] && RELEASE_OPTS="${RELEASE_OPTS} --skip-tests"
[[ "${SKIP_PUSH}" == "true" ]] && RELEASE_OPTS="${RELEASE_OPTS} --skip-push"
[[ "${DRY_RUN}" == "true" ]] && RELEASE_OPTS="${RELEASE_OPTS} --dry-run"

if [[ "${DRY_RUN}" == "true" ]]; then
    log_info "[DRY RUN] Would run: ${SCRIPT_DIR}/release.sh ${NEW_VERSION} ${RELEASE_OPTS}"
else
    bash "${SCRIPT_DIR}/release.sh" ${NEW_VERSION} ${RELEASE_OPTS} || {
        log_error "Release script failed"
        exit 1
    }
fi

echo ""

# Summary
log_info "================================================================"
log_info "Release Preparation Complete!"
log_info "================================================================"
log_info ""
log_info "Version: ${NEW_VERSION}"
log_info ""

if [[ "${DRY_RUN}" == "false" ]]; then
    log_info "What was done:"
    log_info "  ✓ Version bumped to ${NEW_VERSION}"
    log_info "  ✓ Bundle files generated in: bundle/"
    log_info "    - manifests/"
    log_info "    - metadata/"
    log_info "    - tests/"
    log_info "  ✓ Changes committed to git branch: ${BRANCH}"
    if [[ "${SKIP_PUSH}" == "false" ]]; then
        log_info "  ✓ Branch pushed to fork"
        log_info "  ✓ Pull request created"
    fi
    echo ""
    log_info "Next steps:"
    if [[ "${SKIP_PUSH}" == "false" ]]; then
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
