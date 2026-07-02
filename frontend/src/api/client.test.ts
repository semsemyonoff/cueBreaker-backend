import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, coverUrl, preview, scan, search, split, status, version } from './client'

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
}

function mockFetch(response: Response): ReturnType<typeof vi.fn> {
  const fn = vi.fn().mockResolvedValue(response)
  vi.stubGlobal('fetch', fn)
  return fn
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('scan', () => {
  it('GETs /api/scan and returns the parsed pairs', async () => {
    const pairs = [
      {
        path: 'Artist/Album',
        abs_path: '/input/Artist/Album',
        cue_files: ['album.cue'],
        flac_files: ['album.flac'],
        split_done: false,
        output_tracks: 0,
      },
    ]
    const fetchMock = mockFetch(jsonResponse(pairs))

    const result = await scan()

    expect(fetchMock).toHaveBeenCalledWith('/api/scan', undefined)
    expect(result).toEqual(pairs)
  })

  it('throws ApiError with the server message on failure', async () => {
    mockFetch(jsonResponse({ error: 'boom' }, { status: 500, statusText: 'Internal Server Error' }))

    await expect(scan()).rejects.toMatchObject(new ApiError(500, 'boom'))
  })

  it('falls back to statusText when the error body is not JSON', async () => {
    mockFetch(new Response('not json', { status: 502, statusText: 'Bad Gateway' }))

    await expect(scan()).rejects.toMatchObject(new ApiError(502, 'Bad Gateway'))
  })
})

describe('search', () => {
  it('URL-encodes the query string', async () => {
    const fetchMock = mockFetch(jsonResponse([]))

    await search("O'Brien & Sons")

    expect(fetchMock).toHaveBeenCalledWith("/api/search?q=O'Brien%20%26%20Sons", undefined)
  })
})

describe('preview', () => {
  it('POSTs path and cue_file as JSON', async () => {
    const body = {
      performer: 'Artist',
      title: 'Album',
      file: 'album.flac',
      genre: '',
      date: '',
      tracks: [{ number: 1, title: 'T1', performer: '', index: '00:00:00', start_seconds: 0 }],
      has_cover: true,
      cover_name: 'cover.jpg',
      split_done: false,
      output_tracks: 0,
      total_seconds: 3684,
    }
    const fetchMock = mockFetch(jsonResponse(body))

    const result = await preview("Artist's Album", 'album.cue')

    expect(fetchMock).toHaveBeenCalledWith('/api/preview', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: "Artist's Album", cue_file: 'album.cue' }),
    })
    expect(result).toEqual(body)
  })
})

describe('split', () => {
  it('POSTs and returns the accepted job', async () => {
    const fetchMock = mockFetch(jsonResponse({ job_id: 'Artist/Album/album.cue', status: 'queued' }, { status: 202 }))

    const result = await split('Artist/Album', 'album.cue')

    expect(fetchMock).toHaveBeenCalledWith('/api/split', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: 'Artist/Album', cue_file: 'album.cue' }),
    })
    expect(result).toEqual({ job_id: 'Artist/Album/album.cue', status: 'queued' })
  })

  it('throws ApiError on a 409 conflict', async () => {
    mockFetch(
      jsonResponse(
        { error: 'Already in progress', job_id: 'Artist/Album/album.cue' },
        { status: 409, statusText: 'Conflict' },
      ),
    )

    await expect(split('Artist/Album', 'album.cue')).rejects.toMatchObject(new ApiError(409, 'Already in progress'))
  })
})

describe('status', () => {
  it('GETs the job status by id, encoding unsafe characters', async () => {
    const jobStatus = {
      status: 'splitting',
      message: '',
      result_files: [],
      progress_current: 1,
      progress_total: 4,
      progress_detail: 'track 1',
    }
    const fetchMock = mockFetch(jsonResponse(jobStatus))

    const result = await status("Artist's Album/album.cue")

    expect(fetchMock).toHaveBeenCalledWith("/api/status/Artist's%20Album/album.cue", undefined)
    expect(result).toEqual(jobStatus)
  })
})

describe('version', () => {
  it('GETs /api/version', async () => {
    const fetchMock = mockFetch(jsonResponse({ version: '1.2.3' }))

    const result = await version()

    expect(fetchMock).toHaveBeenCalledWith('/api/version', undefined)
    expect(result).toEqual({ version: '1.2.3' })
  })
})

describe('coverUrl', () => {
  it('builds a plain path unchanged', () => {
    expect(coverUrl('Artist/Album')).toBe('/api/cover/Artist/Album')
  })

  it('encodes apostrophes and other unsafe characters per segment', () => {
    expect(coverUrl("Artist's Band/Album #1")).toBe("/api/cover/Artist's%20Band/Album%20%231")
  })

  it('keeps slashes as directory separators', () => {
    expect(coverUrl('A/B/C')).toBe('/api/cover/A/B/C')
  })
})
