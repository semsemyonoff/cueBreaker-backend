# cueBreaker — Go Rewrite + "Waveform & Cuts" Redesign (Monorepo Phase 1)

## Overview
Full rewrite of cueBreaker (a FLAC+CUE album splitter) from Python/Flask into a **Go backend**
plus a **React + Vite + TypeScript SPA**, shipping the approved "Waveform & Cuts" redesign.

- **Problem solved:** the current UI is a purely-technical GitHub-dark tool; the redesign turns it
  into an audio-editor with a waveform whose vertical cut-lines sit at the real track boundaries.
  The Go rewrite gives a single static binary and a clean, typed codebase.
- **Behavior parity:** the backend ports the existing scan/parse/split/tag **endpoint + pipeline**
  behavior (same external tools: `shnsplit`, `cuebreakpoints`, `cueprint`, `metaflac`), with two
  intentional changes — split execution is **serialized** (one at a time) where the Flask app spawned
  an unbounded thread per request, and cover-path containment is tightened to trailing-slash on all
  three routes — plus the small extra data the waveform needs (album duration + numeric per-track
  offsets) and a version endpoint.
- **This plan = monorepo Phase 1.** Work happens in ONE repo with strict `backend/` + `frontend/`
  directory separation, engineered so a later extraction into separate `backend` / `frontend` /
  `deploy` repos (submodules + multi-stage Dockerfile, mirroring the AlbFetcharr/beetDeck pattern)
  is a move, not a refactor. **The repo split and the deploy repo are OUT OF SCOPE here** — but the
  boundaries are designed for it now.

## Context (from discovery)
- **Current app:** `app.py` (~507 lines, Flask) + `templates/index.html` (vanilla-JS SPA, hash
  routing, client-built tree) + `static/style.css` + `static/logo.svg`. `Dockerfile` is multi-stage
  (builds `shntool` 3.0.10 from source + apk `cuetools` + `flac`, runs gunicorn).
- **External tools (kept):** `cuebreakpoints`, `shnsplit` (shntool), `cueprint` (cuetools),
  `metaflac` (flac). The Go layer only orchestrates them via `os/exec`.
- **Endpoints today:** `GET /`, `GET /api/scan`, `GET /api/search?q=`, `POST /api/preview`,
  `GET /api/cover/<path>`, `POST /api/split`, `GET /api/status/<job_id>`.
- **Encodings auto-detected:** `utf-8-sig, utf-8, cp1251, cp1252, shift_jis, euc-kr, latin-1`
  (validated by presence of `TRACK`+`INDEX`; fallback `latin-1`).
- **Security:** realpath containment under `INPUT_DIR` on preview/cover/split; recent commit
  `82d0a66` hardened navigation for apostrophes / JS-unsafe path chars — preserve this robustness.
- **Env:** `CUEBREAKER_INPUT_DIR` (/input), `CUEBREAKER_OUTPUT_DIR` (/output).
- **Design source of truth:** `docs/prototype/cueBreaker-prototype.html` + `docs/prototype/README.md`
  (palette tokens, fonts, all screen states). Stays in this repo for now (moves to the workspace
  repo LATER — not into the frontend repo).
- **Org convention (for the future split):** sibling products `AlbFetcharr` / `beetDeck` use three
  repos (`backend`, `frontend`, `deploy`); `deploy` pins backend+frontend as git submodules and a
  multi-stage Dockerfile builds the SPA then `COPY --from`s it into the backend before the backend
  is built. Both org frontends are React+Vite (JSX); we upgrade to TypeScript.

## Development Approach
- **Testing approach: Regular** (implement, then write tests in the same task).
- **Each task keeps the build green** — the Go module must compile and its tests pass before the
  next task; the frontend module must `npm run build` + `npm run test` clean before the next
  frontend task. Caller/callee signature changes land in the same task.
