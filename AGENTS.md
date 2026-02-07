# AGENTS.md

This document provides guidelines for AI agents working with the CoRag codebase.

## Project Overview

CoRag is a Go + Cloudwego eino powered RAG/Agent platform. Key technologies:
- **Go 1.25.1+**
- **Cloudwego eino**: Workflow/DAG execution, Agent scheduling, Pipeline orchestration
- **Hertz**: HTTP framework for REST APIs
- **Viper**: Configuration management

## Build, Lint, and Test Commands

### Build
```bash
# Build all binaries
go build ./...

# Build specific binary
go build -o bin/api ./cmd/api
go build -o bin/worker ./cmd/worker
go build -o bin/cli ./cmd/cli

# Build with race detector
go build -race ./...
```

### Run
```bash
# API service (default :8080)
go run ./cmd/api

# Worker service
go run ./cmd/worker

# CLI tool
go run ./cmd/cli

# With custom config
CONFIG_PATH=/path/to/config.yaml go run ./cmd/api
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -coverprofile=coverage.out ./...

# View coverage in browser
go tool cover -html=coverage.out

# Run single test file
go test ./internal/pipeline/query/... -v

# Run single test function
go test -run TestQueryPipeline_ValidQuery ./internal/pipeline/query/...

# Run tests with race detector
go test -race ./...
```

### Vet and Lint
```bash
# Go vet
go vet ./...

# Format code
gofmt -w .
gofmt -d .  # Show diff

# Static analysis
go run golang.org/x/tools/go/analysis/cmd/vet@latest ./...
```

### Dependencies
```bash
# Download dependencies
go mod download

# Tidy dependencies
go mod tidy

# Verify dependencies
go mod verify

# List dependencies
go list -m all
```

## Code Style Guidelines

### Imports
Organize imports in three groups with blank lines between:
1. Standard library
2. External packages (github.com/xxx)
3. Internal packages (rag-platform/xxx)

```go
import (
    "context"
    "fmt"
    "time"

    "github.com/cloudwego/hertz/pkg/app"
    "github.com/cloudwego/hertz/pkg/common/hlog"

    appcore "rag-platform/internal/app"
    "rag-platform/internal/runtime/eino"
)
```

### Formatting
- Use `gofmt` for automatic formatting
- Indent with tabs, not spaces
- No trailing whitespace
- Max line length: ~120 characters (soft limit)

### Naming Conventions
- **Packages**: lowercase, concise, meaningful (e.g., `app`, `pipeline`, `storage`)
- **Files**: lowercase with underscores only if needed for naming (e.g., `workflow.go`)
- **Exported types/functions**: PascalCase (e.g., `Workflow`, `CreateWorkflow`)
- **Unexported**: camelCase (e.g., `engine`, `parseDefaultKey`)
- **Interfaces**: Simple noun or verb+noun pattern (e.g., `Client`, `Retriever`)
- **Constants**: PascalCase or SCREAMING_SNAKE_CASE for constants (e.g., `ErrNotFound`, `MaxRetries`)
- **Variables**: camelCase, avoid single letters except loop indices

### Error Handling
- Use `pkg/errors` for error wrapping: `errors.Wrap(err, "message")`
- Use `errors.Wrapf` for formatted error messages
- Sentinel errors in `pkg/errors/errors.go`: `ErrNotFound`, `ErrInvalidArg`
- Return meaningful errors with context
- Handle errors at the appropriate level (don't ignore with `_`)
- Use `context.Context` for cancellation and timeouts

```go
if err != nil {
    return nil, fmt.Errorf("compile workflow failed: %w", err)
}

return nil, errors.Wrap(err, "failed to create client")
```

### Structs and Types
- Use struct tags for JSON serialization
- Use `binding` tags for Hertz request validation
- Keep structs focused and small

```go
type Query struct {
    ID        string                 `json:"id"`
    Text      string                 `json:"text"`
    Metadata  map[string]interface{} `json:"metadata"`
    CreatedAt time.Time              `json:"created_at"`
}
```

### Context Usage
- Pass `context.Context` as first parameter
- Use named context variables for clarity
- Check context cancellation in long-running operations

```go
func (h *Handler) Query(ctx context.Context, c *app.RequestContext) error {
    // ...
}
```

### Comments
- Use Chinese comments for public APIs and documentation
- Comment exported types and functions
- Use sentence case for comments
- No commented-out code

```go
// Workflow 工作流
type Workflow struct {
    // ...
}

// CreateWorkflow 创建工作流
func CreateWorkflow(name, description string) *Workflow {
    // ...
}
```

### HTTP Handlers (Hertz)
- Use `consts.StatusXXX` for status codes
- Return consistent JSON response format
- Log errors with `hlog.CtxErrorf`
- Validate request parameters with `binding` tags

```go
func (h *Handler) Query(ctx context.Context, c *app.RequestContext) {
    var request struct {
        Query string `json:"query" binding:"required"`
        TopK  int    `json:"top_k"`
    }

    if err := c.BindJSON(&request); err != nil {
        c.JSON(consts.StatusBadRequest, map[string]string{
            "error": "请求参数错误",
        })
        return
    }
    // ...
}
```

### Testing
- Use table-driven tests when appropriate
- Test file naming: `xxx_test.go`
- Test function naming: `TestXxx`
- Use `t.Run` for sub-tests
- Prefer `require` over `assert` for clarity on failures

### Project Structure
```
cmd/              # Entry points (api, worker, cli)
internal/          # Private application code
  app/            # Application core (bootstrap, services)
  api/            # HTTP/gRPC API
  runtime/eino/   # Workflow and agent orchestration
  pipeline/       # Domain pipelines (query, specialized)
  model/          # LLM, embedding, vision abstractions
  storage/        # Data storage implementations
  splitter/       # Text splitting implementations
pkg/              # Public libraries
  config/         # Configuration
  errors/         # Error utilities
  log/            # Logging
  tracing/        # Tracing utilities
configs/          # Configuration files
examples/         # Example code
design/           # Design documentation
deployments/      # Docker, K8s configurations
```

### Configuration
- Use Viper for configuration management
- YAML configuration files in `configs/`
- Support environment variable overrides
- Use `${VAR_NAME}` syntax in config for env var substitution

### Workflows and Pipelines
- All pipelines orchestrated via eino
- Workflows: DAG-based execution with nodes and edges
- Use `compose.NewGraph` for workflow definition
- Register workflows with the Engine

### Important Files
- `go.mod`: Module definition and dependencies
- `configs/*.yaml`: Configuration files
- `internal/runtime/eino/workflow.go`: Workflow implementation
- `internal/api/http/handler.go`: HTTP handlers
- `internal/app/app.go`: Core application logic
- `pkg/errors/errors.go`: Error utilities

## Common Tasks

### Adding a New Pipeline
1. Create `internal/pipeline/newpipeline/`
2. Implement `NewPipeline()` function
3. Register with Engine in `internal/app/bootstrap.go`
4. Add handler in `internal/api/http/handler.go`

### Adding a New Model Provider
1. Implement interface in `internal/model/llm/` or similar
2. Register provider in config
3. Use `NewLLMClientFromConfig` pattern

### Adding a New API Endpoint
1. Define request/response types in handler
2. Implement handler method
3. Register route in `internal/api/http/router.go`
