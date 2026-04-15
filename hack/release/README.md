# Release Automation Scripts

This directory contains scripts to automate the release process for trustee-operator.

## Quick Start

For a complete release, use the all-in-one script:

```bash
./hack/release/do-release.sh 0.18.0 \
  --fork git@github.com:yourusername/trustee-operator.git --dry-run  # Verify first
./hack/release/do-release.sh 0.18.0 \
  --fork git@github.com:yourusername/trustee-operator.git            # Execute release
```

## Script Overview

| Script | Purpose | When to Use |
|--------|---------|-------------|
| `do-release.sh` ⭐ | Complete end-to-end release | **Most releases** - handles everything automatically |
| `release.sh` | Version bump + git operations only | When you want to build images separately |
| `build-and-push.sh` | Build and push images (advanced) | Granular control over image building |
| `build-images.sh` | Build and push images (simple) | Quick image rebuild after manual changes |
| `bump-version.sh` | Version bump only | Manual workflow or when testing version changes |
| `prepare-community-operators.sh` | Prepare bundle for community-operators | After release, to submit to OperatorHub |

## Scripts

### do-release.sh

**⭐ Recommended** - Complete end-to-end release automation.

Prepares version, bundle files, and creates pull request. Images are built automatically by GitHub Actions after PR is merged and release is tagged.

**Usage:**
```bash
./hack/release/do-release.sh <new-version> --fork FORK [options]
```

**Required Options:**
- `--fork FORK` - Your fork URL to push to (e.g., `git@github.com:username/trustee-operator.git`)

**Optional Options:**
- `--branch BRANCH` - Branch name to create (default: `release-vX.Y.Z`)
- `--skip-tests` - Skip running tests
- `--skip-push` - Skip git push and PR creation (for testing)
- `--skip-pr` - Skip creating PR (but still push to fork)
- `--dry-run` - Show what would be done without making changes

**Examples:**
```bash
# Complete release preparation (recommended)
./hack/release/do-release.sh 0.18.0 --fork git@github.com:lmilleri/trustee-operator.git

# Dry run first to verify
./hack/release/do-release.sh 0.18.0 --fork git@github.com:lmilleri/trustee-operator.git --dry-run

# Prepare release without pushing
./hack/release/do-release.sh 0.18.0 --fork git@github.com:lmilleri/trustee-operator.git --skip-push

# Push to fork but skip PR creation
./hack/release/do-release.sh 0.18.0 --fork git@github.com:lmilleri/trustee-operator.git --skip-pr
```

**What it does:**
1. Run tests
2. Bump version in all files
3. Regenerate bundle manifests
4. Commit changes to git
5. Push to your fork
6. Create pull request to main

**After PR is merged:**
- Create GitHub release with tag `vX.Y.Z`
- GitHub Actions will automatically build and push multi-arch operator image
- Submit bundle files to community-operators using `prepare-community-operators.sh`

### bump-version.sh

Bumps the version across all relevant files in the repository.

**Usage:**
```bash
./hack/release/bump-version.sh <new-version>
```

**Example:**
```bash
./hack/release/bump-version.sh 0.18.0
```

**What it does:**
- Updates VERSION in Makefile
- Updates image tags in all manifests (Makefile, bundle/, config/)
- Updates CSV (ClusterServiceVersion) files and `replaces` field
- Shows summary of changes

### release.sh

Complete automated release process with multiple steps.

**Usage:**
```bash
./hack/release/release.sh <new-version> --fork FORK [options]
```

**Required Options:**
- `--fork FORK` - Your fork URL to push to (e.g., `git@github.com:username/trustee-operator.git`)

**Optional Options:**
- `--branch BRANCH` - Branch name to create (default: `release-vX.Y.Z`)
- `--skip-tests` - Skip running tests
- `--skip-bundle` - Skip bundle regeneration
- `--skip-commit` - Skip creating git commit
- `--skip-push` - Skip pushing to fork and creating PR
- `--skip-pr` - Skip creating PR (but still push to fork)
- `--dry-run` - Show what would be done without making changes

