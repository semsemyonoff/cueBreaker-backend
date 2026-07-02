import type { KeyboardEvent } from 'react'
import { useState } from 'react'
import type { ScanPair } from '../api/types'
import { loadOpenPaths, toggleOpenPath, type AlbumNode, type FolderNode, type OpenPaths, type TreeNode } from '../tree/buildTree'

export interface TreeProps {
  nodes: TreeNode[]
  selectedPath: string | null
  onSelect: (item: ScanPair) => void
}

function activateOnKey(handler: () => void) {
  return (event: KeyboardEvent) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      handler()
    }
  }
}

export default function Tree({ nodes, selectedPath, onSelect }: TreeProps) {
  const [openPaths, setOpenPaths] = useState<OpenPaths>(() => loadOpenPaths())

  function toggle(path: string) {
    setOpenPaths(toggleOpenPath(openPaths, path))
  }

  return (
    <div className="tree">
      <TreeNodes nodes={nodes} openPaths={openPaths} onToggle={toggle} selectedPath={selectedPath} onSelect={onSelect} />
    </div>
  )
}

interface TreeNodesProps {
  nodes: TreeNode[]
  openPaths: OpenPaths
  onToggle: (path: string) => void
  selectedPath: string | null
  onSelect: (item: ScanPair) => void
}

function TreeNodes({ nodes, openPaths, onToggle, selectedPath, onSelect }: TreeNodesProps) {
  return (
    <>
      {nodes.map((node) =>
        node.type === 'folder' ? (
          <FolderRow
            key={node.path}
            node={node}
            isOpen={openPaths.has(node.path)}
            openPaths={openPaths}
            onToggle={onToggle}
            selectedPath={selectedPath}
            onSelect={onSelect}
          />
        ) : (
          <AlbumRow key={node.path} node={node} isSelected={node.path === selectedPath} onSelect={onSelect} />
        ),
      )}
    </>
  )
}

function FolderRow({
  node,
  isOpen,
  openPaths,
  onToggle,
  selectedPath,
  onSelect,
}: {
  node: FolderNode
  isOpen: boolean
  openPaths: OpenPaths
  onToggle: (path: string) => void
  selectedPath: string | null
  onSelect: (item: ScanPair) => void
}) {
  const toggleThis = () => onToggle(node.path)

  return (
    <>
      <div className="tfolder" role="button" tabIndex={0} onClick={toggleThis} onKeyDown={activateOnKey(toggleThis)}>
        <span className={isOpen ? 'tarrow open' : 'tarrow'}>▶</span>
        <span className="tfname">{node.name}</span>
        <span className="tcount">{node.count}</span>
      </div>
      {isOpen && (
        <div className="tkids">
          <TreeNodes nodes={node.children} openPaths={openPaths} onToggle={onToggle} selectedPath={selectedPath} onSelect={onSelect} />
        </div>
      )}
    </>
  )
}

function AlbumRow({ node, isSelected, onSelect }: { node: AlbumNode; isSelected: boolean; onSelect: (item: ScanPair) => void }) {
  const { item } = node
  const selectThis = () => onSelect(item)
  const classes = ['talbum']
  if (isSelected) classes.push('active')
  if (item.split_done) classes.push('done')

  return (
    <div className={classes.join(' ')} role="button" tabIndex={0} onClick={selectThis} onKeyDown={activateOnKey(selectThis)}>
      <span className="sig">
        <i />
        <i />
        <i />
        <i />
        <i />
      </span>
      <span className="taname">{node.name}</span>
      {item.split_done ? (
        <span className="tcheck">✓</span>
      ) : (
        <span className="tameta">
          {item.cue_files.length} cue{item.cue_files.length === 1 ? '' : 's'}
        </span>
      )}
    </div>
  )
}
