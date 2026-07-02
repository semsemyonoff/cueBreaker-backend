import { beforeEach, describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import Shell from './Shell'

beforeEach(() => {
  window.localStorage.clear()
})

describe('Shell mobile drawer', () => {
  it('opens the sidebar drawer on burger click and closes it on scrim click', () => {
    const { container } = render(
      <Shell items={[]} selectedPath={null} onSelect={() => {}} onRescan={() => {}} version="1.0.0">
        <p>work panel</p>
      </Shell>,
    )

    const sidewrap = container.querySelector('.sidewrap')
    expect(sidewrap?.className).not.toContain('drawer-open')
    expect(container.querySelector('.scrim')).toBeNull()

    fireEvent.click(screen.getByLabelText('Toggle library'))
    expect(sidewrap?.className).toContain('drawer-open')
    expect(container.querySelector('.scrim')).not.toBeNull()

    fireEvent.click(container.querySelector('.scrim')!)
    expect(sidewrap?.className).not.toContain('drawer-open')
    expect(container.querySelector('.scrim')).toBeNull()
  })

  it('closes the drawer when an album is selected', () => {
    const item = {
      path: 'Album',
      abs_path: '/input/Album',
      cue_files: ['album.cue'],
      flac_files: ['album.flac'],
      split_done: false,
      output_tracks: 0,
    }

    const { container } = render(
      <Shell items={[item]} selectedPath={null} onSelect={() => {}} onRescan={() => {}} version="1.0.0" />,
    )

    fireEvent.click(screen.getByLabelText('Toggle library'))
    expect(container.querySelector('.sidewrap')?.className).toContain('drawer-open')

    fireEvent.click(screen.getByText('Album'))
    expect(container.querySelector('.sidewrap')?.className).not.toContain('drawer-open')
  })
})
