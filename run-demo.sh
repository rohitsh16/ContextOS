#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")" && pwd)
TMP=$(mktemp -d)
DEMO_DB="$(dirname "$TMP")/contextos-demo-$$.db"
trap 'rm -rf "$TMP"; rm -f "$DEMO_DB"' EXIT

git init -q "$TMP"
git -C "$TMP" config user.email demo@example.com
git -C "$TMP" config user.name ContextOS-Demo
printf 'package main\n\nfunc main() {}\n' > "$TMP/main.go"
git -C "$TMP" add .
git -C "$TMP" commit -qm init

export CONTEXTOS_DB="$DEMO_DB"
"$ROOT/bin/ctx" setup -repo "$TMP" >/dev/null
"$ROOT/bin/ctx" remember -repo "$TMP" -kind decision -authority user -content 'Use outbox for Kafka transaction retries.' >/dev/null
"$ROOT/bin/ctx" plan -repo "$TMP" -task 'Fix Kafka transaction retries' -budget 1000 -render
printf '\n--- hook injection ---\n'
printf '%s' '{"cwd":"'"$TMP"'","session_id":"demo","prompt":"Fix Kafka transaction retries"}' | "$ROOT/bin/ctx-hook" -agent claude -event UserPromptSubmit
printf '\n--- cache ---\n'
"$ROOT/bin/ctx" plan -repo "$TMP" -task 'Fix Kafka transaction retries' -budget 1000 | grep -o '"cache_hit": [^,]*' | head -1
