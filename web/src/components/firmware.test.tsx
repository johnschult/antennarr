import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { FirmwareStatus } from '@/lib/types'
import { makeDevice, makeState, makeTwoDeviceState, updateAvailable, upToDate } from '@/test/fixtures'
import { renderWithApp, stubFetch } from '@/test/render'
import { CHANGELOG_URL, FirmwareBadge, FirmwareSection } from './firmware'

afterEach(() => vi.unstubAllGlobals())

const section = (name = 'Firmware') => screen.getByRole('region', { name })
const show = (status: FirmwareStatus, mutate: (s: ReturnType<typeof makeState>) => void = () => {}) => {
  const s = makeState({ devices: [makeDevice({ firmware: status })] })
  mutate(s)
  return renderWithApp(<FirmwareSection device={s.devices[0]} />, s)
}

describe('FirmwareBadge', () => {
  it('flags a tuner with newer firmware, linking to the details', () => {
    renderWithApp(<FirmwareBadge status={updateAvailable} />, makeState())
    const link = screen.getByRole('link')
    expect(link).toHaveAttribute('href', '/tuner')
    expect(link).toHaveTextContent('Firmware update')
  })
  it('is more cautious when it is only possible', () => {
    renderWithApp(<FirmwareBadge status={{ ...updateAvailable, state: 'possible-update' }} />, makeState())
    expect(screen.getByRole('link')).toHaveTextContent('Firmware: check')
  })
  it('says nothing otherwise', () => {
    for (const state of ['up-to-date', 'unknown', 'disabled'] as const) {
      const { unmount } = renderWithApp(<FirmwareBadge status={{ ...upToDate, state }} />, makeState())
      expect(screen.queryByRole('link')).not.toBeInTheDocument()
      unmount()
    }
  })
})

