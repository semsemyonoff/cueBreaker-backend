export interface CueSelectorProps {
  cueFiles: string[]
  value: string
  onChange: (cueFile: string) => void
}

/** CUE picker, shown only when an album has more than one CUE file. */
export default function CueSelector({ cueFiles, value, onChange }: CueSelectorProps) {
  if (cueFiles.length <= 1) return null

  return (
    <label className="cuesel">
      <span className="cl">Cue</span>
      <select className="cv" aria-label="CUE file" value={value} onChange={(event) => onChange(event.target.value)}>
        {cueFiles.map((cue) => (
          <option key={cue} value={cue}>
            {cue}
          </option>
        ))}
      </select>
    </label>
  )
}
