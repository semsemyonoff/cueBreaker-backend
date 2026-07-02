import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import App from './App'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('App', () => {
  it('renders the app shell and the empty-scan state when no albums are found', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url === '/api/version') return Promise.resolve(jsonResponse({ version: '1.0.0' }))
        return Promise.resolve(jsonResponse([]))
      }),
    )

    render(<App />)

    expect(await screen.findByLabelText('Toggle library')).toBeInTheDocument()
    expect(await screen.findByText('No unsplit CUE + FLAC albums found')).toBeInTheDocument()
  })

  it('prompts to select an album once albums are scanned', async () => {
    const item = {
      path: 'Artist/Album',
      abs_path: '/input/Artist/Album',
      cue_files: ['album.cue'],
      flac_files: ['album.flac'],
      split_done: false,
      output_tracks: 0,
    }
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url === '/api/version') return Promise.resolve(jsonResponse({ version: '1.0.0' }))
        if (url === '/api/scan') return Promise.resolve(jsonResponse([item]))
        return Promise.resolve(jsonResponse([]))
      }),
    )

    render(<App />)

    expect(await screen.findByText('Select an album from the library.')).toBeInTheDocument()
  })
})
