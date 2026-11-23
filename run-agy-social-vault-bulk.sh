#!/bin/bash
# Jalankan agy build bulk X/Threads untuk extension social-vault di Mac (Orchestrator Neon).
export PATH="/opt/homebrew/bin:/opt/homebrew/sbin:$PATH"
cd ~/Development/social-vault || exit 1

PROMPT=$(cat <<'PROMPT_EOF'
Baca file PLAN-X-THREADS-BULK.md di folder ini sampai paham, lalu baca dan GUNAKAN skill agy /ui-ux-pro-max dan /emil-design-eng (di ~/.gemini/skills/) untuk proyek ini. Tambahkan fitur bulk fetch X/Threads ke extension Social Vault sampai selesai sesuai brief, termasuk semua langkah verifikasi yang tertulis di sana.

Batasan tambahan: kerja di branch main repo lokal ini, commit perubahanmu secara lokal, JANGAN push ke GitHub (belum ada remote). Jangan install dependensi di luar yang dibutuhkan brief. Jangan commit secret apa pun. Jangan menulis apa pun ke vault live saat testing — pakai server test lokal dengan DATA_DIR temporer.

Terakhir, laporkan dengan jelas: file yang dibuat/diubah, cara kerja panel bulk + auto-scroll, hasil validasi (node --check, fixture test), dan batasan/risiko yang diketahui.
PROMPT_EOF
)

~/.local/bin/agy --model gemini-pro-orchestrator-neon --dangerously-skip-permissions --print-timeout 0 -p="$PROMPT"
echo "AGY_EXIT:$?"
