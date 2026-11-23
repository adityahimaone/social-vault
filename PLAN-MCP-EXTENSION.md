# Social Vault v2 — MCP server + browser extension

Repo: `~/Development/social-vault` (branch `main`, commit terakhir
`6c30455`). Kerja di branch `main`, commit lokal, JANGAN push ke GitHub
(belum ada remote). Baca dulu `README.md` dan `PLAN.md` di repo ini
supaya paham API yang sudah ada (REST JSON di `/api`, auth cookie
`sv_session` + API key `Authorization: Bearer` / `X-Api-Key`).

Skill agy yang WAJIB dipakai (seperti build sebelumnya):
- **ui-ux-pro-max** (`~/.gemini/skills/ui-ux-pro-max/SKILL.md`)
- **emil-design-eng** (`~/.gemini/skills/emil-design-eng/SKILL.md`)
Baca kedua SKILL.md itu dulu; terapkan untuk UI extension (popup +
options page).

## Bagian 1 — MCP server (Go, stdio)

Tujuan: agy/dsh/Claude bisa cari, simpan, dan kelola bookmark di vault
lewat MCP, memakai API HTTP vault yang sama (bukan akses DB langsung).

1. Binary baru `social-vault-mcp` dari module yang sama
   (`cmd/social-vault-mcp/main.go` + paket `internal/mcp/`).
2. Transport stdio, protokol MCP (JSON-RPC). Pakai SDK resmi MCP untuk
   Go (`github.com/modelcontextprotocol/go-sdk`) kecuali ada alasan kuat
   tidak — kalau pakai SDK, tambahkan ke `go.mod` dan dokumentasikan.
3. Config via env: `VAULT_URL` (default `https://vault.adityahimaone.space`),
   `VAULT_API_KEY` (wajib; tanpa ini server menolak start dengan pesan
   jelas). Server TIDAK membaca DB lokal dan TIDAK butuh
   `VAULT_PASSWORD`.
4. Tools (nama + deskripsi jelas, schema input JSON yang ketat):
   - `search_bookmarks` (query, limit, archived?, tag?, list?)
   - `add_bookmark` (url, title?, notes?, tags[]?)
   - `list_bookmarks` (limit, offset, archived?)
   - `get_bookmark` (id)
   - `list_tags`, `list_lists`, `get_stats`
   - `update_bookmark` (id, title?, notes?, tags[]?, archived?) — opsional
     tapi usahakan ada.
   Setiap tool mengembalikan JSON ringkas (bukan dump mentah); error API
   diteruskan sebagai error MCP dengan pesan yang bisa dibaca manusia.
5. `README.md` di repo ditambah bagian MCP: cara build, env yang
   dibutuhkan, contoh konfigurasi untuk agy/dsh (stdio command + env).
6. JANGAN hardcode API key atau URL vault lain. JANGAN commit secret.

## Bagian 2 — Browser extension (Manifest V3, Chrome/Brave)

Tujuan: simpan halaman yang sedang dibuka ke vault dengan satu klik,
pengganti extension Karakeep.

1. Folder `extension/` di repo, tanpa build step (HTML/CSS/JS polos):
   `manifest.json`, `popup.html` + `popup.js`, `options.html` +
   `options.js`, `background.js` (service worker), `content.js`
   (opsional, hanya jika dibutuhkan), `icons/` (16/32/48/128 PNG —
   buat dengan script, jangan unduh aset berlisensi).
2. Fitur:
   - Popup: URL + title halaman aktif terisi otomatis, field notes,
     field tags (comma-separated), tombol Save; tampilkan sukses/gagal
     dengan jelas (termasuk pesan error dari API).
   - Context menu: "Save to Social Vault" untuk halaman dan link.
   - Options page: Vault URL (default
     `https://vault.adityahimaone.space`) + API key; disimpan di
     `chrome.storage.local` (JANGAN sync, JANGAN kirim ke mana pun
     selain Vault URL yang dikonfigurasi).
   - Badge/ikon berubah sesaat saat save berhasil.
3. Semua request ke vault memakai `fetch` ke `{VAULT_URL}/api/bookmarks`
   dengan header `Authorization: Bearer <key>`; tangani CORS/HTTP error
   dengan pesan yang dimengerti user awam.
4. `extension/README.md`: cara install unpacked di Brave/Chrome
   (`brave://extensions` → Developer mode → Load unpacked), cara isi
   API key, dan batasan yang diketahui.

## Batasan umum

- Jangan ubah perilaku server vault yang sudah live (`main.go`,
  `internal/api`, `internal/db`, `web/`) kecuali benar-benar perlu
  untuk MCP/extension — dan kalau perlu, hanya secara aditif + jelaskan
  alasannya di laporan.
- Jangan install dependensi di luar yang dibutuhkan brief.
- Jangan commit secret/token/password/API key apa pun (nilai asli hanya
  boleh ada di env lokal saat testing, lalu dihapus).

## Verifikasi (lakukan semuanya, jangan cuma klaim)

- `gofmt -l .` bersih, `go vet ./...` lolos, `go test ./...` lolos
  (tulis test untuk MCP: minimal tools/list + satu tools/call
  `search_bookmarks` dan `add_bookmark` terhadap server test).
- `GOOS=linux GOARCH=amd64 go build -o social-vault-mcp-linux
  ./cmd/social-vault-mcp` lolos; `go build -o social-vault-mcp
  ./cmd/social-vault-mcp` (darwin) lolos.
- MCP end-to-end (PENTING — JANGAN tulis ke vault live saat testing):
  jalankan vault lokal di `127.0.0.1:8898` dengan `DATA_DIR` temporer
  (seperti verifikasi sebelumnya), lalu jalankan `social-vault-mcp`
  dengan `VAULT_URL=http://127.0.0.1:8898` + API key test, kirim
  request JSON-RPC via stdin: `initialize`, `tools/list`,
  `tools/call search_bookmarks`, `tools/call add_bookmark`
  (URL `https://example.com`), lalu cek bookmark-nya ada lewat API
  lokal. Matikan server test + hapus `DATA_DIR` temporer setelahnya.
- Extension: `node --check` semua file JS; validasi `manifest.json`
  (JSON parse + field wajib MV3: manifest_version 3, name, version,
  action, permissions, host_permissions); pastikan tidak ada URL
  hardcode selain default vault dan tidak ada API key di file mana pun
  (`grep -r` untuk pola key yang dipakai saat testing).
- Matikan semua proses test setelah verifikasi.

## Laporan akhir

File/struktur yang dibuat (MCP + extension), hasil gofmt/vet/test/build,
hasil verifikasi MCP end-to-end, hasil validasi extension, dan apa yang
belum dites (registrasi MCP di agy/dsh + install extension di Brave
dilakukan terpisah oleh Muse).
