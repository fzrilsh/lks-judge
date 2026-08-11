# Phase 11 - Scoring, Leaderboard, PDF (Design)

Date: 2026-08-06
Status: Approved (pending spec-file review)

## Goal

Add the scoring subsystem: jury enters raw scores per participant per module,
the public leaderboard ranks participants by a computed WSI scaled score, and a
CIS-style PDF exports the scaled results with award labels.

## Key decisions (deviate from the original spec)

The user changed the scoring model during brainstorming. These deviations are
authorized and MUST be mirrored into `docs/rebuild-spec.md` and `CLAUDE.md` so
all docs stay consistent.

1. **Median population = per-participant total raw points.**
   For each participant, `total_raw = SUM(score over all their modules)`.
   The median is the median of those per-participant totals, NOT a per-module
   median. A participant with every module blank/0 still has `total_raw = 0` and
   is included in the population.

2. **Participant WSI = `ScaleScore(total_raw, median, mad)`** where `median` and
   `mad` come from the population of per-participant totals.
   One number per participant. Ranking is WSI descending.

3. **WSI is never persisted. It is computed on demand** (leaderboard cache
   refresh and PDF export). No DB column stores it.

4. **`scores.wsi_score` column is removed** (nothing reads or writes it).

5. **`scores.score` becomes `REAL`** (raw scores are decimal, 0..100).

6. **`/jury/leaderboard` (jury view) is dropped.** Redundant with the public
   `/leaderboard`; jury can open it directly. Deviation from spec §6 route map.

## Formula (robust z-score, reverse-engineered from CIS output)

The original spec §7 said `700 + (raw - median) * 2.8`. That is wrong: it has a
hardcoded slope and never fits real CIS scaled results. The actual CIS "scale
results" are a **robust standardised score** (median + MAD), reverse-engineered
to an exact fit against a 32-competitor ground-truth export:

```go
// unit = 1.4826 * MAD  (normal-consistent robust SD estimate)
// scaled = 700 + 30 * (raw - median) / unit, clamped [0, 1000], rounded
func ScaleScore(raw, median, mad float64) int {
    unit := 1.4826 * mad
    if unit == 0 {
        unit = 1e-9 // degenerate all-equal population; every raw maps to 700
    }
    s := 700 + 30*(raw-median)/unit
    if s < 0 { s = 0 }
    if s > 1000 { s = 1000 }
    return int(math.Round(s))
}
```

- `median`: median of the per-participant totals (population, computed on demand).
- `MAD`: median absolute deviation, `median(|total - median|)`.
- `1.4826`: standard MAD→SD scale factor (consistency with the normal SD).
- `30`: points per robust-SD unit; anchors 700 at the median.

Both `median` and `MAD` are recomputed from the population on every render, so
the scale is fully dynamic (participant count and spread vary per competition).
The effective slope for the ground-truth data is `30/(1.4826*7.0) ≈ 2.891`, which
is why an early hardcoded-2.889 fit worked; it is not a constant.

Median/MAD: sort ascending. Odd N → middle element. Even N → mean of the two
middle elements. Same rule for both the totals and the abs-deviation list.

Awards (by rank, after WSI-desc sort):
- rank 1 → Gold
- rank 2 → Silver
- rank 3 → Bronze
- rank > 3 AND wsi >= 700 → Medallion for Excellence
- otherwise → none

Tie handling: stable sort. Equal WSI keeps input order (participants supplied in
`pc_number` order). Award follows rank position.

### Test vector (32 participants, locked into `formula_test.go`)

Per-participant totals below → median = **8.0**, MAD = **7.0**, unit = 10.3782.

```
61.42 54.33 39.58 37.75 35.50 28.75 28.42 28.40 19.00 16.92 13.50 13.05
12.80 12.25 9.50 8.75 7.25 6.00 6.00 4.25 3.25 2.75 2.00 1.50 1.40 1.25
0.75 0.50 0.50 0.50 0.25 0.00
```

Expected (rank, raw, wsi, award) - rows the test asserts exactly (verified
against the real CIS export, all 32 match):
```
1   61.42  854  Gold
2   54.33  834  Silver
3   39.58  791  Bronze
4   37.75  786  Medallion
5   35.50  779  Medallion
16   8.75  702  Medallion   <- last Medallion (wsi >= 700), 13 Medallions total
17   7.25  698  none        <- first below threshold
32   0.00  677  none
```

