#!/bin/bash
# Tunggu agy build bulk X/Threads selesai di Mac (max ~60 menit),
# lalu tulis ringkasan ke agy-bulk-wait-result.txt.
export PATH="/opt/homebrew/bin:/opt/homebrew/sbin:$PATH"
cd ~/Development/social-vault || exit 1
for i in $(seq 1 120); do
  if ! pgrep -f "gemini-pro-orchestrato[r]-neon" >/dev/null 2>&1; then
    break
  fi
  sleep 30
done
{
  echo "=== WAIT DONE $(date) ==="
  echo "--- git status ---"
  git status --short | head -n 30
  echo "--- git log ---"
  git log --oneline -5
  echo "--- agy-bulk.log tail ---"
  tail -n 100 agy-bulk.log
  echo "--- agy CLI still running? ---"
  pgrep -fl "gemini-pro-orchestrato[r]-neon" || echo "agy CLI not running"
} | tee agy-bulk-wait-result.txt
