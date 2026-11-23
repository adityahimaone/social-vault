#!/bin/bash
# Jalankan agy build social-vault di Mac (Orchestrator Neon).
# Dijalankan via nohup supaya putusnya SSH tidak membunuh agy.
export PATH="/opt/homebrew/bin:/opt/homebrew/sbin:$PATH"
mkdir -p ~/Development/social-vault
cd ~/Development/social-vault || exit 1
if [ ! -d .git ]; then
  git init -b main
  printf "social-vault\nsocial-vault-linux\n.env\n.env.*\ndata/\n*.log\n.DS_Store\nweb/node_modules/\nweb/dist/\n" > .gitignore
fi

PROMPT=$(cat <<'PROMPT_EOF'
Baca file PLAN.md di folder ini sampai paham, lalu baca dan GUNAKAN skill agy /ui-ux-pro-max dan /emil-design-eng (di ~/.gemini/skills/) untuk proyek ini. Bangun aplikasi Social Vault v2 (Go + Vite) dari nol sampai selesai sesuai brief, termasuk semua langkah verifikasi yang tertulis di sana.

Batasan tambahan: kerja di branch main repo lokal ini, commit perubahanmu secara lokal, JANGAN push ke GitHub (belum ada remote). Jangan install dependensi di luar yang dibutuhkan brief. Jangan commit secret apa pun.

Terakhir, laporkan dengan jelas: struktur yang dibuat, hasil gofmt/vet/test/build, dan hasil verifikasi lokal.
PROMPT_EOF
)

~/.local/bin/agy --model gemini-pro-orchestrator-neon --dangerously-skip-permissions --print-timeout 0 -p="$PROMPT"
echo "AGY_EXIT:$?"
