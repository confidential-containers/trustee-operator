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
NC='\033[0m' # No Color

usage() {
    cat <<EOF
Usage: $0 [options]

Rolls back version changes made by bump-version.sh using git.

Options:
  --hard    Hard reset - discard all changes including untracked files (DESTRUCTIVE)
  -h, --help    Show this help message

Examples:
  $0              # Revert modified files, keep untracked files
  $0 --hard       # Discard all changes including untracked files

This script will:
  - Revert all tracked files to their last committed state
  - Optionally remove untracked files (with --hard)
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

# Parse arguments
HARD_RESET=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --hard)
            HARD_RESET=true
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

cd "${ROOT_DIR}"

# Check if we're in a git repository
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    log_error "Not in a git repository"
    exit 1
fi

# Show current changes
log_info "Current changes:"
echo ""
git status --short

# Check if there are any changes to rollback
if git diff-index --quiet HEAD -- 2>/dev/null && [[ -z "$(git ls-files --others --exclude-standard)" ]]; then
    log_warn "No changes to rollback. Working directory is clean."
    exit 0
fi

echo ""
if [[ "${HARD_RESET}" == "true" ]]; then
    log_warn "WARNING: This will discard ALL changes including untracked files!"
    read -p "Are you sure you want to continue? [y/N] " -n 1 -r
else
    read -p "Do you want to rollback changes to tracked files? [y/N] " -n 1 -r
fi
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log_info "Aborted by user"
    exit 0
fi

log_info "Rolling back changes..."
echo ""

# Revert tracked files
log_info "Reverting modified tracked files..."
git checkout -- .

if [[ "${HARD_RESET}" == "true" ]]; then
    # Remove untracked files
    log_info "Removing untracked files..."
    git clean -fd
fi

log_info ""
log_info "Rollback complete!"
echo ""
git status --short
