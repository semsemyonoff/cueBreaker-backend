import type { Track } from '../api/types'

export interface TrackTableProps {
  tracks: Track[]
  hoveredTrack: number | null
  onHoverTrack: (trackNumber: number | null) => void
}

export default function TrackTable({ tracks, hoveredTrack, onHoverTrack }: TrackTableProps) {
  return (
    <table className="ttable">
      <thead>
        <tr>
          <th>#</th>
          <th>Title</th>
          <th>Artist</th>
          <th style={{ textAlign: 'right' }}>Start</th>
        </tr>
      </thead>
      <tbody>
        {tracks.map((track) => (
          <tr
            key={track.number}
            className={track.number === hoveredTrack ? 'trow-hover' : undefined}
            onMouseEnter={() => onHoverTrack(track.number)}
            onMouseLeave={() => onHoverTrack(null)}
          >
            <td className="tn">{String(track.number).padStart(2, '0')}</td>
            <td className="tt">{track.title}</td>
            <td className="ta">{track.performer}</td>
            <td className="ti">{track.index}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
