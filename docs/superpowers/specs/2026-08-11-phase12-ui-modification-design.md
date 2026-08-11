# Phase 12 - UI Modification Design

**Date:** 2026-08-11
**Branch:** `feat/ui` (off `develop`)
**Status:** design approved, pending spec review

## Goal

Restyle the current (weak) lks-judge templ UI to match the original Laravel UI at
`/Volumes/StorageTeamGroup/Projects/LKS Judge Platform/resources`, visually 1:1.
No behavior, route, handler, or template-signature changes beyond what is
technically required to render the old look under Go/templ.

## Guiding Principle

Follow the old design faithfully. "Adaptasi" is limited to crucial technical
plumbing, NOT cosmetic cleanup:

- Laravel-isms to Go/templ equivalents: `@csrf`, `route()` helpers, `@vite`,
  `@yield`/sections, `laravel-echo`/`pusher` realtime.
- Undefined design tokens that break rendering (define them so pages render).
- Keep real Go features that differ from old blade (pc_number+password auth,
  password/IP columns) but present them in the old design's styling.

Do NOT "rapikan" the old design's inconsistencies (guest pages leaking raw
gray/blue, etc.). Reproduce them. The old look is the target.

## Decisions (locked with user)

- **Fidelity:** follow old design; adapt only crucial technical items.
- **CSS pipeline:** standalone `tailwindcss` CLI wired into `go generate`
  (no `node_modules`, no `package.json`). `input.css` is the source of truth;
  `app.css` stays committed (regenerated), so `go build` alone still works.
- **Scope:** all pages (jury + public + participant), one task per page.
- **Fonts:** self-host Inter (body) + Manrope (headline) as woff2 + `@font-face`.
- **Jury shell:** adopt old fixed header (competition name + Reset button).
  Reset button renders now; `POST /jury/reset` remains a 501 stub until Phase 13.
- **Icons:** full Material Symbols adoption (sidebar nav + primary CTAs), font
  already bundled.

---

## Architecture

### CSS build pipeline

New source file `internal/web/static/css/input.css`:

```css
@import "tailwindcss";
@theme { /* MD3 --color-* tokens, --font-*  */ }
@utility text-headline-large { ... }   /* full Material type scale */
/* @font-face: Material Symbols, Inter, Manrope */
/* component classes: .sidebar-link, .ambient-shadow, .signature-gradient,
   .material-symbols-outlined */
```

`go generate` gains a second directive (new file
`internal/web/static/css/generate.go`):

```
templates/generate.go   -> //go:generate templ generate
static/css/generate.go  -> //go:generate tailwindcss -i input.css -o app.css --minify
```

Output `internal/web/static/css/app.css` stays committed and embedded via the
existing `//go:embed static`. The `tailwindcss` standalone binary acquisition
(committed vendored binary vs a fetch script) is decided in the plan.

### Fonts

Add `internal/web/static/fonts/inter.woff2` + `manrope.woff2`, declared via
`@font-face` in `input.css`. Drop the old design's `* { font-family: sans-serif }`
override so Inter/Manrope actually apply. Material Symbols `@font-face` already
present, carried into `input.css`.

### Token reconciliation (crucial adaptation)

The current templates reference MD3 tokens the committed `app.css` never defined.
`input.css` MUST define, so pages render:

- Full Material type scale utilities: `text-headline-large`,
  `text-headline-medium`, `text-title-large`, `text-title-medium`,
  `text-label-large`, `text-label-medium`, `text-label-small`,
  `text-body-large`, `text-body-medium`, `text-body-small`.
- `font-manrope`.
- `surface-container` (bare) between `-low` and `-high` (old design left it
  no-op; templ chips/cells rely on it). Suggested `#eceef0`.
- `background` as an alias of `surface` (fixes `bg-background` in layout body).
- `hover:bg-surface-container` (now valid once `surface-container` exists).

NO invented roles. There is NO `tertiary` in the old design: remove the
`tertiary`/`tertiary-container` usage from templates during the port and remap
each to what the old design actually used for that spot:

- saved/success banners -> `bg-secondary-container` (green)
- shuffle button -> `signature-gradient`
- set-current-module -> `bg-primary` / `bg-primary-container`

`error`/`error-container` DO exist in the old design and stay.

### Design tokens (`@theme`, old design verbatim)

