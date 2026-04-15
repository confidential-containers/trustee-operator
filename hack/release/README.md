# Release Automation Scripts

This directory contains scripts to automate the release process for trustee-operator.

## Quick Start

For a complete release, use the all-in-one script:

```bash
./hack/release/do-release.sh 0.18.0 --dry-run  # Verify first
./hack/release/do-release.sh 0.18.0            # Execute release
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
| `rollback-version.sh` | Restore backup files | Undo version bump before commit |
| `fix-go-cache.sh` | Fix Go version mismatch | When you get Go compiler version errors |

## Scripts

### do-release.sh

**⭐ Recommended** - Complete end-to-end release automation (combines version bump, git operations, and image building).

**Usage:**
```bash
./hack/release/do-release.sh <new-version> [options]
```

**Options:**
- `--registry REGISTRY` - Container registry (default: `quay.io/confidential-containers`)
- `--skip-tests` - Skip running tests
- `--skip-push` - Skip git push and image push (for testing)
- `--build-catalog` - Also build and push catalog image
- `--docker-buildx` - Use docker buildx for multi-platform builds
- `--dry-run` - Show what would be done without making changes

**Examples:**
```bash
# Complete release (recommended)
./hack/release/do-release.sh 0.18.0

# Dry run first to verify
./hack/release/do-release.sh 0.18.0 --dry-run

# Release with multi-platform images
./hack/release/do-release.sh 0.18.0 --docker-buildx

# Test release without pushing
./hack/release/do-release.sh 0.18.0 --skip-push
```

**What it does (all-in-one):**
1. Run tests
2. Bump version in all files
3. Regenerate bundle manifests
4. Commit changes to git
5. Create git tag
6. Push to remote repository
7. Build and push operator image
8. Build and push bundle image
9. (Optional) Build and push catalog image

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
- Updates CSV (ClusterServiceVersion) files
- Creates backup files (*.bak) for rollback
- Shows summary of changes

### release.sh

Complete automated release process with multiple steps.

**Usage:**
```bash
./hack/release/release.sh <new-version> [options]
```

**Options:**
- `--skip-tests` - Skip running tests
- `--skip-bundle` - Skip bundle regeneration
- `--skip-commit` - Skip creating git commit
- `--skip-tag` - Skip creating git tag
- `--skip-push` - Skip pushing to remote
- `--dry-run` - Show what would be done without making changes

**Examples:**
```bash
# Full release
./hack/release/release.sh 0.18.0

# Dry run to see what would happen
./hack/release/release.sh 0.18.0 --dry-run

# Release without running tests
./hack/release/release.sh 0.18.0 --skip-tests

# Prepare release but don't push
./hack/release/release.sh 0.18.0 --skip-push
```

**What it does:**
1. Validates git working directory is clean
2. Runs tests (unless `--skip-tests`)
3. Bumps version using `bump-version.sh`
4. Regenerates bundle manifests (unless `--skip-bundle`)
5. Creates git commit (unless `--skip-commit`)
6. Creates git tag (unless `--skip-tag`)
7. Pushes to remote (unless `--skip-push`)

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
- `--work-dir DIR` - Working directory (default: `/tmp/community-operators-X.Y.Z`)
- `--skip-clone` - Use existing cloned repository
- `--skip-commit` - Don't create git commit
- `--dry-run` - Show what would be done without making changes

**Examples:**
```bash
# Prepare bundle with your fork (REQUIRED)
./hack/release/prepare-community-operators.sh 0.18.0 \
  --fork git@github.com:lmilleri/community-operators.git

# Use different fork
./hack/release/prepare-community-operators.sh 0.18.0 \
  --fork git@github.com:myuser/community-operators.git

# Dry run first
./hack/release/prepare-community-operators.sh 0.18.0 \
  --fork git@github.com:myuser/community-operators.git --dry-run

# Use existing cloned directory
./hack/release/prepare-community-operators.sh 0.18.0 \
  --fork git@github.com:myuser/community-operators.git \
  --skip-clone --work-dir /tmp/my-operators
