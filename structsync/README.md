# structsync

A tool to synchronize struct definitions from upstream Go repositories to the Casdoor Go SDK.

## Purpose

The Casdoor Go SDK shares struct definitions with multiple upstream repos (Casdoor server, Casvisor SDK, etc.) that diverge over time:
- Server structs have `xorm` tags for database mapping
- Server structs may embed types not available in the SDK (e.g., `*xormadapter.Adapter`)
- Fields get added/removed upstream

This tool automatically clones the upstream repos, extracts struct definitions, and syncs them to the SDK with necessary transformations applied.

## Usage

```bash
cd structsync

# Preview changes without applying (clones repos to temp dir)
go run . --dry-run

# Show unified diff of changes
go run . --diff

# Sync all configured structs
go run .

# Sync a specific struct only
go run . --struct=Record

# Use local checkout instead of cloning (for development)
go run . --source-override=casdoor:../../casdoor/object --dry-run

# Multiple overrides
go run . --source-override=casdoor:../../casdoor/object --source-override=casvisor:../../casvisor-go-sdk/casvisorsdk --dry-run

# Verbose output
go run . --verbose
```

## What it does

1. **Clones upstream repos** — shallow clone (`--depth 1`) to a temp directory, cleaned up on exit
2. **Removes `xorm` tags** — SDK doesn't use xorm, only needs `json` tags
3. **Excludes embedded types** — Types like `*xormadapter.Adapter` don't exist in SDK
4. **Maps external types** — e.g., `pp.PaymentState` → `string`
5. **Marks removed fields as deprecated** — Fields removed upstream get `// Deprecated: removed from server` comment instead of being deleted (preserves backward compatibility)

## Configuration

Edit `structsync.yaml` to configure sources and structs:

```yaml
sources:
  casdoor:
    repo: https://github.com/casdoor/casdoor
    path: object          # subdirectory within the repo
  casvisor:
    repo: https://github.com/casvisor/casvisor-go-sdk
    path: casvisorsdk
    # ref: main          # optional: branch/tag to clone

target: ../casdoorsdk

structs:
  - name: Adapter
    source: casdoor       # which source to read from
    file: adapter.go
  - name: Record
    source: casvisor      # reads from a different repo
    file: record.go
  - name: Application
    source: casdoor
    file: application.go
    include_types: [SigninMethod, SignupItem]  # also sync these types from same file

transform:
  remove_tags:
    - xorm
  exclude_embedded:
    - "*xormadapter.Adapter"
    - "*casbin.Enforcer"
  type_mappings:
    "pp.PaymentState": "string"
```

## Flags

| Flag | Description |
|------|-------------|
| `--config` | Path to config file (default: `structsync.yaml`) |
| `--dry-run` | Preview changes without applying |
| `--diff` | Show unified diff of changes |
| `--struct` | Only sync specific struct |
| `--source-override` | Use local path instead of cloning (format: `name:path`, repeatable) |
| `--mark-deprecated` | Mark removed fields as deprecated (default: true) |
| `--prune-deprecated` | Remove previously deprecated fields |
| `--no-color` | Disable colored output |
| `--verbose` | Verbose output |

## Requirements

- Go 1.21+
- `git` available on PATH (for cloning upstream repos)
