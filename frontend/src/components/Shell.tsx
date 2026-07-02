import { useEffect, useRef, useState, type CSSProperties, type PointerEvent as ReactPointerEvent, type ReactNode } from 'react'
import type { ScanPair } from '../api/types'
import { clampWidth, loadWidth, saveWidth } from '../ui/resizer'
import Sidebar from './Sidebar'
import Topbar from './Topbar'

export interface ShellProps {
  items: ScanPair[]
  selectedPath: string | null
  onSelect: (item: ScanPair) => void
  onRescan: () => void
  version: string
  splittingCount?: number
  children?: ReactNode
}

export default function Shell({ items, selectedPath, onSelect, onRescan, version, splittingCount = 0, children }: ShellProps) {
  const [sidebarWidth, setSidebarWidth] = useState(() => loadWidth())
  const [drawerOpen, setDrawerOpen] = useState(false)
  // Holds the pointerup/pointermove teardown for the drag currently in
  // progress (if any), so unmounting mid-drag removes the exact listeners
  // that were added rather than a stale closure from an earlier render.
  const dragCleanup = useRef<(() => void) | null>(null)

  const albumCount = items.length
  const unsplitCount = items.filter((item) => !item.split_done).length

  function closeDrawer() {
    setDrawerOpen(false)
  }

  function selectAndClose(item: ScanPair) {
    onSelect(item)
    closeDrawer()
  }

  function startResize(event: ReactPointerEvent<HTMLDivElement>) {
    event.preventDefault()
    const startX = event.clientX
    const startWidth = sidebarWidth

    function handlePointerMove(moveEvent: PointerEvent) {
      setSidebarWidth(clampWidth(startWidth + (moveEvent.clientX - startX)))
    }

    function stopResize() {
      window.removeEventListener('pointermove', handlePointerMove)
      window.removeEventListener('pointerup', stopResize)
      dragCleanup.current = null
    }

    window.addEventListener('pointermove', handlePointerMove)
    window.addEventListener('pointerup', stopResize)
    dragCleanup.current = stopResize
  }

  useEffect(() => {
    saveWidth(sidebarWidth)
  }, [sidebarWidth])

  useEffect(() => {
    return () => dragCleanup.current?.()
  }, [])

  return (
    <div className="shell">
      <Topbar
        version={version}
        albumCount={albumCount}
        unsplitCount={unsplitCount}
        splittingCount={splittingCount}
        onBurgerClick={() => setDrawerOpen((open) => !open)}
      />
      <div className="dbody">
        <div
          className={drawerOpen ? 'sidewrap drawer-open' : 'sidewrap'}
          style={{ '--sidebar-w': `${sidebarWidth}px` } as CSSProperties}
        >
          <Sidebar items={items} selectedPath={selectedPath} onSelect={selectAndClose} onRescan={onRescan} />
          <div className="resizer" role="separator" aria-orientation="vertical" onPointerDown={startResize} />
        </div>
        {drawerOpen && <div className="scrim" onClick={closeDrawer} />}
        <div className="work">{children}</div>
      </div>
    </div>
  )
}
