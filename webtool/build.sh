#!/bin/sh
# rebuilds site/convert.wasm from go/ (needs Go 1.21 or newer)
cd "$(dirname "$0")/go" && GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o ../site/convert.wasm . && cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" ../site/ && echo built
