import { beforeEach, describe, expect, it } from 'vitest'
import { clampWidth, DEFAULT_SIDEBAR_WIDTH, loadWidth, MAX_SIDEBAR_WIDTH, MIN_SIDEBAR_WIDTH, saveWidth } from './resizer'

describe('clampWidth', () => {
  it('passes values already within range through unchanged', () => {
    expect(clampWidth(320)).toBe(320)
  })

  it('clamps values below the minimum', () => {
    expect(clampWidth(10)).toBe(MIN_SIDEBAR_WIDTH)
  })

  it('clamps values above the maximum', () => {
    expect(clampWidth(9999)).toBe(MAX_SIDEBAR_WIDTH)
  })

  it('honors custom bounds', () => {
    expect(clampWidth(50, 100, 200)).toBe(100)
    expect(clampWidth(500, 100, 200)).toBe(200)
  })
})

describe('loadWidth / saveWidth', () => {
  const key = 'test.sidebar.width'

  beforeEach(() => {
    window.localStorage.clear()
  })

  it('returns the fallback when nothing is persisted', () => {
    expect(loadWidth(key)).toBe(DEFAULT_SIDEBAR_WIDTH)
  })

  it('round-trips a saved width', () => {
    saveWidth(340, key)
    expect(loadWidth(key)).toBe(340)
  })

  it('clamps a persisted out-of-range width on load', () => {
    window.localStorage.setItem(key, '5000')
    expect(loadWidth(key)).toBe(MAX_SIDEBAR_WIDTH)
  })

  it('falls back on garbage persisted content', () => {
    window.localStorage.setItem(key, 'not-a-number')
    expect(loadWidth(key)).toBe(DEFAULT_SIDEBAR_WIDTH)
  })

  it('uses a custom fallback', () => {
    expect(loadWidth(key, 260)).toBe(260)
  })
})
