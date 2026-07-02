import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import type { JobStatus } from '../api/types'
import { initialPollState, isTerminal, pollReducer, usePoll, type PollState } from './usePoll'

function job(overrides: Partial<JobStatus>): JobStatus {
  return {
    status: 'queued',
    message: '',
    result_files: [],
    progress_current: 0,
    progress_total: 0,
    progress_detail: '',
    ...overrides,
  }
}

describe('pollReducer', () => {
  it('walks queued -> splitting -> tagging -> done, terminal only at done', () => {
    let state: PollState = initialPollState
    expect(isTerminal(state)).toBe(false)

    state = pollReducer(state, { type: 'status', job: job({ status: 'queued' }) })
    expect(state.job?.status).toBe('queued')
    expect(isTerminal(state)).toBe(false)

    state = pollReducer(state, { type: 'status', job: job({ status: 'splitting', progress_current: 1, progress_total: 4 }) })
    expect(state.job?.status).toBe('splitting')
    expect(isTerminal(state)).toBe(false)

    state = pollReducer(state, { type: 'status', job: job({ status: 'tagging', progress_current: 3, progress_total: 4 }) })
    expect(state.job?.status).toBe('tagging')
    expect(isTerminal(state)).toBe(false)

    state = pollReducer(state, {
      type: 'status',
      job: job({ status: 'done', progress_current: 4, progress_total: 4, result_files: ['01 - Track.flac'] }),
    })
    expect(state.job?.status).toBe('done')
    expect(isTerminal(state)).toBe(true)
  })

  it('reaches a terminal error status from job status: error', () => {
    let state: PollState = initialPollState
    state = pollReducer(state, { type: 'status', job: job({ status: 'splitting' }) })
    expect(isTerminal(state)).toBe(false)

    state = pollReducer(state, { type: 'status', job: job({ status: 'error', message: 'shnsplit failed' }) })
    expect(state.job?.status).toBe('error')
    expect(state.job?.message).toBe('shnsplit failed')
    expect(isTerminal(state)).toBe(true)
  })

  it('is terminal on a fetch-level error even without a job status', () => {
    let state: PollState = initialPollState
    state = pollReducer(state, { type: 'fetchError', message: 'network down' })
    expect(state.fetchError).toBe('network down')
    expect(isTerminal(state)).toBe(true)
  })

  it('clears a prior fetch error once a status arrives, and reset clears everything', () => {
    let state: PollState = initialPollState
    state = pollReducer(state, { type: 'fetchError', message: 'timeout' })
    expect(isTerminal(state)).toBe(true)

    state = pollReducer(state, { type: 'status', job: job({ status: 'splitting' }) })
    expect(state.fetchError).toBeNull()
    expect(isTerminal(state)).toBe(false)

    state = pollReducer(state, { type: 'reset' })
    expect(state).toEqual(initialPollState)
  })
})

function jobResponse(body: JobStatus): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

const idle = () => new Promise((resolve) => setTimeout(resolve, 40))

describe('usePoll hook', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('polls repeatedly while active and stops once the job is done', async () => {
    const statuses: JobStatus['status'][] = ['splitting', 'splitting', 'done']
    let i = 0
    const fetchMock = vi.fn(() => {
      const status = statuses[Math.min(i, statuses.length - 1)]
      i += 1
      return Promise.resolve(jobResponse(job({ status })))
    })
    vi.stubGlobal('fetch', fetchMock)

    const { result } = renderHook(() => usePoll('Artist/Album/album.cue', 0, 10))

    await waitFor(() => expect(result.current.job?.status).toBe('done'))
    const callsAtDone = fetchMock.mock.calls.length
    expect(callsAtDone).toBeGreaterThanOrEqual(3)

    await idle()
    expect(fetchMock.mock.calls.length).toBe(callsAtDone) // no polling after terminal
  })

  it('restarts polling when runToken changes even though jobId is unchanged', async () => {
    const fetchMock = vi.fn(() => Promise.resolve(jobResponse(job({ status: 'done' }))))
    vi.stubGlobal('fetch', fetchMock)

    const { result, rerender } = renderHook(({ jobId, runToken }) => usePoll(jobId, runToken, 10), {
      initialProps: { jobId: 'Artist/Album/album.cue', runToken: 0 },
    })

    await waitFor(() => expect(result.current.job?.status).toBe('done'))
    const callsAfterFirst = fetchMock.mock.calls.length

    // A split-again returns the same deterministic jobId; only runToken changes.
    rerender({ jobId: 'Artist/Album/album.cue', runToken: 1 })

    await waitFor(() => expect(fetchMock.mock.calls.length).toBeGreaterThan(callsAfterFirst))
    await waitFor(() => expect(result.current.job?.status).toBe('done'))
  })

  it('records a fetch-level error and stops polling', async () => {
    const fetchMock = vi.fn(() => Promise.resolve(new Response('boom', { status: 500 })))
    vi.stubGlobal('fetch', fetchMock)

    const { result } = renderHook(() => usePoll('Artist/Album/album.cue', 0, 10))

    await waitFor(() => expect(result.current.fetchError).not.toBeNull())
    const callsAtError = fetchMock.mock.calls.length

    await idle()
    expect(fetchMock.mock.calls.length).toBe(callsAtError) // no polling after fetch error
  })
})