- **Every task includes new/updated tests** as separate checklist items (success + error/edge).
  - Backend: Go table-driven tests beside code (`*_test.go`), `t.TempDir()` for filesystem work.
    Tests that shell out to real tools (`metaflac`/`shnsplit`) parse **captured sample output** in
    unit tests and gate any real-tool integration behind `testing.Short()`/tool-presence skips.
  - Frontend: **Vitest** for non-visual logic (tree building, waveform geometry, API client, split
    polling). Pure-visual/CSS work is verified by `npm run build` + manual QA in Post-Completion.
- Run `make test` (or the module-local test command) before considering a task done. Keep this plan
  in sync — mark `[x]` immediately, `➕` new tasks, `⚠️` blockers.

## Testing Strategy
- **Unit (Go):** encoding detection; CUE parse (album + tracks + `INDEX`→seconds); metaflac
  duration-output parsing; scan pair-detection + already-split status; cover discovery; shnsplit
  progress-line parsing; job queue state transitions + serialization (one-at-a-time); HTTP handlers
  via `httptest` incl. path-traversal 403s and SPA fallback.
- **Unit (Vitest):** flat-paths→tree builder + filter + open-state; waveform cut-line geometry
  (INDEX+duration→percent positions, fill/playhead); typed API client (mocked `fetch`); split
  polling reducer (queued→splitting→tagging→done/error).
- **No e2e framework** in Phase 1; cross-screen flows are manual scenarios in Post-Completion.

## Progress Tracking
- Mark items `[x]` immediately when done. New tasks `➕`; blockers `⚠️`. Keep synced with reality.

## Solution Overview
- **Backend (Go, single binary).** `cmd/cuebreaker/main.go` loads env config, builds the job queue
  and HTTP server, and serves the embedded SPA. Packages:
  - `internal/cue` — encoding-detecting reader; regex parser producing album meta + tracks with
    `INDEX 01` as both `MM:SS:FF` string and numeric `StartSeconds`; a UTF-8 temp-copy helper; and a
    FLAC duration reader (`metaflac --show-total-samples --show-sample-rate` → `TotalSeconds`).
  - `internal/scan` — walk `INPUT_DIR`, keep dirs whose CUE references an existing single unsplit
    FLAC, compute already-split status vs `OUTPUT_DIR`, discover cover art.
  - `internal/split` — orchestrate `cuebreakpoints` → `shnsplit` (streaming stderr → progress) →
    per-track tagging (`cueprint` + `metaflac`) → pregap removal → cover copy.
  - `internal/job` — serialized worker (one split at a time), in-memory job registry keyed by
    `path/cue_file`, progress/status fields matching today.
  - `internal/server` — `net/http` ServeMux (Go 1.22+ patterns), JSON handlers, realpath
    containment, `embed.FS` static serving with SPA fallback, `slog` logging.
- **Frontend (React + Vite + TS SPA).** Typed API client; `Shell` (topbar + resizable sidebar +
  work panel); `Tree` (flat-paths→hierarchy, filter, persisted open-state, selection); `AlbumPanel`
  (header/cover/chips/pill/CUE selector/track table); `Waveform` (synthetic bars + real cut-lines,
  variants idle/active/done, progress fill + playhead, hover link segment↔row); split flow with
  status polling; mobile drawer; all states from the prototype. Dark theme only.
- **Orchestration (seeds future `deploy`).** Root `Makefile` builds the SPA into
  `backend/web/dist`, then `go build` embeds it; a root multi-stage `Dockerfile` (node build →
  go build → alpine runtime with the same shntool/cuetools/flac tool stack) produces the image.

## Technical Details
- **Module path:** `git.horn/cueBreaker/backend` (rename-friendly for the split). Go 1.23+.
- **Config:** `CUEBREAKER_INPUT_DIR` (default `/input`), `CUEBREAKER_OUTPUT_DIR` (`/output`),
  `CUEBREAKER_PORT` (`5000`). Version injected via `-ldflags "-X main.version=$APP_VERSION"`,
  overridable by `APP_VERSION` env; surfaced at `GET /api/version` → `{"version": "..."}`.
