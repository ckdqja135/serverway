#!/usr/bin/env bash
# 화면을 빌드하고 게임 서버를 실행한다. 열기: http://127.0.0.1:7800
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/web" && [ -d node_modules ] || npm ci
npm run build >/dev/null
cd "$ROOT/server" && mkdir -p ../.run && go build -o ../.run/serverway ./cmd/serverway
SERVERWAY_WEB="$ROOT/web/dist" exec "$ROOT/.run/serverway"
