# cueBreaker

FLAC+CUE album splitter. Go backend + React/Vite/TypeScript SPA, shipped as a single static
binary with the SPA embedded via `//go:embed`.

## Layout (monorepo)

- `backend/` — Go module (`cmd/cuebreaker` entrypoint; `internal/config`, `internal/cue`,
  `internal/scan`, `internal/split`, `internal/job`, `internal/server`). `backend/web/dist/` holds
  the built SPA assets copied in by `make frontend-build` (embedded by the Go build, gitignored
  except `.gitkeep`).
- `frontend/` — React + Vite + TypeScript SPA (`src/api`, `src/tree`, `src/waveform`,
  `src/split`, `src/components`, `src/ui`, `src/styles`). Vitest for non-visual logic.
- `docs/prototype/` — approved design source of truth ("Waveform & Cuts"); see
  `docs/prototype/README.md` for tokens/screens and the live Claude Design link.
- `docs/plans/` — implementation plans; completed ones move to `docs/plans/completed/`.

This is a **monorepo by design, structured for a future split** into separate `backend`,
`frontend`, and `deploy` repos (submodules + multi-stage Dockerfile, mirroring the
`AlbFetcharr`/`beetDeck` pattern). That split is out of scope until explicitly planned.

## Tool dependencies

The backend shells out to (via `os/exec`, must be on `PATH` for local dev, installed in the
Docker image):

- **shntool** 3.0.10 (`shnsplit`)
- **cuetools** (`cuebreakpoints`, `cueprint`)
- **FLAC** (`metaflac`)

## Make targets

- `make build` — `frontend-build` then `go build` → `backend/cuebreaker` (single binary, SPA embedded).
  Version injected via `-ldflags "-X main.version=$(APP_VERSION)"` (default `dev`), surfaced at `GET /api/version`
- `make frontend-build` — `npm ci` + `npm run build` in `frontend/`, copies `dist/*` into `backend/web/dist/`
- `make dev` — Vite dev server (proxies `/api`) + `go run` for the backend, concurrently
- `make test` — `go test ./...` + `npm run test`
- `make lint` — `go vet ./...` + `npm run lint`
- `make docker-build` — multi-arch `docker buildx` build/push to `$(CUEBREAKER_IMAGE):$(CUEBREAKER_TAG)`
  for `$(CUEBREAKER_PLATFORMS)`

## Conventions

- Keep both modules building/testing green before moving to the next task: `go build`/`go test`
  for `backend/`, `npm run build`/`npm run test` for `frontend/`.
- Backend tests are table-driven, next to the code (`*_test.go`), using `t.TempDir()` for
  filesystem work. Tests that shell out to real tools parse captured sample output and gate real
  invocations behind `testing.Short()`/tool-presence checks.
- Frontend: Vitest for tree building, waveform geometry, the API client, and split-status
  polling. Visual/CSS work is verified by `npm run build` + manual QA against the prototype.
- Env vars: `CUEBREAKER_INPUT_DIR` (default `/input`), `CUEBREAKER_OUTPUT_DIR` (default
  `/output`), `CUEBREAKER_PORT` (default `5000`).