- **Preview response (extended for the waveform)** — superset of today:
  ```json
  {
    "performer": "", "title": "", "file": "", "genre": "", "date": "",
    "has_cover": true, "cover_name": "cover.jpg",
    "split_done": false, "output_tracks": 0,
    "total_seconds": 3684.0,
    "tracks": [
      { "number": 1, "title": "", "performer": "", "index": "00:00:00", "start_seconds": 0.0 }
    ]
  }
  ```
  `total_seconds` from `metaflac` (FLAC) or a cheap RIFF-header read (WAV), `0` if unreadable;
  `start_seconds` from `INDEX 01` (`min*60 + sec + frames/75`). The frontend draws a cut-line at
  `left%` = `start_seconds / total_seconds * 100` only for tracks with `start_seconds > 0`; when
  `total_seconds <= 0` it renders the waveform without cut-lines (no bunching at the left edge).
- **Scan response:** unchanged shape (`path`, `abs_path`, `cue_files[]`, `flac_files[]`,
  `split_done`, `output_tracks`), sorted by `path`. Search = `GET /api/scan` filtered client-side is
  kept server-side as `GET /api/search?q=` for parity.
- **Job status shape:** `{status, message, result_files[], progress_current, progress_total,
  progress_detail}` with `status ∈ queued|splitting|tagging|done|error`. `job_id = "<path>/<cue>"`.
- **Split progress:** total steps = `2 * trackCount` (split half + tag half), matching today.
- **Static serving:** SPA lives at `/`; assets under Vite `base`; `/api/*` is the API namespace;
  `/api/cover/<path>` streams the cover. Unknown non-API paths fall back to `index.html`.
- **embed placeholder:** commit `backend/web/dist/index.html` (a minimal placeholder) + `.gitkeep`
  so `//go:embed all:dist` (in `backend/web/embed.go`, pattern relative to that package) compiles
  before the SPA is built; `make`/Docker overwrite it with the real Vite output (otherwise
  git-ignored). Serve through `fs.Sub(embedded, "dist")`.
- **Frontend↔backend dev:** Vite dev server proxies `/api` (and `/api/cover`) to
  `http://localhost:5000`. `vite.config.ts` `base` matches the Go static mount (root `/`).

## What Goes Where
- **Implementation Steps** (`[ ]`): all code, tests, config, root orchestration in this monorepo.
- **Post-Completion** (no checkboxes): manual visual QA against the prototype, real-tool split
  smoke test, and the future repo-split (explicitly deferred).

## Implementation Steps

### Task 1: Monorepo layout + Go module scaffold + config
**Files:**
- Create: `backend/go.mod`, `backend/cmd/cuebreaker/main.go`, `backend/internal/config/config.go`
- Create: `backend/internal/config/config_test.go`
- Create: `backend/.gitignore` (ignore `web/dist/*`, keep `web/dist/.gitkeep` + placeholder), `.gitignore` (root: node_modules, build artifacts)

- [x] init module `git.horn/cueBreaker/backend` (Go 1.23+); add `cmd/cuebreaker/main.go` with a `var version = "dev"` and a minimal `slog` setup that starts and logs config
- [x] implement `config.Load()` reading `CUEBREAKER_INPUT_DIR`/`OUTPUT_DIR`/`PORT` with defaults
- [x] set up directory skeleton `internal/{cue,scan,split,job,server}` (empty `doc.go` placeholders ok)
- [x] write tests for `config.Load()` (defaults, overrides, invalid port)
- [x] `go build ./...` + `go test ./...` green before next task

### Task 2: `internal/cue` — encoding-detecting reader + UTF-8 temp copy
**Files:**
- Create: `backend/internal/cue/encoding.go`, `backend/internal/cue/encoding_test.go`
- Create: `backend/testdata/cue/*.cue` (fixtures in several encodings)

