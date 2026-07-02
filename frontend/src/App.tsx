import { useCallback, useEffect, useState } from 'react'
import * as api from './api/client'
import type { ScanPair } from './api/types'
import AlbumPanel from './components/AlbumPanel'
import Shell from './components/Shell'
import { EmptyScan, ScanningLibrary } from './components/States'
import './styles/shell.css'
import './styles/states.css'

export default function App() {
  const [items, setItems] = useState<ScanPair[]>([])
  const [selected, setSelected] = useState<ScanPair | null>(null)
  const [version, setVersion] = useState('')
  const [scanning, setScanning] = useState(true)

  const rescan = useCallback(() => {
    setScanning(true)
    api
      .scan()
      .then(setItems)
      .catch(() => setItems([]))
      .finally(() => setScanning(false))
  }, [])

  useEffect(() => {
    rescan()
    api
      .version()
      .then((v) => setVersion(v.version))
      .catch(() => setVersion(''))
  }, [rescan])

  let content
  if (scanning) {
    content = <ScanningLibrary />
  } else if (selected) {
    content = <AlbumPanel item={selected} />
  } else if (items.length === 0) {
    content = <EmptyScan onRescan={rescan} />
  } else {
    content = <p>Select an album from the library.</p>
  }

  return (
    <Shell items={items} selectedPath={selected?.path ?? null} onSelect={setSelected} onRescan={rescan} version={version}>
      {content}
    </Shell>
  )
}
