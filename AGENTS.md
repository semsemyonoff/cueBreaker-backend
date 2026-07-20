# cueBreaker — backend

FLAC+CUE album splitter, Go backend. A single static binary serving a JSON API under
`/api/*` and the embedded SPA on `/`. Part of a three-repo product (`backend`, `frontend`,
`workspace`) mirroring the sibling `beetDeck` / `AlbFetcharr` orgs. Module path:
`git.horn/cueBreaker/backend`.

## Layout

- `cmd/cuebreaker/main.go` — load env config, build the job queue + HTTP server, serve the SPA.
- `internal/config` — `CUEBREAKER_*` env config.
- `internal/cue` — encoding-detecting CUE reader; parser (album meta + tracks, `INDEX 01`
  as `MM:SS:FF` + numeric `StartSeconds`); UTF-8 temp-copy; FLAC/WAV duration.
  `CheckSourceFLAC` returns a distinct error per rejection reason (unreadable, zero/multiple
  `FILE` refs, non-FLAC/WAV, missing source) — compare with `errors.Is` against the exported
  sentinels rather than matching on message text.
- `internal/joblog` — bounded, monotonically-sequenced log ring (`Buffer`, cap 500), shared by
  scan and split. `Since(seq)` survives ring overflow without skipping or replaying a line; a nil
  `*Buffer` is a safe no-op. Imports nothing project-local.
- `internal/scan` — walk `INPUT_DIR` for single-file FLAC+CUE pairs; already-split status; cover
  art. `FindPairs` returns a `Result{Pairs, Log, Summary}`: one log line per rejected directory
  (only when it held a `.cue`), plus a walk summary. `GET /api/scan` mirrors this as an object
  (`{items, log, summary}`), not a bare array — `GET /api/search` stays an array. The rejection
  line's level comes from `rejectLevel`: `info` for expected steady state
  (`ErrMultiFileReference`, `ErrNotFLACOrWAV`), `warn` for anything suggesting the directory or
  CUE is actually broken — extend it when adding a `cue` sentinel. An unreadable directory
  returns `fs.SkipDir` so `WalkDir` does not re-report it and double-count `Summary.Skipped`.
- `internal/split` — `cuebreakpoints` → `shnsplit` (stderr → progress) → tagging (`cueprint` +
  `metaflac`) → pregap removal → cover copy. A `reporter` (built from `Options.Progress` +
  `Options.Log`) is threaded through `runShnsplit`/`finishSplit` and emits synthesized pipeline
  events (parse, source, breakpoints, per-track, tag, cover, done). A failure logs *nothing*
  here: it is returned as an error carrying the tool's own diagnostics (trimmed by
  `toolDiagnostic` to the last few non-blank lines, since `joblog` strips newlines and an
  uncapped join renders as one megaline), and `internal/job` logs it once. Emitting it at both
  layers would double every tool failure in the job log.
- `internal/job` — serialized worker (one split at a time) + in-memory job registry. Each `State`
  carries a `Log *joblog.Buffer`, fresh per `Enqueue` (a re-split starts an empty log); a rejected
  enqueue restores the prior state, log included. `run` writes an `error` entry for *any*
  `splitFn` failure — the SPA auto-expands this log precisely on `status=error`, so a failure
  `split.Run` cannot attribute to a tool (cue parse, missing source, unwritable output dir,
  cancellation) must still leave a line explaining why.
- `internal/server` — `net/http` mux (Go 1.22+ patterns), JSON handlers, realpath containment,
  `embed.FS` SPA serving with fallback, `slog`. Routes are declared in one `apiRoutes()` table
  that `routes()` iterates — add a route there, not with a stray `mux.Handle`.
- `internal/server/openapi` — hand-written `openapi.yaml` + a vendored Scalar bundle, both
  `//go:embed`ed; served at `GET /api/openapi.yaml` and `GET /api/docs`.
- `web/` — `//go:embed all:dist`. `web/dist/` holds only a placeholder here; the real SPA is
  baked in at image-build time by the workspace repo.

## Commands

```bash
make run          # go run ./cmd/cuebreaker  (API on :5000)
make build        # single binary, SPA embedded  (APP_VERSION=x.y.z)
make test         # go test ./...
make test-race    # go test -race ./...
make lint         # golangci-lint run
make fmt          # golangci-lint fmt
make vet          # go vet ./...
```

Also exposed in the DWE workspace as `dwe cmd backend.{run,test,lint}`.

## Conventions

- Table-driven tests next to the code (`*_test.go`), `t.TempDir()` for filesystem work.
  Tests that shell out to real tools (`shnsplit`/`metaflac`) write fake tools onto `PATH` or
  parse captured sample output — `go test ./...` runs without the real tools installed.
- Lint + format via **golangci-lint** (v2; config in `.golangci.yml`): the standard set plus
  bodyclose/errorlint/misspell/unconvert/revive; `golangci-lint fmt` owns formatting. Keep
  `golangci-lint run` clean before committing.
- The OpenAPI spec is hand-written, not generated. `openapi_test.go` asserts in both directions
  that every path in `apiRoutes()` is documented and every documented path is registered, so a new
  or renamed route fails the suite until the spec matches. Update `openapi.yaml` in the same commit
  as the route; the response shapes must mirror the Go JSON tags exactly.
- Keep the module building/testing green (`go build ./...` + `go test ./...`) before moving on.
- External tools: `shnsplit` (shntool), `cuebreakpoints`/`cueprint` (cuetools), `metaflac` (flac).
- Env vars: `CUEBREAKER_INPUT_DIR` (`/input`), `CUEBREAKER_OUTPUT_DIR` (`/output`),
  `CUEBREAKER_PORT` (`5000`). Version via `-ldflags "-X main.version=$APP_VERSION"`, at `GET /api/version`.
- `GET /api/version` reports `server.BuildInfo`: the app's own version plus `shntool_version`,
  probed once at startup by `split.ShntoolVersion` (`shntool -v`) and omitted when unknown — a
  missing tool is an absent version, never a startup error.
- `GET /api/status/{job_id}` accepts `?log_since=N` (missing/unparseable treated as `0`) and
  returns `log`/`log_next` alongside the existing fields, for incremental log polling.

> `CLAUDE.md` is a symlink to this file. Edit `AGENTS.md`.
