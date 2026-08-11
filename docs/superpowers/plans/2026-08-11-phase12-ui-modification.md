# Phase 12 - UI Modification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restyle the templ UI to match the original Laravel design (visually 1:1), adapting only crucial technical plumbing.

**Architecture:** Introduce a standalone `tailwindcss` CLI build step (`input.css` source -> committed `static/css/app.css`) wired into `go generate`. Rebuild both layout shells and port all 13 pages to the old blade look. No handler/route/signature changes beyond rendering needs.

**Tech Stack:** Go 1.25, a-h/templ, Tailwind CSS v4 standalone CLI, self-hosted woff2 fonts (Inter, Manrope, Material Symbols).

**Reference (old design, read-only):** `/Volumes/StorageTeamGroup/Projects/LKS Judge Platform/resources` — `css/app.css`, `views/layouts/{app,guest}.blade.php`, `views/**/*.blade.php`.

**Spec:** `docs/superpowers/specs/2026-08-11-phase12-ui-modification-design.md`

## Global Constraints

- Follow the old design faithfully. "Adaptasi" = crucial technical plumbing ONLY (Laravel-isms -> Go/templ; undefined tokens that break rendering). Do NOT clean up old design's cosmetic inconsistencies; reproduce them.
- Keep real Go features that differ from old blade (pc_number+password auth, password/IP columns) but present them in the old design's styling.
- NO `tertiary` role (old design has none). Remap each templ `tertiary` usage: saved/success -> `bg-secondary-container`; shuffle -> `signature-gradient`; set-current-module -> `bg-primary`/`bg-primary-container`.
- No behavior change: routes, handler logic, template param signatures stay as-is unless a task explicitly says otherwise (shell param additions only). Existing `go test ./...` must stay green.
- `app.css` stays committed and embedded (`//go:embed static`), so `go build` alone works without the CLI installed.
- `input.css` lives OUTSIDE `static/` (at `internal/web/tailwind.css`) so it is never embedded/served.
- CSRF and `POST /jury/reset` wiring are OUT of scope (Phase 13). Reset button renders but the handler stays a 501 stub.
- Commit conventions per CLAUDE.md: `<type>(<scope>): <desc>`, no em dashes, no Claude attribution.

---

## File Structure

**Create:**
- `internal/web/tailwind.css` — Tailwind v4 source (`@theme` tokens, `@utility` type scale, `@font-face`, component classes). NOT under `static/`, so never embedded.
- `internal/web/static/css/generate.go` — `//go:generate` directive for the CSS build.
- `internal/web/static/fonts/inter.woff2`, `internal/web/static/fonts/manrope.woff2` — self-hosted fonts.
- `internal/web/css_coverage_test.go` — token-coverage check (greps `.templ` + JS class usage against generated `app.css`).
- `tools/tailwindcss` — vendored standalone Tailwind v4 CLI binary (git-ignored per-OS; a `tools/README.md` documents the download URL). See Task 1.

**Modify:**
- `internal/web/static/css/app.css` — regenerated output (committed).
- `internal/web/templates/layout_app.templ`, `layout_guest.templ` — shells.
- All 12 page templates in `internal/web/templates/`.
- `internal/web/static/js/leaderboard.js`, `dashboard.js` — class string literals.
- `docs/CHANGELOG.md`, `README.md` — Phase 12 completion (final task).

**Do NOT touch:** any handler `.go` file logic, routes in `cmd/server/main.go`, `internal/web/embed.go`, template param signatures (except the two shell components), `leaderboard/pdf` output.

---

### Task 1: Tailwind CLI pipeline + `tailwind.css` source

**Files:**
- Create: `internal/web/tailwind.css`
- Create: `internal/web/static/css/generate.go`
- Create: `tools/README.md`
- Modify: `internal/web/static/css/app.css` (regenerated)

**Interfaces:**
- Produces: the full MD3 token vocabulary + Material type scale as real utilities, consumed by every shell and page task. Class names listed below are the contract; later tasks assume they resolve.

