# tools

## tailwindcss (standalone CLI)

Compiles `internal/web/tailwind.css` into the committed `internal/web/static/css/app.css`.

The binary is **git-ignored** (`tools/tailwindcss`): a 100 MB+ per-OS executable does
not belong in the repo. `app.css` is committed, so `go build` never needs the CLI.

Download the Tailwind v4 standalone CLI for your OS/arch from the release assets:

```
https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-<os>-<arch>
```

Examples:
- macOS arm64: `tailwindcss-macos-arm64`
- macOS x64:   `tailwindcss-macos-x64`
- Linux x64:   `tailwindcss-linux-x64`
- Windows x64: `tailwindcss-windows-x64.exe`

```bash
mkdir -p tools
curl -sL -o tools/tailwindcss https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-macos-arm64
chmod +x tools/tailwindcss
./tools/tailwindcss --help | head -3
```

Regenerate `app.css`:

```bash
go generate ./...
```
