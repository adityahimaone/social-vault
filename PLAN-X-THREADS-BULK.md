# Social Vault extension — bulk fetch X/Threads (follow-up)

Repo: `~/Development/social-vault` (branch `main`). Ini lanjutan dari
build MCP + extension (PLAN-MCP-EXTENSION.md). Baca dulu struktur
`extension/` yang sudah ada + `README.md` repo sebelum mulai. Kerja di
branch `main`, commit lokal, JANGAN push ke GitHub (belum ada remote).

## Tujuan

Extension nempel di halaman Bookmark/Likes X dan Saved/Likes Threads,
lalu bisa bulk-fetch semua post yang tampil dan menyimpannya ke vault.

## Scope

1. **Content script baru** (`extension/content-x.js`,
   `extension/content-threads.js` — atau satu file dengan deteksi host)
   + tambah `host_permissions` + `content_scripts` di `manifest.json`:
   - `https://x.com/i/bookmarks`, `https://x.com/*/likes`
     (juga `twitter.com` yang sama, kalau masih dipakai).
   - `https://www.threads.net/*/saved`, `https://www.threads.net/*/likes`
     (sesuaikan dengan URL halaman Saved/Likes Threads yang sebenarnya —
     cek dulu, jangan asal tebak).
2. **Deteksi halaman**: content script aktif hanya di halaman-halaman
   itu (cek `location.pathname`). Di halaman lain, tidak ada UI tambahan.
3. **Floating panel kecil** (UI mengikuti skill ui-ux-pro-max /
   emil-design-eng yang sudah dipakai di extension):
   - Tombol "Save all to Vault" + counter post yang terdeteksi + progress
     saat menyimpan + hasil (berhasil/gagal/skip duplikat).
   - Toggle "Auto-scroll to load more" (default ON): saat bulk save
     berjalan, scroll otomatis ke bawah sampai tidak ada post baru atau
     mencapai batas (maks 200 post per sesi, boleh dihentikan kapan saja
     dengan tombol Stop).
4. **Ekstraksi URL post**:
   - X: dari elemen `article` di timeline, ambil link ke
     `/status/<id>` (link post). Abaikan tweet yang sudah tidak ada
     (deleted/unavailable).
   - Threads: dari link ke `/post/<id>` atau `/t/<id>` di halaman.
     DOM Threads pakai class obfuscated — JANGAN andalkan class name,
     andalkan struktur link dan atribut yang stabil. Kalau pola link
     Threads tidak bisa dipastikan, laporkan dan buat ekstraktornya
     se-defensif mungkin (coba beberapa pola, jangan crash).
   - Dedup URL sebelum dikirim.
5. **Bulk save**: kirim tiap URL ke `{VAULT_URL}/api/bookmarks` dengan
   `Authorization: Bearer <key>` (satu per satu, jeda kecil ~150ms,
   lanjut walau ada yang gagal; catat hasilnya). JANGAN buat endpoint
   bulk baru di server — pakai API yang sudah ada. Judul/notes dikosongi
   (server yang fetch metadata dari URL).
   - Hati-hati: kirim secukupnya dan beri progress; jangan spam API.
6. **Izin**: tambah host permission yang dibutuhkan; JANGAN minta
   permission yang tidak dipakai (`tabs` hanya jika benar-benar perlu —
   lebih baik pakai `activeTab` + content script saja).
7. Update `extension/README.md`: cara pakai fitur bulk (halaman mana,
   tombol di mana, batas 200/sesi), plus batasan yang diketahui
   (khususnya kerapuhan DOM Threads dan kemungkinan X mengubah markup).

## Batasan

- Jangan ubah MCP server atau backend vault.
- Jangan commit secret/API key. Jangan kirim data ke mana pun selain
  Vault URL yang dikonfigurasi user.
- Jangan auto-save tanpa aksi user: bulk fetch hanya jalan setelah user
  menekan tombol di panel (auto-scroll hanya jalan di dalam sesi bulk
  yang user mulai).

## Verifikasi

- `node --check` semua file JS baru/berubah; `manifest.json` valid.
- Uji ekstraktor dengan HTML contoh (fixture kecil di
  `extension/test/` atau script node): masukkan cuplikan DOM X
  (article + link status) dan Threads (link post) -> pastikan URL yang
  diekstrak benar + ter-dedup.
- Pastikan tidak ada URL hardcode selain default vault dan tidak ada API
  key di file mana pun.
- Uji live di browser nyata (Brave) dilakukan terpisah oleh user/Muse —
  laporkan apa yang belum dites di browser.

## Laporan akhir

File yang dibuat/diubah, cara kerja panel bulk + auto-scroll, hasil
validasi (node --check, fixture test), dan batasan/risiko yang diketahui.
