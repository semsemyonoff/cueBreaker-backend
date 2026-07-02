// Domain types mirroring the Go backend's JSON wire shapes
// (backend/internal/scan, backend/internal/cue, backend/internal/job,
// backend/internal/server/handlers.go).

/** One scanned album directory, as returned by GET /api/scan and /api/search. */
export interface ScanPair {
  path: string
  abs_path: string
  cue_files: string[]
  flac_files: string[]
  split_done: boolean
  output_tracks: number
}

/** One CUE track entry, with both the raw INDEX string and its numeric offset. */
export interface Track {
  number: number
  title: string
  performer: string
  index: string
  start_seconds: number
}

/** POST /api/preview response: parsed CUE album plus cover/status/duration data. */
export interface Preview {
  performer: string
  title: string
  file: string
  genre: string
  date: string
  tracks: Track[]
  has_cover: boolean
  cover_name?: string
  split_done: boolean
  output_tracks: number
  total_seconds: number
}

export type JobStatusValue = 'queued' | 'splitting' | 'tagging' | 'done' | 'error'

/** GET /api/status/{job_id} response. */
export interface JobStatus {
  status: JobStatusValue
  message: string
  result_files: string[]
  progress_current: number
  progress_total: number
  progress_detail: string
}

/** POST /api/split success response (202 Accepted). */
export interface SplitAccepted {
  job_id: string
  status: string
}

/** GET /api/version response. */
export interface Version {
  version: string
}