**Examples:**
```bash
# Full release
./hack/release/release.sh 0.18.0 --fork git@github.com:lmilleri/trustee-operator.git

# Dry run to see what would happen
./hack/release/release.sh 0.18.0 --fork git@github.com:lmilleri/trustee-operator.git --dry-run

# Release without running tests
./hack/release/release.sh 0.18.0 --fork git@github.com:lmilleri/trustee-operator.git --skip-tests

# Prepare release but don't push
./hack/release/release.sh 0.18.0 --fork git@github.com:lmilleri/trustee-operator.git --skip-push

# Push to fork but skip PR creation
./hack/release/release.sh 0.18.0 --fork git@github.com:lmilleri/trustee-operator.git --skip-pr
```

**What it does:**
1. Validates git working directory is clean
2. Runs tests (unless `--skip-tests`)
3. Bumps version using `bump-version.sh`
4. Regenerates bundle manifests (unless `--skip-bundle`)
5. Creates git commit (unless `--skip-commit`)
6. Pushes to your fork (unless `--skip-push`)
7. Creates pull request to main (unless `--skip-pr` or `--skip-push`)

### prepare-community-operators.sh

Prepares the operator bundle for submission to the community-operators catalog.

**Usage:**
```bash
./hack/release/prepare-community-operators.sh [version] --fork FORK [options]
```

**Required Options:**
- `--fork FORK` - **REQUIRED** Your fork URL to push to (e.g., `git@github.com:username/community-operators.git`)

**Optional Options:**
- `--upstream REPO` - Upstream repository URL (default: `git@github.com:k8s-operatorhub/community-operators.git`)
- `--branch BRANCH` - Branch name to create (default: `trustee-operator-vX.Y.Z`)
- `--catalog TYPE` - Catalog type: `community` or `upstream` (default: `community`)
- `--work-dir DIR` - Working directory (default: `${TMPDIR:-/tmp}/community-operators-X.Y.Z`)
  - **Note:** Must be under system temp directory for safety (enforced before `rm -rf`)
  - Uses `$TMPDIR` if set (common on macOS, e.g., `/var/folders/...`), otherwise `/tmp`
- `--skip-clone` - Use existing cloned repository
- `--skip-commit` - Don't create git commit
- `--skip-push` - Skip pushing to fork and creating PR
- `--skip-pr` - Skip creating PR (but still push to fork)
- `--dry-run` - Show what would be done without making changes

**Examples:**
```bash
# Complete automated submission with PR (RECOMMENDED)
./hack/release/prepare-community-operators.sh 0.18.0 \
  --fork git@github.com:lmilleri/community-operators.git

# Dry run first to verify
./hack/release/prepare-community-operators.sh 0.18.0 \
  --fork git@github.com:myuser/community-operators.git --dry-run

# Push to fork but skip PR creation
./hack/release/prepare-community-operators.sh 0.18.0 \
  --fork git@github.com:myuser/community-operators.git --skip-pr

# Prepare files only (no push/PR)
./hack/release/prepare-community-operators.sh 0.18.0 \
  --fork git@github.com:myuser/community-operators.git --skip-push

# Use existing cloned directory
./hack/release/prepare-community-operators.sh 0.18.0 \
  --fork git@github.com:myuser/community-operators.git \
  --skip-clone --work-dir /tmp/my-operators
```

**What it does:**
1. Removes existing working directory (if present) to ensure fresh clone
2. Clones upstream community-operators repository
3. Adds your fork as 'fork' remote
4. Creates a new branch (e.g., `trustee-operator-v0.18.0`)
5. Copies bundle files to `operators/trustee-operator/X.Y.Z/`:
   - `bundle/manifests/` - Operator manifests and CSV
   - `bundle/metadata/` - Bundle metadata
   - `bundle/tests/` - Scorecard test configuration
   - Note: `bundle.Dockerfile` is NOT copied (auto-generated by community-operators)