- [x] port `read_cue`: try `utf-8-sig, utf-8, cp1251, cp1252, shift_jis, euc-kr, latin-1` (via `golang.org/x/text/encoding`), accept first decode that contains `TRACK`+`INDEX`, else fallback `latin-1`; strip BOM
- [x] port `make_utf8_cue`: write a temp UTF-8 `.cue` (caller cleans up)
- [x] add encoded fixtures (UTF-8, CP1251, Shift-JIS at minimum)
- [x] write table-driven tests for detection across fixtures + fallback path
- [x] write test for temp-copy content + cleanup contract
- [x] `go test ./internal/cue` green

### Task 3: `internal/cue` — CUE parser (album + tracks + numeric offsets)
**Files:**
- Modify: `backend/internal/cue/cue.go` (new), `backend/internal/cue/cue_test.go`

- [x] parse album `PERFORMER/TITLE/FILE/REM GENRE/REM DATE` (`REM GENRE` tolerates optional quotes; `REM DATE` is unquoted `\S+`); split `TRACK N AUDIO` blocks; per track `TITLE/PERFORMER/INDEX 01`, with track `performer` defaulting to the album performer when absent
- [x] compute `StartSeconds` from `INDEX 01` `MM:SS:FF` (`m*60+s+f/75`); keep raw `index` string
- [x] add `cue_has_source_flac` (single existing `.flac`/`.wav` FILE ref ⇒ unsplit; multi-file ⇒ skip)
- [x] add pure `SourceFLAC(cueInfo, dir) (string, bool)` (resolve source: CUE `FILE` if it exists, else first `.flac`/`.wav` in dir) — shared by split (Task 7) and preview `total_seconds` (Task 10); resolve via `filepath.EvalSymlinks` and return the file only if its real path stays under `dir`/`INPUT_DIR` (reject `FILE "../x.flac"` and symlink escapes)
- [x] write tests for parse (multi-track, missing fields, track-performer defaulting, quoted/unquoted GENRE+DATE, multi-FILE detection) + `StartSeconds` math + `SourceFLAC` (FILE hit, fallback, none, `../` traversal + symlink-escape rejected)
- [x] `go test ./internal/cue` green

### Task 4: `internal/cue` — FLAC duration via metaflac
**Files:**
- Create: `backend/internal/cue/duration.go`, `backend/internal/cue/duration_test.go`

- [x] `TotalSeconds(path)`: dispatch by extension — FLAC via `metaflac --show-total-samples --show-sample-rate` (→ `total_samples/sample_rate`); **WAV via a cheap RIFF-header read** (`data` chunk bytes / byte-rate, no decode); return 0 + error on failure (non-fatal — callers/frontend degrade gracefully)
- [x] factor the numeric parsing into pure `parseMetaflacDuration(stdout)` and `parseWavDuration(header)` for testing
- [x] write tests for `parseMetaflacDuration` (valid, zero rate, garbage) + `parseWavDuration` (valid header, truncated/garbage) using captured samples
- [x] add a tool-presence-guarded test that runs real `metaflac` only when available (else skip)
- [x] `go test ./internal/cue` green

### Task 5: `internal/scan` — pair discovery + already-split status
**Files:**
- Create: `backend/internal/scan/scan.go`, `backend/internal/scan/scan_test.go`

- [x] port `find_cue_pairs`: walk `INPUT_DIR`, keep dirs with a CUE referencing an existing single FLAC; collect `cue_files[]`/`flac_files[]`; sort by rel path
- [x] port `check_output_status`: compare CUE track count vs FLAC count in `OUTPUT_DIR/<rel>` → `done/output_tracks`
- [x] provide a `Search(pairs, q)` filter (case-insensitive path contains; empty `q` ⇒ `[]`, as in `app.py`) for parity
- [x] write tests with `t.TempDir()` fixtures (unsplit pair, already-split, multi-file cue skipped, nested dirs) + `Search` (match, no-match, empty `q` ⇒ `[]`)
- [x] `go test ./internal/scan` green

### Task 6: `internal/scan` — cover discovery
**Files:**
- Create: `backend/internal/scan/cover.go`, `backend/internal/scan/cover_test.go`

