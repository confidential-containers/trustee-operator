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

# Common functions and variables for release automation scripts
# Source this file in other scripts: source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

# Ensure this file is sourced, not executed
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    echo "Error: common.sh should be sourced, not executed directly"
    exit 1
fi

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*" >&2
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

log_step() {
    echo -e "${BLUE}==>${NC} $*"
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

# Validate version format (X.Y.Z)
validate_version() {
    local version="$1"
    if ! [[ "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        log_error "Invalid version format: ${version}"
        log_error "Version must be in format X.Y.Z (e.g., 0.18.0)"
        return 1
    fi
    return 0
}

# Get version from Makefile
# Safe to use with set -u even if ROOT_DIR is unbound
get_version_from_makefile() {
    local root_dir="${1:-${ROOT_DIR:-$COMMON_ROOT_DIR}}"
    local version
    version=$(awk '/^VERSION \?=/ {print $3}' "${root_dir}/Makefile" || echo "")
    if [[ -z "${version}" ]]; then
        log_error "Could not determine version from Makefile"
        return 1
    fi
    echo "${version}"
}

# Remove duplicate image entries from kustomization.yaml
# Kustomize sometimes adds duplicate image entries; this cleans them up
remove_duplicate_kustomization_images() {
    local root_dir="${1:-${ROOT_DIR:-$COMMON_ROOT_DIR}}"
    local kustomization_file="${root_dir}/config/manager/kustomization.yaml"

    # Remove all image entries except the first "controller" entry
    # Kustomize may add duplicates with name: controller or name: <full-image-path>
    local total_images
    total_images=$(grep -c "^[[:space:]]*- name:" "${kustomization_file}" 2>/dev/null || echo "0")

    if [[ "${total_images}" -gt 1 ]]; then
        log_info "Removing duplicate image entries from config/manager/kustomization.yaml"
        # Keep only the first image entry (name: controller)
        awk '
            /^[[:space:]]*-[[:space:]]+name:/ {
                in_image++
                if (in_image > 1) {
                    skip = 1
                    next
                }
            }
            skip && /^[[:space:]]+(newName|newTag):/ {
                next
            }
            skip && /^[[:space:]]*$/ {
                skip = 0
                next
            }
            skip && /^[^[:space:]]/ {
                skip = 0
            }
            !skip
        ' "${kustomization_file}" > "${kustomization_file}.tmp" && \
        mv "${kustomization_file}.tmp" "${kustomization_file}"
    fi
}

# Common directory setup
# These are set relative to common.sh (BASH_SOURCE[0] refers to this file when sourced)
COMMON_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMON_ROOT_DIR="$(cd "${COMMON_SCRIPT_DIR}/../.." && pwd)"
