# lks-judge

Single-binary judging server for LKS (Indonesian vocational skills competition).
Runs on a Windows LAN for ~16 simultaneous participants with **zero runtime
dependencies**: no PHP, no Node, no system SQLite, no CGO. Replaces a previous
Laravel + FrankenPHP + Reverb stack that buckled under concurrent load.

## Status

Scaffold. Phases 1–6 are done; the judging loop itself is not built yet.

| Phase | Scope | State |
| ----- | ----- | ----- |
| 1 | Foundation: dual SQLite pool, migrations, embedded assets | done |
| 2 | Auth: jury IP allowlist, participant session login | done |
| 3 | Layouts: templ views, app + guest shells | done |
| 4 | Competition setup | done |
| 5 | Participants: CRUD, Excel import/export, seat shuffle | done |
| 6 | Modules: CRUD, current-module selection | done |
| 7 | Countdown | not started |
| 8 | WebSocket hub | not started |
| 9 | Chunked upload | not started |
| 10 | Scoring + leaderboard | not started |

Per-phase detail lives in [`docs/CHANGELOG.md`](docs/CHANGELOG.md).

## Requirements

- Go 1.25+
- [`templ`](https://templ.guide), install with `go install github.com/a-h/templ/cmd/templ@latest`

Contributors also need [`golangci-lint`](https://golangci-lint.run) and
[`lefthook`](https://lefthook.dev) for the commit gates.

SQLite is compiled in via `modernc.org/sqlite` (pure Go), so there is nothing to
install and `CGO_ENABLED=0` works everywhere.

## Build

```bash
go generate ./...          # compiles .templ files, required before every build
go build ./cmd/server
```

Any change to a `.templ` file needs `go generate ./...` (or `templ generate`)
before `go build`, or you will build against stale views.

Windows target:

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o lks-judge.exe ./cmd/server
```

## Run

```bash
./server --data ./data --listen 0.0.0.0:8080
```

| Flag | Default | Meaning |
| ---- | ------- | ------- |
| `--data` | `./data` | Data directory; the database lives at `{data}/lks.sqlite` |
| `--listen` | `0.0.0.0:8080` | HTTP listen address |
| `--dev` | `false` | Seed a default competition and one participant |

`--dev` seeds a participant whose password is `123456`. **Never run a real
competition with `--dev`.**

There are no environment variables, all configuration is flags.

## Layout

```
cmd/server/       assembler; wires router, store, backup ticker
internal/model/   plain structs, no logic
internal/store/   SQLite pools, migrations, caches
internal/web/     handlers, middleware, templ views, embedded static assets
internal/excel/   participant import/export
internal/backup/  periodic VACUUM INTO
docs/             changelog
```

Two import rules keep the package graph acyclic: `model` imports nothing
internal, and `realtime` must never import `store`.

## Test

```bash
go test ./...
go test ./... -race
```

## Contributing

Conventional commits: `feat(scope): ...`, `fix`, `docs`, `chore`, `refactor`.

Branch flow: `feat/*` → `develop` → `staging` → `main`. No direct commits to
`main`, no force-pushing shared branches.

Run `lefthook install` before your first commit. Pre-commit runs gofmt, `go
vet`, and golangci-lint; pre-push runs `go build ./...` and `go test ./...`.