- [x] port `find_cover`: patterns `cover/folder/front/album` (case variants) with exts `jpg,jpeg,png,bmp,gif,webp`; else first image in dir; return the match only if its `EvalSymlinks` real path stays under `INPUT_DIR` (reject symlink-escape covers)
- [x] write tests (named match priority, extension filter, fallback-to-first, none found, symlink-escape rejected)
- [x] `go test ./internal/scan` green

### Task 7: `internal/split` — shnsplit orchestration + progress parsing
**Files:**
- Create: `backend/internal/split/split.go`, `backend/internal/split/progress.go`, `backend/internal/split/progress_test.go`

- [x] implement pipeline start: locate source FLAC via `cue.SourceFLAC`, make UTF-8 temp cue, run `cuebreakpoints` (error surfaced), run `shnsplit` (`-f cue -O always -o flac -t "%n - %t" -d outdir source`) streaming stderr
- [x] run tools through a shared context-aware helper (`exec.CommandContext`): `cuebreakpoints` gets a short timeout (~30s, as in `app.py`); `shnsplit` runs cancellable with **no** hard timeout (long jobs), killed when the job context is canceled; stream `shnsplit` stderr with a `bufio.Scanner` sized for long lines; `defer` removal of the temp UTF-8 cue on every exit path
- [x] pure `parseShnsplitLine(line)` → `{trackName, isProgressStep}` (matches `-->` / `OK` lines)
- [x] emit progress via an injected callback (so `internal/job` drives status); no direct global state; cap the split-half progress at `min(stepsSeen, trackCount)` (the pregap line also matches `-->`/`OK`, as in `app.py`)
- [x] write tests for `parseShnsplitLine` (captured shnsplit stderr samples, incl. no-match lines) + the progress cap (pregap line does not push past `trackCount`)
- [x] `go test ./internal/split` green

### Task 8: `internal/split` — tagging, pregap removal, cover copy
**Files:**
- Modify: `backend/internal/split/split.go`
- Create: `backend/internal/split/tag.go`, `backend/internal/split/tag_test.go`

- [x] port tagging: per track `cueprint` (`%t,%p,%n`; album `%T,%P`) + `metaflac --remove-all-tags` then `--set-tag`; add `GENRE/DATE/TRACKTOTAL` (each `cueprint`/`metaflac` call via the shared context helper with a short ~10s timeout, as in `app.py`)
- [x] exclude the pregap `00 - *.flac` from the tag list **before** tagging (so `TRACKTOTAL` = real, non-pregap track count), then delete the pregap file **after** tagging (ordering as in `app.py`); copy discovered cover into `outdir`; collect `result_files[]`
- [x] build the tag-set for a track as a pure, testable function (`buildTags(...) map[string]string`)
- [x] write tests for `buildTags` (title/artist/track/album, genre+date fallbacks) + `TRACKTOTAL` equals non-pregap count (pregap excluded from input)
- [x] `go test ./internal/split` green

### Task 9: `internal/job` — serialized queue + in-memory state
**Files:**
- Create: `backend/internal/job/job.go`, `backend/internal/job/queue.go`, `backend/internal/job/job_test.go`

- [x] `Manager` with an in-memory map keyed by `path/cue_file` + a single worker goroutine draining a channel (one split at a time); `Enqueue` rejects a job already `queued|splitting|tagging` (409-style result — a queued duplicate must NOT enqueue a second run)
- [x] job record: `status,message,result_files,progress_current,progress_total,progress_detail`; thread-safe `Update`/`Get` (guard all reads/writes so concurrent status polling is race-free — verify with `-race`); wire `internal/split`'s progress callback to `Update`
- [x] give each job a cancelable `context.Context` (passed to `internal/split`); map split errors/timeouts/cancellation to `status=error` with message; success → `done`
- [x] write tests (run under `-race`): enqueue→run→done transitions; duplicate rejected while the first is still `queued` (behind another job) AND while `splitting`; serialization (2 enqueued run sequentially, not concurrently); error mapping
- [x] `go test ./internal/job` green

