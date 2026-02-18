# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This is `drone-cache`, a Drone CI/CD plugin written in Go that provides caching functionality for build pipelines. It supports multiple storage backends (S3, Azure, GCS, filesystem, SFTP, Harness) and archive formats to cache workspace files between builds, significantly reducing build times.

## Build System

### Go Module Setup
This project uses Go modules with vendored dependencies:

```bash
# Update dependencies and vendor them
make vendor

# Build the main binary
make drone-cache

# Clean build artifacts  
make clean
```

### Common Development Commands

```bash
# Build the drone-cache binary
make drone-cache

# Run all tests (unit + integration)
make test

# Run only unit tests
make test-unit

# Run only integration tests  
make test-integration

# Run end-to-end tests
make test-e2e

# Lint code with golangci-lint
make lint

# Fix linting issues automatically
make fix

# Format code with gofmt
make format

# Generate documentation
make docs

# Build Docker container
make container

# Compress binary with UPX
make compress
```

### Testing Infrastructure
Tests require Docker Compose for integration testing:
- Integration tests use `docker-compose up -d` to start dependencies (storage backends)
- Tests are tagged with build constraints (`// +build integration`)
- Unit tests run with race detection: `go test -race -cover`
- E2E tests focus on the plugin package: `make test-e2e`

## Code Architecture

### Core Components

**Main Entry Point (`main.go`)**
- CLI application built with `urfave/cli/v2`
- Configures storage backends, archive formats, and cache operations  
- Handles Drone CI environment variables and plugin settings
- Factory pattern creates storage backends from configuration

**Cache Package (`cache/`)**
- Core cache interface with three main operations:
  - `Rebuilder`: Creates cache from source files (`Rebuild([]string)`)
  - `Restorer`: Restores files from cache (`Restore([]string, string)`)
  - `Flusher`: Removes cached files (`Flush([]string)`)
- `cache.New()` factory creates cache instances with storage, archiver, and key generator
- Options pattern for configuration (WithOverride, WithFallbackGenerator, etc.)

**Storage Backends (`storage/backend/`)**
- Pluggable storage system with backends:
  - `s3/`: AWS S3 and S3-compatible storage (Minio, Ceph, etc.)
  - `azure/`: Azure Blob Storage
  - `gcs/`: Google Cloud Storage with OIDC support
  - `filesystem/`: Local filesystem storage 
  - `sftp/`: SFTP remote storage with SSH auth
  - `harness/`: Harness-specific storage integration with unified APIs
- Each backend implements the `storage.Storage` interface
- Factory pattern: `backend.FromConfig()` creates appropriate backend

**Archive Formats (`archive/`)**
- Support for multiple compression formats:
  - `tar/`: Uncompressed tar archives with metadata preservation  
  - `gzip/`: Gzip-compressed tar archives
  - `zstd/`: Zstandard-compressed archives (high performance)
- Configurable compression levels and metadata preservation
- Platform-specific metadata handling (Unix/Windows/Darwin)

**Plugin Integration (`internal/plugin/`)**
- Drone plugin configuration and execution logic in `plugin.go`
- Auto-detection system (`autodetect/`) for cache directories and keys:
  - Language/tool detection: Go, Node, Maven, Gradle, Bazel, etc.
  - Automatic cache key generation based on lock files
  - Graceful fallback when no tools detected
- Cache key templating with build metadata
- Integration with Drone CI environment variables

### Key Generation and Templating

Cache keys support Go template syntax with build metadata:
- Template variables: `{{ .Repo.Name }}`, `{{ .Commit.Branch }}`, `{{ .Build.Number }}`
- Helper functions: `checksum`, `epoch`, `arch`, `os`
- Example: `"{{ .Repo.Name }}-{{ checksum "go.mod" }}-{{ arch }}"`
- Fallback generators for robustness (metadata → hash → static)

### Configuration Patterns

**Environment Variables**
- Plugin settings via `PLUGIN_*` variables (mapped from CLI flags)
- Backend-specific credentials: `AWS_*`, `AZURE_*`, `GCS_*`, `SFTP_*`
- Drone CI metadata via `DRONE_*` variables (repo, commit, build info)

**Storage Backend Selection**
- Configured via `--backend` flag or `PLUGIN_BACKEND` env var
- Each backend has specific configuration requirements in `backend/config.go`
- Graceful fallback and error handling for missing credentials
- Support for role assumption (AWS IAM, GCP OIDC)

**Auto-Detection Logic**
- Tool detection matrix determines behavior:
  - Tool detected + custom key + custom path = use both user settings
  - Tool detected + no custom key + custom path = skip (early exit option)
  - Tool detected + custom key + no custom path = use auto path with custom key
  - No tool + no custom settings = skip with helpful message
- Configurable via `auto-detect` and `auto-detect-early-exit` flags

## Development Workflow

### Local Development Setup
```bash
# Install dependencies and setup dev environment
make setup

# Format and lint before committing  
make format
make lint

# Run tests before pushing
make test-unit
make test-integration
```

### Docker Development
```bash
# Build and test with Docker
make container
docker run --rm -v "$(pwd)":/app meltwater/drone-cache:latest --help

# Integration tests use docker-compose for backend services
docker-compose up -d
make test-integration
docker-compose down -v
```

### Code Quality
- Uses `golangci-lint` with comprehensive rule set (`.golangci.yml`)
- Line length limit: 200 characters
- Enforces Go formatting, imports organization, and security checks (`gosec`)
- Integration with CI pipeline for automated quality checks
- Race detection in unit tests, coverage reporting

## Key Architectural Patterns

**Factory Pattern**
- `backend.FromConfig()` creates storage backends based on configuration
- `archive.FromFormat()` creates appropriate archiver
- `cache.New()` assembles complete cache with all components

**Interface Segregation**
- Storage backends implement `storage.Storage` interface
- Cache operations split into `Rebuilder`, `Restorer`, `Flusher` interfaces
- Archive formats implement `archive.Archive` interface

**Options Pattern**
- Cache creation uses variadic options: `cache.New(..., opts ...Option)`
- Archive creation uses options for compression, metadata, symlinks
- Flexible configuration without parameter explosion

**Metadata-Driven Operation**
- Drone CI metadata flows through `metadata.Metadata` struct
- Build information drives cache key generation and namespacing
- Template engine processes metadata for dynamic cache keys

**Error Handling Strategy**
- Plugin-specific errors via `plugin.Error` type for expected failures
- Silent failure mode (default) vs exit-code mode for CI integration
- Graceful degradation when cache operations fail (won't break builds)