```

**What it does:**
1. Clones upstream community-operators repository
2. Adds your fork as 'fork' remote
3. Creates a new branch (e.g., `trustee-operator-v0.18.0`)
4. Removes old bundle directory if exists
5. Copies bundle files to `operators/trustee-operator/X.Y.Z/`:
   - `bundle/manifests/` - Operator manifests and CSV
   - `bundle/metadata/` - Bundle metadata
   - `bundle/tests/` - Scorecard test configuration
   - Note: `bundle.Dockerfile` is NOT copied (auto-generated by community-operators)
6. Creates git commit with the changes
7. Provides instructions for pushing and creating PR

**After running:**
1. Review changes in the working directory
2. Test the bundle: `operator-sdk bundle validate <path>`
3. Push branch: `git push fork <branch>`
4. Create PR from your fork to upstream on GitHub

### rollback-version.sh

Restores backup files created by `bump-version.sh`.

**Usage:**
```bash
./hack/release/rollback-version.sh
```

**What it does:**
- Finds all `*.bak` backup files
- Restores them to their original locations
- Useful if you need to undo a version bump

### build-images.sh

Simple script for building and pushing operator and bundle images (wrapper around make targets).

**Usage:**
```bash
./hack/release/build-images.sh [version]
```

**Examples:**
```bash
# Build using version from Makefile
./hack/release/build-images.sh

# Build specific version
./hack/release/build-images.sh 0.18.0

# Use custom registry
REGISTRY=myregistry.io/myorg ./hack/release/build-images.sh 0.18.0
```

**What it does:**
1. Generates manifests
2. Generates bundle
3. Builds operator docker image
4. Pushes operator image
5. Builds bundle image
6. Pushes bundle image

**Environment variables:**
- `REGISTRY` - Container registry (default: `quay.io/confidential-containers`)
- `IMAGE_TAG_BASE` - Base image name (default: `${REGISTRY}/trustee-operator`)
- `IMG` - Operator image (default: `${IMAGE_TAG_BASE}:v${VERSION}`)
- `BUNDLE_IMG` - Bundle image (default: `${IMAGE_TAG_BASE}-bundle:v${VERSION}`)

### build-and-push.sh

Advanced build and push script with granular control over each step.

**Usage:**
```bash
./hack/release/build-and-push.sh [version] [options]
```

**Options:**
- `--registry REGISTRY` - Container registry
- `--skip-manifests` - Skip generating manifests
- `--skip-bundle` - Skip generating bundle
- `--skip-docker-build` - Skip building operator image
- `--skip-docker-push` - Skip pushing operator image
- `--skip-bundle-build` - Skip building bundle image
- `--skip-bundle-push` - Skip pushing bundle image
- `--build-catalog` - Also build and push catalog image
- `--docker-buildx` - Use docker buildx for multi-platform builds
- `--platforms PLATFORMS` - Platforms for buildx (default: `linux/amd64,linux/arm64`)
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
./hack/release/build-and-push.sh 0.18.0 --skip-docker-push --skip-bundle-push

# Build with catalog
./hack/release/build-and-push.sh 0.18.0 --build-catalog

# Custom registry
./hack/release/build-and-push.sh 0.18.0 --registry myregistry.io/myorg
```

**What it does:**
1. Validates version and configuration
2. Generates manifests (unless skipped)
3. Generates bundle (unless skipped)
4. Builds operator image (unless skipped)
5. Pushes operator image (unless skipped)
6. Builds bundle image (unless skipped)
7. Pushes bundle image (unless skipped)
8. Optionally builds and pushes catalog image

## Release Workflow

### Recommended: Complete Automated Release

**For most releases, use the all-in-one `do-release.sh` script:**

```bash
# 1. Ensure you're on main branch and it's up to date
git checkout main
git pull

# 2. Run the complete release (dry-run first recommended)
./hack/release/do-release.sh 0.18.0 --dry-run
./hack/release/do-release.sh 0.18.0

# 3. Create GitHub release
# Go to https://github.com/confidential-containers/trustee-operator/releases/new
# - Tag: v0.18.0
# - Title: Release v0.18.0
# - Description: Add changelog and release notes

# 4. Submit to community-operators (OperatorHub)
./hack/release/prepare-community-operators.sh 0.18.0
```

This single command handles version bumping, git operations, and image building/pushing.

### Alternative: Step-by-Step Release

For more control, use the `release.sh` script followed by `build-and-push.sh`:

```bash
# 1. Ensure you're on main branch and it's up to date
git checkout main
git pull

# 2. Run the release script (optionally with --dry-run first)
./hack/release/release.sh 0.18.0 --dry-run
./hack/release/release.sh 0.18.0

# 3. Build and push images
./hack/release/build-and-push.sh 0.18.0

# Or use the simpler build-images.sh
./hack/release/build-images.sh 0.18.0

# 4. Monitor CI/CD pipeline

# 5. Create GitHub release
# Go to https://github.com/confidential-containers/trustee-operator/releases/new
# - Tag: v0.18.0
# - Title: Release v0.18.0
# - Description: Add changelog and release notes
```

