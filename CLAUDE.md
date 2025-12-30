# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

CLIProxyAPI is a Go-based proxy server that provides OpenAI/Gemini/Claude/Codex compatible API interfaces for CLI models. It enables multi-account load balancing, OAuth authentication flows, and schema translation between different AI provider APIs.

**Key capabilities:**
- Unified API gateway for multiple AI providers (Gemini, Claude, OpenAI Codex, Qwen, iFlow, Antigravity)
- Multi-account OAuth authentication with automatic token refresh
- Request/response translation between provider schemas (OpenAI ↔ Claude ↔ Gemini ↔ etc.)
- Hot-reloading of configuration and authentication credentials
- Management API for runtime configuration
- Reusable Go SDK for embedding the proxy in other applications

## Build & Run Commands

**Build the server:**
```bash
go build -o cli-proxy-api ./cmd/server/
```

**Run the server (default mode):**
```bash
go run ./cmd/server/
# or with custom config:
go run ./cmd/server/ -config path/to/config.yaml
```

**OAuth login flows:**
```bash
# Google/Gemini login
go run ./cmd/server/ -login [-project_id PROJECT_ID] [-no-browser]

# Claude Code login
go run ./cmd/server/ -claude-login [-no-browser]

# OpenAI Codex login
go run ./cmd/server/ -codex-login [-no-browser]

# Qwen login
go run ./cmd/server/ -qwen-login [-no-browser]

# iFlow login
go run ./cmd/server/ -iflow-login [-no-browser]
go run ./cmd/server/ -iflow-cookie [-no-browser]

# Antigravity login
go run ./cmd/server/ -antigravity-login [-no-browser]
```

**Import Vertex AI credentials:**
```bash
go run ./cmd/server/ -vertex-import path/to/service-account.json
```

**Run tests:**
```bash
# Run all tests
go test ./...

# Run tests in a specific package
go test ./internal/api/modules/amp/...
go test ./sdk/cliproxy/auth/...

# Run tests with verbose output
go test -v ./...

# Run a specific test
go test -v ./internal/cache -run TestSignatureCache
```

**Run integration tests (in test/ directory):**
```bash
go test -v ./test/...
```

**Build with release flags (used by goreleaser):**
```bash
go build -ldflags="-s -w -X 'main.Version=v1.0.0' -X 'main.Commit=abc123' -X 'main.BuildDate=2025-01-01'" -o cli-proxy-api ./cmd/server/
```

## Architecture

### Core Components

**1. Authentication System (`internal/auth/` & `sdk/cliproxy/auth/`)**
- Provider-specific OAuth flows for each AI service
- Token storage, refresh, and rotation logic
- `auth.Manager`: Core authentication manager that handles credential selection, execution, and auto-refresh
- `auth.ProviderExecutor`: Interface for implementing provider-specific API executors
- Multi-account load balancing with round-robin and fill-first strategies

**2. Translator System (`internal/translator/` & `sdk/translator/`)**
- Schema conversion between provider formats
- Registry-based routing: translators register themselves for specific source→target pairs
- Example: `internal/translator/gemini/claude/` translates Gemini API requests to Claude format
- All translators are imported in `internal/translator/init.go` via blank imports for side-effect registration
- The system allows bidirectional translation: request (inbound→provider) and response (provider→inbound)

**3. API Server (`internal/api/`)**
- Gin-based HTTP server with CORS and authentication middleware
- Routes for OpenAI (`/v1/chat/completions`), Claude (`/v1/messages`), and Gemini (`/v1beta/models/*/generateContent`) endpoints
- Management API (`/v0/management/*`) for runtime configuration when `remote-management.secret-key` is set
- Streaming (SSE) and non-streaming response support
- WebSocket API support (`/v1/ws`)

**4. Reusable SDK (`sdk/cliproxy/`)**
- Embedding API for using the proxy as a Go library
- `cliproxy.Builder`: Fluent builder for configuring and starting the service
- Custom hooks for lifecycle events (OnBeforeStart, OnAfterStart)
- Pluggable token stores (file, Postgres, Git, S3-compatible object storage)
- See `docs/sdk-usage.md` for detailed SDK documentation

**5. Configuration System (`internal/config/` & `internal/watcher/`)**
- YAML-based configuration with hot-reload support
- File watcher monitors `config.yaml` and `auths/` directory for changes
- Config reconciliation and diff detection to minimize reload impact
- Support for TLS, proxy URLs, model mappings, and routing strategies

**6. Storage Backends (`internal/store/`)**
- Local filesystem storage (default)
- PostgreSQL storage (`store.PostgresStore`) via `PGSTORE_DSN` env var
- Git-based storage (`store.GitTokenStore`) via `GITSTORE_GIT_URL` env var
- S3-compatible object storage (`store.ObjectTokenStore`) via `OBJECTSTORE_*` env vars
- Cloud deploy mode (`DEPLOY=cloud`) for containerized deployments

**7. Amp CLI Support (`internal/api/modules/amp/`)**
- Integrated support for Amp CLI and IDE extensions
- Provider route aliases (`/api/provider/{provider}/v1...`)
- Model mapping and fallback for unavailable models
- Management proxy for OAuth authentication

### Key Architectural Patterns

**Provider Registration Pattern:**
Providers (executors, translators, access handlers) register themselves during `init()` via side-effect imports:
```go
// internal/translator/init.go
import (
    _ "github.com/router-for-me/CLIProxyAPI/v6/internal/translator/gemini/claude"
    _ "github.com/router-for-me/CLIProxyAPI/v6/internal/translator/claude/gemini"
    // ...
)
```

**Translator Registry:**
Each translator package registers itself with the global registry in its `init()` function. The registry routes requests based on source schema, target schema, and API endpoint type.

