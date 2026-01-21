# AGENTS.md - ordiff Development Guide

This file provides guidelines for agentic coding agents working on the ordiff codebase.

## Build Commands

### Build
```bash
go build -o ordiff .
```

### Run
```bash
go run . [commands]
./ordiff [commands]
```

### Run Single Test
```bash
go test -v ./internal/cache
go test -v ./internal/github
go test -v ./cmd/...
```

### Run All Tests
```bash
go test ./...
```

### Linting
```bash
go fmt ./...           # Format all code
go vet ./...           # Static analysis
go vet -shadow ./...   # Check for shadowed variables
```

### Dependency Management
```bash
go mod tidy            # Clean up go.mod/go.sum
go mod download        # Download dependencies
```

## Code Style Guidelines

### Imports (3 Groups)
Imports are organized into 3 groups with blank lines between:
1. Standard library
2. Internal packages (ordiff/*)
3. External packages (github.com/*)

```go
import (
    "context"
    "fmt"
    "log"

    "ordiff/internal/cache"
    "ordiff/internal/github"

    "github.com/google/go-github/v81/github"
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)
```

### Naming Conventions

- **Types/Exported Identifiers**: PascalCase
  - `Release`, `Fetcher`, `CompareResult`

- **Variables/Unexported Identifiers**: camelCase
  - `allReleases`, `page`, `commitSHA`

- **Acronyms**: Mixed case (not all caps)
  - `URL`, `API`, not `url`, `api`

- **Receiver Variables**: 1-2 letter abbreviations
  - `f` for Fetcher, `d` for DB, `r` for Release

### Error Handling

- **Wrap errors with context**: Use `fmt.Errorf("context: %w", err)`
  ```go
  return nil, fmt.Errorf("failed to fetch releases: %w", err)
  ```

- **CLI fatal errors**: Use `log.Fatalf`
  ```go
  db, err := cache.NewDB("ordiff.db")
  if err != nil {
      log.Fatalf("Failed to open database: %v", err)
  }
  ```

- **Non-fatal errors in loops**: Log and continue
  ```go
  if err := db.SaveRelease(r); err != nil {
      log.Printf("Warning: failed to save release %s: %v\n", r.TagName, err)
      continue
  }
  ```

### Context Usage

- Always pass context explicitly
- Use `context.Background()` for initialization
- Pass ctx to all client calls
  ```go
  ctx := context.Background()
  client.Repositories.ListReleases(ctx, owner, repo, opts)
  ```

### Code Comments

- Minimal comments - code should be self-documenting
- Only comment non-obvious logic or business rules
- Comments on exported functions/types in package docs

### Type Definitions

- Use structs for data models
- Use pointers for nullable fields
- Pointer receiver for methods that modify the struct

```go
type Release struct {
    TagName     string
    Name        string
    PublishedAt time.Time
    CommitSHA   string
    Body        string
    Owner       string
    Repo        string
}

type Commit struct {
    SHA         string
    Message     string
    Author      string
    Date        time.Time
    PrNumber    *int  // Pointer for nullable
}
```

### Database Operations

- Use prepared statements for queries
- Always defer `rows.Close()`
- Use `?` for placeholders (SQLite)
- Handle null values with `sql.NullString` or pointer types

### Project Structure

```
ordiff/
├── main.go              # Entry point
├── cmd/
│   ├── cli/             # CLI commands (index, list, compare)
│   └── mcp/             # MCP server implementation
├── internal/
│   ├── cache/           # SQLite database layer
│   └── github/          # GitHub API client
├── .ordiff.yaml         # Config file
└── ordiff.db            # SQLite cache
```

### MCP Server Notes

- Uses `metoro-io/mcp-golang` library
- Transport: stdio (JSON-RPC over stdin/stdout)
- Tools registered via `server.RegisterTool(name, description, handler)`
- Arguments deserialized from JSON to struct automatically

### GitHub API Considerations

- Set `GITHUB_TOKEN` env var for higher rate limits
- Sleep 100ms between requests to avoid rate limiting
- Pagination handled via `NextPage` field

### Smart Caching

The indexing process skips already-cached release pairs to avoid redundant API calls:

- `HasFileChangesCached(owner, repo, fromRelease, toRelease)` - checks if a pair is cached
- `GetReleasePairCount(owner, repo)` - counts cached file change records
- Both CLI and MCP indexing use this check

This allows resuming interrupted indexing without re-fetching.

### Async MCP Indexing

MCP `index_repo` tool runs indexing asynchronously:

- Returns immediately with status message
- Progress available via `get_index_status` tool
- Uses mutex-protected `IndexStatus` struct for thread-safe updates
- Prevents duplicate indexing runs

## Configuration

- Uses Viper for config management
- Config file: `.ordiff.yaml`
- Supports environment variables via Viper
