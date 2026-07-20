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
- `internal/scan` — walk `INPUT_DIR` for single-file FLAC+CUE pairs; already-split status; cover art.
- `internal/split` — `cuebreakpoints` → `shnsplit` (stderr → progress) → tagging (`cueprint` +
  `metaflac`) → pregap removal → cover copy.
- `internal/job` — serialized worker (one split at a time) + in-memory job registry.
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

> `CLAUDE.md` is a symlink to this file. Edit `AGENTS.md`.
