const WIDTH_KEY = 'cuebreaker.sidebar.width'

export const MIN_SIDEBAR_WIDTH = 220
export const MAX_SIDEBAR_WIDTH = 480
export const DEFAULT_SIDEBAR_WIDTH = 300

/** Clamps a sidebar width to the allowed drag range. */
export function clampWidth(width: number, min = MIN_SIDEBAR_WIDTH, max = MAX_SIDEBAR_WIDTH): number {
  return Math.min(max, Math.max(min, width))
}

/** Loads the persisted sidebar width, clamped; falls back on any failure or missing value. */
export function loadWidth(key = WIDTH_KEY, fallback = DEFAULT_SIDEBAR_WIDTH): number {
  try {
    const raw = window.localStorage.getItem(key)
    if (!raw) return fallback
    const parsed = Number(raw)
    if (!Number.isFinite(parsed)) return fallback
    return clampWidth(parsed)
  } catch {
    return fallback
  }
}

/** Persists the sidebar width. Silently no-ops if storage is unavailable. */
export function saveWidth(width: number, key = WIDTH_KEY): void {
  try {
    window.localStorage.setItem(key, String(width))
  } catch {
    // localStorage unavailable (private mode, quota) — width just won't persist
  }
}
