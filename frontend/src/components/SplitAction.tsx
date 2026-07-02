import type { JobStatus } from '../api/types'

export interface SplitActionProps {
  trackCount: number
  splitDone: boolean
  outputTracks: number
  job: JobStatus | null
  error: string | null
  onSplit: () => void
}

const ACTIVE_STATUSES = new Set(['queued', 'splitting', 'tagging'])

/** Split button + results/error/overwrite-warning states below the track table. */
export default function SplitAction({ trackCount, splitDone, outputTracks, job, error, onSplit }: SplitActionProps) {
  const active = job !== null && ACTIVE_STATUSES.has(job.status)
  const done = job?.status === 'done'
  const failMessage = job?.status === 'error' ? job.message : error
  const showOverwriteWarning = splitDone && job === null && !failMessage

  if (done) {
    return (
      <>
        <div className="results">
          <div className="rhead">
            <CheckIcon /> Split completed successfully
          </div>
          <div className="rlist">
            {job.result_files.map((file) => (
              <div className="rl" key={file}>
                {file}
              </div>
            ))}
          </div>
        </div>
        <div className="actionbar">
          <button className="btn-ghost" type="button" onClick={onSplit}>
            Split again
          </button>
        </div>
      </>
    )
  }

  return (
    <>
      {showOverwriteWarning && (
        <div className="errbox errbox-warn">
          <div className="eh">
            <WarnIcon />
            Output already exists
          </div>
          <div className="em">
            {outputTracks} tracks in output — splitting again overwrites them
          </div>
        </div>
      )}
      {failMessage && (
        <div className="errbox">
          <div className="eh">
            <ErrIcon />
            Split failed
          </div>
          <div className="em">{failMessage}</div>
        </div>
      )}
      <div className="actionbar">
        <button className={active ? 'btn-split dis' : 'btn-split'} type="button" disabled={active} onClick={onSplit}>
          {active && <span className="spin" />}
          {active
            ? 'Processing…'
            : failMessage
              ? 'Retry'
              : showOverwriteWarning
                ? `Split again (${outputTracks} in output)`
                : `Split ${trackCount} tracks`}
        </button>
        <span className="actionhint">{active ? 'splitting & tagging · do not close' : `writes ${trackCount} tracks to output`}</span>
      </div>
    </>
  )
}

function CheckIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="#2fbf83" strokeWidth="2.5">
      <path d="M5 12l5 5L20 6" />
    </svg>
  )
}

function WarnIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#e2a13a" strokeWidth="2.4">
      <path d="M12 9v4M12 17h.01M10.3 4.3 2.5 18a2 2 0 0 0 1.7 3h15.6a2 2 0 0 0 1.7-3L13.7 4.3a2 2 0 0 0-3.4 0z" />
    </svg>
  )
}

function ErrIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#f36a6f" strokeWidth="2.4">
      <path d="M12 8v5M12 17h.01" />
      <circle cx="12" cy="12" r="9" />
    </svg>
  )
}
