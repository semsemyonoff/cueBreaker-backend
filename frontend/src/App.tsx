import { useCallback, useEffect, useState } from 'react'
import * as api from './api/client'
import type { ScanPair } from './api/types'
import AlbumPanel from './components/AlbumPanel'
import Shell from './components/Shell'
import './styles/shell.css'

export default function App() {
  const [items, setItems] = useState<ScanPair[]>([])
  const [selected, setSelected] = useState<ScanPair | null>(null)
  const [version, setVersion] = useState('')

  const rescan = useCallback(() => {
    api
      .scan()
      .then(setItems)
      .catch(() => setItems([]))
  }, [])

  useEffect(() => {
    rescan()
    api
      .version()
      .then((v) => setVersion(v.version))
      .catch(() => setVersion(''))
  }, [rescan])

  return (
    <Shell items={items} selectedPath={selected?.path ?? null} onSelect={setSelected} onRescan={rescan} version={version}>
      {selected ? <AlbumPanel item={selected} /> : <p>Select an album from the library.</p>}
    </Shell>
  )
}