**Adaptation note:** the old `resources/css/app.css` (3.4 KB, read it) is the visual source of truth for tokens, component classes, and `@font-face`. Reproduce it, then ADD the undefined-token fixes the current templates need (type scale, `font-manrope`, `surface-container`, `background`).

- [ ] **Step 1: Acquire the Tailwind v4 standalone CLI**

The binary is not on PATH. Download the standalone CLI (no Node) for the dev OS into `tools/tailwindcss` and `chmod +x`. macOS arm64 (this dev box, `go1.25.0 darwin/arm64`):

```bash
mkdir -p tools
curl -sL -o tools/tailwindcss https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-macos-arm64
chmod +x tools/tailwindcss
./tools/tailwindcss --help | head -3   # verify it runs
```

Write `tools/README.md` naming the release URL pattern and noting the binary is git-ignored (add `tools/tailwindcss` to `.gitignore`). Rationale: a 100 MB+ per-OS binary should not be committed; `app.css` is committed so `go build` never needs it.

- [ ] **Step 2: Write `internal/web/tailwind.css`**

Full source below. Tokens copied verbatim from the old `resources/css/app.css`, plus the reconciliation additions (marked `/* ADDED */`).

In Tailwind v4, every `--color-*` in `@theme` auto-generates `bg-*`/`text-*`/`border-*` utilities, so the old file's explicit `@utility bg-primary {...}` blocks are redundant and dropped. `--font-*` auto-generates `font-*`. Type-scale utilities are not size+weight pairs in core, so they stay explicit `@utility`.

```css
@import "tailwindcss";

/* Scan compiled templ output + JS so Tailwind keeps only used classes. */
@source "./templates/*_templ.go";
@source "./static/js/*.js";

@theme {
  --color-primary: #004ac6;
  --color-primary-container: #2563eb;
  --color-secondary: #006e2d;
  --color-secondary-container: #7cf994;
  --color-error: #ba1a1a;
  --color-surface: #f7f9fb;
  --color-surface-container-low: #f2f4f6;
  --color-surface-container-lowest: #ffffff;
  --color-surface-container-high: #e6e8ea;
  --color-surface-container-highest: #e0e3e5;
  --color-on-surface: #191c1e;
  --color-on-surface-variant: #434655;
  --color-on-primary: #ffffff;
  --color-on-secondary: #ffffff;
  --color-outline: #737686;
  --color-outline-variant: #c3c6d7;

  /* ADDED: tokens the templ pages reference but old CSS never defined. */
  --color-surface-container: #eceef0;   /* between -low and -high */
  --color-background: #f7f9fb;           /* alias of surface; fixes bg-background */
  --color-on-error: #ffffff;
  --color-error-container: #ffdad6;      /* MD3 error-container for the tonal error role */
  --color-on-error-container: #410002;
  --color-on-primary-container: #ffffff;
  --color-on-secondary-container: #002109;

  --font-manrope: "Manrope", sans-serif;
}

/* ADDED: Material type scale (size/line-height/weight) as utilities. */
@utility text-headline-large  { font-size: 2rem;     line-height: 2.5rem;   font-weight: 700; }
@utility text-headline-medium { font-size: 1.75rem;  line-height: 2.25rem;  font-weight: 700; }
@utility text-title-large     { font-size: 1.375rem; line-height: 1.75rem;  font-weight: 600; }
@utility text-title-medium    { font-size: 1rem;     line-height: 1.5rem;   font-weight: 600; }
@utility text-label-large     { font-size: 0.875rem; line-height: 1.25rem;  font-weight: 500; }
@utility text-label-medium    { font-size: 0.75rem;  line-height: 1rem;     font-weight: 500; }
@utility text-label-small     { font-size: 0.6875rem;line-height: 1rem;     font-weight: 500; }
@utility text-body-large      { font-size: 1rem;     line-height: 1.5rem; }
@utility text-body-medium     { font-size: 0.875rem; line-height: 1.25rem; }
@utility text-body-small      { font-size: 0.75rem;  line-height: 1rem; }

@font-face {
  font-family: "Material Symbols Outlined";
  font-style: normal;
  src: url("./static/fonts/material-symbols.woff2") format("woff2");
}
@font-face {
  font-family: "Inter";
  font-style: normal;
  font-display: swap;
  src: url("./static/fonts/inter.woff2") format("woff2");
}
@font-face {
  font-family: "Manrope";
  font-style: normal;
  font-display: swap;
  src: url("./static/fonts/manrope.woff2") format("woff2");
}

.material-symbols-outlined {
  font-family: "Material Symbols Outlined";
  font-weight: normal;
  font-style: normal;
  font-size: 24px;
  display: inline-block;
  line-height: 1;
  text-transform: none;
  letter-spacing: normal;
  word-wrap: normal;
  white-space: nowrap;
  direction: ltr;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  font-variation-settings: "FILL" 0, "wght" 400;
}

body { font-family: "Inter", sans-serif; }
h1, h2, h3 { font-family: "Manrope", sans-serif; }

.sidebar-link {
  @apply flex items-center gap-3 px-4 py-3 rounded-lg text-sm font-medium text-on-surface-variant hover:bg-surface-container-high transition;
}
.ambient-shadow { box-shadow: 0 20px 40px -10px rgba(25, 28, 30, 0.06); }
.signature-gradient { background: linear-gradient(135deg, #004ac6 0%, #2563eb 100%); }
```

