# PLAN: Hybrid three-layer auto-tagging (rules + Jev + agent review queue)

## Goal

Bookmarks saved to Social Vault get tagged automatically in three layers, in
order, cheapest and most certain first. Nothing the user or another layer
tagged is ever removed. No new tag names outside the fixed vocabulary.

## Current state (verified)

- Repo: VM clone at `f9f89ac`, branch `main`. Live vault has 400 bookmarks:
  377 tagged by an earlier agent pass, 23 untagged because their saved
  metadata is too thin (emoji-only title, bare handle, protected account).
- `POST /api/bookmarks` (`internal/api/bookmarks.go`, `handleCreateBookmark`):
  validates URL, returns 409 on duplicate, crawls metadata with
  `crawler.Fetcher` when title/description/favicon are missing, then calls
  `s.store.CreateBookmark` with `Tags: body.Tags` (user-supplied only).
  There is no server-side auto-tagging today.
- Server wiring: `main.go` reads `PORT`, `DATA_DIR`, `VAULT_PASSWORD`,
  `VAULT_API_KEY`; `api.NewServer(store, auth, fetcher, webFS)`.
- Migrations live in `migrations/` (`001_init.sql`, `002_bookmark_image_url.sql`).
- MCP server in `internal/mcp` has 8 tools; its workflow is documented in
  `docs/mcp.md`. The agent tags today via `update_bookmark`.

## Fixed vocabulary (19 tags, never invent others)

ai, design, tools, development, deals, web, reading, career, news,
entertainment, lifestyle, kpop, finance, gaming, nsfw, security, rust,
sports, golang.

## Evidence that shapes the design

- Rule-based analysis of the 377 AI-tagged bookmarks
  (`~/workspace/analyze-tag-patterns.py`): exact-set match 16%, coverage
  (>=1 correct tag) 70%, mean recall 54%, mean precision 53%. Per-tag recall:
  kpop 100%, rust 100%, career 77%, deals 71%, ai 68%, gaming 64%,
  design 57%, tools 50%, development 48%, web 48%, entertainment 48%,
  reading 45%, news 35%, nsfw 33%, lifestyle 17%, finance 0%, security 0%,
  golang 0% (no keyword signal at all). Generic words (free, you, your, try,
  out) pollute derived rules and must be excluded. "design" appears in
  ai-tagged posts and "deals" shares free/flash/deepseek with ai.
- Jev (TypeSafe AI) validation on a 30-bookmark stratified sample, AI tags as
  reference: threshold 0.5 gives precision 0.76, recall 0.68, wrong-tag 40%,
  abstain 7%; 0.7 gives 0.73 / 0.52 / 27% / 13%; 0.8 gives 0.61 / 0.41 /
  13% / 33%. Prototype on the 23 untagged bookmarks: 0/23 reached 0.7.
  Conclusion: Jev is a conservative second layer, not a 1:1 replacement
  for the agent pass. Cost is negligible (~$0.002 for 30 calls).
- Jev API (verified live 24 Nov 2026): `POST https://api.typesafe.ai/v1/systemone`,
  model `jev-1.13.0` pinned. `jev-1.13`, `jev`, `typesafe/jev` all return
  400 "Unknown model"; the `model` field is required (422 if omitted).
  One call carries `state` = {title, description, site_name, url} and 19
  parallel `noul` questions, one per tag, each with `instructions` and
  `criteria` {true, false}. Answers come back as
  `answers.<tag>.noul`, a probability from 0 to 1. About 1,800 input tokens
  per call (the 19 questions dominate), $0.042 per 1M input tokens, output
  free. Response also carries `usage` {input_tokens, output_tokens}.

## Layer 1: rules (synchronous, at save time)

- New package `internal/tagging`, file `rules.go`. Curated keyword rules
  derived from the 377 tagged bookmarks. Start from
  `~/workspace/analyze-tag-patterns.py` (it computes top keywords per tag
  with support >= 3 and precision >= 0.5) and hand-curate the final lists.
- Rules cover only tags with a real keyword signal. They must NOT cover
  finance, security, or golang (zero signal; those go to Jev/agent only).
  Exclude the noise words (free, you, your, try, out) unless a rule needs
  one inside a longer phrase.
