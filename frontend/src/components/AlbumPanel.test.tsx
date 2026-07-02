import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { ScanPair } from '../api/types'
import AlbumPanel from './AlbumPanel'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

function stubPreview(byCue: Record<string, unknown>) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      if (url === '/api/preview' && init?.body) {
        const { cue_file: cueFile } = JSON.parse(String(init.body)) as { cue_file: string }
        return Promise.resolve(jsonResponse(byCue[cueFile]))
      }
      return Promise.resolve(jsonResponse({}))
    }),
  )
}

const item: ScanPair = {
  path: 'Artist/Album',
  abs_path: '/input/Artist/Album',
  cue_files: ['album.cue'],
  flac_files: ['album.flac'],
  split_done: false,
  output_tracks: 0,
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AlbumPanel', () => {
  it('renders title, chips, and the unsplit pill from preview data', async () => {
    stubPreview({
      'album.cue': {
        performer: 'Artist',
        title: 'Album',
        file: 'album.flac',
        genre: 'Rock',
        date: '1999',
        has_cover: false,
        split_done: false,
        output_tracks: 0,
        total_seconds: 120,
        tracks: [
          { number: 1, title: 'One', performer: 'Artist', index: '00:00:00', start_seconds: 0 },
          { number: 2, title: 'Two', performer: 'Artist', index: '02:00:00', start_seconds: 120 },
        ],
      },
    })

    render(<AlbumPanel item={item} />)

    expect(await screen.findByText('Album')).toBeInTheDocument()
    expect(screen.getAllByText('Artist').length).toBeGreaterThan(0)
    expect(screen.getByText('Rock')).toBeInTheDocument()
    expect(screen.getByText('1999')).toBeInTheDocument()
    expect(screen.getByText('Unsplit')).toBeInTheDocument()
    expect(screen.getByText('no cover')).toBeInTheDocument()
  })

  it('shows the done pill and cover image when split_done and has_cover are true', async () => {
    stubPreview({
      'album.cue': {
        performer: 'Artist',
        title: 'Album',
        file: 'album.flac',
        genre: '',
        date: '',
        has_cover: true,
        cover_name: 'cover.jpg',
        split_done: true,
        output_tracks: 1,
        total_seconds: 60,
        tracks: [{ number: 1, title: 'One', performer: 'Artist', index: '00:00:00', start_seconds: 0 }],
      },
    })

    const { container } = render(<AlbumPanel item={item} />)

    expect(await screen.findByText('Split')).toBeInTheDocument()
    expect(container.querySelector('img.cover')).not.toBeNull()
  })

  it('shows a CUE selector only when the album has more than one CUE file, and re-previews on change', async () => {
    stubPreview({
      'a.cue': {
        performer: 'Artist',
        title: 'A',
        file: 'a.flac',
        genre: '',
        date: '',
        has_cover: false,
        split_done: false,
        output_tracks: 0,
        total_seconds: 60,
        tracks: [{ number: 1, title: 'One', performer: 'Artist', index: '00:00:00', start_seconds: 0 }],
      },
      'b.cue': {
        performer: 'Artist',
        title: 'B',
        file: 'b.flac',
        genre: '',
        date: '',
        has_cover: false,
        split_done: false,
        output_tracks: 0,
        total_seconds: 60,
        tracks: [{ number: 1, title: 'Uno', performer: 'Artist', index: '00:00:00', start_seconds: 0 }],
      },
    })

    const multiCue: ScanPair = { ...item, cue_files: ['a.cue', 'b.cue'] }
    render(<AlbumPanel item={multiCue} />)

    const select = (await screen.findByLabelText('CUE file')) as HTMLSelectElement
    expect(select.querySelectorAll('option')).toHaveLength(2)

    expect(await screen.findByText('A')).toBeInTheDocument()

    select.value = 'b.cue'
    select.dispatchEvent(new Event('change', { bubbles: true }))

    expect(await screen.findByText('B')).toBeInTheDocument()
  })

  it('renders no CUE selector for a single-CUE album', async () => {
    stubPreview({
      'album.cue': {
        performer: 'Artist',
        title: 'Album',
        file: 'album.flac',
        genre: '',
        date: '',
        has_cover: false,
        split_done: false,
        output_tracks: 0,
        total_seconds: 60,
        tracks: [{ number: 1, title: 'One', performer: 'Artist', index: '00:00:00', start_seconds: 0 }],
      },
    })

    render(<AlbumPanel item={item} />)

    await screen.findByText('Album')
    expect(screen.queryByLabelText('CUE file')).toBeNull()
  })
})