**Config Access Pattern:**
Configuration values can be accessed programmatically via the `internal/access/` system, which provides a reconciliation layer for detecting config changes without full restarts.

**Hot-Reload Pattern:**
The watcher system (`internal/watcher/`) uses fsnotify to monitor file changes and triggers config/auth reloads without restarting the HTTP server.

## Module Structure

- `cmd/server/` - Main entry point with CLI flag parsing
- `internal/` - Internal application code (not importable by external modules)
  - `auth/` - OAuth implementations per provider (claude, codex, gemini, qwen, iflow, vertex)
  - `translator/` - Schema translation logic organized by provider pairs
  - `api/` - HTTP server, routes, middleware, handlers
  - `config/` - Configuration loading and parsing
  - `watcher/` - File system watching for hot-reload
  - `store/` - Storage backend implementations
  - `cmd/` - CLI command implementations (login flows, server startup)
- `sdk/` - Reusable SDK code (importable by external Go modules)
  - `cliproxy/` - Main SDK package for embedding the proxy
  - `auth/` - Core authentication interfaces and token stores
  - `translator/` - Schema translation registry
  - `api/handlers/` - Handler implementations for OpenAI/Claude/Gemini
- `examples/` - Example code for SDK usage
  - `custom-provider/` - Example of adding a custom provider
  - `translator/` - Example of custom translators
- `test/` - Integration tests
- `docs/` - Documentation for SDK usage and advanced features

## Key Files

- `cmd/server/main.go` - Application entry point with flag parsing and mode selection
- `internal/api/server.go` - HTTP server construction and routing
- `internal/translator/init.go` - Central import point for all translator registrations
- `sdk/cliproxy/builder.go` - Fluent builder API for SDK embedding
- `sdk/cliproxy/auth/manager.go` - Core authentication and execution manager
- `config.example.yaml` - Template configuration with all available options
- `.goreleaser.yml` - Release build configuration for multi-platform binaries

## Configuration

The server uses a YAML configuration file (`config.yaml`) with sections for:
- Server settings (host, port, TLS)
- Management API (secret-key, allow-remote, control panel)
- Authentication directory (`auth-dir`)
- API keys for client authentication
- Routing strategy (round-robin, fill-first)
- Model mappings (aliasing unavailable models to alternatives)
- Provider-specific settings (Gemini Web, Vertex AI, custom upstream providers)

See `config.example.yaml` for the complete reference.

## Authentication Storage

OAuth credentials are stored in the `auth-dir` (default: `~/.cli-proxy-api/auths/`). Each provider has JSON files with tokens:
- `gemini_*.json` - Gemini OAuth tokens
- `claude_*.json` - Claude OAuth tokens
- `codex_*.json` - OpenAI Codex OAuth tokens
- `qwen_*.json` - Qwen OAuth tokens
- `iflow_*.json` - iFlow OAuth tokens
- `vertex_*.json` - Vertex AI service account credentials

The server watches this directory and reloads credentials when files change.

## Testing Strategy

- Unit tests are colocated with source code (`*_test.go`)
- Integration tests live in `test/` directory
- Tests use standard Go testing patterns with `testing.T`
- Amp module has comprehensive test coverage in `internal/api/modules/amp/*_test.go`
- Use table-driven tests for translator validation
- Mock providers are available for testing without real credentials

## Environment Variables

Key environment variables (see `.env.example`):
- `PGSTORE_DSN` - PostgreSQL connection string for token storage
- `PGSTORE_SCHEMA` - PostgreSQL schema name (optional)
- `PGSTORE_LOCAL_PATH` - Local spool directory for Postgres store
- `GITSTORE_GIT_URL` - Git repository URL for token storage
- `GITSTORE_GIT_USERNAME` - Git username for authentication
- `GITSTORE_GIT_TOKEN` - Git token/password for authentication
- `GITSTORE_LOCAL_PATH` - Local clone path for Git store
- `OBJECTSTORE_ENDPOINT` - S3-compatible endpoint URL
- `OBJECTSTORE_ACCESS_KEY` - S3 access key
- `OBJECTSTORE_SECRET_KEY` - S3 secret key
- `OBJECTSTORE_BUCKET` - S3 bucket name
- `OBJECTSTORE_LOCAL_PATH` - Local cache directory for object store
- `DEPLOY=cloud` - Enables cloud deploy mode (waits for config upload before starting)

## Development Notes

**Adding a New Provider:**
1. Implement OAuth flow in `internal/auth/{provider}/`
2. Create provider executor implementing `auth.ProviderExecutor` in `sdk/cliproxy/executor/`
3. Register translators in `internal/translator/{provider}/` for each target schema
4. Add CLI command in `internal/cmd/{provider}_login.go`
5. Update `internal/translator/init.go` with new translator imports
6. Register executor in the auth manager

**Adding a New Translator:**
1. Create package in `internal/translator/{source}/{target}/`
2. Implement `translator.TranslateRequest` and `translator.TranslateResponse` functions
3. Register with the global registry in `init()` function
4. Add blank import to `internal/translator/init.go`
5. Add tests for bidirectional translation

**Modifying the API Server:**
- Server options use functional pattern: `WithMiddleware()`, `WithEngineConfigurator()`, etc.
- Request/response logging is pluggable via `WithRequestLoggerFactory()`
- Management endpoints are in `internal/api/handlers/management/`
- Provider-specific handlers are in `sdk/api/handlers/{openai,claude,gemini}/`

**SDK Embedding:**
See `docs/sdk-usage.md` and `docs/sdk-advanced.md` for comprehensive guides on using the SDK in external Go applications.

## Version Management

This project uses semantic versioning with the Go module path `/v6`. The major version is embedded in all import paths:
```go
import "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy"
```

Build version info is injected at compile time via ldflags (see `.goreleaser.yml`).
