import type { Track } from '../api/types'

/** Number of synthetic bars drawn across the waveform (matches the prototype). */
export const BAR_COUNT = 68

const MIN_BAR_HEIGHT = 1
const MAX_BAR_HEIGHT = 10

/**
 * Deterministic synthetic bar heights (1-10, mapped to CSS classes `h1`..`h10`).
 * There's no real decoded audio in Phase 1 — the waveform is decorative — but the
 * heights must be stable across renders/tests, so this uses a fixed-seed LCG
 * instead of `Math.random()`.
 */
export function barHeights(count: number = BAR_COUNT): number[] {
  const heights: number[] = []
  let seed = 0
  for (let i = 0; i < count; i++) {
    seed = (seed * 9301 + 49297) % 233280
    const rand = seed / 233280
    heights.push(MIN_BAR_HEIGHT + Math.floor(rand * (MAX_BAR_HEIGHT - MIN_BAR_HEIGHT + 1)))
  }
  return heights
}

export interface CutPosition {
  trackNumber: number
  leftPercent: number
}

function clampPercent(value: number): number {
  return Math.min(100, Math.max(0, value))
}

/**
 * Cut-line positions (`left%`) for tracks with a real `start_seconds > 0`.
 * Track 1 always starts at 0 and is intentionally skipped (the prototype
 * labels cuts from `02`); an unreadable `totalSeconds` (`<= 0`) yields no
 * cut-lines at all rather than bunching everything at the left edge.
 */
export function cutPositions(tracks: Track[], totalSeconds: number): CutPosition[] {
  if (totalSeconds <= 0) return []

  const positions: CutPosition[] = []
  for (const track of tracks) {
    if (track.start_seconds <= 0) continue
    positions.push({
      trackNumber: track.number,
      leftPercent: clampPercent((track.start_seconds / totalSeconds) * 100),
    })
  }
  return positions
}

export interface FillState {
  /** Progress clamped to 0-100. */
  fillPercent: number
  /** `clip-path` for the `.wave-fill` layer. */
  clipPath: string
  /** CSS `left` for the playhead. */
  playheadLeft: string
}

/** Derives the fill clip-path + playhead position for a given progress percent. */
export function fillState(progress: number): FillState {
  const fillPercent = clampPercent(progress)
  return {
    fillPercent,
    clipPath: `inset(0 ${100 - fillPercent}% 0 0)`,
    playheadLeft: `${fillPercent}%`,
  }
}
