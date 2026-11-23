# Social Vault v2 — Go + Vite, dari nol, scope kecil

Repo BARU: `~/Development/social-vault` (local dulu, belum ada remote
GitHub). Ini pengganti ringan untuk fork Karakeep (bookmark-manager) yang
terlalu berat untuk VPS kecil — BUKAN lanjutan fork itu. Fork Karakeep
tetap dibiarkan apa adanya (branch `muse/turso-phase1`, DB Turso
`bookmark-manager` sudah live) sampai app baru ini siap.

## Skill yang WAJIB dipakai

Agy punya dua skill di `~/.gemini/skills/` — pakai keduanya:
- **ui-ux-pro-max** (`~/.gemini/skills/ui-ux-pro-max/SKILL.md`)
- **emil-design-eng** (`~/.gemini/skills/emil-design-eng/SKILL.md`)

Baca kedua file SKILL.md itu dulu, lalu ikuti panduan dan prinsip
keduanya selama mengerjakan app ini (UI, layout, tipografi, warna,
aksesibilitas, interaksi).

## Scope MVP (kecil, jangan melebar)

Fokus ke yang benar-benar dipakai user: simpan link (terutama hasil
harvest X/Threads), cari, tag, baca lagi. Yang TIDAK masuk scope:
AI tagging, RSS, importers selain di bawah, sharing publik, multi-user,
mobile app, extension resmi Karakeep.

1. **Auth**: single-user — login password dari env (`VAULT_PASSWORD`),
   cookie session (seperti pola tailmon). Plus `VAULT_API_KEY` untuk
   endpoint tambah bookmark (dipakai harvester X/Threads user).
2. **Bookmark CRUD** (REST JSON + UI):
   - Tambah via URL: fetch halaman, parse title/description/favicon
     (fetch + parse HTML, TANPA headless Chrome).
   - List, detail, edit (title/notes/tags), archive/unarchive, delete.
   - Tag + lists (koleksi) — CRUD sederhana, filter by tag/list.
   - Search full-text (SQLite FTS5) + filter (all/archived/tag/list).
3. **Import**: file bookmark browser format Netscape HTML + JSON
   Karakeep export (upload file, lalu proses di server).
4. **API** (JSON, prefix `/api`): `GET/POST /api/bookmarks`,
   `GET/PATCH/DELETE /api/bookmarks/:id`, `GET/POST/DELETE /api/tags`,
   `GET/POST/DELETE /api/lists`, `POST /api/import`,
   `GET /api/health`. API key lewat header `Authorization: Bearer` atau
   `X-Api-Key`; session cookie juga berlaku untuk UI.
5. **Frontend**: Vite + React + TypeScript. Dashboard: kartu statistik
   (total/archived/tag terbanyak), daftar bookmark dengan search,
   drawer detail, form tambah (paste URL), halaman tags/lists.
   Dark/light theme. Responsif. Build output di-embed ke binary Go.
6. **Backend**: Go (stdlib sebisa mungkin) + `modernc.org/sqlite`
   (pure Go, tanpa CGO) + FTS5. Single binary yang serve API + frontend
   statis (go:embed). Config via env: `PORT` (default 127.0.0.1:8787),
   `DATA_DIR` (file SQLite + upload), `VAULT_PASSWORD`, `VAULT_API_KEY`.
   Migrasi SQL di-embed, jalan otomatis saat start.
7. **Struktur repo**: `go.mod`, `main.go`, `internal/` (db, api, auth,
   crawler), `migrations/`, `web/` (Vite app), `README.md`,
   `.gitignore` (binary, DATA_DIR, .env, node_modules, dist).

## Batasan

- JANGAN pakai Docker, Meilisearch, atau headless Chrome.
- JANGAN commit secret/token/password/API key apa pun. `.env` hanya
  contoh di `.env.sample` (tanpa nilai asli).
- Commit lokal di branch `main` (repo baru, `git init -b main`),
  JANGAN push ke GitHub (belum ada remote).
- Jangan install dependensi di luar yang dibutuhkan brief.

## Verifikasi (lakukan semuanya, jangan cuma klaim)

- `gofmt -l .` bersih, `go vet ./...` lolos, `go test ./...` lolos
  (tulis test untuk: tambah + cari + hapus bookmark, FTS search,
  auth salah -> 401).
- `go build` (darwin) dan `GOOS=linux GOARCH=amd64 go build -o
  social-vault-linux .` lolos. `pnpm --prefix web build` (atau npm)
  lolos dan hasil `dist/` ter-embed (cek ukuran binary berubah).
- Jalankan binary lokal di Mac (`PORT=127.0.0.1:8898`,
  `DATA_DIR` temporer, password + API key test), lalu:
  - `GET /api/health` -> `HTTP 200`.
  - `GET /api/bookmarks` tanpa auth -> `HTTP 401`.
  - Login via UI/API -> cookie; `POST /api/bookmarks` dengan URL
    `https://example.com` (pakai API key) -> `HTTP 201` dan title
    terisi dari hasil fetch; `GET /api/bookmarks?q=example` -> ketemu;
    `DELETE` -> terhapus.
  - Buka `/` dan pastikan HTML hasil embed Vite ter-serve.
  - Cek JS frontend: `node --check` tidak relevan untuk bundle —
    sebagai gantinya pastikan `pnpm build` tanpa error dan tanpa
    warning fatal.
- Matikan binary test + hapus DATA_DIR temporer setelah verifikasi.

## Laporan akhir

File/struktur yang dibuat, hasil gofmt/vet/test/build, hasil verifikasi
lokal (endpoint + UI), dan apa yang belum dites (deploy ke VPS + uji
dengan URL X/Threads asli dilakukan terpisah oleh Muse).