- Runs inside `handleCreateBookmark` after the metadata crawl, before
  `CreateBookmark`. Merge rule tags with user-supplied tags, deduplicate,
  never drop a user tag. Rules are pure functions on title + description +
  site_name + URL host, so they are unit-testable without network or DB.

## Layer 2: Jev (asynchronous, after save)

- New file `internal/tagging/jev.go`: a small client for
  `POST https://api.typesafe.ai/v1/systemone` with model `jev-1.13.0`,
  the 19 `noul` questions, and a 15-second timeout. API key from env
  `JEV_API_KEY`, read in `main.go` and passed into the tagging service.
  If the key is empty, layer 2 is skipped with a startup log warning in the
  same style as the existing `VAULT_API_KEY` warning. Never fail a save
  because Jev failed.
- Runs in a goroutine after `CreateBookmark` returns, so the POST response
  is never slowed down. A Jev call happens only when layer 1 left the
  bookmark with zero tags, or when the caller asks for a full pass (see
  backfill). Tags with probability >= 0.8 are added to the bookmark: read
  the current tags first, merge, and write back through the existing update
  path, never overwriting tags another layer or the user added.
- Tags with probability in [0.5, 0.8) are NOT applied. They go to the review
  queue with their scores. Below 0.5 is ignored.
- Log Jev failures (non-2xx, timeout, malformed response) and continue.
  Do not retry inside the request path.

## Layer 3: agent review queue

- Migration `003_tag_review_queue.sql`: table `tag_review_queue`
  (id, bookmark_id FK, suggested_tags JSON, jev_scores JSON,
  source TEXT e.g. 'jev', status TEXT pending/applied/dismissed,
  created_at, resolved_at). A bookmark gets at most one pending queue row.
- DB methods in `internal/db`: list pending queue rows (with the bookmark's
  title/url), mark a row applied or dismissed.
- API (protected, same auth middleware as the other routes):
  `GET /api/tag-review` lists pending rows;
  `POST /api/tag-review/{id}/apply` merges the suggested tags into the
  bookmark and marks the row applied;
  `POST /api/tag-review/{id}/dismiss` marks it dismissed.
  `POST /api/tagging/backfill` (protected, body {limit?: number, default 50})
  runs layers 1 and 2 over untagged bookmarks and returns counts
  {processed, rules_tagged, jev_tagged, queued, skipped, failed}. It must be
  safe to run twice: skip bookmarks that already have tags and skip
  bookmarks that already have a pending queue row.
- MCP: add tools `list_tag_review` and `resolve_tag_review` to
  `internal/mcp`, following the existing 8-tool pattern, and document the
  review workflow in `docs/mcp.md`: the agent lists pending items, reads the
  bookmark, applies the good suggestions and dismisses the bad ones.

## Scope limits

- No frontend changes in this task. The queue is worked through MCP and the
  API for now.
- No new tag names. No removal of existing tags by any layer.
- No API key value anywhere in code, docs, tests, or logs. Tests use a stub
  HTTP server for Jev.
- Do NOT deploy. Deploy is a separate step the user approves afterwards.
  At deploy time the Jev key must be added to `/etc/social-vault.env` on the
  VPS as `JEV_API_KEY` (the user holds the key); this task only reads it
  from the environment.

## Verification (must all pass before reporting done)

- `gofmt -l` clean, `go vet ./...`, `go test ./...`.
- Unit tests: rules (including the noise-word exclusions and that
  finance/security/golang are never rule-tagged); Jev client against a stub
  server (threshold behavior at 0.8/0.5, timeout, non-2xx handling);
  queue store tests.
- `go build` for linux, then a local test server smoke test: health 200,
  create a bookmark with an obvious keyword (e.g. a Rust post) and confirm
  the rule tag lands, create one with the Jev key unset and confirm the save
  still succeeds and the queue stays empty, backfill endpoint returns sane
  counts, review endpoints list/apply/dismiss correctly.
- `pnpm build` in `web/` still passes (no frontend changes expected, but
  the embedded build must stay green).

## Deliverable

- Commit on `main` in this VM clone with the message
  `feat(tagging): three-layer hybrid auto-tagging (rules + Jev + review queue)`.
  Do NOT push (no GitHub credentials in this VM).
- Report: what each layer does, the rule lists you chose (per tag, with the
  evidence for the borderline ones), verification results, and what is still
  manual (adding `JEV_API_KEY` to the VPS env at deploy time).