describe('FirmwareSection', () => {
  it('shows the version as a date, and that it is up to date', () => {
    show(upToDate)
    const sec = section()
    expect(within(sec).getByText('26 Mar 2026')).toBeInTheDocument()
    expect(within(sec).getByText('hdhomerun_dvr_atsc3 · 20260326')).toBeInTheDocument()
    expect(within(sec).getByText('Up to date')).toBeInTheDocument()
    expect(within(sec).queryByText(/What's new/)).not.toBeInTheDocument()
  })

  it("lists what's new when there is an update, naming the model each note is for", () => {
    show({ ...updateAvailable, current: '20250815' })
    const sec = section()
    expect(within(sec).getByText('Update available · 26 Mar 2026')).toBeInTheDocument()
    expect(within(sec).getByText("What's new since 15 Aug 2025")).toBeInTheDocument()
    expect(within(sec).getByText('ATSC3 models:')).toBeInTheDocument()
    expect(within(sec).getByText('Security update.')).toBeInTheDocument()
    expect(within(sec).getByText('All models:')).toBeInTheDocument()
    expect(within(sec).getByText('A line with no scope.')).toBeInTheDocument()
    expect(within(sec).getByText(/Install downloads the newest release from SiliconDust/)).toBeInTheDocument()
  })

  it('shows two releases and the rest on request', async () => {
    const many: FirmwareStatus = {
      ...updateAvailable,
      updates: ['20260326', '20260313', '20250815', '20250623'].map((version) => ({ version, certain: true, notes: [{ text: `change in ${version}` }] })),
    }
    const user = userEvent.setup()
    show(many)
    expect(screen.queryByText('change in 20250623')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Show all 4 releases' }))
    expect(screen.getByText('change in 20250623')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Show fewer' }))
    expect(screen.queryByText('change in 20250623')).not.toBeInTheDocument()
  })

  it('does not offer "show all" for two releases or fewer', () => {
    show(updateAvailable)
    expect(screen.queryByRole('button', { name: /Show all/ })).not.toBeInTheDocument()
  })

  it('is honest when a release may not apply', () => {
    show({
      ...updateAvailable, state: 'possible-update',
      updates: [{ version: '20260401', certain: false, notes: [{ scope: 'IPv6 models', text: 'a fix' }] }],
    })
    const sec = section()
    expect(within(sec).getByText('Newer release may apply')).toBeInTheDocument()
    expect(within(sec).getByText('may not apply to your model')).toBeInTheDocument()
  })

  it("says so when the model isn't recognised", () => {
    show({ ...updateAvailable, modelRecognised: false })
    expect(screen.getByText(/doesn't recognise this model/)).toBeInTheDocument()
  })

  it("mentions the tuner's own report", () => {
    show({ ...updateAvailable, deviceReported: '20270101', latest: '20270101' })
    expect(screen.getByText(/The tuner itself reports 1 Jan 2027 as available/)).toBeInTheDocument()
  })

  it('says when the changelog was last read, or that it has not been', () => {
    const { unmount } = show({ ...upToDate, checkedAt: '0001-01-01T00:00:00Z' })
    expect(screen.getByText(/Not read yet/)).toBeInTheDocument()
    unmount()
    show({ ...upToDate, checkedAt: new Date(Date.now() - 3 * 3600_000).toISOString() })
    expect(screen.getByText(/Last read 3h ago/)).toBeInTheDocument()
  })

  it('explains why it does not know, and when it is working from an old copy', () => {
    const { unmount } = show({ ...upToDate, state: 'unknown', checkedAt: '0001-01-01T00:00:00Z', error: 'the changelog page answered 503 Service Unavailable' })
    expect(screen.getByText('Unknown')).toBeInTheDocument()
    expect(screen.getByText(/The changelog page answered 503/)).toBeInTheDocument()
    unmount()
    show({ ...upToDate, error: "couldn't refresh the changelog: timeout" })
    expect(screen.getByText('Up to date')).toBeInTheDocument()
    expect(screen.getByText(/Couldn't refresh the changelog: timeout/)).toBeInTheDocument()
  })

  it('links to the changelog without leaking the page', () => {
    show(upToDate)
    const link = screen.getByRole('link', { name: /Changelog/ })
    expect(link).toHaveAttribute('href', CHANGELOG_URL)
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
    expect(CHANGELOG_URL.startsWith('https://www.silicondust.com/')).toBe(true)
  })

  describe('Check now', () => {
    const checkRoutes = (device: FirmwareStatus) => ({
      'POST /firmware/check': () => ({ body: { checkedAt: '2026-10-17T10:00:00Z' } }),
      'GET /state': () => ({ body: makeState({ devices: [makeDevice({ firmware: device })] }) }),
    })

    it('says there is no new firmware when the tuner is up to date', async () => {
      const { fn, calls } = stubFetch(checkRoutes(upToDate))
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      show(upToDate)
      await user.click(screen.getByRole('button', { name: 'Check now' }))
      expect(await screen.findByText('No new firmware')).toBeInTheDocument()
      expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/firmware/check'))).toBe(true)
      expect(screen.queryByText(/Read SiliconDust/)).not.toBeInTheDocument()
    })

    it('says when new firmware is found', async () => {
      const { fn } = stubFetch(checkRoutes(updateAvailable))
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      show(upToDate)
      await user.click(screen.getByRole('button', { name: 'Check now' }))
      expect(await screen.findByText('New firmware found')).toBeInTheDocument()
    })

    it('is disabled and says it is checking until the answer is in', async () => {
      const { fn } = stubFetch(checkRoutes(upToDate))
      let release!: () => void
      const gate = new Promise<void>((resolve) => (release = resolve))
      vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
        if (init?.method === 'POST') await gate
        return fn(input, init)
      })
      const user = userEvent.setup()
      show(upToDate)
      await user.click(screen.getByRole('button', { name: 'Check now' }))
      expect(await screen.findByRole('button', { name: 'Checking…' })).toBeDisabled()
      expect(screen.queryByText(/checking/i, { selector: '[data-sonner-toast] *' })).not.toBeInTheDocument()
      release()
      expect(await screen.findByRole('button', { name: 'Check now' })).toBeEnabled()
    })

    it("shows the server's reason when it can't", async () => {
      const { fn } = stubFetch({ 'POST /firmware/check': () => ({ status: 429, body: { error: 'the changelog was checked a moment ago; try again in a little while' } }) })
      vi.stubGlobal('fetch', fn)
      const user = userEvent.setup()
      show(upToDate)
      await user.click(screen.getByRole('button', { name: 'Check now' }))
      expect(await screen.findByText(/checked a moment ago/)).toBeInTheDocument()
    })

    it('is unavailable while the check is turned off', () => {
      show(upToDate, (s) => (s.settings.firmwareCheck = false))
      const button = screen.getByRole('button', { name: 'Check now' })
      expect(button).toBeDisabled()
      expect(button).toHaveAttribute('title', expect.stringContaining('turned off'))
    })
  })

  it('names the tuner in its title when there are several', () => {
    const s = makeTwoDeviceState()
    s.devices[1].label = 'Cable box'
    renderWithApp(
      <>
        <FirmwareSection device={s.devices[0]} />
        <FirmwareSection device={s.devices[1]} />
      </>,
      s,
    )
    expect(section('Firmware — Steve FLEX 4K')).toBeInTheDocument()
    expect(section('Firmware — Cable box')).toBeInTheDocument()
  })
})