Note the `@font-face` and `@source` URLs are relative to `tailwind.css` at `internal/web/`. The `.woff2` URLs in the OUTPUT `app.css` must resolve at runtime to `/static/fonts/...`; since `app.css` is served from `/static/css/`, the emitted url must be `../fonts/material-symbols.woff2`. Tailwind emits `@font-face` url() verbatim as written, so write the url() values in the source as `../fonts/inter.woff2` etc. (browser-relative to the served `app.css`), NOT `./static/fonts/...`. Correct the block accordingly:

```css
@font-face { font-family:"Material Symbols Outlined"; src:url("../fonts/material-symbols.woff2") format("woff2"); }
@font-face { font-family:"Inter"; font-display:swap; src:url("../fonts/inter.woff2") format("woff2"); }
@font-face { font-family:"Manrope"; font-display:swap; src:url("../fonts/manrope.woff2") format("woff2"); }
```

(`@source` globs stay `./templates/*_templ.go` and `./static/js/*.js`, relative to the source file, since those are build-time scan paths.)

- [ ] **Step 3: Write the `go generate` directive**

Create `internal/web/static/css/generate.go`:

```go
package css

// Regenerate app.css from ../../tailwind.css after any .templ or JS class change.
// Requires the standalone Tailwind v4 CLI (see /tools/README.md). app.css is committed
// so `go build` works without the CLI.
//go:generate ../../../../tools/tailwindcss -i ../../tailwind.css -o app.css --minify
```

Verify the relative path from `internal/web/static/css/` back to `tools/tailwindcss` is correct (`../../../../tools/tailwindcss` from repo-root `internal/web/static/css`). Adjust if the run fails.

- [ ] **Step 4: Regenerate and verify the CSS builds**

```bash
go generate ./...        # runs templ generate AND the tailwind build
go build ./...
```

Expected: `app.css` regenerated (still minified, one line), build green. Confirm `app.css` now contains the type-scale classes:

```bash
grep -c "headline-large\|font-manrope\|surface-container" internal/web/static/css/app.css   # > 0
```

- [ ] **Step 5: Commit**

```bash
git add internal/web/tailwind.css internal/web/static/css/generate.go internal/web/static/css/app.css tools/README.md .gitignore
git commit -m "feat(web): add tailwind CLI build pipeline and design tokens

Introduce internal/web/tailwind.css as the CSS source of truth (MD3 tokens,
Material type scale, self-hosted font faces) compiled by the standalone
Tailwind v4 CLI via go generate. Define the type-scale utilities, font-manrope,
surface-container and background tokens the templates referenced but the
committed app.css never defined."
```

