# Social Vault v2

Lightweight single-binary bookmark vault built with **Go**, **SQLite (pure Go `modernc.org/sqlite` + FTS5)**, and an embedded **Vite + React + TypeScript** interface.

Designed as a fast, low-memory replacement for heavy bookmark stacks on small VPS instances — save links (including X/Threads harvests), search full-text, tag, organize into lists, and read again.

## Features

- **Single binary**: Go backend + SQLite migrations + built Vite frontend embedded via `go:embed`.
- **Zero CGO / zero external services**: Pure Go SQLite with WAL mode and FTS5 full-text search.
- **Metadata fetcher**: Automatically parses `<title>`, OpenGraph/Twitter meta tags, and favicons on save (standard HTTP + HTML tokenizer, no headless browser).
- **Single-user auth**: Password-protected cookie sessions (`VAULT_PASSWORD`) for the web UI + API key (`VAULT_API_KEY`) via `Authorization: Bearer <key>` or `X-Api-Key: <key>` for scripts/harvesters.
- **Importers**: Netscape HTML browser bookmark exports and Karakeep JSON exports.
- **Responsive UI**: Dark/light themes, keyboard shortcuts (`/` search, `n` new URL, `Esc` close), slide-over detail drawer, tag & list management.

## Configuration

Copy `.env.sample` for local runs (never commit `.env`):

| Variable | Default | Description |
| --- | --- | --- |
| `PORT` | `127.0.0.1:8787` | Listen address (`host:port` or port number) |
| `DATA_DIR` | `./data` | Directory for `vault.db` SQLite database |
| `VAULT_PASSWORD` | _(required for UI login)_ | Password for single-user web session login |
| `VAULT_API_KEY` | _(optional)_ | API key for `Authorization: Bearer` / `X-Api-Key` |

## Build & Run

### 1. Build frontend & Go binary

```bash
pnpm --prefix web install
pnpm --prefix web build
go build -o social-vault .
```

### 2. Cross-compile for Linux (`amd64`)

```bash
GOOS=linux GOARCH=amd64 go build -o social-vault-linux .
```

### 3. Run locally

```bash
VAULT_PASSWORD="your-password" VAULT_API_KEY="your-api-key" ./social-vault
```

Open `http://127.0.0.1:8787`.

### Frontend dev mode

```bash
# Terminal 1 (API)
PORT=127.0.0.1:8787 VAULT_PASSWORD="dev" VAULT_API_KEY="dev-key" go run .

# Terminal 2 (Vite dev server with /api proxy)
pnpm --prefix web dev
```

## REST API

All routes except `/api/health`, `/api/auth/me`, `/api/auth/login`, and `/api/auth/logout` require either a valid `sv_session` cookie or an API key (`Authorization: Bearer <VAULT_API_KEY>` or `X-Api-Key: <VAULT_API_KEY>`).

- `GET /api/health`
- `GET /api/auth/me`, `POST /api/auth/login`, `POST /api/auth/logout`
- `GET /api/stats`
- `GET /api/bookmarks` (`q`, `archived=all|true|false`, `tag`, `list`, `limit`, `offset`)
- `POST /api/bookmarks` (`{"url", "title", "description", "notes", "tags", "lists", "source"}`)
- `GET /api/bookmarks/{id}`, `PATCH /api/bookmarks/{id}`, `DELETE /api/bookmarks/{id}`
- `GET /api/tags`, `POST /api/tags`, `DELETE /api/tags/{id}`
- `GET /api/lists`, `POST /api/lists`, `PATCH /api/lists/{id}`, `DELETE /api/lists/{id}`
- `POST /api/import` (`multipart/form-data` with `file` and optional `format=auto|netscape|karakeep`)

### Example: Add a harvested link via API key

```bash
curl -X POST http://127.0.0.1:8787/api/bookmarks \
  -H "Authorization: Bearer $VAULT_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com/interesting-thread",
    "tags": ["threads", "reading"],
    "source": "harvester"
  }'
```

## MCP Server (`social-vault-mcp`)

Full docs (tools, client setup for agy/dsh, agent tagging workflow, troubleshooting): [docs/mcp.md](docs/mcp.md)

`social-vault-mcp` exposes Social Vault over the Model Context Protocol (`stdio` JSON-RPC) using the official Go SDK (`github.com/modelcontextprotocol/go-sdk`). It talks strictly to the vault's HTTP REST API — it never reads the local SQLite database and does not require `VAULT_PASSWORD`.

### Build

```bash
# macOS / local
go build -o social-vault-mcp ./cmd/social-vault-mcp

# Linux amd64 (for VPS)
GOOS=linux GOARCH=amd64 go build -o social-vault-mcp-linux ./cmd/social-vault-mcp
```

### Environment Variables

| Variable | Default | Description |
| --- | --- | --- |
| `VAULT_API_KEY` | _(required)_ | API key sent via `Authorization: Bearer` (server exits on startup if missing) |
| `VAULT_URL` | `https://vault.adityahimaone.space` | Base URL of the Social Vault HTTP API |

### Tools

- `search_bookmarks` — full-text search with `query`, `limit`, and optional `archived`, `tag`, `list` filters.
- `add_bookmark` — save an `http`/`https` `url` with optional `title`, `notes`, and `tags[]`.
- `list_bookmarks` — paginated bookmark listing (`limit`, `offset`, `archived`).
- `get_bookmark` — fetch full details for a bookmark by `id`.
- `update_bookmark` — update `title`, `notes`, `tags[]`, or `archived` status by `id`.
- `list_tags` — list all tags with bookmark counts.
- `list_lists` — list all collections with bookmark counts.
- `get_stats` — fetch total, active, archived, tag/list counts, and top tags.

### Example Client Configuration (`agy` / `dsh` / Claude)

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

## Browser Extension

A zero-build Manifest V3 extension for Brave and Chrome lives in [`extension/`](extension/README.md) with a one-click save popup, page/link context menus, badge status feedback, and `chrome.storage.local` settings.
