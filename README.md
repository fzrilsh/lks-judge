# lks-judge

Single-binary judging server for LKS (Indonesian vocational skills competition).
Runs on a Windows LAN for ~16 simultaneous participants with **zero runtime
dependencies**: no PHP, no Node, no system SQLite, no CGO. Replaces a previous
Laravel + FrankenPHP + Reverb stack that buckled under concurrent load.

## Status

All thirteen phases are done: setup, participants,
modules, the competition countdown, the WebSocket hub that pushes live events,
chunked resumable file upload with jury file management, participant submissions
with the jury review matrix, scoring with the robust WSI scale, a cached
public leaderboard, a CIS PDF export, and the final polish (nuclear reset,
Windows binary + `server.bat`). A subsequent UI redesign pass gave the whole app
a softer palette, a responsive CSS-only sidebar drawer, a jury dashboard at
`/jury/` (with vendored Chart.js insight charts), and the accessibility and
usability fixes from a heuristic audit. The server is ready for a LAN competition.

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
| 11 | Scoring + leaderboard: robust WSI, cached leaderboard, CIS PDF | done |
| 12 | UI modification: port the old Laravel design to the templ views | done |
| 13 | Polish & build: nuclear reset, session expiry sweep, Windows binary | done |
| UI | UI redesign: soft palette, responsive drawer, jury dashboard at `/jury/`, heuristic fixes | done |

Per-phase detail lives in [`docs/CHANGELOG.md`](docs/CHANGELOG.md).

## Requirements

- Go 1.25+
- [`templ`](https://templ.guide), install with `go install github.com/a-h/templ/cmd/templ@latest`

Contributors also need [`golangci-lint`](https://golangci-lint.run) and
[`lefthook`](https://lefthook.dev) for the commit gates.

SQLite is compiled in via `modernc.org/sqlite` (pure Go), so there is nothing to
install and `CGO_ENABLED=0` works everywhere. The CIS PDF export uses
`github.com/go-pdf/fpdf` (also pure Go), so it ships in the binary too.

## Build

```bash
go generate ./...          # compiles .templ files AND rebuilds static/css/app.css, required before every build
go build ./cmd/server
```

Any change to a `.templ` file needs `go generate ./...` (or `templ generate`)
before `go build`, or you will build against stale views.

### CSS

Styling is Tailwind v4. The source is `internal/web/tailwind.css` (MD3 tokens,
type scale, self-hosted `@font-face`, component classes); `go generate` runs the
standalone `tools/tailwindcss` CLI to regenerate the committed, embedded
`internal/web/static/css/app.css`. The CLI binary is git-ignored and per-OS
(see [`tools/README.md`](tools/README.md) for the download URL). Because
`app.css` is committed and embedded, `go build` alone works without the CLI;
you only need it to regenerate `app.css` after changing `tailwind.css` or the
class strings in `.templ` / `static/js/*.js` files. Self-hosted fonts live under
`internal/web/static/fonts/`. Vendored front-end libraries (currently Chart.js
for the jury dashboard) live under `internal/web/static/js/vendor/`, which
Tailwind does not scan, and ship embedded so the app works offline on the LAN.

Windows target:

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o lks-judge.exe ./cmd/server
```

Ship `lks-judge.exe` and `server.bat` together on the competition host.
Double-clicking `server.bat` launches the server on `0.0.0.0:80` with `./data`
and pauses on exit so a crash message stays readable.

## Run

```bash
./server --data ./data --listen 0.0.0.0:8080
```

| Flag | Default | Meaning |
| ---- | ------- | ------- |
| `--data` | `./data` | Data directory; the database lives at `{data}/lks.sqlite` |
| `--listen` | `0.0.0.0:8080` | HTTP listen address |
| `--dev` | `false` | Seed a default competition and one participant |
| `--jury-ip` | (none) | Extra jury IP or CIDR granted `/jury/*` access, in memory only. Repeatable or comma-separated. |

`--jury-ip` is a runtime-only allowlist: it is never written to the database,
so it is not shown in the competition setup form and it survives a Reset. Use it
to keep a fixed operator machine reachable before any competition exists or after
a wipe, e.g. `--jury-ip 192.168.1.10 --jury-ip 10.0.0.0/24`. It is additive to the
persisted `allowed_ips`. Loopback (`127.0.0.1`, `::1`) is always allowed on top of
both lists, so the machine running the server can always reach `/jury/*`.

`--dev` seeds a participant whose password is `123456`. **Never run a real
competition with `--dev`.**

There are no environment variables, all configuration is flags.

Under `--data` the server keeps `backups/`, `files/` (jury files), `submissions/`
(participant work, laid out `submissions/{participant_id}/{module_id}/`),
`uploads_tmp/` (in-flight chunks), and `logs/` (per-day `YYYY-MM-DD.log`, also
teed to the terminal) alongside `lks.sqlite`.

Six routes are public, everything else is behind the jury IP allowlist or a
participant session (plus `GET /login` and the `GET /static/` asset tree):

| Route | Purpose |
| ----- | ------- |
| `GET /countdown` | Full-screen countdown for a projector; plays an alert at zero |
| `GET /countdown/time` | `{"seconds":N,"status":"..."}`, polled once a second |
| `GET /leaderboard` | Public leaderboard; refreshes live on the `ScoreUpdated` WS event |
| `GET /leaderboard.json` | Cached leaderboard snapshot (WSI, ranks, awards) |
| `GET /ws` | WebSocket; anonymous clients get a reduced event set (countdown + score only) |
| `GET /healthz` | Liveness check |

The jury scoring surface adds `GET/POST /jury/scoring` (raw-score matrix and
bulk upsert) and `GET /jury/scoring/export-pdf` (the CIS PDF). `/leaderboard`,
`/leaderboard.json`, and `GET /jury/scoring` are gzip-scoped.

`GET /jury/` is the jury dashboard (competition status, countdown controls,
progress, top 3, activity feed, submission-timing charts); the competition setup
form lives at `GET/POST /jury/competition`.

The header **Reset** button (`POST /jury/reset`) is a nuclear wipe: it snapshots
the DB to `backups/` first, then deletes every competition, participant, module,
score, submission, session, and the `files/`, `submissions/`, `uploads_tmp/`
directories. A confirm dialog plus typing `RESET` guards it. Note that a reset
clears `allowed_ips`, so the jury allowlist falls back to loopback only: after a
reset you must recreate the competition from `127.0.0.1` before remote jury
machines can reach `/jury/*` again.

## Layout

```
cmd/server/         assembler; wires router, store, backup ticker, countdown ticker, WS hub
internal/model/     plain structs, no logic
internal/store/     SQLite pools, migrations, caches
internal/web/       handlers, middleware, templ views, embedded static assets
internal/realtime/  countdown timing and the WebSocket hub
internal/upload/    filesystem chunk tracker, resumable upload handlers, session cleanup
internal/scoring/   robust WSI scale, leaderboard cache, CIS PDF export
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