```
--color-primary:                    #004ac6
--color-primary-container:          #2563eb
--color-secondary:                  #006e2d
--color-secondary-container:        #7cf994
--color-on-secondary-container:     #002109   (hardcoded in @utility)
--color-error:                      #ba1a1a
--color-surface:                    #f7f9fb
--color-surface-container-low:      #f2f4f6
--color-surface-container-lowest:   #ffffff
--color-surface-container-high:     #e6e8ea
--color-surface-container-highest:  #e0e3e5
--color-on-surface:                 #191c1e
--color-on-surface-variant:         #434655
--color-on-primary:                 #ffffff
--color-on-secondary:               #ffffff
--color-outline:                    #737686
--color-outline-variant:            #c3c6d7
/* added: surface-container #eceef0; background = surface; on-error #ffffff */
```

Component classes carried over verbatim:
- `.ambient-shadow` -> `box-shadow: 0 20px 40px -10px rgba(25,28,30,0.06)`
- `.signature-gradient` -> `linear-gradient(135deg, #004ac6 0%, #2563eb 100%)`
- `.sidebar-link` -> `@apply flex items-center gap-3 px-4 py-3 rounded-lg text-sm font-medium text-on-surface-variant hover:bg-surface-container-high transition`
- `.material-symbols-outlined` base + `font-variation-settings: 'FILL' 0, 'wght' 400`
- Drop dead `.gold-tier`/`.silver-tier`/`.bronze-tier` (leaderboard uses inline gradients).

---

## Layout shells

### AppLayout (jury) -> old `layouts/app.blade.php`

- `<body class="bg-surface text-on-surface">`.
- Fixed header `h-16`: `fixed top-0 w-full z-50 flex justify-between items-center px-6 h-16 bg-white/80 backdrop-blur-md shadow-sm`.
  - Left: competition name `text-xl font-bold font-headline`.
  - Right: Reset form `POST /jury/reset`, button `bg-error text-white px-4 py-2 rounded-xl text-sm font-bold`. (Handler is 501 stub until Phase 13; button renders now.)
- Sidebar `<aside class="fixed left-0 top-16 h-[calc(100vh-64px)] flex flex-col p-4 bg-surface-container-low w-64">`, `<nav class="flex-1 space-y-1">`, items via `.sidebar-link`, active adds `bg-primary/10 text-primary`.
- Nav items (icon -> label -> route), exact old order:

  | Material icon | Label | Route |
  |---|---|---|
  | `dashboard` | Competition | `/jury/` |
  | `alarm` | Countdown | `/jury/countdown` |
  | `groups` | Participants | `/jury/participants` |
  | `upload_file` | Submissions | `/jury/submissions` |
  | `inventory_2` | Modules | `/jury/modules` |
  | `edit_document` | Scoring | `/jury/scoring` |
  | `folder_open` | Files | `/jury/files` |

- Main: `<main class="ml-64 mt-16 p-10 min-h-screen">{ children }</main>`.
- Signature `AppLayout(title, activePage string)` unchanged; `activePage` drives highlight. Competition name read from `CompetitionCache` (thread as needed).

### GuestLayout (public/auth) -> old `layouts/guest.blade.php`

- `<body class="bg-gray-50">`.
- Top nav `h-16`, same bar style as header. Left slot default "Lomba Kompetensi Siswa", right slot (participant name on dashboard).
- Main `pt-20 min-h-screen`.
- templ has no Blade `@yield` -> add params: `GuestLayout(title, navLeft, navRight string)` (empty navLeft falls back to default). Dashboard passes participant name to navRight.

Standalone pages outside layouts (as old design): `countdown_public`, PDF export.

---

## Per-page port (one task each)

Route/handler/params unchanged; markup + classes only. Adapt Laravel-isms.

### Jury (AppLayout)

1. **competition.templ** -> old `competition/index`: `max-w-6xl mx-auto px-12`,
   2-col grids (name/level, start/end date), allowed_ips textarea, uppercase
   tracking labels, inputs `bg-surface-container-high rounded-lg focus:ring-1
   focus:ring-primary`, `signature-gradient` submit.
2. **countdown_jury.templ** -> old `countdown/index`: `max-w-7xl`, timer card
   `text-[10rem] tabular-nums` + decorative blur blobs, 3-col config grid,
   conditional RESUME/SAVE (`signature-gradient`) / PAUSE (`bg-amber-500`) /
   STOP (`bg-red-500`). Keep existing `countdown.js`.
3. **participants.templ** -> old `participants/index`: header + Export
   (`bg-green-600`) + Shuffle (`signature-gradient`); 12-col grid: left col-4
   Registration + Import cards (dashed dropzone, `description`/`warning`/`sync`
   icons, `confirm()`), right col-8 Queue (`divide-y`) + Seat Assignments table.
   Keep current data columns (PC/password/IP are real Go features) in old
   card+table styling.
