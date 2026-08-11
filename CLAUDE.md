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
go test ./internal/store/...   # single package
```

## Key Architecture Decisions

**SQLite dual-pool:** writer (`SetMaxOpenConns(1)`) + read pool (`?mode=ro`, `SetMaxOpenConns(16)`). Never use the writer for SELECT. See `internal/store/db.go`.

**`model` package has zero logic and zero internal imports.** This is the cycle-breaker. All other packages import `model`; `model` imports nothing internal.

**Competition state cache:** `atomic.Pointer[model.Competition]` updated on every write. Hot-path handlers (countdown, jury IP check, form-open check) read from cache, no DB hit.

**Session cache:** `sync.Map[token → *model.Participant]` in `internal/store/session.go`. DB only on cold miss. (Periodic expiry sweep is planned for Phase 13; not yet implemented.)

**WS hub:** single goroutine owns `clients` map, so no mutex needed. Anonymous connections (no cookie) receive only `CountdownTick` and `ScoreUpdated`.

**Chunked upload:** zero DB writes per chunk. Chunk presence = filesystem file `data/uploads_tmp/{upload_id}/chunk-{n}`. Final assembly via sequential `io.Copy` → `os.Rename` → single DB INSERT.

**Leaderboard cache:** `atomic.Pointer[[]byte]` pre-rendered JSON in `internal/scoring/cache.go`, primed at startup in `main.go` and refreshed after every score write, then broadcast via WS `ScoreUpdated`.

## Package Dependency Graph

```
model    ← nothing internal
store    ← model
upload   ← model, store
scoring  ← model, store
realtime ← model only (time.Time passed as param; no store dep)
excel    ← store only
backup   ← *sql.DB only
web      ← model, store, realtime, excel, upload
cmd/server/main.go ← model, store, realtime, upload, backup, web (assembler leaf)
```

Phase 11 added `scoring ← model, store`. Cycles are impossible by design. `realtime` must never import `store`.

## Auth

- **Jury:** stateless IP-allowlist check on every `/jury/*` request. No session, no DB lookup. IPs from `competitions.allowed_ips` JSON column, read via competition state cache.
- **Participant:** `pc_number` + `password` login → session cookie `participant_session`. Passwords are bcrypt(cost=8). Import generates 6-digit random numeric passwords.

## Scoring Formula

Robust standardised z-score, centre 700, spread from the median absolute deviation (MAD):

```go
// scaled = 700 + 30*(raw-median)/(1.4826*mad), clamped [0, 1000], math.Round
// mad==0 (no spread) → 700
```

Population = per-participant TOTAL raw points (`COALESCE(SUM(score),0)` over all modules, LEFT JOIN so blank participants count as total 0). Median and MAD are computed on demand from the live population on every leaderboard refresh. WSI is NEVER persisted: `scores.score` is `REAL` (raw points) and there is no `wsi_score` column. Lives in `internal/scoring/formula.go` (stdlib only, no internal imports).

Awards by WSI-descending rank: rank 1/2/3 → Gold/Silver/Bronze; rank >3 AND WSI >= 700 → Medallion for Excellence; otherwise none. (`AwardGold`/`AwardSilver`/`AwardBronze`/`AwardMedallion` consts.)

## SQLite Pragmas (applied every connection open)

WAL, busy_timeout=5000, foreign_keys=ON, synchronous=NORMAL, cache_size=-32000, temp_store=MEMORY, mmap_size=268435456.

## bcrypt Cost

Cost=8 (not default 10). ~8× faster for bulk import. Acceptable for internal LAN competition.

## Gzip Scope

Scoped middleware `web.Gzip`, applied to `GET /jury/scoring` and both `/leaderboard` routes (the large repeated payloads). Not global, not on export-pdf.

## Branching Strategy

`action/short-description`, e.g. `feat/chunked-upload`, `fix/countdown-drift`, `develop`, `staging`. No direct commits to `main`.

## Git Workflow

Flow: feature branch → PR into `develop` → `staging` → `main`. Push feature branches only, never force-push shared branches.

### Commit Message Format

```
<type>(<scope>): <description>

<body>

<footer>
```

- `<type>`: one of `feat` (new feature), `fix` (bug fix), `docs` (documentation), `chore` (maintenance or tooling), `refactor` (neither fixes a bug nor adds a feature), `test`.
- `<scope>`: the area touched, e.g. `server`, `store`, `web`, `repo`.
- Blank line before the body. Body is optional but expected for anything non-trivial: explain the full change and why, not a one-liner.
- Footer optional, for breaking changes or issue references.

### Attribution

Never add Claude attribution to commits, PRs, or issues. No `Co-Authored-By: Claude`, no "Generated with Claude Code", no emoji trailer, no `--author` override.

### Writing Style

No em dashes (`—`) anywhere: commit messages, code comments, docs, PR bodies. Use a comma, a colon, parentheses, or split the sentence. The one exception is the historical CHANGELOG phase-header format below (`## Phase N - Title`), which is a fixed template, not prose. Same for other AI-slop tells: en dashes used as punctuation, `–`, curly quotes, decorative emoji, and filler openers like "In today's fast-paced world" or "It's worth noting that". Plain ASCII punctuation, plain sentences.

### When to Commit and Push

- Commit after every completed task. Do not batch several tasks into one commit.
- Push only at the end of a phase, or when the job is otherwise finished. Intermediate task commits stay local.

## Pre-commit / Pre-push Hooks

Managed via `lefthook` (already installed). `lefthook.yml` should run:
- pre-commit: `gofmt`, `go vet`, `golangci-lint run`
- pre-push: `go build ./...`, `go test ./...`

Set up `lefthook.yml` + `lefthook install` before first commit if not already present.

## Ambiguity

If a requirement, scope, or design choice is ambiguous, use `AskUserQuestion` before implementing. Do not guess.

## Post-Phase Review

After finishing any task or implementation phase, review the result against `docs/rebuild-spec.md` (DoD/spec) before considering it done. Confirm it's correct and complete, not just "runs".

When a phase or final job is complete, update every documentation file before calling it done.

`docs/CHANGELOG.md`:
- Add new phase section with completion date and status
- List all implemented features, files created, routes added
- Note spec compliance and any deviations
- Replace the existing "Next" section with one for the upcoming phase. There is exactly one "Next" section in the file at any time, sitting directly under the newest phase. Never leave the old one behind.
- Every phase section carries a `## Phase N - Title (date) ✅` header. Sections run newest first. Non-phase maintenance passes (audits, cleanups) may use a descriptive `## Title (date)` header instead of a `Phase N` number.

`README.md`:
- Flip the phase table row from "not started" to "done"
- Add any new flag, command, or directory the phase introduced
- Revise the Status paragraph when the project stops being a scaffold

This list is exhaustive as of now. If a new doc file is added later, add it here too.

## Spec Authority

`docs/rebuild-spec.md` is the source of truth for behavior, schema, and routes. Any deviation or ambiguity → ask via `AskUserQuestion` (see Ambiguity above), don't improvise silently.
