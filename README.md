# lks-judge

Single-binary judging server for LKS (Indonesian vocational skills competition).
Runs on a Windows LAN for ~16 simultaneous participants with **zero runtime
dependencies**: no PHP, no Node, no system SQLite, no CGO. Replaces a previous
Laravel + FrankenPHP + Reverb stack that buckled under concurrent load.

## Status

Phases 1-10 are done: setup, participants, modules, the competition
countdown, the WebSocket hub that pushes live events, chunked resumable
file upload with jury file management, and participant submissions with the
jury review matrix. Scoring and the leaderboard are not built yet.

| Phase | Scope | State |
| ----- | ----- | ----- |
| 1 | Foundation: dual SQLite pool, migrations, embedded assets | done |
| 2 | Auth: jury IP allowlist, participant session login | done |
| 3 | Layouts: templ views, app + guest shells | done |
| 4 | Competition setup | done |
| 5 | Participants: CRUD, Excel import/export, seat shuffle | done |
| 6 | Modules: CRUD, current-module selection | done |
| 7 | Countdown: jury control, public display, polling endpoint | done |
| 8 | WebSocket hub: `GET /ws`, live countdown and module events | done |
| 9 | Chunked upload: resumable 2MB chunks, jury file manager, Range download | done |
| 10 | Submissions: live dashboard, per-module upload, jury matrix + ZIP export | done |
| 11 | Scoring + leaderboard | not started |

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

Under `--data` the server keeps `backups/`, `files/` (jury files), `submissions/`
(participant work, laid out `submissions/{participant_id}/{module_id}/`), and
`uploads_tmp/` (in-flight chunks) alongside `lks.sqlite`.

Four routes are public, everything else is behind the jury IP allowlist or a
participant session (plus `GET /login` and the `GET /static/` asset tree):

| Route | Purpose |
| ----- | ------- |
| `GET /countdown` | Full-screen countdown for a projector; plays an alert at zero |
| `GET /countdown/time` | `{"seconds":N,"status":"..."}`, polled once a second |
| `GET /ws` | WebSocket; anonymous clients get a reduced event set (countdown + score only) |
| `GET /healthz` | Liveness check |

## Layout

```
cmd/server/         assembler; wires router, store, backup ticker, countdown ticker, WS hub
internal/model/     plain structs, no logic
internal/store/     SQLite pools, migrations, caches
internal/web/       handlers, middleware, templ views, embedded static assets
internal/realtime/  countdown timing and the WebSocket hub
internal/upload/    filesystem chunk tracker, resumable upload handlers, session cleanup
internal/excel/     participant import/export
internal/backup/    periodic VACUUM INTO
docs/               changelog and the rebuild spec (source of truth)
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
