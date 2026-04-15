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

# Fix Go version mismatch issues by cleaning the build cache

set -euo pipefail

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_info "Checking Go version..."
go version

log_info ""
log_info "Cleaning Go build cache and test cache to fix version mismatches..."
go clean -cache -testcache

log_info ""
log_info "Rebuilding Go toolchain..."
go install std

log_info ""
log_info "Cleaning Go module cache (optional, may take time to rebuild)..."
read -p "Do you want to clean the module cache as well? This will require re-downloading modules. [y/N] " -n 1 -r
echo ""
if [[ $REPLY =~ ^[Yy]$ ]]; then
    go clean -modcache
    log_info "Module cache cleaned"
else
    log_info "Skipped module cache cleaning"
fi

log_info ""
log_info "Go cache cleaned successfully!"
log_info "You can now run 'make test' or 'make docker-build' again."
