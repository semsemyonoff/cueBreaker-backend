import type { ScanPair } from '../api/types'

/** A folder grouping node in the library tree, keyed by its rel-path (e.g. "Lossless/Marlow Trio"). */
export interface FolderNode {
  type: 'folder'
  name: string
  path: string
  count: number
  children: TreeNode[]
}

/** A leaf album node wrapping the scan item it was built from. */
export interface AlbumNode {
  type: 'album'
  name: string
  path: string
  item: ScanPair
}

export type TreeNode = FolderNode | AlbumNode

function splitPath(path: string): string[] {
  return path.split('/').filter(Boolean)
}

/**
 * Builds a nested folder/album tree from flat scan-relative paths.
 *
 * An album can share its path with a folder (e.g. "Lossless" is itself an album
 * AND a directory containing further albums like "Lossless/Deep Cut") — that
 * album is nested as a synthetic "[this folder]" leaf inside the folder node
 * instead of colliding with it.
 */
export function buildTree(items: ScanPair[]): TreeNode[] {
  const sorted = [...items].sort((a, b) => a.path.localeCompare(b.path))

  const folderPaths = new Set<string>()
  for (const item of sorted) {
    const segments = splitPath(item.path)
    let prefix = ''
    for (let i = 0; i < segments.length - 1; i++) {
      prefix = prefix ? `${prefix}/${segments[i]}` : segments[i]
      folderPaths.add(prefix)
    }
  }

  const folderNodes = new Map<string, FolderNode>()
  const roots: TreeNode[] = []

  function getOrCreateFolder(path: string, name: string, siblings: TreeNode[]): FolderNode {
    const existing = folderNodes.get(path)
    if (existing) return existing
    const node: FolderNode = { type: 'folder', name, path, count: 0, children: [] }
    folderNodes.set(path, node)
    siblings.push(node)
    return node
  }

  for (const item of sorted) {
    const segments = splitPath(item.path)
    if (segments.length === 0) {
      roots.push({ type: 'album', name: item.path, path: item.path, item })
      continue
    }

    let siblings = roots
    let prefix = ''
    for (let i = 0; i < segments.length - 1; i++) {
      prefix = prefix ? `${prefix}/${segments[i]}` : segments[i]
      const folder = getOrCreateFolder(prefix, segments[i], siblings)
      siblings = folder.children
    }

    const lastSegment = segments[segments.length - 1]
    if (folderPaths.has(item.path)) {
      const folder = getOrCreateFolder(item.path, lastSegment, siblings)
      folder.children.push({ type: 'album', name: '[this folder]', path: item.path, item })
    } else {
      siblings.push({ type: 'album', name: lastSegment, path: item.path, item })
    }
  }

  applyCounts(roots)
  return roots
}

function applyCounts(nodes: TreeNode[]): number {
  let total = 0
  for (const node of nodes) {
    if (node.type === 'album') {
      total += 1
    } else {
      node.count = applyCounts(node.children)
      total += node.count
    }
  }
  return total
}

function countAlbums(nodes: TreeNode[]): number {
  let total = 0
  for (const node of nodes) {
    total += node.type === 'album' ? 1 : node.count
  }
  return total
}

function matchesQuery(node: TreeNode, query: string): boolean {
  return node.path.toLowerCase().includes(query) || node.name.toLowerCase().includes(query)
}

/**
 * Filters a tree to nodes matching `query` (case-insensitive, path or name
 * substring). A folder whose own name matches is kept whole (all descendants
 * shown); otherwise it's kept only if it has matching descendants, pruned to
 * just those. An empty/whitespace query returns `nodes` unchanged.
 */
export function filterTree(nodes: TreeNode[], query: string): TreeNode[] {
  const q = query.trim().toLowerCase()
  if (!q) return nodes

  const filtered: TreeNode[] = []
  for (const node of nodes) {
    if (node.type === 'album') {
      if (matchesQuery(node, q)) filtered.push(node)
      continue
    }

    if (node.name.toLowerCase().includes(q)) {
      filtered.push(node)
      continue
    }

    const children = filterTree(node.children, query)
    if (children.length > 0) {
      filtered.push({ ...node, children, count: countAlbums(children) })
    }
  }
  return filtered
}

const OPEN_STATE_KEY = 'cuebreaker.tree.open'

export type OpenPaths = Set<string>

/** Loads the persisted set of open folder paths; returns an empty set on any failure. */
export function loadOpenPaths(key = OPEN_STATE_KEY): OpenPaths {
  try {
    const raw = window.localStorage.getItem(key)
    if (!raw) return new Set()
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return new Set()
    return new Set(parsed.filter((v): v is string => typeof v === 'string'))
  } catch {
    return new Set()
  }
}

/** Persists the given set of open folder paths. Silently no-ops if storage is unavailable. */
export function saveOpenPaths(paths: OpenPaths, key = OPEN_STATE_KEY): void {
  try {
    window.localStorage.setItem(key, JSON.stringify([...paths]))
  } catch {
    // localStorage unavailable (private mode, quota) — open-state just won't persist
  }
}

/** Returns a new open-paths set with `path` toggled, and persists it. */
export function toggleOpenPath(paths: OpenPaths, path: string, key = OPEN_STATE_KEY): OpenPaths {
  const next = new Set(paths)
  if (next.has(path)) {
    next.delete(path)
  } else {
    next.add(path)
  }
  saveOpenPaths(next, key)
  return next
}