6. Creates git commit with the changes
7. Pushes to your fork (unless `--skip-push`)
8. Creates pull request to upstream (unless `--skip-pr` or `--skip-push`)

**After running:**
- If PR was created automatically: Monitor the PR, respond to reviews, wait for CI checks
- If using `--skip-pr`: Create PR manually with `gh pr create --base main`
- If using `--skip-push`: Push manually and then create PR
- Optional: Test the bundle with `operator-sdk bundle validate <path>`

### build-images.sh

Simple script for building and pushing multi-arch operator images (for manual testing).

**Note:** This is not part of the standard release workflow. GitHub Actions builds images automatically when you create a GitHub release.

**Usage:**
```bash
./hack/release/build-images.sh [version] [options]
```

**Options:**
- `--registry REGISTRY` - Container registry (default: `quay.io/confidential-containers`)
- `--platforms PLATFORMS` - Target platforms (default: `linux/amd64,linux/arm64,linux/s390x`)

**Examples:**
```bash
# Build using version from Makefile
./hack/release/build-images.sh

# Build specific version
./hack/release/build-images.sh 0.18.0

# Build with custom registry
./hack/release/build-images.sh 0.18.0 --registry myregistry.io/myorg

# Build for specific platforms
./hack/release/build-images.sh 0.18.0 --platforms linux/amd64,linux/arm64
```

**What it does:**
1. Generates manifests and bundle
2. Cleans up duplicate kustomization.yaml entries
3. Runs tests
4. Builds and pushes multi-arch operator image (linux/amd64, linux/arm64, linux/s390x)

**Note:** Bundle/catalog images are not built. Only bundle files (in bundle/ directory) are submitted to community-operators.

### build-and-push.sh

Advanced build and push script with granular control over each step (for manual testing).

**Note:** This is not part of the standard release workflow. GitHub Actions builds images automatically when you create a GitHub release.

**Usage:**
```bash
./hack/release/build-and-push.sh [version] [options]
```

**Options:**
- `--registry REGISTRY` - Container registry
- `--skip-manifests` - Skip generating manifests
- `--skip-bundle` - Skip generating bundle files
- `--skip-docker-build` - Skip building operator image
- `--skip-docker-push` - Skip pushing operator image
- `--docker-buildx` - Use docker buildx for multi-platform builds
- `--platforms PLATFORMS` - Platforms for buildx (default: `linux/amd64,linux/arm64,linux/s390x`)
- `--dry-run` - Show what would be done without making changes

**Examples:**
```bash
# Full build and push
./hack/release/build-and-push.sh 0.18.0

# Dry run first
./hack/release/build-and-push.sh 0.18.0 --dry-run

# Multi-platform build
./hack/release/build-and-push.sh 0.18.0 --docker-buildx

# Build only (no push)
./hack/release/build-and-push.sh 0.18.0 --skip-docker-push

# Custom registry
./hack/release/build-and-push.sh 0.18.0 --registry myregistry.io/myorg
```

**What it does:**
1. Validates version and configuration
2. Generates manifests (unless skipped)
3. Generates bundle files (unless skipped)
4. Cleans up duplicate kustomization.yaml entries
5. Runs tests
6. Builds and pushes multi-arch operator image (unless skipped)

**Note:** Bundle/catalog images are not built. Community-operators uses bundle files only.

## Release Workflow

### Recommended: Complete Automated Release

**For most releases, use the all-in-one `do-release.sh` script:**

```bash
# 1. Ensure you're on main branch and it's up to date
git checkout main
git pull

# 2. Run the complete release (dry-run first recommended)
./hack/release/do-release.sh 0.18.0 \
  --fork git@github.com:yourusername/trustee-operator.git --dry-run
./hack/release/do-release.sh 0.18.0 \
  --fork git@github.com:yourusername/trustee-operator.git

# 3. Merge the PR that was created

# 4. Create GitHub release
# Go to https://github.com/confidential-containers/trustee-operator/releases/new
# - Tag: v0.18.0
# - Title: Release v0.18.0
# - Description: Add changelog and release notes
# GitHub Actions will automatically build and push multi-arch operator image

# 5. Submit to community-operators (OperatorHub)
./hack/release/prepare-community-operators.sh 0.18.0 \
  --fork git@github.com:yourusername/community-operators.git
```

