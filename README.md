# ordiff

Compare GitHub releases with ease. ordiff caches releases, commits, PRs, and file changes locally, letting you track exactly what changed between any two releases without rate limits or API delays.

## Why ordiff?

- **Local cache** - Fetch once, compare forever. No API rate limits after indexing.
- **Track any repo** - Works with any public or private GitHub repository.
- **Detailed comparisons** - See commits, PRs, and file changes with stats.
- **AI-ready** - MCP server for integration with Claude, OpenCode, and other AI tools.
- **Resumable indexing** - Smart caching skips already-indexed release pairs.

## Quick Start

```bash
# Build
git clone https://github.com/maternion/ordiff
cd ordiff
go build -o ordiff .

# Index a repository
./ordiff index ollama ollama

# List releases
./ordiff list

# Compare two releases
./ordiff compare v0.13.0 v0.14.0
```

## Example Output

```
=== v0.13.0 → v0.14.0 ===

Commits: 47 | PRs: 12 | Files Changed: 28

Top Changed Files:
  +Add  -Del  File
  ---- ----  ----
   432    12  llama.go
   156    89  api.go
    78     3  main.go
    45    120  utils.go
    34     0  README.md

Recent Commits:
  a1b2c3d  Add GPU memory optimization for large models
  d4e5f6g  Fix timeout handling in API endpoints
  h8j9k0l  Update llama.go with new context handling
  ...
   ... and 44 more commits
```

## Installation

```bash
git clone https://github.com/maternion/ordiff
cd ordiff
go build -o ordiff .
```

Or download a binary from the [releases page](https://github.com/maternion/ordiff/releases).

## CLI Commands

### index

Index a repository to build the local cache.

```bash
./ordiff index <owner> <repo>

# Examples
./ordiff index ollama ollama
./ordiff index kubernetes kubernetes
./ordiff index vercel next.js
```

### list

List cached releases for the default repository.

```bash
./ordiff list          # Human-readable output
./ordiff list --json   # JSON output
```

### compare

Compare two releases.

```bash
./ordiff compare <from> <to>

# Examples
./ordiff compare v0.1.0 v0.2.0
./ordiff compare v0.13.0 v0.14.0
./ordiff compare abc123 def456  # by commit SHA
./ordiff compare v0.1.0 v0.2.0 --json
```

### mcp

Run as an MCP server for AI integration.

```bash
./ordiff mcp
```

## MCP Server

ordiff works as a Model Context Protocol server, enabling AI assistants to index and compare releases.

### Available Tools

| Tool | Description |
|------|-------------|
| `index_repo` | Index a repository (async, use `get_index_status` to track) |
| `get_index_status` | Check indexing progress |
| `list_releases` | List cached releases |
| `compare_releases` | Compare two releases |
| `summarize_data` | Get structured JSON for AI summarization |

### opencode Configuration

Add to `~/.config/opencode/opencode.jsonc`:

```json
{
  "mcpServers": {
    "ordiff": {
      "command": "/path/to/ordiff",
      "args": ["mcp"],
      "env": {
        "GITHUB_TOKEN": "ghp_..."
      }
    }
  }
}
```

### Claude Desktop Configuration

Add to `~/.config/claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "ordiff": {
      "command": "/path/to/ordiff",
      "args": ["mcp"]
    }
  }
}
```

## Configuration

After the first index, a `.ordiff.yaml` file stores the default repository:

```yaml
default_owner: ollama
default_repo: ollama
```

## Environment Variables

- `GITHUB_TOKEN`: GitHub personal access token (optional, increases rate limit from 60 to 5000 requests/hour)

## How It Works

1. **Index** - ordiff fetches all releases, then walks through consecutive release pairs to fetch commits and file changes.
2. **Cache** - Everything is stored in a local SQLite database (`ordiff.db`).
3. **Compare** - Query the cache for detailed diffs between any two releases.

### Smart Caching

When re-indexing, ordiff skips release pairs already in the cache:

```
Processing 0.9.0 → 0.10.0 (1/169, 0 skipped)
Skipping 0.8.0 → 0.9.0 (already cached)
Processing 0.7.0 → 0.8.0 (2/169, 1 skipped)
```

This means:
- Interrupted indexing resumes where it left off
- New releases only fetch the new pairs
- Subsequent indexing is nearly instant

## Use Cases

- **Debug release issues** - See exactly what changed in a problematic release
- **Audit trails** - Track changes across versions for compliance
- **Changelog generation** - Extract commit summaries between releases
- **Large repo monitoring** - Track changes in repos with many releases

## Project Structure

```
ordiff/
├── main.go              # Entry point
├── cmd/
│   ├── cli/             # CLI commands (index, list, compare)
│   └── mcp/             # MCP server
├── internal/
│   ├── cache/           # SQLite database
│   └── github/          # GitHub API client
├── .ordiff.yaml         # Config file
└── ordiff.db            # SQLite cache
```

## License

MIT