## Architecture

New package `internal/scoring` (imports `model`, `store` only - matches the §11
dependency graph; cycles remain impossible).

### `internal/scoring/formula.go`
- `type Entry struct { ParticipantID int64; Name, School string; PCNumber *int; TotalRaw float64; WSI int; Rank int; Award string }`
- `ScaleScore(raw, median, mad float64) int`
- `Median(xs []float64) float64` - shared by both the totals median and the MAD
  inner/outer median (sorts a copy; even N → mean of two middles).
- `MAD(totals []float64, median float64) float64` - `Median(|x - median|)`.
- `Rank(entries []Entry) []Entry` - pure: computes median + MAD from `TotalRaw`,
  fills `WSI` via `ScaleScore`, sorts desc (stable), assigns `Rank` and `Award`.
  No DB.

### `internal/scoring/cache.go`
- `atomic.Pointer[[]byte]` holding pre-rendered leaderboard JSON.
- `Refresh(st *store.Store, competitionID int64) error` - one query
  (`ListParticipantTotals`) → `Rank` → `json.Marshal` → store into the pointer.
  Called at startup and after every score write.
- `Snapshot() []byte` - returns the current bytes; 16 concurrent polls → 0 DB reads.

### `internal/scoring/pdf.go`
- `PDF(comp *model.Competition, entries []scoring.Entry) ([]byte, error)` using
  `github.com/go-pdf/fpdf` (pure Go, the only new dependency; spec tech stack).
- Header: left logo (`imgs/logo.png`), center title
  ("Web Technologies / WorldSkills Scale Results / {competition.name}"),
  right logo (`imgs/logo-worldskills.jpg`).
- Table: Name | Member | Result (WSI) | Award. PC numbers zero-padded.

### `internal/store/submission.go` (extend)
- `ListParticipantTotals(competitionID int64) ([]ParticipantTotal, error)`:
  `SELECT p.id, p.name, p.school, p.pc_number, COALESCE(SUM(s.score),0)
   FROM participants p LEFT JOIN scores s ON s.participant_id = p.id
   WHERE p.competition_id = ? GROUP BY p.id ORDER BY p.pc_number`.
  LEFT JOIN so a participant with no scores still appears with total 0. NULL
  module scores are ignored by SUM but the participant row remains.
- `UpsertScore` signature `*int` → `*float64` (decimal raw scores).

### `internal/excel/excel.go`
- Score parse `strconv.Atoi` → `strconv.ParseFloat`; `scores` map `*int` → `*float64`.
- Export score cells: fill from a totals/score join (removes the existing
  `ponytail:` stub) - render raw decimal per module.

### `internal/model/types.go`
- `Score.Score` `*int` → `*float64`; drop `Score.WSIScore`.

### `internal/store/migrations/001_initial.sql` (edit in place)
DB is disposable (dev, no score data to preserve), so no rebuild migration.
Edit the initial schema directly: `scores.score` `INTEGER` → `REAL`, drop the
`scores.wsi_score` column. Old dev DB files are deleted and recreated from the
edited schema. No `003`, no `scores_new`, one `scores` table only.

## Routes

```
GET   /jury/scoring              matrix (participant x module) raw-score inputs
POST  /jury/scoring              bulk upsert → Refresh cache → broadcast ScoreUpdated
GET   /jury/scoring/export-pdf   fpdf: scaled WSI + awards
GET   /leaderboard               public, no auth; renders WSI ranking + award badges
```
(POST not PUT - HTML forms, precedent Phases 5/6/9/10. `/jury/leaderboard` dropped.)

## Handlers (`internal/web/handlers_scoring.go`)

- `HandleScoringGET(st)` - render matrix; each cell `<input type=number step=0.1 min=0 max=100>`.
- `HandleScoringPOST(st, hub)` - parse form; validate each value `0 <= x <= 100`
  (reject out-of-range → redirect `?error=` escaped); blank → leave NULL / skip;
  `UpsertScore` each; then `scoring.Refresh(st, compID)` and `hub` broadcast
  `EvScoreUpdated{}`. Log the write with acting IP (consistent with Logging Pass).
