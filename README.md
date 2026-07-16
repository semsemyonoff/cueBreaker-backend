# cueBreaker — backend

The Go backend for [cueBreaker](https://git.horn/cueBreaker), a "Waveform & Cuts" tool
for splitting single-file FLAC albums using CUE sheets. A single static binary that
serves a JSON API under `/api/*` and the embedded SPA on `/`.

This repo is **the backend only**. The SPA lives in
[`cueBreaker/frontend`](https://git.horn/cueBreaker/frontend); it is built and copied into
`web/dist/` (embedded via `//go:embed`) when the combined production image is built. The
image, and the local dev environment, are assembled by the workspace repo
([`cueBreaker/workspace`](https://git.horn/cueBreaker/workspace)) — the same three-repo
pattern used across the sibling `beetDeck` / `AlbFetcharr` products.

## Layout

```
cmd/cuebreaker/    entrypoint: load config, build queue + HTTP server, serve SPA
internal/
  config/          env-based config (CUEBREAKER_*)
  cue/             encoding-detecting CUE reader/parser; FLAC/WAV duration
  scan/            walk INPUT_DIR for unsplit FLAC+CUE pairs; cover discovery
  split/           orchestrate cuebreakpoints → shnsplit → tagging → cover copy
  job/             serialized split worker + in-memory job registry
  server/          net/http mux, JSON handlers, path containment, SPA fallback
    openapi/       embedded OpenAPI spec + vendored Scalar bundle (/api/docs)
web/               //go:embed of the built SPA (web/dist placeholder in this repo)
testdata/          sample CUE files (encodings) + a tiny FLAC
```

## Development

```bash
make run          # go run ./cmd/cuebreaker  (API on :5000)
make build        # single binary with the SPA embedded (APP_VERSION=x.y.z)
make test         # go test ./...
make test-race    # go test -race ./...
make lint         # golangci-lint run
make fmt          # golangci-lint fmt  (gofmt + goimports)
make vet          # go vet ./...
```

### Tool dependencies

The backend shells out to these via `os/exec` (installed in the production image;
required on your `PATH` for a fully working local `make run`):

- **shntool** 3.0.10 (`shnsplit`)
- **cuetools** (`cuebreakpoints`, `cueprint`)
- **FLAC** (`metaflac`)

Tests that touch these tools parse captured sample output and gate real invocations
behind tool-presence checks, so `go test ./...` runs without them installed.

## API documentation

The server documents itself. Two routes, both served from the binary with no network access
required:

| Route               | Serves                                                       |
|---------------------|--------------------------------------------------------------|
| `GET /api/docs`     | The [Scalar](https://scalar.com) API reference, rendered from the spec below |
| `GET /api/openapi.yaml` | The OpenAPI 3.1 spec itself (`application/yaml`)          |

Both are `//go:embed`ed from `internal/server/openapi/`, including the Scalar bundle — it is
vendored rather than loaded from a CDN so the reference renders on an isolated network. In the
DWE dev stack the reference is at `http://localhost:5100/api/docs`.

The spec is **hand-written**, so `openapi_test.go` guards it against drift: it reads the same
`apiRoutes()` table the mux is built from and asserts in both directions that every registered
path is documented and every documented path is registered. Adding or renaming a route fails
`go test ./...` until `openapi.yaml` is updated to match — do both in one commit, and keep the
response schemas mirroring the Go JSON tags.

## Configuration

| Environment Variable    | Default   | Description                          |
|-------------------------|-----------|--------------------------------------|
| `CUEBREAKER_INPUT_DIR`  | `/input`  | Directory to scan for FLAC+CUE pairs |
| `CUEBREAKER_OUTPUT_DIR` | `/output` | Directory for split results          |
| `CUEBREAKER_PORT`       | `5000`    | HTTP port the server listens on      |

The build version is injected via `-ldflags "-X main.version=$APP_VERSION"` (default
`dev`) and surfaced at `GET /api/version` → `{"version": "..."}`.

## Full stack

To run the backend together with the Vite dev server (hot reload on both), use the
workspace repo:

```bash
git clone ssh://git@git.horn:2222/cueBreaker/workspace.git
cd workspace && dwe deploy run
```
