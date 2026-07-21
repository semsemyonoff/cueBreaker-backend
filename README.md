# cueBreaker — backend

The Go backend for **cueBreaker**, a "Waveform & Cuts" tool for splitting single-file
FLAC albums using their CUE sheets. A single static binary that serves a JSON API under
`/api/*` and the embedded SPA on `/`.

This repo is **the backend only**. The SPA lives in its own repository and is built into
`web/dist/` (embedded via `//go:embed`) when the combined production image is built; the
release image and the `docker-compose.yml` you self-host with live in the cueBreaker
deployment repository.

## What it does

Given a library of albums ripped as one FLAC per disc plus a CUE sheet, the backend:

- **scans** the input directory for CUE sheets that reference an existing single source
  file, skipping directories that are already split;
- **parses** the CUE (in any of the encodings these files show up in) into album metadata
  and per-track `INDEX 01` positions;
- **splits** the source with `shnsplit`, tags each resulting track from the CUE, removes
  the pregap, and copies the album cover across;
- **reports** progress and a bounded log while a split runs — one split at a time.

## Layout

```
cmd/cuebreaker/    entrypoint: load config, build queue + HTTP server, serve SPA
internal/
  config/          env-based config (CUEBREAKER_*)
  cue/             encoding-detecting CUE reader/parser; FLAC/WAV duration
  joblog/          bounded, monotonically-sequenced log ring shared by scan + split
  scan/            walk INPUT_DIR for unsplit FLAC+CUE pairs; cover discovery
  split/           orchestrate cuebreakpoints → shnsplit → tagging → cover copy
  job/             serialized split worker + in-memory job registry
  server/          net/http mux, JSON handlers, path containment, SPA fallback
    openapi/       embedded OpenAPI spec + vendored Scalar bundle (/api/docs)
web/               //go:embed of the built SPA (web/dist placeholder in this repo)
testdata/          sample CUE files (encodings) + a tiny FLAC
```

## Development

Requires Go 1.26+.

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

### Running with the UI

`web/dist/` holds only a placeholder in this repo. For UI work, run the frontend repo's
Vite dev server — it proxies `/api` to this backend on `:5000`. To serve the real SPA from
the binary, build the frontend (`npm run build`) and copy its `dist/` into `web/dist/`
before `make build`; that is exactly what the production image does.

## API documentation

The server documents itself. Two routes, both served from the binary with no network
access required:

| Route                   | Serves                                                       |
|-------------------------|--------------------------------------------------------------|
| `GET /api/docs`         | The [Scalar](https://scalar.com) API reference, rendered from the spec below |
| `GET /api/openapi.yaml` | The OpenAPI 3.1 spec itself (`application/yaml`)             |

Both are `//go:embed`ed from `internal/server/openapi/`, including the Scalar bundle — it is
vendored rather than loaded from a CDN so the reference renders on an isolated network.

The spec is **hand-written**, so `openapi_test.go` guards it against drift: it reads the same
`apiRoutes()` table the mux is built from and asserts in both directions that every registered
path is documented and every documented path is registered. Adding or renaming a route fails
`go test ./...` until `openapi.yaml` is updated to match — do both in one commit, and keep the
response schemas mirroring the Go JSON tags.

### Process logs

`GET /api/scan` returns an **object** — `{items, log, summary}` — not a bare array.
`GET /api/search` returns an array. `GET /api/status/{job_id}` carries `log` and `log_next`,
plus an optional `?log_since=N` cursor for incremental polling: pass back the previous
response's `log_next` to receive only the entries added since. A missing, negative or
unparseable `log_since` is treated as `0` and returns the whole retained buffer.

Both logs are bounded rings of 500 entries; a scan or split that emits more silently drops its
oldest lines, so the log is a tail, not a transcript.

## Configuration

| Environment variable    | Default   | Description                          |
|-------------------------|-----------|--------------------------------------|
| `CUEBREAKER_INPUT_DIR`  | `/input`  | Directory to scan for FLAC+CUE pairs |
| `CUEBREAKER_OUTPUT_DIR` | `/output` | Directory for split results          |
| `CUEBREAKER_PORT`       | `5000`    | HTTP port the server listens on      |

The build version is injected via `-ldflags "-X main.version=$APP_VERSION"` (default
`dev`) and surfaced at `GET /api/version`, alongside the detected `shntool` version.

## License

MIT — see [LICENSE](LICENSE).
