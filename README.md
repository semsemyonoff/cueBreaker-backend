# cueBreaker

Lightweight web application for splitting single-file FLAC albums using CUE sheets. Built on Alpine Linux with Python, shntool, and cuetools.

## Features

- Scans a directory tree for unsplit FLAC+CUE pairs (skips already-split per-track CUEs)
- Auto-detects CUE file encoding (UTF-8, CP1251, Shift-JIS, and others)
- Hierarchical tree view of discovered albums with search
- Album detail page with cover art preview, track listing, and tag info
- Splits FLAC and writes Vorbis tags (artist, title, album, genre, date, track number) via `cueprint` + `metaflac`
- Copies cover art (cover/folder/front images) to the output directory
- Progress bar with per-track status updates
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
| `TZ`                     | `UTC`     | Timezone                              |

## How It Works

1. **Scan** — recursively walks the input directory looking for `.cue` files that reference a single existing `.flac` file (whole album). Directories with per-track CUE sheets are ignored.
2. **Preview** — parses the CUE file, extracts album metadata and track listing, detects encoding automatically.
3. **Split** — converts the CUE to UTF-8, then runs `shnsplit` to split the FLAC into individual tracks using breakpoints from `cuebreakpoints`.
4. **Tag** — extracts tags with `cueprint` and writes them to each track via `metaflac` (TITLE, ARTIST, ALBUM, ALBUMARTIST, GENRE, DATE, TRACKNUMBER, TRACKTOTAL).
5. **Finalize** — removes pregap (track 00), copies cover art, reports results.

Output files are placed in `OUTPUT_DIR/<same-relative-path-as-source>/`.

## Volumes

- **Input** (`/input`) — mount read-only. The source FLAC and CUE files are never modified.
- **Output** (`/output`) — mount read-write. Split tracks and cover art are written here.

## Tech Stack

- **Python 3.14** / Flask / Gunicorn
- **shntool** 3.0.10
- **cuetools** (cuebreakpoints, cueprint)
- **FLAC** (metaflac)
- **Alpine Linux** base image
