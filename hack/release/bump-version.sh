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
Usage: $0 <new-version>

Bumps the version of trustee-operator from the current version to the specified new version.

Arguments:
  new-version    The new version to set (e.g., 0.18.0)

Examples:
  $0 0.18.0
  $0 1.0.0

This script will update version references in:
  - Makefile
  - bundle/manifests/trustee-operator.clusterserviceversion.yaml (including replaces field)
  - config/manager/manager.yaml
  - config/manager/kustomization.yaml
  - config/manifests/bases/trustee-operator.clusterserviceversion.yaml (including replaces field)
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

if [[ $# -ne 1 ]]; then
    usage
    exit 1
fi

NEW_VERSION="$1"

# Validate version format (X.Y.Z)
if ! [[ "${NEW_VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    log_error "Invalid version format: ${NEW_VERSION}"
    log_error "Version must be in format X.Y.Z (e.g., 0.18.0)"
    exit 1
fi

# Extract current version from Makefile
CURRENT_VERSION=$(awk '/^VERSION \?=/ {print $3}' "${ROOT_DIR}/Makefile")
if [[ -z "${CURRENT_VERSION}" ]]; then
    log_error "Could not extract current version from Makefile"
    exit 1
fi

log_info "Current version: ${CURRENT_VERSION}"
log_info "New version: ${NEW_VERSION}"

if [[ "${CURRENT_VERSION}" == "${NEW_VERSION}" ]]; then
    log_warn "Current version and new version are the same. Nothing to do."
    exit 0
fi

# Escape dots in version strings for use in regex patterns
# In sed -E, . matches any character, so we need to escape it to match literal dots
# Only the search pattern (left side of s///) needs escaping; replacement string does not
CURRENT_VERSION_ESCAPED="${CURRENT_VERSION//./\\.}"

# Confirm with user
echo ""
read -p "Do you want to proceed with version bump from ${CURRENT_VERSION} to ${NEW_VERSION}? [y/N] " -n 1 -r
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log_info "Aborted by user"
    exit 0
fi

log_info "Updating version references..."

# Update version references in files
log_info "Updating files..."

# Update Makefile - VERSION variable and image tags
sed_inplace "s/^VERSION \?= ${CURRENT_VERSION_ESCAPED}/VERSION \?= ${NEW_VERSION}/" "${ROOT_DIR}/Makefile"
sed_inplace "s/:built-in-as-v${CURRENT_VERSION_ESCAPED}/:built-in-as-v${NEW_VERSION}/g" "${ROOT_DIR}/Makefile"
sed_inplace "s/:v${CURRENT_VERSION_ESCAPED}/:v${NEW_VERSION}/g" "${ROOT_DIR}/Makefile"
log_info "  Updated Makefile"

# Update bundle manifests
if [[ -f "${ROOT_DIR}/bundle/manifests/trustee-operator.clusterserviceversion.yaml" ]]; then
    sed_inplace "s/:v${CURRENT_VERSION_ESCAPED}/:v${NEW_VERSION}/g" "${ROOT_DIR}/bundle/manifests/trustee-operator.clusterserviceversion.yaml"
    sed_inplace "s/:built-in-as-v${CURRENT_VERSION_ESCAPED}/:built-in-as-v${NEW_VERSION}/g" "${ROOT_DIR}/bundle/manifests/trustee-operator.clusterserviceversion.yaml"
    sed_inplace "s/trustee-operator\\.v${CURRENT_VERSION_ESCAPED}/trustee-operator.v${NEW_VERSION}/g" "${ROOT_DIR}/bundle/manifests/trustee-operator.clusterserviceversion.yaml"
    sed_inplace "s/version: ${CURRENT_VERSION_ESCAPED}/version: ${NEW_VERSION}/" "${ROOT_DIR}/bundle/manifests/trustee-operator.clusterserviceversion.yaml"
    # Update replaces field to point to the current version (the version we're replacing)
    sed_inplace "s/replaces: trustee-operator\\.v[0-9]+\\.[0-9]+\\.[0-9]+/replaces: trustee-operator.v${CURRENT_VERSION}/" "${ROOT_DIR}/bundle/manifests/trustee-operator.clusterserviceversion.yaml"
    log_info "  Updated bundle/manifests/trustee-operator.clusterserviceversion.yaml"
fi

# Update config/manager/manager.yaml
if [[ -f "${ROOT_DIR}/config/manager/manager.yaml" ]]; then
    sed_inplace "s/:v${CURRENT_VERSION_ESCAPED}/:v${NEW_VERSION}/g" "${ROOT_DIR}/config/manager/manager.yaml"
    sed_inplace "s/:built-in-as-v${CURRENT_VERSION_ESCAPED}/:built-in-as-v${NEW_VERSION}/g" "${ROOT_DIR}/config/manager/manager.yaml"
    log_info "  Updated config/manager/manager.yaml"
fi

# Update config/manager/kustomization.yaml
if [[ -f "${ROOT_DIR}/config/manager/kustomization.yaml" ]]; then
    sed_inplace "s/newTag: v${CURRENT_VERSION_ESCAPED}/newTag: v${NEW_VERSION}/" "${ROOT_DIR}/config/manager/kustomization.yaml"
    log_info "  Updated config/manager/kustomization.yaml"
fi

# Update config/manifests/bases/trustee-operator.clusterserviceversion.yaml
if [[ -f "${ROOT_DIR}/config/manifests/bases/trustee-operator.clusterserviceversion.yaml" ]]; then
    sed_inplace "s/:v${CURRENT_VERSION_ESCAPED}/:v${NEW_VERSION}/g" "${ROOT_DIR}/config/manifests/bases/trustee-operator.clusterserviceversion.yaml"
    sed_inplace "s/trustee-operator\\.v${CURRENT_VERSION_ESCAPED}/trustee-operator.v${NEW_VERSION}/g" "${ROOT_DIR}/config/manifests/bases/trustee-operator.clusterserviceversion.yaml"
    sed_inplace "s/version: ${CURRENT_VERSION_ESCAPED}/version: ${NEW_VERSION}/" "${ROOT_DIR}/config/manifests/bases/trustee-operator.clusterserviceversion.yaml"
    # Update replaces field to point to the current version (the version we're replacing)
    sed_inplace "s/replaces: trustee-operator\\.v[0-9]+\\.[0-9]+\\.[0-9]+/replaces: trustee-operator.v${CURRENT_VERSION}/" "${ROOT_DIR}/config/manifests/bases/trustee-operator.clusterserviceversion.yaml"
    log_info "  Updated config/manifests/bases/trustee-operator.clusterserviceversion.yaml"
fi

log_info ""
log_info "Version bump complete!"
log_info ""
log_info "Summary of changes:"
git diff --stat 2>/dev/null || log_warn "Git not available or not a git repository"

log_info ""
log_info "Next steps - Use the automated release workflow:"
log_info "  ${SCRIPT_DIR}/release.sh ${NEW_VERSION}     # For version bump + git operations"
log_info "  ${SCRIPT_DIR}/do-release.sh ${NEW_VERSION}  # For complete release with images"
