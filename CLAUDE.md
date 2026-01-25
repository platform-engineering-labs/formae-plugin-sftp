# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

This is the **Formae Plugin Template** - a starter template for creating cloud resource plugins for [Formae](https://github.com/platform-engineering-labs/formae), an Infrastructure-as-Code tool. Plugins enable Formae to manage resources across different cloud providers (AWS, Azure, GCP, OpenStack, etc.).

Create a new plugin via CLI: `formae plugin init <name>`

## Build & Test Commands

```bash
make build              # Build plugin binary to bin/
make test               # Run all tests
make test-unit          # Run unit tests (//go:build unit)
make test-integration   # Run integration tests (//go:build integration)
make lint               # Run golangci-lint
make install            # Build + install to ~/.pel/formae/plugins/<namespace>/v<version>/

# Conformance testing (CRUD lifecycle + discovery)
make conformance-test                  # Latest formae version
make conformance-test VERSION=0.77.0   # Specific version
```

## Architecture

### Plugin SDK Integration

The SDK handles all boilerplate. You only implement:

1. **`main.go`** - Entry point (don't modify):
   ```go
   sdk.RunWithManifest(&Plugin{}, sdk.RunConfig{})
   ```

2. **`formae-plugin.pkl`** - Plugin manifest (name, version, namespace)

3. **`plugin.go`** - The `ResourcePlugin` interface with 9 methods:
   - **Config**: `RateLimit()`, `DiscoveryFilters()`, `LabelConfig()`
   - **CRUD**: `Create()`, `Read()`, `Update()`, `Delete()`, `Status()`, `List()`

4. **`schema/pkl/*.pkl`** - Resource definitions using Pkl

### Resource Schemas (Pkl)

Define resource types in `schema/pkl/` using annotations:

```pkl
@formae.ResourceHint {
    type = "MYPROVIDER::Service::Resource"  // NAMESPACE::SERVICE::RESOURCE format
    identifier = "$.Id"                      // JSONPath to unique ID in API response
}
class MyResource extends formae.Resource {
    @formae.FieldHint { createOnly = true }  // Cannot change after creation
    region: String?
}
```

### Async Operations

For long-running operations, return `InProgress` with a `RequestID`. The agent will poll `Status()`:

```go
return &resource.CreateResult{
    ProgressResult: &resource.ProgressResult{
        OperationStatus: resource.OperationStatusInProgress,
        RequestID:       operationID,
    },
}, nil
```

## Key Files

| File | Purpose |
|------|---------|
| `plugin.go` | Your plugin implementation (implement CRUD methods here) |
| `formae-plugin.pkl` | Plugin manifest (name, version, namespace, license) |
| `schema/pkl/` | Pkl resource type definitions |
| `conformance_test.go` | Conformance tests (CRUD lifecycle + discovery) |
| `scripts/ci/clean-environment.sh` | Test resource cleanup hook (idempotent) |

## Development Workflow

1. Update `formae-plugin.pkl` with your plugin metadata
2. Define resource types in `schema/pkl/*.pkl`
3. Implement CRUD operations in `plugin.go`
4. Test locally: `make install && formae agent start && formae apply examples/basic/main.pkl`
5. Run conformance tests: `make conformance-test`

## Scripts

- **`scripts/ci/clean-environment.sh`**: Delete test resources (idempotent, runs before AND after conformance tests)

### CI/CD
Configure credentials in `.github/workflows/ci.yml` using GitHub secrets or OIDC. See the workflow file for examples.