This handles version bumping, bundle generation, and PR creation. Images are built by GitHub Actions after the release is tagged.

### Alternative: Step-by-Step Release

For more control, use the `release.sh` script:

```bash
# 1. Ensure you're on main branch and it's up to date
git checkout main
git pull

# 2. Run the release script (optionally with --dry-run first)
./hack/release/release.sh 0.18.0 \
  --fork git@github.com:yourusername/trustee-operator.git --dry-run
./hack/release/release.sh 0.18.0 \
  --fork git@github.com:yourusername/trustee-operator.git

# 3. Merge the PR that was created

# 4. Create GitHub release
# Go to https://github.com/confidential-containers/trustee-operator/releases/new
# - Tag: v0.18.0
# - Title: Release v0.18.0
# - Description: Add changelog and release notes
# GitHub Actions will automatically build and push multi-arch operator image

# 5. Submit to community-operators
./hack/release/prepare-community-operators.sh 0.18.0 \
  --fork git@github.com:yourusername/community-operators.git
```

**Note:** If you need to manually build images for testing, use `build-images.sh` or `build-and-push.sh`, but this is not part of the standard release workflow.

### Manual Step-by-Step Workflow

If you prefer to run each step separately:

```bash
VERSION=0.18.0

# Step 1: Bump version
./hack/release/bump-version.sh ${VERSION}

# Step 2: Review changes
git diff

# Step 3: Regenerate bundle
make bundle IMG=quay.io/confidential-containers/trustee-operator:v${VERSION}

# Step 4: Commit and push to fork
git add -A
git commit -m "Release v${VERSION}"
git push fork release-v${VERSION}

# Step 5: Create PR manually
gh pr create --base main --title "Release v${VERSION}"

# Step 6: After PR merge, create GitHub release
# GitHub Actions will build and push the operator image

# Step 7: Submit to community-operators
./hack/release/prepare-community-operators.sh ${VERSION} \
  --fork git@github.com:yourusername/community-operators.git
```

### Manual Image Building (For Testing Only)

**Note:** This is not part of the standard release workflow. GitHub Actions builds images automatically when you create a GitHub release.

For local testing or manual builds:

```bash
VERSION=0.18.0
REGISTRY=quay.io/confidential-containers
IMAGE_TAG_BASE=${REGISTRY}/trustee-operator
IMG=${IMAGE_TAG_BASE}:v${VERSION}

# Build and push operator image (multi-arch)
./hack/release/build-images.sh ${VERSION}

# Or for more granular control
./hack/release/build-and-push.sh ${VERSION} --docker-buildx
```

## Files Updated During Release

The version bump process updates the following files:

- `Makefile` - VERSION variable and image tags
- `bundle/manifests/trustee-operator.clusterserviceversion.yaml` - CSV manifest
- `config/manager/manager.yaml` - Manager deployment manifest
- `config/manager/kustomization.yaml` - Image tag in kustomization
- `config/manifests/bases/trustee-operator.clusterserviceversion.yaml` - Base CSV

## Version Format

Versions must follow semantic versioning: `MAJOR.MINOR.PATCH` (e.g., `0.18.0`, `1.0.0`)

## Troubleshooting

### Go Version Mismatch Error

If you encounter errors like:
```
compile: version "go1.XX.Y" does not match go tool version "go1.XX.Z"
```

This happens when the Go toolchain version doesn't match the version specified in `go.mod`, often after a Go update or when using different Go installations.

**Solution:**

All release scripts automatically set `GOTOOLCHAIN=local` to use your local Go installation instead of the version specified in `go.mod`. This prevents version mismatch issues.

