# Vectrex Data Tools

A static web page that makes the data files for the pw64vec and mk64vec cartridge games from your own ROM and soundtrack. The conversion runs in the visitor's browser (Go compiled to WebAssembly, in a Web Worker): nothing is uploaded and no server code runs.

- `site/` the page (index.html, worker.js, convert.wasm, wasm_exec.js): this is what gets hosted
- `go/` the converter's source (the same code as the Windows tools pw64tool and bgmtool); `./build.sh` rebuilds `site/convert.wasm`
- `render.yaml` a Render Blueprint for the static site

## Host it on Render
1. Put this folder in a Git repository you own (GitHub or GitLab) and push it.
2. In Render: New > Blueprint, pick the repository. Render reads `render.yaml` and publishes `site/` as a static site (free plan is fine).
3. Or: New > Static Site, publish directory `site`, no build command.

Tested here in headless Chromium: the page's output for a Pilotwings 64 (USA) ROM is byte for byte what the native tool writes (all 21 files).
