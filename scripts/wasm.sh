#!/usr/bin/env bash
# 게임 규칙을 브라우저에서 돌릴 수 있게 web/src/wasm 에 빌드한다. 규칙 코드를 바꾸면 다시 실행한다.
set -euo pipefail
cd "$(dirname "$0")/../server"
OUT=../web/src/wasm
mkdir -p "$OUT"
GOOS=js GOARCH=wasm go build -trimpath -buildvcs=false -ldflags="-s -w" -o "$OUT/game.wasm" ./cmd/wasm
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$OUT/wasm_exec.js"
chmod 644 "$OUT"/*
WASM_SUM=write go test -count=1 ./cmd/wasm >/dev/null
ls -l "$OUT/game.wasm"
