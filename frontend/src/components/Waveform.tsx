import { useMemo } from 'react'
import type { Track } from '../api/types'
import { barHeights, cutPositions, fillState } from '../waveform/geometry'
import '../styles/waveform.css'

export type WaveformVariant = 'idle' | 'active' | 'done'

export interface WaveformProps {
  variant: WaveformVariant
  tracks: Track[]
  totalSeconds: number
  /** Split progress percent (0-100), ignored for `idle`/`done` (treated as 0/100). */
  progress?: number
  /** Track number to highlight (hover-linked with the track table row), if any. */
  hoveredTrack?: number | null
  onHoverTrack?: (trackNumber: number | null) => void
}

export default function Waveform({ variant, tracks, totalSeconds, progress = 0, hoveredTrack = null, onHoverTrack }: WaveformProps) {
  const heights = useMemo(() => barHeights(), [])
  const cuts = useMemo(() => cutPositions(tracks, totalSeconds), [tracks, totalSeconds])
  const fill = useMemo(() => fillState(variant === 'done' ? 100 : progress), [variant, progress])

  return (
    <div className={`wave wave--${variant}`}>
      <div className="wave-layer wave-base">
        {heights.map((h, i) => (
          <span key={i} className={`wbar h${h}`} />
        ))}
      </div>
      <div className="wave-layer wave-fill" style={{ clipPath: fill.clipPath }}>
        {heights.map((h, i) => (
          <span key={i} className={`wbar h${h}`} />
        ))}
      </div>
      {cuts.map((cut) => (
        <div
          key={cut.trackNumber}
          className={cut.trackNumber === hoveredTrack ? 'cut cut-hover' : 'cut'}
          style={{ left: `${cut.leftPercent}%` }}
          onMouseEnter={() => onHoverTrack?.(cut.trackNumber)}
          onMouseLeave={() => onHoverTrack?.(null)}
        >
          <span className="cut-num">{String(cut.trackNumber).padStart(2, '0')}</span>
        </div>
      ))}
      <div className="wave-head" style={{ left: fill.playheadLeft }} />
    </div>
  )
}