### Task 10: `internal/server` — routes, JSON API, path security
**Files:**
- Create: `backend/internal/server/server.go`, `backend/internal/server/handlers.go`, `backend/internal/server/handlers_test.go`

- [x] ServeMux (Go 1.22+ patterns) with `GET /api/scan`, `GET /api/search`, `POST /api/preview`, `GET /api/cover/{path...}`, `POST /api/split`, `GET /api/status/{job_id...}`, `GET /api/version`
- [x] `preview` returns the extended shape (Technical Details): resolves the FLAC via `cue.SourceFLAC` and fills `total_seconds` from `cue.TotalSeconds` (0 if unreadable, non-fatal), plus per-track `start_seconds`; realpath containment under `INPUT_DIR` (403 on escape) on preview/cover/split — trailing-slash containment on all three (a deliberate tightening vs `app.py`'s looser cover check)
- [x] `slog` request/pipeline logging preserving today's per-track detail
- [x] write handler tests (`httptest`): scan/preview/version JSON shape; `total_seconds` populated from the resolved FLAC; preview missing-cue → 404; path-traversal → 403; split enqueue → 202/409; status not-found → 404
- [x] `go test ./internal/server` green

### Task 11: `internal/server` — embed.FS SPA serving + SPA fallback + wire main
**Files:**
- Create: `backend/internal/server/static.go`, `backend/internal/server/static_test.go`
- Create: `backend/web/dist/index.html` (placeholder), `backend/web/dist/.gitkeep`, `backend/web/embed.go`
- Modify: `backend/cmd/cuebreaker/main.go`

- [x] embed with `//go:embed all:dist` in `backend/web/embed.go` (embed patterns are relative to the package dir, so `dist`, NOT `web/dist`; `all:` includes the `.gitkeep`/dotfiles); serve via `fs.Sub(embedded, "dist")`, fall back to `index.html` for unknown non-`/api` paths
- [x] wire `main.go`: `config.Load` → `job.Manager` → `server.New(...)` → `ListenAndServe` on `CUEBREAKER_PORT`; inject `version`
- [x] write tests: asset served, unknown path → index.html, `/api/*` never falls back to SPA
- [x] `go build ./... && go test ./...` green (full backend compiles with placeholder dist)

### Task 12: Frontend scaffold (Vite + React + TS + Vitest) + design tokens
**Files:**
- Create: `frontend/package.json`, `frontend/vite.config.ts`, `frontend/tsconfig.json`, `frontend/index.html`, `frontend/src/main.tsx`, `frontend/src/App.tsx`, `frontend/src/styles/tokens.css`, `frontend/src/styles/base.css`, `frontend/src/main.test.ts`

- [x] init npm with `react@19`, `react-dom@19`, `vite`, `typescript`, `vitest`, `@testing-library/react`, `jsdom`; scripts `dev/build/test/lint`
- [x] `vite.config.ts`: `base: '/'`, `build.outDir` default `dist`, dev `server.proxy` for `/api` → `http://localhost:5000`; Vitest config (jsdom)
- [x] port palette tokens + fonts (Manrope, JetBrains Mono) from `docs/prototype/README.md`/`cueBreaker-prototype.html` into `tokens.css`/`base.css`
- [x] write a sanity Vitest test (renders `App` shell) to prove the toolchain
- [x] `npm run build` + `npm run test` green

### Task 13: Typed API client + domain types
**Files:**
- Create: `frontend/src/api/types.ts`, `frontend/src/api/client.ts`, `frontend/src/api/client.test.ts`

- [x] define TS types for scan item, preview (incl. `total_seconds`, track `start_seconds`), job status, version
- [x] client functions: `scan()`, `search(q)`, `preview(path,cue)`, `split(path,cue)`, `status(jobId)`, `version()`, `coverUrl(path)` (with correct encoding for apostrophes/unsafe chars)
- [x] write Vitest tests with mocked `fetch` (success + error + URL encoding of tricky paths)
- [x] `npm run test` green

### Task 14: Library tree — builder + Sidebar component
**Files:**
- Create: `frontend/src/tree/buildTree.ts`, `frontend/src/tree/buildTree.test.ts`, `frontend/src/components/Sidebar.tsx`, `frontend/src/components/Tree.tsx`

- [x] `buildTree(items)`: flat rel-paths → nested folders/leaves with per-node leaf counts; `filterTree(q)`; open-state map (persisted to `localStorage`)
- [x] `Sidebar` (search + Rescan) + `Tree` (folders with counts, album leaves with signal icon, done ✓, active selection) per prototype
- [x] write Vitest tests for `buildTree` (nesting, "[this folder]" leaf case, counts) + filter
- [x] `npm run test` + `npm run build` green

### Task 15: App shell — topbar, two-pane layout, sidebar resize + mobile drawer
**Files:**
- Create: `frontend/src/components/Shell.tsx`, `frontend/src/components/Topbar.tsx`, `frontend/src/ui/resizer.ts`, `frontend/src/ui/resizer.test.ts`, `frontend/src/styles/shell.css`
- Modify: `frontend/src/App.tsx`

- [x] `Shell` grid (topbar + sidebar + work panel); `Topbar` with logo (`logo.svg`), wordmark, version badge, counts (albums/unsplit/splitting)
- [x] sidebar resizer (drag, clamp, `localStorage` persistence); mobile: sidebar becomes a drawer + scrim toggled by a burger
- [x] wire `scan()` on load → tree; selecting an album sets the active path
- [x] write Vitest tests for `resizer` (clamp + persist); render test for drawer open/close
- [x] `npm run test` + `npm run build` green

### Task 16: Waveform component + cut-line geometry
**Files:**
- Create: `frontend/src/waveform/geometry.ts`, `frontend/src/waveform/geometry.test.ts`, `frontend/src/components/Waveform.tsx`, `frontend/src/styles/waveform.css`

- [x] `geometry.ts`: synthetic bar heights (deterministic) + `cutPositions(tracks, totalSeconds)` → `left%` **only for tracks with `start_seconds > 0`** (skip the track-1 cut at 0%, matching the prototype which labels from `02`); when `totalSeconds <= 0` (unknown, e.g. a WAV without a readable duration) render bars with **no** cut-lines instead of bunching them at 0; `fillClip(progress)` + playhead position
- [x] `Waveform` variants `idle|active|done` with real cut-lines + progress fill + playhead; `prefers-reduced-motion` respected
- [x] write Vitest tests for `cutPositions` (INDEX+duration→percent, clamp, first cut is track `02` not `01`, `totalSeconds<=0` ⇒ no cut-lines) + fill/playhead
- [x] `npm run test` + `npm run build` green

### Task 17: AlbumPanel — header, chips, CUE selector, track table, hover link
**Files:**
- Create: `frontend/src/components/AlbumPanel.tsx`, `frontend/src/components/TrackTable.tsx`, `frontend/src/components/CueSelector.tsx`, `frontend/src/styles/album.css`
- Modify: `frontend/src/App.tsx`

- [ ] on album select → `preview()`; render breadcrumb, cover (or placeholder), title/artist/chips, status pill (unsplit/split), CUE selector when >1, `Waveform`, `TrackTable`
- [ ] hover link: segment↔row highlight both ways; `MM:SS:FF` timings in mono
- [ ] handle no-cover placeholder + multi-CUE switch (re-preview on change)
- [ ] write Vitest tests for AlbumPanel data→render (pill state, chips, CUE options) with a mocked client
- [ ] `npm run test` + `npm run build` green

### Task 18: Split flow + status polling + remaining states
**Files:**
- Create: `frontend/src/split/usePoll.ts`, `frontend/src/split/usePoll.test.ts`, `frontend/src/components/SplitAction.tsx`, `frontend/src/components/States.tsx`, `frontend/src/styles/states.css`
- Modify: `frontend/src/components/AlbumPanel.tsx`

- [ ] Split button → `split()` → poll `status()` (~500ms) driving `Waveform` fill + status row + counter + disabled "Processing…"; done → results list + "Split again"; error → red status; overwrite warning when `split_done`
- [ ] empty-scan + scanning states in the sidebar/panel per prototype
- [ ] write Vitest tests for the polling reducer (queued→splitting→tagging→done and →error; stops on terminal)
- [ ] `npm run test` + `npm run build` green

### Task 19: Root orchestration — Makefile, multi-stage Dockerfile, README, cleanup
**Files:**
- Create: `Makefile`, `Dockerfile` (root, monorepo), `README.md` (rewrite)
- Delete: `app.py`, `templates/index.html`, `static/style.css`, `build.sh`, old `Dockerfile`/`Makefile` (replaced)
- Keep: `static/logo.svg` referenced by the frontend (copy into `frontend/public/logo.svg`)

- [ ] `Makefile`: `frontend-build` (`npm --prefix frontend ci && npm --prefix frontend run build` → copy `frontend/dist/*` into `backend/web/dist/`), `build` (frontend-build → `go build -ldflags "-X main.version=$(VERSION)"`), `dev` (Vite + `go run` with proxy), `test`, `lint`
- [ ] root multi-stage `Dockerfile`: node stage builds SPA → go stage `COPY --from` dist into `backend/web/dist` then `go build` → alpine runtime building `shntool` 3.0.10 from source + apk `cuetools flac` (mirror current tool stack); `ENV CUEBREAKER_*`; expose port
- [ ] copy `static/logo.svg` → `frontend/public/logo.svg`; remove superseded Python/build files
- [ ] rewrite `README.md` (monorepo layout, `make dev`/`make build`, env vars, tool deps, note on the future repo split)
- [ ] smoke: `make build` yields a binary serving the real SPA; `docker build` succeeds

### Task 20: Verify acceptance criteria
- [ ] verify Overview parity: scan/search/preview/cover/split/status behave like the Python app; extended preview carries `total_seconds` + `start_seconds`; `/api/version` works
- [ ] verify edge cases: multi-file CUE skipped, already-split status, multi-CUE album, no-cover, apostrophe/unsafe paths, split error surfaced, one-split-at-a-time serialization
- [ ] run full backend suite `go test ./...` and frontend `npm --prefix frontend run test` + `npm --prefix frontend run build`
- [ ] confirm `make build` single-binary serves the SPA and drives a split end-to-end

### Task 21: [Final] Update documentation
- [ ] finalize `README.md`; add/refresh `CLAUDE.md` (Go+React monorepo layout, tool deps, `make` targets, the deferred repo-split direction)
- [ ] ensure `docs/prototype/` note points at the design source of truth
- [ ] move this plan to `docs/plans/completed/`

## Post-Completion
*Items requiring manual intervention or external systems — informational only.*

**Manual verification:**
- Visual QA of every screen/state against `docs/prototype/cueBreaker-prototype.html` (desktop +
  mobile drawer): shell, tree, waveform+cuts, splitting progress, done, error, multi-CUE, no-cover,
  overwrite, empty/scanning. Confirm `prefers-reduced-motion` disables waveform/scan animations.
- Real-tool split smoke test against a genuine single-file FLAC+CUE (incl. a CP1251/Shift-JIS CUE):
  correct breakpoints, tags, pregap removed, cover copied, output matches track count.

**Deferred (future, explicitly out of scope here):**
- Split into `cueBreaker/backend` + `cueBreaker/frontend` + `cueBreaker/deploy` repos: move
  `backend/`/`frontend/` out; create `deploy` with `.gitmodules` pinning both, the multi-stage
  Dockerfile (`COPY --from=spa dist` into backend before `go build`), `build.sh` (buildx multi-arch),
  `VERSION`/`CHANGELOG.md`, `docker-compose.yml` (pull-only), and Forgejo `release-cut`/`release`
  workflows — mirroring `AlbFetcharr`/`beetDeck`.
- Move `docs/prototype/` into the workspace repo.
