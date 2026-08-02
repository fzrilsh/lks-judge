# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

Single-binary Go server for LKS (Indonesian vocational skills competition) judging. Replaces Laravel+FrankenPHP+Reverb. Target: Windows LAN, ~16 simultaneous participants, zero runtime dependencies.

## Build

```bash
# Dev (host OS)
go generate ./...   # compile templ templates first
go build ./cmd/server

# Production (Windows binary)
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o lks-judge.exe ./cmd/server
```

templ templates must be compiled before `go build`. Run `go generate ./...` or `templ generate` after any `.templ` file change.

## Run

```bash
./server --data ./data --listen 0.0.0.0:8080
```

`--data` defaults to `./data`. SQLite file lives at `{data}/lks.sqlite`.

## Test

```bash
go test ./...
go test ./internal/scoring/...   # single package
```

## Key Architecture Decisions

**SQLite dual-pool:** writer (`SetMaxOpenConns(1)`) + read pool (`?mode=ro`, `SetMaxOpenConns(16)`). Never use the writer for SELECT. See `internal/store/db.go`.

**`model` package has zero logic and zero internal imports.** This is the cycle-breaker. All other packages import `model`; `model` imports nothing internal.

**Competition state cache:** `atomic.Pointer[model.Competition]` updated on every write. Hot-path handlers (countdown, jury IP check, form-open check) read from cache — no DB hit.

**Session cache:** `sync.Map[token → *model.Participant]` in `internal/store/session.go`. DB only on cold miss or expiry sweep.

**WS hub:** single goroutine owns `clients` map — no mutex needed. Anonymous connections (no cookie) receive only `CountdownTick` and `ScoreUpdated`.

**Chunked upload:** zero DB writes per chunk. Chunk presence = filesystem file `data/uploads_tmp/{upload_id}/chunk-{n}`. Final assembly via sequential `io.Copy` → `os.Rename` → single DB INSERT.

**Leaderboard cache:** `atomic.Pointer[[]byte]` pre-rendered JSON in `internal/scoring/cache.go`. Refreshed after every score write. 16 concurrent polls → 0 DB reads.

## Package Dependency Graph

```
model    ← nothing internal
store    ← model
scoring  ← model, store
upload   ← model, store
realtime ← model only (time.Time passed as param; no store dep)
excel    ← model, store
backup   ← *sql.DB only
cmd/server/main.go ← everything (assembler leaf)
```

Cycles are impossible by design. `realtime` must never import `store`.

## Auth

- **Jury:** stateless IP-allowlist check on every `/jury/*` request. No session, no DB lookup. IPs from `competitions.allowed_ips` JSON column, read via competition state cache.
- **Participant:** `pc_number` + `password` login → session cookie `participant_session`. Passwords are bcrypt(cost=8). Import generates 6-digit random numeric passwords.

## Scoring Formula

```go
// scaled = 700 + (raw - median) * 2.8, clamped [0, 1000]
```

`wsi_score` written to DB on every score upsert (pre-computed, not derived at query time).

Awards: rank 1/2/3 → Gold/Silver/Bronze; rank >3 AND wsi_score ≥ 700 → Medallion for Excellence.

## SQLite Pragmas (applied every connection open)

WAL, busy_timeout=5000, foreign_keys=ON, synchronous=NORMAL, cache_size=-32000, temp_store=MEMORY, mmap_size=268435456.

## bcrypt Cost

Cost=8 (not default 10). ~8× faster for bulk import. Acceptable for internal LAN competition.

## Gzip Scope

Only `/leaderboard` and `/jury/scoring` — large repeated payloads. Not global middleware.

## Branching Strategy

`action/short-description`, e.g. `feat/chunked-upload`, `fix/countdown-drift`, `develop`, `staging`. No direct commits to `main`.

## Git Workflow

Conventional commits: `feat:`, `fix:`, `chore:`, `refactor:`, `docs:`, `test:`. Flow: feature branch → PR into `develop` → `staging` → `main`. Push feature branches only, never force-push shared branches.

## Pre-commit / Pre-push Hooks

Managed via `lefthook` (already installed). `lefthook.yml` should run:
- pre-commit: `gofmt`, `go vet`, `golangci-lint run`
- pre-push: `go build ./...`, `go test ./...`

Set up `lefthook.yml` + `lefthook install` before first commit if not already present.

## Ambiguity

If a requirement, scope, or design choice is ambiguous, use `AskUserQuestion` before implementing — do not guess.

## Post-Phase Review

After finishing any task or implementation phase, review the result against `docs/rebuild-spec.md` (DoD/spec) before considering it done. Confirm it's correct and complete, not just "runs".

When a phase is complete, update `docs/CHANGELOG.md`:
- Add new phase section with completion date and status
- List all implemented features, files created, routes added
- Note spec compliance and any deviations
- Update "Next" section for the upcoming phase

## Spec Authority

`docs/rebuild-spec.md` is the source of truth for behavior, schema, and routes. Any deviation or ambiguity → ask via `AskUserQuestion` (see Ambiguity above), don't improvise silently.
