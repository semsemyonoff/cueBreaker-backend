import { describe, expect, it } from 'vitest'
import type { JobStatus } from '../api/types'
import { initialPollState, isTerminal, pollReducer, type PollState } from './usePoll'

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
