import { useEffect, useState } from 'react'
import * as api from '../api/client'
import type { Preview, ScanPair } from '../api/types'
import CueSelector from './CueSelector'
import TrackTable from './TrackTable'
import Waveform from './Waveform'
import '../styles/album.css'

export interface AlbumPanelProps {
  item: ScanPair
}

interface Breadcrumb {
  parents: string[]
  leaf: string
}

function breadcrumb(path: string): Breadcrumb {
  const parts = path.split('/').filter(Boolean)
  return { parents: parts.slice(0, -1), leaf: parts[parts.length - 1] ?? path }
}

export default function AlbumPanel({ item }: AlbumPanelProps) {
  const [cueFile, setCueFile] = useState(item.cue_files[0] ?? '')
  const [preview, setPreview] = useState<Preview | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [hoveredTrack, setHoveredTrack] = useState<number | null>(null)

  useEffect(() => {
    setCueFile(item.cue_files[0] ?? '')
  }, [item.path, item.cue_files])

  useEffect(() => {
    if (!cueFile) return
    let cancelled = false
    setPreview(null)
    setError(null)
    api
      .preview(item.path, cueFile)
      .then((result) => {
        if (!cancelled) setPreview(result)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err))
      })
    return () => {
      cancelled = true
    }
  }, [item.path, cueFile])

  const { parents, leaf } = breadcrumb(item.path)
  const crumbs = (
    <div className="crumbs">
      {parents.length > 0 ? `${parents.join(' / ')} / ` : null}
      <b>{leaf}</b>
    </div>
  )

  if (error) {
    return (
      <div className="album">
        {crumbs}
        <p className="album-error">{error}</p>
      </div>
    )
  }

  if (!preview) {
    return (
      <div className="album">
        {crumbs}
        <p className="album-loading">Loading…</p>
      </div>
    )
  }

  const splitDone = preview.split_done

  return (
    <div className="album">
      {crumbs}
      <div className="ahead">
        {preview.has_cover ? (
          <img className="cover" src={api.coverUrl(item.path)} alt="" />
        ) : (
          <div className="cover ph">
            <span className="phw">no cover</span>
          </div>
        )}
        <div className="ameta">
          <div className="atitle">{preview.title}</div>
          <div className="aartist">{preview.performer}</div>
          <div className="chips">
            {preview.date && <span className="chip">{preview.date}</span>}
            {preview.genre && <span className="chip">{preview.genre}</span>}
            <span className="chip">
              <b>{preview.tracks.length}</b> tracks
            </span>
          </div>
          <div className="metarow">
            <span className={splitDone ? 'pill pill-done' : 'pill pill-idle'}>
              <span className="pdot" />
              {splitDone ? 'Split' : 'Unsplit'}
            </span>
            <CueSelector cueFiles={item.cue_files} value={cueFile} onChange={setCueFile} />
          </div>
        </div>
      </div>
      <div className="wavewrap">
        <div className="wavecap">
          <span>{splitDone ? 'COMPLETE' : 'WAVEFORM'} · {preview.tracks.length} cue breakpoints</span>
          <span className="cuename">{cueFile}</span>
        </div>
        <Waveform
          variant={splitDone ? 'done' : 'idle'}
          tracks={preview.tracks}
          totalSeconds={preview.total_seconds}
          hoveredTrack={hoveredTrack}
          onHoverTrack={setHoveredTrack}
        />
      </div>
      <TrackTable tracks={preview.tracks} hoveredTrack={hoveredTrack} onHoverTrack={setHoveredTrack} />
    </div>
  )
}
