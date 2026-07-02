# cueBreaker Redesign Prototype — "Waveform & Cuts"

Approved design prototype for the Go rewrite + redesign. Open `cueBreaker-prototype.html`
directly in a browser (self-contained; fonts load from Google Fonts, logo from `./logo.svg`).

- **Live/editable source (Claude Design):** https://claude.ai/design/p/a54c515e-bcdb-4539-8d94-8512059d8277
- `cueBreaker-prototype.html` — decoded static copy of all screens (waveform DC component
  expanded to plain markup so it renders without the Claude Design runtime).
- `logo.svg` — current brand mark (copy of `static/logo.svg`).

## Concept
Metaphor: an audio editor. The app splits one long FLAC into tracks at CUE breakpoints, so the
hero element is a **waveform with vertical cut-lines** at the real track boundaries (positions
derived from CUE `INDEX` timings). Two-pane shell: library tree (left) + album work panel (right).
Desktop-first, with a dedicated responsive mobile mode (sidebar becomes a drawer).

## Screens in the prototype
1. **Desktop — album unsplit** (hero): shell, tree, album header, idle waveform, track table, Split button.
2. **Desktop — splitting**: waveform fills cyan to progress% + playhead; status row `Tagging 07 — … · 07/12 · 62%`.
3. **Work panel — done**: waveform in green, Split badge, result files list, "Split again".
4. **Mobile — detail**: hamburger + drawer sidebar, sticky bottom Split button.
5. **Micro-states**: empty scan · scanning · no cover · split error · multiple CUE files · overwrite warning.

## Design tokens
Dark theme only (v1).

| Token | Value | Use |
|-------|-------|-----|
| `--bg` | `#0f111b` | app background |
| `--bg2` | `#161a28` | top bar, chips |
| `--panel` | `#181d2e` | waveform panel |
| `--sidebar` | `#12141f` | left tree |
| `--border` / `--border2` | `#242a3d` / `#2f3654` | dividers / inputs |
| `--text` / `--dim` / `--dim2` | `#e8ecf6` / `#8b93ad` / `#646d88` | text scale |
| `--cyan` / `--cyan2` / `--blue` | `#1BA4F0` / `#91F2FF` / `#226EC7` | accent gradient (from logo) |
| `--green` / `--red` / `--amber` | `#2fbf83` / `#f36a6f` / `#e2a13a` | done / error / warning |

- **Fonts:** Manrope (UI, 400–800) + JetBrains Mono (paths, timings `MM:SS:FF`, counters).
- **Waveform:** synthetic bar amplitude; **cut-line positions are real** (from CUE `INDEX`), no audio decode.
  States: `idle` (calm steel-blue), `active` (cyan fill + playhead), `done` (green fill).
- **Motion:** waveform fill on split, cyan glow on active segment; all gated by `prefers-reduced-motion`.

> Sample album metadata ("Blue Meridian" / "Halogen Quartet") is placeholder content for layout only.
