import { beforeEach, describe, expect, it } from 'vitest'
import type { ScanPair } from '../api/types'
import {
  buildTree,
  filterTree,
  loadOpenPaths,
  saveOpenPaths,
  toggleOpenPath,
  type AlbumNode,
  type FolderNode,
  type TreeNode,
} from './buildTree'

function pair(path: string, overrides: Partial<ScanPair> = {}): ScanPair {
  return {
    path,
    abs_path: `/input/${path}`,
    cue_files: ['album.cue'],
    flac_files: ['album.flac'],
    split_done: false,
    output_tracks: 0,
    ...overrides,
  }
}

function folder(node: TreeNode | undefined): FolderNode {
  expect(node?.type).toBe('folder')
  return node as FolderNode
}

function album(node: TreeNode | undefined): AlbumNode {
  expect(node?.type).toBe('album')
  return node as AlbumNode
}

describe('buildTree', () => {
  it('nests flat rel-paths into folders and album leaves', () => {
    const items = [
      pair('Lossless/Halogen Quartet/Blue Meridian'),
      pair('Lossless/Halogen Quartet/Nightfall Sessions'),
      pair('Lossless/Marlow Trio/Some Album'),
      pair('Compilations/Comp One'),
    ]

    const tree = buildTree(items)

    expect(tree.map((n) => n.name)).toEqual(['Compilations', 'Lossless'])

    const compilations = folder(tree.find((n) => n.name === 'Compilations'))
    expect(compilations.children.map((n) => n.name)).toEqual(['Comp One'])
    expect(album(compilations.children[0]).path).toBe('Compilations/Comp One')

    const lossless = folder(tree.find((n) => n.name === 'Lossless'))
    expect(lossless.children.map((n) => n.name)).toEqual(['Halogen Quartet', 'Marlow Trio'])

    const halogen = folder(lossless.children.find((n) => n.name === 'Halogen Quartet'))
    expect(halogen.children.map((n) => n.name)).toEqual(['Blue Meridian', 'Nightfall Sessions'])
    expect(album(halogen.children[0]).item.path).toBe('Lossless/Halogen Quartet/Blue Meridian')
  })

  it('computes per-node leaf counts recursively', () => {
    const items = [
      pair('Lossless/Halogen Quartet/Blue Meridian'),
      pair('Lossless/Halogen Quartet/Nightfall Sessions'),
      pair('Lossless/Marlow Trio/Some Album'),
      pair('Compilations/Comp One'),
    ]

    const tree = buildTree(items)

    const lossless = folder(tree.find((n) => n.name === 'Lossless'))
    expect(lossless.count).toBe(3)
    const halogen = folder(lossless.children.find((n) => n.name === 'Halogen Quartet'))
    expect(halogen.count).toBe(2)
    const compilations = folder(tree.find((n) => n.name === 'Compilations'))
    expect(compilations.count).toBe(1)
  })

  it('nests an album that shares its path with a folder as "[this folder]"', () => {
    const items = [pair('Lossless'), pair('Lossless/Deep Cut')]

    const tree = buildTree(items)

    expect(tree).toHaveLength(1)
    const lossless = folder(tree[0])
    expect(lossless.name).toBe('Lossless')
    expect(lossless.count).toBe(2)
    expect(lossless.children.map((n) => n.name)).toEqual(['[this folder]', 'Deep Cut'])
    expect(album(lossless.children[0]).path).toBe('Lossless')
    expect(album(lossless.children[0]).item.path).toBe('Lossless')
  })

  it('places single-segment paths as top-level albums when there is no conflicting folder', () => {
    const items = [pair('Standalone Album')]

    const tree = buildTree(items)

    expect(tree).toHaveLength(1)
    expect(album(tree[0]).name).toBe('Standalone Album')
  })

  it('returns an empty tree for no items', () => {
    expect(buildTree([])).toEqual([])
  })
})

describe('filterTree', () => {
  const items = [
    pair('Lossless/Halogen Quartet/Blue Meridian'),
    pair('Lossless/Halogen Quartet/Nightfall Sessions'),
    pair('Lossless/Marlow Trio/Some Album'),
    pair('Compilations/Comp One'),
  ]
  const tree = buildTree(items)

  it('returns the tree unchanged for an empty query', () => {
    expect(filterTree(tree, '')).toBe(tree)
    expect(filterTree(tree, '   ')).toBe(tree)
  })

  it('keeps only matching album leaves, pruning empty folders', () => {
    const filtered = filterTree(tree, 'blue')

    expect(filtered.map((n) => n.name)).toEqual(['Lossless'])
    const lossless = folder(filtered[0])
    expect(lossless.children.map((n) => n.name)).toEqual(['Halogen Quartet'])
    const halogen = folder(lossless.children[0])
    expect(halogen.children.map((n) => n.name)).toEqual(['Blue Meridian'])
    expect(lossless.count).toBe(1)
  })

  it('keeps an entire folder subtree when the folder name itself matches', () => {
    const filtered = filterTree(tree, 'marlow trio')

    const lossless = folder(filtered.find((n) => n.name === 'Lossless'))
    const marlow = folder(lossless.children.find((n) => n.name === 'Marlow Trio'))
    expect(marlow.children.map((n) => n.name)).toEqual(['Some Album'])
  })

  it('returns no matches for an unmatched query', () => {
    expect(filterTree(tree, 'nonexistent')).toEqual([])
  })
})

describe('open-state persistence', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('loads an empty set when nothing is persisted', () => {
    expect(loadOpenPaths()).toEqual(new Set())
  })

  it('round-trips saved paths', () => {
    saveOpenPaths(new Set(['Lossless', 'Lossless/Halogen Quartet']))

    expect(loadOpenPaths()).toEqual(new Set(['Lossless', 'Lossless/Halogen Quartet']))
  })

  it('ignores corrupt persisted data', () => {
    window.localStorage.setItem('cuebreaker.tree.open', 'not json')

    expect(loadOpenPaths()).toEqual(new Set())
  })

  it('toggleOpenPath adds then removes a path, persisting each time', () => {
    let paths = toggleOpenPath(new Set(), 'Lossless')
    expect(paths.has('Lossless')).toBe(true)
    expect(loadOpenPaths()).toEqual(new Set(['Lossless']))

    paths = toggleOpenPath(paths, 'Lossless')
    expect(paths.has('Lossless')).toBe(false)
    expect(loadOpenPaths()).toEqual(new Set())
  })
})