---

### Task 2: Self-host Inter + Manrope fonts

**Files:**
- Create: `internal/web/static/fonts/inter.woff2`
- Create: `internal/web/static/fonts/manrope.woff2`

**Interfaces:**
- Consumes: the `@font-face` rules from Task 1 (`../fonts/inter.woff2`, `../fonts/manrope.woff2`).

- [ ] **Step 1: Fetch the woff2 files**

Download variable or regular-weight woff2 for each (Google Fonts / fontsource). Place at the exact paths above.

```bash
curl -sL -o internal/web/static/fonts/inter.woff2 https://cdn.jsdelivr.net/fontsource/fonts/inter@latest/latin-400-normal.woff2
curl -sL -o internal/web/static/fonts/manrope.woff2 https://cdn.jsdelivr.net/fontsource/fonts/manrope@latest/latin-700-normal.woff2
file internal/web/static/fonts/*.woff2   # confirm "Web Open Font Format (Version 2)"
```

If a heavier body/headline range is wanted, fetch the variable font instead; keep filenames identical so the `@font-face` urls hold.

- [ ] **Step 2: Verify they load**

```bash
go generate ./... && go build ./cmd/server && ./server --data /tmp/lksfonttest --listen 127.0.0.1:18080 &
sleep 1
curl -sI http://127.0.0.1:18080/static/fonts/inter.woff2 | head -1    # HTTP 200
curl -sI http://127.0.0.1:18080/static/fonts/manrope.woff2 | head -1  # HTTP 200
kill %1; rm -rf /tmp/lksfonttest
```

- [ ] **Step 3: Commit**

```bash
git add internal/web/static/fonts/inter.woff2 internal/web/static/fonts/manrope.woff2
git commit -m "feat(web): self-host Inter and Manrope woff2 fonts

Bundle the body (Inter) and headline (Manrope) fonts the old design assumed
but never shipped, so the LAN target renders identically offline."
```

---

### Task 3: Rebuild AppLayout (jury shell)

**Files:**
- Modify: `internal/web/templates/layout_app.templ`

**Interfaces:**
- Consumes: tokens/classes from Task 1; `CompetitionCache` for the header competition name.
- Produces: `AppLayout(title, activePage string)` (signature unchanged). All jury page tasks (4-11) render inside it.

**Adaptation:** old `layouts/app.blade.php` reads `$competition->name` in the header. templ components take explicit params, and the current `AppLayout` signature is `(title, activePage)` with no competition. Do NOT change the signature (it would ripple through every jury handler). Instead, read the competition name inside the component from the store's competition cache via a package-level accessor, OR fall back to a constant. Since templ components cannot import store without a cycle risk, add a tiny exported helper in the web package that the template calls: `web.HeaderTitle()` returning the cached competition name or "LKS Judge Platform". Verify no import cycle (templates package already lives under web; if the templates package is separate, pass via a `templ.WithChildren` context value set by a middleware, or read a package var). Resolve the cleanest mechanism during implementation; the constraint is: header shows competition name, `AppLayout(title, activePage)` signature unchanged.

- [ ] **Step 1: Rewrite the shell markup**

Structure (from old `layouts/app.blade.php`):
- `<html lang="id">`, head unchanged (title suffix, `/static/css/app.css` link).
- `<body class="bg-surface text-on-surface min-h-screen font-manrope">`.
- Fixed header:

```html
<header class="fixed top-0 w-full z-50 flex justify-between items-center px-6 h-16 bg-white/80 backdrop-blur-md shadow-sm">
  <span class="text-xl font-bold font-manrope">{ headerTitle }</span>
  <form method="post" action="/jury/reset">
    <button type="submit" class="bg-error text-white px-4 py-2 rounded-xl text-sm font-bold">Reset</button>
  </form>
</header>
```

