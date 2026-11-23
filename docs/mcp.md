# Social Vault MCP Server

`social-vault-mcp` exposes Social Vault over the [Model Context Protocol](https://modelcontextprotocol.io/)
as a `stdio` JSON-RPC server, built with the official Go SDK
(`github.com/modelcontextprotocol/go-sdk`). Any MCP client — `agy`, `dsh`,
Claude Code, or anything else that speaks MCP — can search, save, and
organize bookmarks in the vault.

## Architecture

```
MCP client (agy / dsh / Claude)
  │  stdio JSON-RPC
  ▼
social-vault-mcp
  │  HTTPS + API key
  ▼
Social Vault HTTP API (https://vault.adityahimaone.space)
  │
  ▼
SQLite (FTS5)
```

The MCP server talks **only** to the vault's HTTP REST API. It never reads
the local SQLite database directly and does not need `VAULT_PASSWORD`.
All writes go through the same API (and the same `UNIQUE(url)` dedup) as
the web UI and the browser extension.

## Build

```bash
# macOS / local
go build -o social-vault-mcp ./cmd/social-vault-mcp

# Linux amd64 (for VPS)
GOOS=linux GOARCH=amd64 go build -o social-vault-mcp-linux ./cmd/social-vault-mcp
```

## Environment variables

| Variable | Default | Description |
| --- | --- | --- |
| `VAULT_API_KEY` | _(required)_ | API key, sent as `Authorization: Bearer`. The server exits on startup if missing. |
| `VAULT_URL` | `https://vault.adityahimaone.space` | Base URL of the Social Vault HTTP API. |

The API key is the vault's single master key: anyone holding it can read
and write every bookmark. Keep it in the client's config file (0600) and
never commit it.

## Tools

| Tool | Description | Key inputs |
| --- | --- | --- |
| `search_bookmarks` | Full-text search (title, URL, description, notes, tags) | `query` (required), `limit`, `archived`, `tag`, `list` |
| `add_bookmark` | Save a URL; missing title/description is auto-fetched by the vault | `url` (required), `title`, `notes`, `tags[]` |
| `list_bookmarks` | Paginated listing, newest first | `limit` (default 20), `offset`, `archived` |
| `get_bookmark` | Full details for one bookmark | `id` (required) |
| `update_bookmark` | Update `title`, `notes`, `tags[]` (full replacement), or `archived` | `id` (required) |
| `list_tags` | All tags with bookmark counts | — |
| `list_lists` | All collections with bookmark counts | — |
| `get_stats` | Totals, active/archived counts, tag/list counts, top tags | — |

### Dedup

The vault enforces `UNIQUE(url)`. Saving a URL that already exists returns
HTTP `409` with the existing bookmark instead of creating a duplicate, so
bulk re-runs and retries are safe: already-saved items are skipped, not
duplicated.

## Connecting a client

### agy

```bash
agy mcp add --env VAULT_URL=https://vault.adityahimaone.space \
  --env VAULT_API_KEY="<your-vault-api-key>" \
  social-vault /path/to/social-vault-mcp

agy mcp list   # verify: social-vault, stdio, enabled
```

Restart (or start a new) `agy` session after adding the server.

### dsh (web profile)

Add a server entry under `mcp-manager` in the profile patch
(`~/.dsh/profiles/web/cordis.patch.yml`):

```yaml
- id: mcp-manager
  config:
    servers:
      social-vault:
        transport: stdio
        command: /path/to/social-vault-mcp
        args: []
        env:
          VAULT_URL: https://vault.adityahimaone.space
          VAULT_API_KEY: <your-vault-api-key>
        enabled: true
```

Restart the dsh web host for the new server to be picked up
(Settings → MCP).

### Generic MCP client (Claude Code / Claude Desktop)

```json
{
  "mcpServers": {
    "social-vault": {
      "command": "/path/to/social-vault-mcp",
      "env": {
        "VAULT_URL": "https://vault.adityahimaone.space",
        "VAULT_API_KEY": "<your-vault-api-key>"
      }
    }
  }
}
```

### Smoke test (no client needed)

Feed JSON-RPC over stdin — keep stdin open until the session ends, or the
server exits on EOF before answering:

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  | (cat; sleep 2) | VAULT_URL=https://vault.adityahimaone.space VAULT_API_KEY="<key>" ./social-vault-mcp
```

Expect an `initialize` result (`serverInfo.name: "social-vault-mcp"`) and a
`tools/list` result with all 8 tools.

## Agent tagging workflow

Bulk saves from the browser extension arrive **untagged by design** —
tagging depends on what each post actually is, so it belongs to an agent,
not to a fixed form. The intended flow:

1. **Find untagged bookmarks.** `list_bookmarks` with a high `limit`, or
   `search_bookmarks` for a topic, then `get_bookmark` for the ones with
   empty `tags`.
2. **Read the context.** Use `title`, `description`, `site_name`, and
   `url` (and the post content when reachable) to decide what the
   bookmark is about.
3. **Assign tags.** `update_bookmark` with `tags` — a full replacement
   list, so include existing tags you want to keep.
4. **Verify.** `get_stats` / `list_tags` to see tag coverage grow;
   `search_bookmarks` with `tag` to spot-check a tag.

Example: tag everything about Go recently saved from X:

```
1. search_bookmarks { "query": "golang", "limit": 50 }
2. For each result with tags: []:
   get_bookmark { "id": <id> }            # confirm context
   update_bookmark { "id": <id>, "tags": ["golang", "reading"] }
3. list_tags                               # verify counts
```

Suggested agent rules:

- Prefer a small, stable tag vocabulary (`golang`, `reading`, `threads`,
  `x`, `design`, …) over inventing a new tag per bookmark.
- Never remove tags another pass added unless they are clearly wrong.
- When unsure between two tags, keep both — a wrong tag is cheaper to fix
  than a missing one.
- Batch reads (`list_bookmarks` / `search_bookmarks`) before writing; one
  `update_bookmark` per bookmark.

## Troubleshooting

| Symptom | Likely cause |
| --- | --- |
| Server exits immediately with a configuration error | `VAULT_API_KEY` is missing or `VAULT_URL` is not `http(s)://` |
| `401` on every tool call | Wrong or revoked API key — check `VAULT_API_KEY` |
| `409` on `add_bookmark` | URL already saved — not an error; the response carries the existing bookmark |
| Client shows no tools / server won't start | Binary path in the client config is wrong, or the binary is for another OS/arch |
| `agy` says "authentication required" | That is `agy`'s own login, not the vault — run `agy` interactively once, or copy the `~/.gemini` + `~/.config/antigravity` auth files from a logged-in machine (see `agy` docs) |