If you're running commands manually outside the scripts, set the environment variable:
```bash
export GOTOOLCHAIN=local
make test
make bundle
```

**Alternative solutions:**

1. **Reinstall Go to match the go.mod version:**
   ```bash
   # Check your Go installation
   which go
   go version
   go env GOROOT
   
   # Reinstall Go to match versions
   # For Fedora/RHEL:
   sudo dnf reinstall golang
   # Or download from https://go.dev/dl/
   ```

2. **Temporary workaround - skip tests:**
   ```bash
   ./hack/release/release.sh 0.18.0 --skip-tests
   ```

### Tests Failing

If tests fail during the release process:
```bash
# Run tests manually to see detailed output
make test

# Skip tests during release (not recommended for production)
./hack/release/release.sh 0.18.0 --skip-tests
```

### Docker Build Issues

If docker build fails:
```bash
# Check Docker is running
docker info

# Try building manually
make docker-build IMG=quay.io/confidential-containers/trustee-operator:v0.18.0

# Check if you're logged into the registry
docker login quay.io
```

### Bundle Generation Issues

If bundle generation fails:
```bash
# Regenerate manifests first
make manifests

# Then regenerate bundle
make bundle
```

## CI/CD Integration

After creating a GitHub release with a tag (e.g., `v0.18.0`), GitHub Actions will automatically:
1. Build and test the operator
2. Build and push multi-arch operator image (linux/amd64, linux/arm64, linux/s390x)
3. Create release artifacts

**Note:** Bundle and catalog images are not built by CI/CD. Only bundle files (in the `bundle/` directory) are submitted to community-operators.

Monitor the pipeline at: https://github.com/confidential-containers/trustee-operator/actions

## Submitting to Community Operators (OperatorHub)

After a successful release, submit the operator to OperatorHub:

### Prerequisites

1. Fork the community-operators repository:
   - https://github.com/k8s-operatorhub/community-operators

### Automated Submission Workflow (Recommended)

```bash
# Complete automated submission with PR creation
./hack/release/prepare-community-operators.sh 0.18.0 \
  --fork git@github.com:yourusername/community-operators.git

# The script will:
#  1. Remove existing work directory (if present)
#  2. Clone k8s-operatorhub/community-operators
#  3. Add your fork as 'fork' remote
#  4. Create branch trustee-operator-v0.18.0
#  5. Copy bundle to operators/trustee-operator/0.18.0/
#  6. Create and push commit
#  7. Create pull request to upstream

# Then monitor the PR and respond to reviews
```

### Manual Submission Workflow

If you prefer to review before pushing:

```bash
# 1. Prepare bundle (skip push/PR)
./hack/release/prepare-community-operators.sh 0.18.0 \
  --fork git@github.com:yourusername/community-operators.git \
  --skip-push

# 2. Review the changes
cd /tmp/community-operators-0.18.0
git status
git diff

# 3. Test the bundle (optional but recommended)
operator-sdk bundle validate operators/trustee-operator/0.18.0

# 4. Push to your fork and create PR
git push fork trustee-operator-v0.18.0
gh pr create --base main
```

### PR Guidelines

Follow the community-operators contribution guidelines:
- https://k8s-operatorhub.github.io/community-operators/contributing-via-pr/

Your PR should:
- Include only the new version directory
- Pass all CI checks
- Include testing evidence
- Follow the operator certification requirements

### Important Notes

- Each version can only be submitted once to community-operators
- The script automatically removes stale work directories to ensure fresh clones
- If a version already exists upstream, the script will detect it and skip submission
- Use `--skip-pr` if you want to review the changes in a browser before creating the PR
- **Safety:** Custom `--work-dir` must be under the system temp directory to prevent accidental deletion of important directories
  - The script validates against `$TMPDIR` (if set) or `/tmp`
  - On macOS, `$TMPDIR` is commonly `/var/folders/.../T/`