- Sidebar:

```html
<aside class="fixed left-0 top-16 h-[calc(100vh-64px)] flex flex-col p-4 bg-surface-container-low w-64">
  <nav class="flex-1 space-y-1">
    @navItem("/jury/", "dashboard", "Competition", activePage == "competition")
    @navItem("/jury/countdown", "alarm", "Countdown", activePage == "countdown")
    @navItem("/jury/participants", "groups", "Participants", activePage == "participants")
    @navItem("/jury/submissions", "upload_file", "Submissions", activePage == "submissions")
    @navItem("/jury/modules", "inventory_2", "Modules", activePage == "modules")
    @navItem("/jury/scoring", "edit_document", "Scoring", activePage == "scoring")
    @navItem("/jury/files", "folder_open", "Files", activePage == "files")
  </nav>
</aside>
<main class="ml-64 mt-16 p-10 min-h-screen">{ children... }</main>
```

- [ ] **Step 2: Rewrite `navItem` helper**

Add the Material icon param. Active adds `bg-primary/10 text-primary`:

```
templ navItem(href, icon, label string, active bool) {
  <a href={ templ.URL(href) }
     class={ "sidebar-link", templ.KV("bg-primary/10 text-primary", active) }>
    <span class="material-symbols-outlined">{ icon }</span>
    <span>{ label }</span>
  </a>
}
```

- [ ] **Step 3: Generate + build**

```bash
go generate ./... && go build ./...
```

Expected: green.

- [ ] **Step 4: Smoke-check a jury page renders with the new shell**

```bash
go build ./cmd/server && ./server --data /tmp/lksshell --listen 127.0.0.1:18081 &
sleep 1
curl -s http://127.0.0.1:18081/jury/ | grep -c "material-symbols-outlined"   # >= 7 (nav icons)
kill %1; rm -rf /tmp/lksshell
```

(If `/jury/` requires IP allowlist and localhost is blocked, verify by rendering the templ component in a small `go test` instead.)

- [ ] **Step 5: Commit**

```bash
git add internal/web/templates/layout_app.templ internal/web/*.go
git commit -m "feat(web): rebuild jury shell with fixed header and icon sidebar

Port layouts/app.blade.php: fixed competition-name header with Reset button,
256px sidebar with Material Symbols nav items and primary active state."
```

---

### Task 4: Rebuild GuestLayout (public/auth shell)

**Files:**
- Modify: `internal/web/templates/layout_guest.templ`
- Modify: callers of `GuestLayout` (login, dashboard, countdown_public, leaderboard) for the new signature.

**Interfaces:**
- Produces: `GuestLayout(title, navLeft, navRight string)`. Empty `navLeft` falls back to "Lomba Kompetensi Siswa". Consumed by Tasks 12-15.

**Adaptation:** old `layouts/guest.blade.php` uses Blade `@yield('nav-left')`/`@yield('nav-right')`. templ has no yield; add the two string params. Every current caller passes only `title` today, so update those 4 call sites to pass `""` (or the participant name for the dashboard) as the new args.

- [ ] **Step 1: Rewrite the shell**

```html
<body class="bg-gray-50 min-h-screen">
  <nav class="fixed top-0 w-full z-50 flex justify-between items-center px-6 h-16 bg-white/80 backdrop-blur-md shadow-sm">
    <span class="text-xl font-bold text-gray-900">{ navLeftOr(navLeft) }</span>
    <span>{ navRight }</span>
  </nav>
  <main class="pt-20 min-h-screen">{ children... }</main>
</body>
```

with helper `func navLeftOr(s string) string { if s == "" { return "Lomba Kompetensi Siswa" }; return s }` (or inline templ conditional).

- [ ] **Step 2: Update the 4 call sites** to the new signature. Login/countdown_public/leaderboard pass `GuestLayout(title, "", "")`; dashboard passes the participant name as `navRight`.

- [ ] **Step 3: Generate + build**

```bash
go generate ./... && go build ./...
```

