import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { LivePage } from '@/pages/live'
import { TunerPage } from '@/pages/tuner'
import { makeCableDevice, makeDevice, makeState } from '@/test/fixtures'
import { renderWithApp, stubFetch } from '@/test/render'

afterEach(() => vi.unstubAllGlobals())

const none = () => makeState({ devices: [], viewers: [] })
const found = [
  { id: '10ABCC87', address: 'http://192.168.1.50', tuners: 4, added: '' },
  { id: '1111AAA2', address: 'http://192.168.1.51', tuners: 2, added: 'cable' },
]
const okState = { 'GET /state': () => ({ body: makeState() }) }

describe('with no tuner yet', () => {
  it('the live page says so and offers to find one, instead of showing 0 of 0', () => {
    renderWithApp(<LivePage />, none())
    expect(screen.getByRole('heading', { name: 'No tuners yet' })).toBeInTheDocument()
    expect(screen.getByText('No tuner yet')).toBeInTheDocument()
    expect(screen.queryByText(/of 0 tuners live/)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Find tuners' })).toBeEnabled()
  })

  it('cannot add one when there is nowhere to save it, and says why', () => {
    renderWithApp(<LivePage />, { ...none(), editable: false })
    expect(screen.getByRole('button', { name: 'Find tuners' })).toBeDisabled()
    expect(screen.getByText(/no settings file/)).toBeInTheDocument()
  })

  it('searches when the dialog opens, lists what answered, and adds the one chosen', async () => {
    const { fn, calls } = stubFetch({
      'POST /discover': () => ({ body: { tuners: found } }),
      'POST /devices': () => ({ status: 201, body: makeDevice() }),
      ...okState,
    })
    vi.stubGlobal('fetch', fn)
    renderWithApp(<LivePage />, none())
    await userEvent.click(screen.getByRole('button', { name: 'Find tuners' }))

    const dialog = await screen.findByRole('dialog')
    expect(await within(dialog).findByText('10ABCC87')).toBeInTheDocument()
    expect(within(dialog).getByText('192.168.1.50 · 4 tuners')).toBeInTheDocument()
    // One already added is shown as such and has no Add button.
    expect(within(dialog).getByText('Already added as cable')).toBeInTheDocument()
    expect(within(dialog).queryByRole('button', { name: 'Add 1111AAA2' })).not.toBeInTheDocument()

    await userEvent.click(within(dialog).getByRole('button', { name: 'Add 10ABCC87' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/devices'))).toBe(true))
    expect(calls.find((c) => c.url.endsWith('/devices'))?.body).toEqual({ tuner: 'http://192.168.1.50', id: '10ABCC87' })
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(calls.filter((c) => c.url.endsWith('/discover'))).toHaveLength(1) // searched once, not again
  })

  it('adds by address when nothing is found, and shows a refusal in place', async () => {
    const { fn, calls } = stubFetch({
      'POST /discover': () => ({ body: { tuners: [] } }),
      'POST /devices': () => ({ status: 422, body: { error: "couldn't reach an HDHomeRun at 10.0.0.9" } }),
    })
    vi.stubGlobal('fetch', fn)
    renderWithApp(<LivePage />, none())
    await userEvent.click(screen.getByRole('button', { name: 'Find tuners' }))
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
      'POST /discover': () => (++n === 1 ? { status: 502, body: { error: "couldn't search the network for tuners" } } : { body: { tuners: found.slice(0, 1) } }),
    })
    vi.stubGlobal('fetch', fn)
    renderWithApp(<LivePage />, none())
    await userEvent.click(screen.getByRole('button', { name: 'Find tuners' }))
    const dialog = await screen.findByRole('dialog')
    expect(await within(dialog).findByText("couldn't search the network for tuners")).toBeInTheDocument()
    await userEvent.click(within(dialog).getByRole('button', { name: 'Search again' }))
    expect(await within(dialog).findByText('10ABCC87')).toBeInTheDocument()
  })
})

describe('adding and removing tuners on the Tuners page', () => {
  const two = () => makeState({ devices: [makeDevice(), makeCableDevice()] })

  it('gives each tuner its port and a way to remove it, and the page a way to add another', () => {
    renderWithApp(<TunerPage />, two())
    const antenna = screen.getByRole('region', { name: 'Manage antenna' })
    expect(within(antenna).getByLabelText('Port')).toHaveValue(5004)
    expect(within(antenna).getByRole('button', { name: 'Remove antenna' })).toBeEnabled()
    expect(within(screen.getByRole('region', { name: 'Manage cable' })).getByLabelText('Port')).toHaveValue(5005)
    expect(screen.getByRole('button', { name: 'Remove cable' })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Add tuner' })).toBeEnabled()
  })

  it('asks before removing, warning about streams that will be cut, then removes it', async () => {
    const { fn, calls } = stubFetch({ 'DELETE /devices/antenna': () => ({ status: 204 }), ...okState })
    vi.stubGlobal('fetch', fn)
    renderWithApp(<TunerPage />, two())
    await userEvent.click(screen.getByRole('button', { name: 'Remove antenna' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Remove antenna?')).toBeInTheDocument()
    expect(within(dialog).getByText(/One stream is running on it and will be cut off/)).toBeInTheDocument()
    expect(calls.some((c) => c.method === 'DELETE')).toBe(false) // nothing yet

    await userEvent.click(within(dialog).getByRole('button', { name: 'Remove' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url.endsWith('/devices/antenna'))).toBe(true))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
  })

  it('keeping it sends nothing; a refusal stays on screen', async () => {
    const { fn, calls } = stubFetch({ 'DELETE /devices/cable': () => ({ status: 409, body: { error: 'the tuner was set with command-line flags' } }) })
    vi.stubGlobal('fetch', fn)
    renderWithApp(<TunerPage />, two())
    await userEvent.click(screen.getByRole('button', { name: 'Remove cable' }))
    await userEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Keep' }))
    expect(calls).toHaveLength(0)

    await userEvent.click(screen.getByRole('button', { name: 'Remove cable' }))
    await userEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Remove' }))
    expect(await screen.findByText('the tuner was set with command-line flags')).toBeInTheDocument()
  })

  it('cannot be changed when there is no settings file, and says why', () => {
    renderWithApp(<TunerPage />, { ...two(), editable: false })
    expect(screen.getByRole('button', { name: 'Remove antenna' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Add tuner' })).toBeDisabled()
    expect(screen.getByText(/no settings file to save tuners in/)).toBeInTheDocument()
  })
})
