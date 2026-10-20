import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { LOOK_EVERY_MS, LOOK_FOR_MS } from '@/hooks/use-find-tuners'
import { LivePage } from '@/pages/live'
import { makeCableDevice, makeDevice, makeState } from '@/test/fixtures'
import { renderTunerRoutes, renderWithApp, stubFetch } from '@/test/render'

afterEach(() => vi.unstubAllGlobals())

const none = () => makeState({ devices: [], viewers: [] })
const found = [
  { id: '10ABCC87', address: 'http://192.168.1.50', tuners: 4, added: '', label: 'HDHomeRun FLEX 4K', name: 'flex-4k-87' },
  { id: '1111AAA2', address: 'http://192.168.1.51', tuners: 2, added: 'cable', label: 'HDHomeRun CONNECT', name: 'connect-a2' },
]
const okState = { 'GET /state': () => ({ body: makeState() }) }

describe('with no tuner yet', () => {
  const look = (tuners: unknown[] = []) => stubFetch({ 'POST /discover': () => ({ body: { tuners } }), ...okState })

  it('the live page says so and looks for tuners by itself, instead of showing 0 of 0', async () => {
    const { fn, calls } = look()
    vi.stubGlobal('fetch', fn)
    renderWithApp(<LivePage />, none())
    expect(screen.getByRole('heading', { name: 'No tuners yet' })).toBeInTheDocument()
    expect(screen.getByText('No tuner yet')).toBeInTheDocument()
    expect(screen.queryByText(/of 0 tuners live/)).not.toBeInTheDocument()
    expect(screen.getByText(/Looking for HDHomeRun tuners/)).toBeInTheDocument()
    await waitFor(() => expect(calls.some((c) => c.url.endsWith('/discover'))).toBe(true)) // no click needed
    expect(screen.getByRole('button', { name: 'Add by address' })).toBeEnabled()
  })

  it('cannot add one when there is nowhere to save it, and says why', () => {
    vi.stubGlobal('fetch', look().fn)
    renderWithApp(<LivePage />, { ...none(), editable: false })
    expect(screen.getByRole('button', { name: 'Add by address' })).toBeDisabled()
    expect(screen.getByText(/no settings file/)).toBeInTheDocument()
  })

  it('shows what it found with a name filled in, and adds nothing until Add is pressed', async () => {
    const { fn, calls } = look(found)
    vi.stubGlobal('fetch', fn)
    renderWithApp(<LivePage />, none())
    expect(await screen.findByText('Tuners Yagi can see')).toBeInTheDocument()
    expect(screen.getByLabelText('Name for 10ABCC87')).toHaveValue('HDHomeRun FLEX 4K')
    expect(screen.getByLabelText('Address name for 10ABCC87')).toHaveValue('flex-4k-87')
    expect(screen.getByText('Already added as cable')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Add 1111AAA2' })).not.toBeInTheDocument()
    expect(calls.some((c) => c.url.endsWith('/devices'))).toBe(false)

    const name = screen.getByLabelText('Name for 10ABCC87')
    await userEvent.clear(name)
    await userEvent.type(name, 'Roof antenna')
    await userEvent.click(screen.getByRole('button', { name: 'Add 10ABCC87' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/devices'))).toBe(true))
    expect(calls.find((c) => c.url.endsWith('/devices'))?.body).toEqual({ tuner: 'http://192.168.1.50', id: '10ABCC87', name: 'flex-4k-87', label: 'Roof antenna' })
  })

  it('adds every tuner found with Add all, each with its own name', async () => {
    const two = [found[0]!, { ...found[1]!, added: '' }]
    const { fn, calls } = stubFetch({ 'POST /discover': () => ({ body: { tuners: two } }), 'POST /devices': () => ({ status: 201, body: makeDevice() }), ...okState })
    vi.stubGlobal('fetch', fn)
    renderWithApp(<LivePage />, none())
    await userEvent.click(await screen.findByRole('button', { name: 'Add all 2' }))
    await waitFor(() => expect(calls.filter((c) => c.method === 'POST' && c.url.endsWith('/devices'))).toHaveLength(2))
    expect(calls.filter((c) => c.url.endsWith('/devices')).map((c) => (c.body as { name: string }).name)).toEqual(['flex-4k-87', 'connect-a2'])
  })

  it('stops looking after a few minutes, says so, explains Docker, and looks again when asked', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      const { fn, calls } = look()
      vi.stubGlobal('fetch', fn)
      renderWithApp(<LivePage />, none())
      await screen.findByText(/Looking for HDHomeRun tuners/)
      await vi.advanceTimersByTimeAsync(LOOK_FOR_MS + LOOK_EVERY_MS)
      expect(await screen.findByText(/looked for a few minutes and no tuner answered/)).toBeInTheDocument()
      expect(screen.getByText(/host networking/)).toBeInTheDocument()
      const before = calls.length
      await vi.advanceTimersByTimeAsync(60_000)
      expect(calls.length).toBe(before) // stopped

      await userEvent.click(screen.getByRole('button', { name: 'Look again' }))
      await waitFor(() => expect(calls.length).toBeGreaterThan(before))
    } finally {
      vi.useRealTimers()
    }
  })

  it('adds by address, and shows a refusal in place', async () => {
    const { fn, calls } = stubFetch({
      'POST /discover': () => ({ body: { tuners: [] } }),
      'POST /devices': () => ({ status: 422, body: { error: "couldn't reach an HDHomeRun at 10.0.0.9" } }),
    })
    vi.stubGlobal('fetch', fn)
    renderWithApp(<LivePage />, none())
    await userEvent.click(screen.getByRole('button', { name: 'Add by address' }))
    const dialog = await screen.findByRole('dialog')
    expect(await within(dialog).findByText(/No tuner answered/)).toBeInTheDocument()

    await userEvent.click(within(dialog).getByRole('button', { name: 'Add' }))
    expect(within(dialog).getByRole('alert')).toHaveTextContent(/Enter the tuner's address/) // nothing sent
    expect(calls.some((c) => c.url.endsWith('/devices'))).toBe(false)

    await userEvent.type(within(dialog).getByLabelText('Or add one by its address'), ' 10.0.0.9 ')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Add' }))
    expect(await within(dialog).findByText("couldn't reach an HDHomeRun at 10.0.0.9")).toBeInTheDocument()
    expect(calls.find((c) => c.url.endsWith('/devices'))?.body).toEqual({ tuner: '10.0.0.9' })
    expect(screen.getByRole('dialog')).toBeInTheDocument() // stays open to correct it
  })

  it('shows a failed search and lets it be run again', async () => {
    let n = 0
    const { fn } = stubFetch({
      'POST /discover': () => (++n === 2 ? { status: 502, body: { error: "couldn't search the network for tuners" } } : { body: { tuners: found.slice(0, 1) } }),
    })
    vi.stubGlobal('fetch', fn)
    renderWithApp(<LivePage />, none())
    await userEvent.click(screen.getByRole('button', { name: 'Add by address' }))
    const dialog = await screen.findByRole('dialog')
    expect(await within(dialog).findByText("couldn't search the network for tuners")).toBeInTheDocument()
    await userEvent.click(within(dialog).getByRole('button', { name: 'Search again' }))
    expect(await within(dialog).findByText('10ABCC87')).toBeInTheDocument()
  })
})

describe('adding and removing tuners', () => {
  const two = () => makeState({ devices: [makeDevice(), makeCableDevice()] })

  it('gives each tuner its port and a way to remove it on its Settings tab, and the Tuners page a way to add another', () => {
    const { unmount } = renderTunerRoutes(two(), '/tuners/antenna/settings')
    const antenna = screen.getByRole('region', { name: 'Settings' })
    expect(within(antenna).getByLabelText('Port')).toHaveValue(5004)
    expect(within(antenna).getByRole('button', { name: 'Remove Steve FLEX 4K' })).toBeEnabled()
    unmount()
    renderTunerRoutes(two(), '/tuners/cable/settings')
    expect(within(screen.getByRole('region', { name: 'Settings' })).getByLabelText('Port')).toHaveValue(5005)
    expect(screen.getByRole('button', { name: 'Remove Cable HDHR' })).toBeEnabled()
  })

  it('asks before removing, warning about streams that will be cut, then removes it', async () => {
    const { fn, calls } = stubFetch({ 'DELETE /devices/antenna': () => ({ status: 204 }), ...okState })
    vi.stubGlobal('fetch', fn)
    renderTunerRoutes(two(), '/tuners/antenna/settings')
    await userEvent.click(screen.getByRole('button', { name: 'Remove Steve FLEX 4K' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Remove Steve FLEX 4K?')).toBeInTheDocument()
    expect(within(dialog).getByText(/One stream is running on it and will be cut off/)).toBeInTheDocument()
    expect(calls.some((c) => c.method === 'DELETE')).toBe(false) // nothing yet

    await userEvent.click(within(dialog).getByRole('button', { name: 'Remove' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url.endsWith('/devices/antenna'))).toBe(true))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
  })

  it('keeping it sends nothing; a refusal stays on screen', async () => {
    const { fn, calls } = stubFetch({ 'DELETE /devices/cable': () => ({ status: 409, body: { error: 'the tuner was set with command-line flags' } }) })
    vi.stubGlobal('fetch', fn)
    renderTunerRoutes(two(), '/tuners/cable/settings')
    await userEvent.click(screen.getByRole('button', { name: 'Remove Cable HDHR' }))
    await userEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Keep' }))
    expect(calls.filter((c) => c.method !== 'GET')).toHaveLength(0)

    await userEvent.click(screen.getByRole('button', { name: 'Remove Cable HDHR' }))
    await userEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Remove' }))
    expect(await screen.findByText('the tuner was set with command-line flags')).toBeInTheDocument()
  })

  it('cannot be changed when there is no settings file, and says why', () => {
    const { unmount } = renderTunerRoutes({ ...two(), editable: false }, '/tuners/antenna/settings')
    expect(screen.getByRole('button', { name: 'Remove Steve FLEX 4K' })).toBeDisabled()
    unmount()
    renderTunerRoutes({ ...two(), editable: false }, '/tuners')
    expect(screen.getByRole('button', { name: 'Add tuner' })).toBeDisabled()
    expect(screen.getByText(/no settings file to save tuners in/)).toBeInTheDocument()
  })
})
