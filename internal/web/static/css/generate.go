package css

// Regenerate app.css from ../../tailwind.css after any .templ or JS class change.
// Requires the standalone Tailwind v4 CLI (see /tools/README.md). app.css is committed
// so `go build` works without the CLI.
//go:generate ../../../../tools/tailwindcss -i ../../tailwind.css -o app.css --minify