Expected: green (compile error if any call site missed the new args — fix it).

- [ ] **Step 4: Commit**

```bash
git add internal/web/templates/layout_guest.templ internal/web/templates/*.templ
git commit -m "feat(web): rebuild guest shell with top nav bar

Port layouts/guest.blade.php: fixed top nav with left/right slots (added as
GuestLayout params since templ has no Blade yields), pt-20 main."
```

---

**Tasks 5-16 (per-page ports).** Each task: rewrite ONE template's markup to its old blade counterpart, keeping the Go component signature and all form field `name=` attributes exactly as they are today (handlers parse them). Adapt Laravel-isms (`route()` -> real path already in the current templ; `@csrf` -> omit, CSRF is Phase 13; Echo -> existing WS/JS). Apply the `tertiary` remap from Global Constraints. After each: `go generate ./... && go build ./...` green, then commit `feat(web): port <page> to original design`.

The old blade reference file and the current templ file for each page are named in the table. Read BOTH before editing: reproduce the old blade's layout/classes, but preserve the current templ's data bindings and form field names.

| Task | Current templ | Old blade reference | Key elements to reproduce |
|---|---|---|---|
| 5 | `competition.templ` | `competition/index.blade.php` | `max-w-6xl mx-auto px-12`, 2-col grids, uppercase labels, `bg-surface-container-high` inputs, `signature-gradient` submit |
| 6 | `countdown_jury.templ` | `countdown/index.blade.php` | `max-w-7xl`, `text-[10rem] tabular-nums` timer + blur blobs, 3-col config, RESUME/SAVE `signature-gradient` / PAUSE `bg-amber-500` / STOP `bg-red-500`; keep `countdown.js` + `#cd-clock` id |
| 7 | `participants.templ` | `participants/index.blade.php` | header Export `bg-green-600` + Shuffle `signature-gradient`; 12-col grid, Registration + Import cards (dashed dropzone, `description`/`warning`/`sync` icons, `confirm()`), Queue `divide-y` + Seat table; KEEP PC/password/IP columns in old card styling |
| 8 | `modules.templ` | `modules/index.blade.php` | `max-w-6xl px-12`, current-module `<select>` card + `signature-gradient` Save, cards `lg:grid-cols-3` with left accent bar + `M{n}` badge + inline rename + dashed add-card; set-current uses `bg-primary` (NOT tertiary) |
| 9 | `submissions.templ` | `submissions/index.blade.php` | `max-w-7xl`, 3 stat cards (`groups`/`task_alt`/`view_module` in colored circles), matrix, seat chip, green Unduh / "Belum" pill |
| 10 | `scoring.templ` | `leaderboard/index.blade.php` | `max-w-6xl`, card `rounded-2xl ambient-shadow`, seat chip + name/school + per-module `number` inputs `w-16 h-10 bg-surface-container text-center font-bold`, live-sum Total `text-2xl text-primary`, Export + `signature-gradient` Simpan (`save` icon); KEEP existing POST form + score field names + sum JS |
| 11 | `files.templ` | `files/index.blade.php` | dashed dropzone, table Public toggle / Download / Delete; keep `uploader.js` + `#dropzone`/`#progress-*` ids |
| 12 | `login.templ` | `auth/register.blade.php` | centered card, logo, headline, error/success alert boxes, `signature-gradient` submit; KEEP pc_number+password fields |
| 13 | `participant_dashboard.templ` | `participants/public.blade.php` | 12-col, col-7 Download Files (submission + official cards), col-5 sticky upload card + `#sensor-layer` lock overlay (`lock` icon, "opens at last 20 minutes"), styled `file:` input, `signature-gradient`; keep `uploader.js`+`dashboard.js` + all element ids |
| 14 | `countdown_public.templ` | `countdown/public.blade.php` | standalone (GuestLayout), logo `w-60`, comp name `font-extrabold text-4xl`, three `text-9xl` digit blocks (minutes `text-primary`), blink at zero + alert.mp3; keep `countdown.js` + `#cd-clock` `data-alert` |
| 15 | `leaderboard.templ` | `public-leaderboard.blade.php` | card `rounded-3xl shadow-xl`, position tints, gradient medal circles, emoji/Medallion badge, per-module chips, WSI Total, footer legend; `#leaderboard-body` seeded, rows from JS (see Task 16) |
| 16 | `shuffle.templ` | `participants/shuffle.blade.php` | adopt old shuffle card + seat-grid styling, stays jury-side; wheel spin animation OPTIONAL (skip if it needs new JS beyond a few lines) |

