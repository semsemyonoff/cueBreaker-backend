import { useEffect, useReducer } from 'react'
import * as api from '../api/client'
import type { JobStatus } from '../api/types'

/** Poll state: the last-seen job status, plus a fetch-level error distinct from a job `status: 'error'`. */
export interface PollState {
  job: JobStatus | null
  fetchError: string | null
}

export const initialPollState: PollState = { job: null, fetchError: null }

export type PollAction = { type: 'reset' } | { type: 'status'; job: JobStatus } | { type: 'fetchError'; message: string }

export function pollReducer(state: PollState, action: PollAction): PollState {
  switch (action.type) {
    case 'reset':
      return initialPollState
    case 'status':
      return { job: action.job, fetchError: null }
    case 'fetchError':
      return { ...state, fetchError: action.message }
  }
}

/** True once polling has reached a state it will not advance from on its own. */
export function isTerminal(state: PollState): boolean {
  return state.fetchError !== null || state.job?.status === 'done' || state.job?.status === 'error'
}

/**
 * Polls `GET /api/status/{jobId}` every `intervalMs` while `jobId` is set,
 * stopping once the job reaches a terminal status (`done`/`error`) or a
 * fetch fails. Resets to `initialPollState` whenever `jobId` changes.
 */
export function usePoll(jobId: string | null, intervalMs = 500): PollState {
  const [state, dispatch] = useReducer(pollReducer, initialPollState)

  useEffect(() => {
    dispatch({ type: 'reset' })
    if (!jobId) return

    let cancelled = false

    async function tick() {
      try {
        const job = await api.status(jobId as string)
        if (cancelled) return
        dispatch({ type: 'status', job })
        if (job.status === 'done' || job.status === 'error') clearInterval(timer)
      } catch (err) {
        if (cancelled) return
        dispatch({ type: 'fetchError', message: err instanceof Error ? err.message : String(err) })
        clearInterval(timer)
      }
    }

    void tick()
    const timer = setInterval(tick, intervalMs)

    return () => {
      cancelled = true
      clearInterval(timer)
    }
  }, [jobId, intervalMs])

  return state
}