### Two-Step Release (Separate Version and Build)

If you prefer to separate version bumping from image building:

```bash
VERSION=0.18.0

# Step 1: Version bump and git operations
./hack/release/release.sh ${VERSION}

# Step 2: Build and push images
./hack/release/build-images.sh ${VERSION}
# Or for more control:
./hack/release/build-and-push.sh ${VERSION}
```

### Manual Release

For more control, use individual steps:

```bash
VERSION=0.18.0
REGISTRY=quay.io/confidential-containers
IMAGE_TAG_BASE=${REGISTRY}/trustee-operator
IMG=${IMAGE_TAG_BASE}:v${VERSION}
BUNDLE_IMG=${IMAGE_TAG_BASE}-bundle:v${VERSION}

# 1. Run tests
make test

# 2. Bump version
./hack/release/bump-version.sh ${VERSION}

# 3. Review changes
git diff

# 4. Regenerate bundle
make bundle

# 5. Commit changes
git add -A
git commit -m "Release v${VERSION}"

# 6. Create tag
git tag -a v${VERSION} -m "Release v${VERSION}"

# 7. Push to remote
git push origin main --tags

# 8. Build and push images
IMG=${IMG} make manifests bundle docker-build docker-push

# 9. Build and push bundle
make bundle-build bundle-push BUNDLE_IMG=${BUNDLE_IMG}

# 10. (Optional) Build and push catalog
# make catalog-build catalog-push CATALOG_IMG=${IMAGE_TAG_BASE}-catalog:v${VERSION}
```

### Rollback

If you need to undo a version bump before committing:

```bash
./hack/release/rollback-version.sh
```

If you've already committed but not pushed:

```bash
# Remove the commit
git reset --soft HEAD~1

# Or restore from backup files
./hack/release/rollback-version.sh
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
compile: version "go1.25.9" does not match go tool version "go1.25.8"
```

This happens when Go is updated but the toolchain binaries don't match.

**Solutions (try in order):**

1. **Use the fix script:**
   ```bash
   ./hack/release/fix-go-cache.sh
   ```

2. **Manual cleanup:**
   ```bash
   go clean -cache -testcache
   go install std
   ```

3. **If issue persists, reinstall Go:**
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

4. **Temporary workaround - skip tests:**
   ```bash
   ./hack/release/release.sh 0.18.0 --skip-tests
   ./hack/release/build-and-push.sh 0.18.0 --skip-manifests
   ```

The build scripts (`build-and-push.sh` and `build-images.sh`) automatically clean the cache before building to minimize this issue.

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

After pushing a tagged release, CI/CD will:
1. Build and test the operator
2. Build and push container images
3. Build and push bundle images
4. Create release artifacts

Monitor the pipeline at: https://github.com/confidential-containers/trustee-operator/actions

## Submitting to Community Operators (OperatorHub)

After a successful release, submit the operator to OperatorHub:

### Prerequisites

1. Fork the community-operators repository:
   - https://github.com/k8s-operatorhub/community-operators

2. Update the default fork URL in the script or use `--repo` flag

### Submission Workflow

```bash
# 1. Prepare the bundle for submission
./hack/release/prepare-community-operators.sh 0.18.0

# The script will:
#  - Clone k8s-operatorhub/community-operators
#  - Add your fork (lmilleri/community-operators) as 'fork' remote
#  - Create branch trustee-operator-v0.18.0
#  - Copy bundle to operators/trustee-operator/0.18.0/
#  - Create commit

# 2. Review the changes
cd /tmp/community-operators-0.18.0
git status
git diff

# 3. Test the bundle (optional but recommended)
operator-sdk bundle validate operators/trustee-operator/0.18.0

# 4. Push to your fork
git push fork trustee-operator-v0.18.0

# 5. Create Pull Request
# Go to: https://github.com/k8s-operatorhub/community-operators
# Create PR from your fork's branch to upstream main
# Base: k8s-operatorhub/community-operators:main
# Compare: lmilleri:trustee-operator-v0.18.0
```

### PR Guidelines

Follow the community-operators contribution guidelines:
- https://k8s-operatorhub.github.io/community-operators/contributing-via-pr/

Your PR should:
- Include only the new version directory
- Pass all CI checks
- Include testing evidence
- Follow the operator certification requirements

### Alternative: Using Custom Fork

```bash
# Use your own fork
./hack/release/prepare-community-operators.sh 0.18.0 \
  --fork git@github.com:youruser/community-operators.git

# Or edit the default in the script:
# FORK_REPO="git@github.com:youruser/community-operators.git"
```