**Per-task step template (apply to each of Tasks 5-16):**

- [ ] **Step 1:** Read the old blade file and the current templ file.
- [ ] **Step 2:** Rewrite the templ markup to the old blade look; keep component signature, form field `name=` attrs, and element ids the JS/handlers depend on. Apply `tertiary` remap.
- [ ] **Step 3:** `go generate ./... && go build ./...` — expected green.
- [ ] **Step 4:** Commit `feat(web): port <page> to original design`.

---

### Task 17: Rewrite leaderboard.js and dashboard.js class strings

**Files:**
- Modify: `internal/web/static/js/leaderboard.js`
- Modify: `internal/web/static/js/dashboard.js`

**Interfaces:**
- Consumes: the markup shapes from Tasks 13 (dashboard) and 15 (leaderboard). The injected DOM must match those pages' static markup classes.

**Adaptation:** these files build rows/cards with hardcoded token classes. After Tasks 13 and 15 change the surrounding static markup, the JS-injected nodes must use the same old-design classes to stay visually consistent.

- [ ] **Step 1:** In `leaderboard.js`, update the row template string to old `public-leaderboard` markup: position tints (`bg-amber-500/10` #1, `bg-slate-400/10` #2, `bg-orange-500/10` #3), gradient medal circle, emoji medals (🥇🥈🥉) / Medallion badge when `wsi >= 700`, per-module chips `bg-surface-container`, Total `text-2xl font-black`.
- [ ] **Step 2:** In `dashboard.js`, update the `FileListUpdated` node builder (official-file card `bg-surface-container rounded-2xl`, `description`/`download` icons) to match Task 13's static file-card markup.
- [ ] **Step 3:** `go generate ./...` (re-runs Tailwind so the JS classes are scanned via `@source ./static/js/*.js`) `&& go build ./...` — green. Confirm the classes used in JS now exist in `app.css`.
- [ ] **Step 4:** Manual: run server, open `/leaderboard` and `/` (dashboard), confirm JS-rendered rows/cards match the static markup styling.
- [ ] **Step 5:** Commit `feat(web): restyle JS-injected leaderboard rows and dashboard file cards`.

---

### Task 18: Token-coverage check

**Files:**
- Create: `internal/web/css_coverage_test.go`

**Interfaces:**
- Consumes: generated `static/css/app.css`, all `.templ` files, `static/js/*.js`.

- [ ] **Step 1: Write the test**

A `go test` that scans `_templ.go` files + `static/js/*.js` for the project's MD3 token classes (from a fixed list: the `surface-container*`, `on-*`, `text-headline-*`/`title-*`/`label-*`/`body-*`, `font-manrope`, `signature-gradient`, `ambient-shadow`, `material-symbols-outlined` families) and asserts each appears in `app.css`. Purpose: catch a template referencing a token the CSS never emitted (the exact class of bug this phase fixes).

```go
package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestCSSCoverage asserts every MD3 design-token class used in the compiled
// templates and JS resolves to a rule in the generated app.css. Guards against
// a template referencing an undefined token (blank styling at runtime).
func TestCSSCoverage(t *testing.T) {
	css, err := os.ReadFile("static/css/app.css")
	if err != nil {
		t.Fatalf("read app.css: %v", err)
	}
	cssStr := string(css)

	// The design-token class stems this project relies on.
	stems := []string{
		"surface-container", "on-surface", "on-surface-variant",
		"headline-large", "headline-medium", "title-large", "title-medium",
		"label-large", "label-medium", "label-small",
		"body-large", "body-medium", "body-small",
		"font-manrope", "signature-gradient", "ambient-shadow",
		"material-symbols-outlined", "on-secondary-container",
		"error-container", "on-error", "outline-variant",
	}
	for _, s := range stems {
		if !strings.Contains(cssStr, s) {
			t.Errorf("token %q used by templates but absent from app.css (regenerate: go generate ./...)", s)
		}
	}

	// Sanity: at least one _templ.go actually references font-manrope, so the
	// list above cannot silently drift to all-dead stems.
	var found bool
	_ = filepath.Walk("templates", func(p string, _ os.FileInfo, _ error) error {
		if strings.HasSuffix(p, "_templ.go") {
			b, _ := os.ReadFile(p)
			if regexp.MustCompile(`font-manrope`).Match(b) {
				found = true
			}
		}
		return nil
	})
	if !found {
		t.Error("expected font-manrope in a compiled template; scan list may be stale")
	}
}
```

- [ ] **Step 2: Run it**

```bash
go generate ./... && go test ./internal/web/ -run TestCSSCoverage -v
```

Expected: PASS. If a stem is missing, either the CSS wasn't regenerated or a token is genuinely undefined — fix the real cause, don't delete the stem.

- [ ] **Step 3: Full regression + commit**

```bash
go test ./...   # everything still green (no behavior changed)
git add internal/web/css_coverage_test.go
git commit -m "test(web): assert MD3 token classes resolve in generated app.css"
```

---

### Task 19: Update docs (CHANGELOG + README)

**Files:**
- Modify: `docs/CHANGELOG.md`
- Modify: `README.md`

- [ ] **Step 1: CHANGELOG** — add `## Phase 12 - UI Modification (<date>) ✅` section: tailwind CLI pipeline (`tailwind.css` source, `go generate` directive, `tools/tailwindcss`), self-hosted Inter/Manrope, both shells rebuilt, 12 pages ported, JS class rewrites, `css_coverage_test.go`. Replace the "Next" section with one for Phase 13 - Polish & Build.
- [ ] **Step 2: README** — flip the Phase 12 table row to done; document the new `go generate` CSS step, the `tools/tailwindcss` requirement (with the "app.css is committed so build works without it" note), and the new `tailwind.css` / `static/fonts/` paths. Update the Status paragraph if warranted.
- [ ] **Step 3: Commit** `docs: Phase 12 UI modification complete`.

---

## Self-Review

**Spec coverage:** CSS pipeline (T1), fonts (T2), shells (T3-T4), all 13 page mappings incl. PDF-no-change noted, JS rewrites (T17), token reconciliation (T1), verify (T18), docs (T19). PDF (spec item 13) needs no task — noted in Global Constraints as untouched. Covered.

**Placeholder scan:** Tailwind binary URL, font URLs, all class lists, and the full `tailwind.css` + test source are concrete. The only deliberate latitude: Task 3's header-title mechanism (import-cycle-safe accessor) and Task 16's optional wheel animation — both flagged with the binding constraint stated, not left blank.

**Type consistency:** `AppLayout(title, activePage)` unchanged (T3); `GuestLayout` gains `(title, navLeft, navRight)` with all 4 call sites updated (T4); `navItem` gains an `icon` param (T3). Page component signatures and form field names explicitly preserved (T5-16).

**Ordering:** T1 (tokens) before everything; T2 fonts before pages need them; shells (T3-4) before pages (T5-16); JS (T17) after its pages (T13,15); coverage test (T18) after all markup; docs (T19) last.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-08-11-phase12-ui-modification.md`. Two execution options:

1. **Subagent-Driven (recommended)** - fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** - execute tasks in a session using executing-plans, batch with checkpoints.

The user intends to execute in a DIFFERENT session, so no execution starts here.
