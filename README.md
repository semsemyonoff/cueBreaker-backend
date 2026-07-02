# cueBreaker

A "Waveform & Cuts" web app for splitting single-file FLAC albums using CUE sheets. Single Go
binary with an embedded React SPA, built on Alpine Linux with shntool and cuetools.

## Features

- Scans a directory tree for unsplit FLAC+CUE pairs (skips already-split per-track CUEs)
- Auto-detects CUE file encoding (UTF-8, CP1251, Shift-JIS, and others)
- Hierarchical tree view of discovered albums with search
- Album view with a waveform whose cut-lines sit at the real track boundaries, cover art
  preview, track listing, and tag info
- Splits FLAC and writes Vorbis tags (artist, title, album, genre, date, track number) via
  `cueprint` + `metaflac`
- Copies cover art (cover/folder/front images) to the output directory
- Live split progress (waveform fill + per-track status), one split at a time
- Marks already-split albums with a checkmark

## Quick Start

### Docker Compose

```yaml
services:
  cuebreaker:
    image: semsemyonoff/cuebreaker
    container_name: cuebreaker
    user: "1000:1000"
    environment:
      - TZ=Europe/Moscow
      - CUEBREAKER_INPUT_DIR=/input
      - CUEBREAKER_OUTPUT_DIR=/output
    volumes:
      - /path/to/your/downloads:/input:ro
      - /path/to/split/output:/output
    ports:
      - "5000:5000"
    restart: unless-stopped
```

```bash
docker compose up -d
```

Open `http://localhost:5000` in your browser.

### Docker Run

```bash
docker run -d \
  --name cuebreaker \
  -p 5000:5000 \
  -e TZ=Europe/Moscow \
  -v /path/to/your/downloads:/input:ro \
  -v /path/to/split/output:/output \
  cuebreaker
```

## Configuration

| Environment Variable     | Default   | Description                          |
|--------------------------|-----------|--------------------------------------|
| `CUEBREAKER_INPUT_DIR`   | `/input`  | Directory to scan for FLAC+CUE pairs |
| `CUEBREAKER_OUTPUT_DIR`  | `/output` | Directory for split results           |
| `CUEBREAKER_PORT`        | `5000`    | HTTP port the server listens on       |
| `TZ`                     | `UTC`     | Timezone                              |

## How It Works

1. **Scan** — recursively walks the input directory looking for `.cue` files that reference a single existing `.flac`/`.wav` file (whole album). Directories with per-track CUE sheets are ignored.
2. **Preview** — parses the CUE file, extracts album metadata, track listing, and total duration; detects encoding automatically.
3. **Split** — converts the CUE to UTF-8, then runs `shnsplit` to split the FLAC into individual tracks using breakpoints from `cuebreakpoints`.
4. **Tag** — extracts tags with `cueprint` and writes them to each track via `metaflac` (TITLE, ARTIST, ALBUM, ALBUMARTIST, GENRE, DATE, TRACKNUMBER, TRACKTOTAL).
5. **Finalize** — removes pregap (track 00), copies cover art, reports results.

Output files are placed in `OUTPUT_DIR/<same-relative-path-as-source>/`. Splits are serialized —
one job runs at a time; further requests for the same album are rejected while it's in progress.

## Volumes

- **Input** (`/input`) — mount read-only. The source FLAC and CUE files are never modified.
- **Output** (`/output`) — mount read-write. Split tracks and cover art are written here.

## Development

This is a monorepo: a Go backend under `backend/` and a React + Vite + TypeScript SPA under
`frontend/`. The backend embeds the built SPA via `//go:embed` and serves it from `/`, with the
API under `/api/*`.

```bash
make build   # npm ci + build the SPA into backend/web/dist, then go build the single binary
make dev     # run the Vite dev server (proxies /api) alongside `go run` for the backend
make test    # go test ./... + npm run test
make lint    # go vet ./... + npm run lint
```

`make build` produces `backend/cuebreaker`, a single binary serving the full app on
`CUEBREAKER_PORT` (default `5000`).

### Tool dependencies

The backend orchestrates these external tools via `os/exec` (installed in the Docker image,
required on your `PATH` for local `make dev`/`make build`):

- **shntool** 3.0.10 (`shnsplit`)
- **cuetools** (`cuebreakpoints`, `cueprint`)
- **FLAC** (`metaflac`)

### Docker image

`docker build .` runs a multi-stage build: Node builds the SPA, Go builds the backend with the
SPA embedded, and the runtime stage is Alpine with `shntool`/`cuetools`/`flac` installed —
mirroring the tool stack above. `./build.sh` (used by `make docker-build`) builds and pushes a
multi-arch image via `docker buildx`.

### Future: repo split

The `backend/`/`frontend/` split in this repo is deliberately structured so that a later
extraction into separate `backend`, `frontend`, and `deploy` repos (submodules + multi-stage
Dockerfile, mirroring the `AlbFetcharr`/`beetDeck` pattern) is a move, not a refactor. That split
is explicitly out of scope for now.

## Tech Stack

- **Go** backend, single static binary
- **React** + **Vite** + **TypeScript** SPA
- **shntool** 3.0.10
- **cuetools** (cuebreakpoints, cueprint)
- **FLAC** (metaflac)
- **Alpine Linux** base image
