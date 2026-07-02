export interface TopbarProps {
  version: string
  albumCount: number
  unsplitCount: number
  splittingCount: number
  onBurgerClick: () => void
}

export default function Topbar({ version, albumCount, unsplitCount, splittingCount, onBurgerClick }: TopbarProps) {
  const running = splittingCount > 0

  return (
    <div className="dtop">
      <button className="burger" type="button" aria-label="Toggle library" onClick={onBurgerClick}>
        <i />
        <i />
        <i />
      </button>
      <div className="brand">
        <img className="blogo" src="/logo.svg" alt="cueBreaker logo" />
        <div className="word">
          cue<span>Breaker</span>
        </div>
        <span className="ver">v{version}</span>
      </div>
      <div className="tstat">
        <span className={running ? 'dot run' : 'dot'} />
        {running ? (
          <span>
            <b>{splittingCount}</b> splitting
          </span>
        ) : (
          <span>
            <b>{albumCount}</b> albums
          </span>
        )}
        <span>
          <b>{unsplitCount}</b> unsplit
        </span>
      </div>
    </div>
  )
}