4. **modules.templ** -> old `modules/index`: `max-w-6xl px-12`, current-module
   `<select>` card + `signature-gradient` Save, module cards grid
   `lg:grid-cols-3` with left accent bar, `M{n}` badge, inline rename, dashed
   add-card.
5. **submissions.templ** -> old `submissions/index`: `max-w-7xl`, 3 stat cards
   (`groups`/`task_alt`/`view_module` in colored circles), participants x modules
   matrix, seat chip, green Unduh link / "Belum" pill.
6. **scoring.templ** -> old `leaderboard/index`: `max-w-6xl`, card `rounded-2xl
   ambient-shadow`, table seat chip + name/school + per-module `number` inputs
   (`w-16 h-10 bg-surface-container text-center font-bold`) + live-sum Total
   `text-2xl text-primary`, footer Export + `signature-gradient` Simpan (`save`
   icon). Keep existing POST form + client-side sum JS.
7. **files.templ** -> old `files/index`: dashed dropzone, table Public toggle /
   Download / Delete. Keep `uploader.js`.
8. **shuffle.templ** -> adopt old `participants/shuffle` visual card + seat-grid
   styling but stays jury-side (Go route `/jury/participants/shuffle`). Wheel
   spin animation optional; flag in plan.

### Guest / participant

9. **login.templ** -> old `auth/register` card style (logo, headline,
   error/success alert boxes, `signature-gradient` submit). Keep Go
   pc_number+password fields, old card styling.
10. **participant_dashboard.templ** -> old `participants/public`: 12-col, left
    col-7 Download Files (submission + official file cards), right col-5 sticky
    upload card with `#sensor-layer` lock overlay (`lock` icon, "opens at last
    20 minutes"), styled `file:` input, `signature-gradient`. Realtime via
    existing WS (`dashboard.js`), not Echo.
11. **countdown_public.templ** -> old `countdown/public`: standalone, logo
    `w-60`, comp name `font-extrabold text-4xl`, three `text-9xl` digit blocks
    (minutes `text-primary`), blink at zero + alert.mp3. Keep `countdown.js`.
12. **leaderboard.templ** -> old `public-leaderboard`: card `rounded-3xl
    shadow-xl`, ranked table with position tints (`bg-amber-500/10` etc.),
    gradient medal circles, emoji medals / Medallion badge, per-module chips,
    WSI Total, footer legend. Rows client-rendered -> **`leaderboard.js` class
    strings rewritten** to old markup.

### PDF

13. `leaderboard/pdf` already matches old WorldSkills print doc (6 cols).
    No change unless the plan finds drift.

### JS class updates

`leaderboard.js` and `dashboard.js` inject markup with token classes. Rewrite
the string literals to the old design's classes, not just the `.templ` files.

---

## Testing & Verification

- **Build gate every task:** `go generate ./...` (templ + tailwindcss) then
  `go build ./...` green. Catches unknown `@apply`/token references and templ
  syntax errors.
- **Token-coverage check (one runnable check for the CSS work):** a script or
  `go test` that greps every class used in `.templ` files + JS string literals
  against generated `app.css`; asserts zero unresolved MD3 tokens.
- **Regression guard:** handlers/params/routes untouched, so existing
  `go test ./...` stays green.
- **Manual smoke:** run server, load all 13 routes, eyeball against old blade.
  Checklist in plan, not automated.

## Out of scope / deferred

- **CSRF:** old Laravel `@csrf` was framework-provided; current templ has none.
  Adding Go CSRF is cross-cutting (not UI-visual) -> defer to Phase 13. Do NOT
  silently add. If added, it is the one logic change and needs a test.
- **`POST /jury/reset` wiring:** stays 501 stub (Phase 13). Button renders only.
- **Session expiry sweep, cross-compile, server.bat, smoke test:** Phase 13.

## Deliverables summary

- `input.css` source + `tailwindcss` CLI in `go generate`; `app.css` regenerated.
- Inter + Manrope self-hosted; Material Symbols wired.
- Full MD3 token set + type scale (old design verbatim + `surface-container` +
  `background` alias; no `tertiary`).
- Both shells rebuilt (header + Reset stub, sidebar with icons).
- 13 pages ported 1:1 to old blade look, one task each.
- `leaderboard.js` + `dashboard.js` class strings rewritten.
- Verify: build gate + token-coverage check + smoke checklist.
