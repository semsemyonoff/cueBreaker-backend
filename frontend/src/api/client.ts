import type { JobStatus, Preview, ScanPair, SplitAccepted, Version } from './types'

/** Thrown when the API responds with a non-2xx status. */
export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    const message = await extractErrorMessage(res)
    throw new ApiError(res.status, message)
  }
  return (await res.json()) as T
}

async function extractErrorMessage(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string }
    if (body?.error) return body.error
  } catch {
    // body wasn't JSON; fall through to the status text
  }
  return res.statusText || `request failed with status ${res.status}`
}

function postJSON<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export function scan(): Promise<ScanPair[]> {
  return request<ScanPair[]>('/api/scan')
}

export function search(q: string): Promise<ScanPair[]> {
  return request<ScanPair[]>(`/api/search?q=${encodeURIComponent(q)}`)
}

export function preview(path: string, cueFile: string): Promise<Preview> {
  return postJSON<Preview>('/api/preview', { path, cue_file: cueFile })
}

export function split(path: string, cueFile: string): Promise<SplitAccepted> {
  return postJSON<SplitAccepted>('/api/split', { path, cue_file: cueFile })
}

export function status(jobId: string): Promise<JobStatus> {
  return request<JobStatus>(`/api/status/${encodeJobPath(jobId)}`)
}

export function version(): Promise<Version> {
  return request<Version>('/api/version')
}

/**
 * Cover art URL for a scan-relative directory path. Encodes each path
 * segment individually so slashes stay as directory separators while
 * apostrophes and other JS/URL-unsafe characters within a segment are
 * escaped, matching the backend's {path...} wildcard route.
 */
export function coverUrl(path: string): string {
  return `/api/cover/${encodeJobPath(path)}`
}

function encodeJobPath(path: string): string {
  return path.split('/').map(encodeURIComponent).join('/')
}