- `HandleScoringExportPDF(st)` - load totals → `Rank` → `scoring.PDF` → stream
  `application/pdf` with `Content-Disposition`.
- `HandleLeaderboardGET(cache)` - render from `cache.Snapshot()` (public page).

## Real-time

- `internal/web/static/js/leaderboard.js` (NEW): anonymous WS client; on
  `ScoreUpdated` refetch `/leaderboard` data and re-render (spec §10: WS replaces
  polling). `EvScoreUpdated` already exists (Phase 8) and anonymous clients are
  already allowed to receive it; this adds the broadcast call site + the client.

## Gzip (spec §Gzip Scope)

`compress/gzip` (stdlib) middleware scoped to `/leaderboard` and `/jury/scoring`
only, gated on `Accept-Encoding: gzip`. Not global (small responses pay nothing).

## Security awareness

- Raw-score input validated at the trust boundary (jury POST): numeric, `0..100`,
  out-of-range rejected before write.
- Leaderboard/PDF: participant names escaped (templ auto-escapes; fpdf writes text,
  not markup).
- Gzip reflects no raw user input; no compression-oracle concern (no secrets in body).
- All score writes logged with acting IP.
- `/jury/scoring*` behind the existing jury IP allowlist; `/leaderboard` is public
  read-only by design.

## Testing

`internal/scoring/formula_test.go`:
- `ScaleScore`: median case (→700), clamp low (0), clamp high (1000), rounding,
  degenerate MAD=0 (→700).
- `Median`: odd N, even N (mean of two middles), single element.
- `MAD`: known median/MAD pair from the fixture (median 8.0, MAD 7.0).
- `Rank`: the 32-row fixture above - asserts median 8.0, MAD 7.0, all 32
  ranks/wsi/awards exact (854/834/791/786/779 ... 677); Medallion boundary at
  wsi 700 (13 Medallions, last at rank 16); stable tie order for the two 6.00
  and three 0.50 rows.

`internal/store` tests:
- `ListParticipantTotals`: SUM correct; participant with no scores → total 0 and
  present; decimal SUM precision.
- `UpsertScore` decimal roundtrip (`85.5`).
- Schema: fresh DB has `scores.score` REAL and no `wsi_score` column.

`internal/scoring/cache_test.go`:
- `Refresh` changes `Snapshot()` bytes after a score write; `Snapshot()` does no
  DB read.

## Verification (DoD)

- `go generate ./...`, `go build ./...`, `go vet ./...`, `go test ./... -race`,
  golangci-lint: clean.
- Enter scores → `/leaderboard` updates via `ScoreUpdated` WS → PDF scaled scores
  and award labels match the fixture → 16 concurrent leaderboard polls do 0 DB reads.

## Doc updates (required by CLAUDE.md post-phase review)

- `docs/rebuild-spec.md`: schema §3 drop `scores.wsi_score`, `scores.score` → REAL;
  §7 WSI section rewritten (robust median+MAD z-score, NOT the old `*2.8` slope;
  median = per-participant totals, WSI computed on demand, not persisted);
  §Leaderboard Result Cache (no wsi_score in DB); route map drop
  `/jury/leaderboard`; §16 Phase 11 steps aligned (no formula.go persist, no
  cache.go writing wsi_score).
- `CLAUDE.md`: "Scoring Formula" and "Leaderboard cache" sections updated to the
  robust median+MAD formula and compute-on-demand model; note `scores.score` is
  REAL and `wsi_score` removed.
- `docs/CHANGELOG.md`: new Phase 11 section (date, files, routes, deviations,
  verification); replace the "Next" section with Phase 12.
- `README.md`: flip Phase 11 row to done; add `/leaderboard`, `/jury/scoring*`
  routes and the `go-pdf/fpdf` dependency.
- `internal/model/types.go` comments referencing `wsi_score` cleaned up.
- `internal/store/submission.go`: `UpsertScore` INSERT drops the `wsi_score`
  column (and its NULL comment); `score` param `*int` → `*float64`.
- `internal/store/migrations/001_initial.sql`: `scores.score` REAL, no
  `wsi_score` (edited in place; dev DB `data/lks.sqlite*` deleted so it rebuilds).
