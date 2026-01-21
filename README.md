# ordiff

Compare GitHub releases with ease. CLI tool + MCP server.

## Features

- **Index repositories**: Fetch all releases, commits, PRs and file changes
- **Cache locally**: SQLite database for fast subsequent comparisons
- **Smart caching**: Skips already-cached release pairs when re-indexing
- **Compare releases**: See what changed between any two releases
- **MCP server**: Run as MCP server for AI integration
- **Async indexing**: MCP index operations run in background with progress tracking

## Installation

```bash
git clone https://github.com/maternion/ordiff
cd ordiff
go build -o ordiff .
```

## Usage

### Index a repository
```bash
./ordiff index ollama ollama
```

### List releases
```bash
./ordiff list
./ordiff list --json
```

### Compare releases
```bash
./ordiff compare v0.1.0 v0.2.0
./ordiff compare v0.1.0 v0.2.0 --json
./ordiff compare abc123 def456  # by commit SHA
```

### Run as MCP server
```bash
./ordiff mcp
```

## MCP Tools

When running as MCP server, ordiff exposes 4 tools:

### index_repo
Index a GitHub repository's releases and commits for caching.

```json
{
  "owner": "ollama",
  "repo": "ollama"
}
```

Runs asynchronously. Use `get_index_status` to check progress.

### get_index_status
Get the status of the current indexing operation.

```json
{}
```

Returns progress percentage, processed/skipped counts, and any errors.

### list_releases
List all cached releases for the default repository.

```json
{}
```

### compare_releases
Compare two releases and get detailed change information.

```json
{
  "from": "v0.1.0",
  "to": "v0.2.0"
}
```

### summarize_data
Get structured JSON data about release changes for AI summarization.

```json
{
  "from": "v0.1.0",
  "to": "v0.2.0"
}
```

## opencode Configuration

Add to your `~/.config/opencode/opencode.jsonc`:

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

## Claude Desktop Configuration

Add to your `~/.config/claude/claude_desktop_config.json`:

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

Default repository is stored in `.ordiff.yaml` after first index:

```yaml
default_owner: ollama
default_repo: ollama
```

## Environment Variables

- `GITHUB_TOKEN`: GitHub personal access token (optional, for higher rate limits)
- `OLLAMA_HOST`: Ollama server URL (not used directly, for reference only)

## Smart Caching

When re-indexing a repository, ordiff skips release pairs that are already cached:

```
Processing 0.9.0 -> 0.10.0 (1/169, 0 skipped)
Skipping 0.8.0 -> 0.9.0 (already cached)
Processing 0.7.0 -> 0.8.0 (2/169, 1 skipped)
```

This allows:
- Resuming interrupted indexing
- Adding new releases without re-fetching old data
- Faster subsequent indexing runs

## License

MIT
