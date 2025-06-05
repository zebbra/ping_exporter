# CI/CD Setup for Ping Exporter

This document describes the CI/CD setup for the ping exporter using GitHub Actions, GoReleaser, and Docker.

## Overview

The CI/CD pipeline consists of:

1. **Continuous Integration (CI)**: Runs on every push and pull request
2. **Release Pipeline**: Triggers on Git tags to build and publish releases
3. **Docker Images**: Published to quay.io/zebbra/ping_exporter
4. **Binary Releases**: Created using GoReleaser for multiple platforms

## GitHub Actions Workflows

### CI Workflow (`.github/workflows/ci.yml`)

Triggers on:
- Push to `main`/`master` branches
- Pull requests to `main`/`master` branches
- Ignores changes to documentation and dist files

Jobs:
- **Test**: Runs Go tests, linting, formatting checks
- **Build**: Compiles the binary and tests Docker build

### Release Workflow (`.github/workflows/release.yml`)

Triggers on:
- Git tags matching `v*.*.*` pattern

Jobs:
- **Test**: Runs tests before release
- **Docker**: Builds and pushes multi-platform Docker images to Quay.io
- **GoReleaser**: Creates GitHub releases with binaries for multiple platforms

## Required Secrets

Configure these secrets in your GitHub repository settings:

### For Quay.io Docker Registry
- `QUAY_USERNAME`: Your Quay.io username
- `QUAY_PASSWORD`: Your Quay.io password or robot token

### Optional
- `CODECOV_TOKEN`: For code coverage reports (optional)

## Release Process

### Creating a Release

1. Ensure all changes are committed and pushed to main
2. Create and push a Git tag:
   ```bash
   git tag -a v1.0.0 -m "Release version 1.0.0"
   git push origin v1.0.0
   ```

3. GitHub Actions will automatically:
   - Run tests
   - Build Docker images for `linux/amd64` and `linux/arm64`
   - Push images to `quay.io/zebbra/ping_exporter:v1.0.0` and `:latest`
   - Create GitHub release with binaries for multiple platforms
   - Generate changelog from commit messages

### Supported Platforms

GoReleaser builds binaries for:
- **Linux**: amd64, arm64, arm
- **macOS**: amd64, arm64
- **Windows**: amd64

### Docker Images

Multi-platform Docker images are built and pushed to:
- `quay.io/zebbra/ping_exporter:v1.0.0` (version tag)
- `quay.io/zebbra/ping_exporter:latest` (latest tag)

## Local Development

### Testing GoReleaser Configuration

```bash
# Check configuration syntax
make goreleaser-check

# Test release build locally (no publishing)
make release-snapshot

# Dry run of release process
make release-dry-run
```

### Building Docker Images Locally

```bash
# Build local image
make docker-build

# Build and push to registry (requires authentication)
make docker-push
```

### Manual Release (if needed)

```bash
# Install GoReleaser (if not installed)
go install github.com/goreleaser/goreleaser@latest

# Create release
make release
```

## GoReleaser Configuration

The `.goreleaser.yml` file defines:

- **Builds**: Cross-compilation for multiple OS/architecture combinations
- **Archives**: Packaging of binaries with documentation
- **Packages**: Creation of .deb and .rpm packages
- **Changelog**: Automatic generation from Git commits

### Version Information

Version information is automatically injected during build using ldflags:
- `main.version`: Git tag version
- `main.commit`: Git commit hash
- `main.date`: Build timestamp

These are available through the Prometheus common/version package.

## Changelog Generation

The release process automatically generates changelogs from commit messages. For better changelogs, use conventional commit format:

- `feat: add new feature` → Features section
- `fix: resolve bug` → Bug fixes section
- `docs: update documentation` → Excluded from changelog

## Troubleshooting

### Failed Docker Push
- Verify `QUAY_USERNAME` and `QUAY_PASSWORD` secrets are set correctly
- Check if the repository exists on Quay.io
- Ensure the account has push permissions

### GoReleaser Failures
- Run `make goreleaser-check` to validate configuration
- Check if all required files (LICENSE, README.md) exist
- Verify Git tag format matches `v*.*.*`

### Test Failures
- Ensure all tests pass locally before creating a tag
- Check if any dependencies need updating
- Verify Docker build works locally

## Security Considerations

- Secrets are stored securely in GitHub repository settings
- Docker images are scanned for vulnerabilities during build
- Only signed Git tags trigger releases
- Limited permissions are granted to GitHub Actions tokens

## Monitoring

Monitor the CI/CD pipeline through:
- GitHub Actions tab in the repository
- Quay.io repository for Docker image status
- GitHub Releases page for published releases