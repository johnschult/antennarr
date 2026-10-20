import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Viewer } from '@/lib/types'
import { john, makeState, makeTwoDeviceState, mom } from '@/test/fixtures'
import { renderWithApp, stubFetch } from '@/test/render'
import { ViewerWizard } from './viewer-wizard'

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

const block = (name: string) => screen.getByRole('region', { name })
const host = () => screen.getByLabelText(/Address viewers will use/)

type State = ReturnType<typeof makeState>

/** The wizard for an existing viewer, which starts at how they connect; or, with no viewer, at adding one. */
function mount(state: State = makeState(), viewer: Viewer | null = john, onOpenChange = vi.fn()) {
  renderWithApp(<ViewerWizard open onOpenChange={onOpenChange} devices={state.devices} viewer={viewer ?? undefined} />, state)
  return onOpenChange
}

async function typeHost(value: string) {
  const user = userEvent.setup()
  // The dialog moves the focus to its first control a moment after it opens; typing before that loses the focus.
  await waitFor(() => expect(document.activeElement).not.toBe(document.body))
  await user.clear(host())
  if (value) await user.type(host(), value)
  return user
}

const next = (user: ReturnType<typeof userEvent.setup>) => user.click(screen.getByRole('button', { name: /^Next/ }))

describe('ViewerWizard: the steps', () => {
  it('adds a viewer first, then how they connect, then their media server', async () => {
    const { fn, calls } = stubFetch({
      'POST /viewers': () => ({ status: 201, body: {} }),
      'GET /state': () => ({ body: makeState() }),
      'GET /network': () => ({ body: { addresses: [] } }),
    })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    mount(makeState(), null)

    expect(screen.getByRole('heading', { name: 'Add a viewer' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Next: Connect' })).toBeInTheDocument() // says where it goes, not 'Add viewer'
    expect(screen.getByText(/saved when you continue/)).toBeInTheDocument()
    const steps = within(screen.getByRole('list', { name: 'Steps' }))
    expect(steps.getAllByRole('listitem').map((s) => s.textContent)).toEqual(['1Who', '2Connect', '3Media server'])
    expect(steps.getByRole('listitem', { current: 'step' })).toHaveTextContent('Who')

    await user.type(screen.getByLabelText('Name'), 'Cabin')
    await user.type(screen.getByLabelText('Addresses'), '10.9.0.6')
    await user.click(screen.getByRole('button', { name: 'Next: Connect' }))

    expect(await screen.findByRole('region', { name: 'The address' })).toBeInTheDocument()
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ name: 'Cabin', addresses: ['10.9.0.6'], limit: 1 })
    expect(await screen.findByText('Added Cabin')).toBeInTheDocument() // the toast, as when adding without steps
    expect(screen.getByRole('listitem', { current: 'step' })).toHaveTextContent('Connect')
    expect(screen.getByText("How Cabin's media server will reach this machine.")).toBeInTheDocument()

    expect(screen.getByRole('button', { name: 'Next: Media server' })).toBeInTheDocument()
    await next(user)
    expect(screen.getByRole('region', { name: 'In their media server' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Done' })).toBeInTheDocument()
  })

  it("uses the new viewer's addresses for the firewall rules", async () => {
    vi.stubGlobal(
      'fetch',
      stubFetch({
        'POST /viewers': () => ({ status: 201, body: {} }),
        'GET /state': () => ({ body: makeState() }),
        'GET /network': () => ({ body: { addresses: [] } }),
      }).fn,
    )
    const user = userEvent.setup()
    mount(makeState(), null)
    await user.type(screen.getByLabelText('Name'), 'Cabin')
    await user.type(screen.getByLabelText('Addresses'), '10.9.0.6')
    await user.click(screen.getByRole('button', { name: 'Next: Connect' }))
    await user.click(await screen.findByRole('switch', { name: /has a firewall/ }))
    expect(screen.getByLabelText('firewall rules')).toHaveTextContent('sudo ufw allow from 10.9.0.6 to any port 5004 proto tcp')
  })

  it('stays on the first step, with the reason, when the viewer cannot be added', async () => {
    vi.stubGlobal('fetch', stubFetch({ 'POST /viewers': () => ({ status: 409, body: { error: 'a viewer called Cabin already exists' } }) }).fn)
    const user = userEvent.setup()
    mount(makeState(), null)
    await user.type(screen.getByLabelText('Name'), 'Cabin')
    await user.type(screen.getByLabelText('Addresses'), '10.9.0.6')
    await user.click(screen.getByRole('button', { name: 'Next: Connect' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('already exists')
    expect(screen.getByLabelText('Name')).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'The address' })).not.toBeInTheDocument()
  })

  it('checks the form before bothering the server', async () => {
    const { fn, calls } = stubFetch({})
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    mount(makeState(), null)
    await user.click(screen.getByRole('button', { name: 'Next: Connect' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Give the viewer a name.')
    expect(calls).toEqual([])
  })

  it("does not read this machine's addresses until the connect step", async () => {
    const { fn, calls } = stubFetch({
      'POST /viewers': () => ({ status: 201, body: {} }),
      'GET /state': () => ({ body: makeState() }),
      'GET /network': () => ({ body: { addresses: [] } }),
    })
    vi.stubGlobal('fetch', fn)
    const user = userEvent.setup()
    mount(makeState(), null)
    expect(calls.filter((c) => c.url.includes('/network'))).toEqual([])
    await user.type(screen.getByLabelText('Name'), 'Cabin')
    await user.type(screen.getByLabelText('Addresses'), '10.9.0.6')
    await user.click(screen.getByRole('button', { name: 'Next: Connect' }))
    await screen.findByRole('region', { name: 'The address' })
    expect(calls.filter((c) => c.url.includes('/network'))).toHaveLength(1)
  })

  it('cannot go back to adding once the viewer is saved, so they are not added twice', async () => {
    vi.stubGlobal(
      'fetch',
      stubFetch({
        'POST /viewers': () => ({ status: 201, body: {} }),
        'GET /state': () => ({ body: makeState() }),
        'GET /network': () => ({ body: { addresses: [] } }),
      }).fn,
    )
    const user = userEvent.setup()
    const onOpenChange = mount(makeState(), null)
    expect(screen.getByRole('button', { name: /Cancel/ })).toBeInTheDocument()
    await user.type(screen.getByLabelText('Name'), 'Cabin')
    await user.type(screen.getByLabelText('Addresses'), '10.9.0.6')
    await user.click(screen.getByRole('button', { name: 'Next: Connect' }))
    await screen.findByRole('region', { name: 'The address' })
    expect(screen.queryByRole('button', { name: /Back/ })).not.toBeInTheDocument()
    await user.click(screen.getByText('Close', { selector: 'button' })) // the footer's, not the dialog's corner X
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })
})

describe('ViewerWizard: an existing viewer', () => {
  it("starts at how they connect, with only the steps that are left, and doesn't change the viewer", async () => {
    const { fn, calls } = stubFetch({ 'GET /network': () => ({ body: { addresses: [] } }) })
    vi.stubGlobal('fetch', fn)
    mount(makeState(), john)
    expect(screen.getByRole('heading', { name: 'Connect John' })).toBeInTheDocument()
    expect(within(screen.getByRole('list', { name: 'Steps' })).getAllByRole('listitem').map((s) => s.textContent)).toEqual(['1Connect', '2Media server'])
    expect(screen.queryByLabelText('Name')).not.toBeInTheDocument()
    await screen.findByLabelText(/Address viewers will use/)
    expect(calls.some((c) => c.method !== 'GET')).toBe(false)
  })

  it('goes back and forward, and closes when it is done', async () => {
    const user = userEvent.setup()
    const onOpenChange = mount()
    await next(user)
    expect(screen.getByRole('listitem', { current: 'step' })).toHaveTextContent('Media server')
    await user.click(screen.getByRole('button', { name: /Back/ }))
    expect(screen.getByRole('listitem', { current: 'step' })).toHaveTextContent('Connect')
    await next(user)
    await user.click(screen.getByRole('button', { name: 'Done' }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('keeps the address and connection you chose when you go forward and back', async () => {
    mount()
    const user = await typeHost('box.lan')
    await next(user)
    expect(screen.getByLabelText('check commands')).toHaveTextContent('box.lan:5004')
    await user.click(screen.getByRole('button', { name: /Back/ }))
    expect(host()).toHaveValue('box.lan')
  })
})

describe('ViewerWizard: how they connect', () => {
  it('starts from the address in the browser, and warns that a local one will not work for others', () => {
    mount()
    expect(host()).toHaveValue(window.location.hostname)
    expect(within(block('The address')).getByRole('alert')).toHaveTextContent(/only works on this machine|Enter/)
  })

  it('shows the address a viewer enters, with the tuner port', async () => {
    mount()
    await typeHost('antenna.example.com')
    expect(screen.getByLabelText('tuner address')).toHaveTextContent('antenna.example.com:5004')
    expect(within(block('The address')).queryByRole('alert')).not.toBeInTheDocument()
  })

  it('lets you pick the tuner when there are several, and uses its port and name', async () => {
    const s = makeTwoDeviceState()
    s.devices[0].label = 'Living room antenna'
    mount(s)
    const user = await typeHost('box.lan')
    expect(screen.getByLabelText('tuner address')).toHaveTextContent('box.lan:5004')
    const choice = within(screen.getByRole('radiogroup', { name: 'Tuner' }))
    expect(choice.getByRole('radio', { name: /Living room antenna/ })).toBeChecked()
    await user.click(choice.getByRole('radio', { name: /Cable HDHR/ }))
    expect(screen.getByLabelText('tuner address')).toHaveTextContent('box.lan:5005')
  })

  it('has no tuner choice with one tuner', () => {
    mount()
    expect(screen.queryByRole('radiogroup', { name: 'Tuner' })).not.toBeInTheDocument()
  })

  it('shows nothing about firewalls until you say there is one, and hides it again when you say there is not', async () => {
    mount()
    const user = await typeHost('box.lan')
    const gate = screen.getByRole('switch', { name: 'This machine has a firewall that blocks port 5004' })
    expect(gate).toHaveAttribute('aria-checked', 'false')
    expect(screen.queryByRole('radiogroup', { name: 'Firewall' })).not.toBeInTheDocument()
    expect(screen.queryByLabelText('firewall rules')).not.toBeInTheDocument()
    expect(document.body.textContent).not.toMatch(/ufw|firewalld|iptables/)

    await user.click(gate)
    expect(screen.getByRole('radiogroup', { name: 'Firewall' })).toBeInTheDocument()
    expect(screen.getByLabelText('firewall rules')).toBeInTheDocument()
    await user.click(gate)
    expect(screen.queryByLabelText('firewall rules')).not.toBeInTheDocument()
  })

  it("writes this viewer's firewall rules, and switches firewall", async () => {
    mount(makeState({ viewers: [john, mom] }), mom)
    const user = await typeHost('box.lan')
    await user.click(screen.getByRole('switch', { name: /has a firewall/ }))
    const fw = () => screen.getByLabelText('firewall rules')

    expect(fw()).toHaveTextContent(`sudo ufw allow from ${mom.addresses[0]} to any port 5004 proto tcp`)
    expect(fw()).not.toHaveTextContent(john.addresses[0])
    await user.click(within(screen.getByRole('radiogroup', { name: 'Firewall' })).getByRole('radio', { name: 'iptables' }))
    expect(fw()).toHaveTextContent(`sudo iptables -I INPUT -p tcp -s ${mom.addresses[0]} --dport 5004 -j ACCEPT`)
    expect(screen.getByLabelText('undo firewall rules')).toHaveTextContent('iptables -D INPUT')
  })

  it('says so when a viewer has no address a firewall rule can be written for', async () => {
    mount(makeState(), { ...john, addresses: ['example.com'] })
    const user = await typeHost('box.lan')
    await user.click(screen.getByRole('switch', { name: /has a firewall/ }))
    expect(within(block('On this machine')).getByText('This viewer has no addresses to allow.')).toBeInTheDocument()
  })

  it('changes the connection steps with the way they will connect', async () => {
    mount()
    const user = await typeHost('box.lan')
    const steps = () => block('How will they reach it?')
    expect(steps()).toHaveTextContent(/tailscale ip -4/)
    await user.click(within(steps()).getByRole('radio', { name: /Internet/ }))
    expect(steps()).toHaveTextContent(/no passwords/)
    await user.click(within(steps()).getByRole('radio', { name: /Same network/ }))
    expect(steps()).toHaveTextContent(/both servers are on the same network/)
  })

  it('does not repeat an unusable address in the connection steps', async () => {
    mount()
    await typeHost('localhost')
    expect(block('How will they reach it?')).not.toHaveTextContent('localhost')
  })

  it('never puts a typed address into a command', async () => {
    mount()
    await typeHost('box.lan; rm -rf ~')
    const where = [...document.querySelectorAll('*')].filter((e) => e.children.length === 0 && e.textContent?.includes('rm -rf')).map((e) => e.outerHTML)
    expect(where).toEqual([])
  })

  it('copies a command block', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    mount()
    const user = await typeHost('box.lan')
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true }) // after setup(), which installs its own
    await user.click(screen.getByRole('button', { name: 'Copy tuner address' }))
    expect(writeText).toHaveBeenCalledWith('box.lan:5004')
    expect(await screen.findByText('Copied')).toBeInTheDocument()
  })

  it('says so when copying is not allowed', async () => {
    mount()
    const user = await typeHost('box.lan')
    Object.defineProperty(navigator, 'clipboard', { value: { writeText: vi.fn().mockRejectedValue(new Error('no')) }, configurable: true })
    await user.click(screen.getByRole('button', { name: 'Copy tuner address' }))
    expect(await screen.findByText(/Couldn't copy/)).toBeInTheDocument()
  })
})

describe('ViewerWizard on a machine that knows its own addresses', () => {
  const ts = { ip: '100.64.0.9', kind: 'tailscale', interface: 'tailscale0' }
  const lan = { ip: '192.168.1.5', kind: 'lan', interface: 'eth0' }
  const machine = (...addresses: unknown[]) => vi.stubGlobal('fetch', stubFetch({ 'GET /network': () => ({ body: { addresses } }) }).fn)
  const field = async () => (await screen.findByLabelText(/Address viewers will use/)) as HTMLInputElement

  it("fills in the machine's Tailscale address, and says Tailscale is already running", async () => {
    machine(ts, lan)
    mount()
    await screen.findByRole('group', { name: 'Addresses for this way' })
    expect(await field()).toHaveValue('100.64.0.9')
    expect(screen.getByLabelText('tuner address')).toHaveTextContent('100.64.0.9:5004')
    expect(within(block('The address')).queryByRole('alert')).not.toBeInTheDocument()
    const steps = block('How will they reach it?')
    expect(steps).toHaveTextContent('Tailscale is running on this machine (100.64.0.9)')
    expect(steps).not.toHaveTextContent(/Install Tailscale|tailscale ip -4|tailscale ping/) // nothing to install or look up: we know it
    expect(within(steps).getByRole('radio', { name: /Tailscale/ })).toHaveTextContent('Running on this machine (100.64.0.9)')
  })

  it('offers only the addresses that belong to the way chosen, each saying what it is, and clicking one uses it', async () => {
    const lan2 = { ip: '10.0.0.5', kind: 'lan', interface: 'wlan0' }
    machine(ts, lan, lan2)
    mount()
    const user = userEvent.setup()
    const chips = () => within(screen.getByRole('group', { name: 'Addresses for this way' }))
    await screen.findByRole('group', { name: 'Addresses for this way' })
    // Tailscale on this machine: its own address, and none of the LAN ones
    expect(chips().getByRole('button', { name: /100\.64\.0\.9.*Tailscale/ })).toHaveAttribute('aria-pressed', 'true')
    expect(chips().queryByRole('button', { name: /192\.168\.1\.5/ })).not.toBeInTheDocument()
    // the same network: the LAN ones, and not the Tailscale one
    await user.click(within(screen.getByRole('radiogroup', { name: 'Connection' })).getByRole('radio', { name: /Same network/ }))
    expect(chips().queryByRole('button', { name: /100\.64\.0\.9/ })).not.toBeInTheDocument()
    expect(chips().getByRole('button', { name: /192\.168\.1\.5.*LAN/ })).toHaveAttribute('aria-pressed', 'true')
    expect(chips().getByRole('button', { name: /10\.0\.0\.5.*LAN/ })).toHaveAttribute('aria-pressed', 'false')

    await user.click(chips().getByRole('button', { name: /10\.0\.0\.5/ }))
    expect(await field()).toHaveValue('10.0.0.5')
    expect(chips().getByRole('button', { name: /10\.0\.0\.5/ })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByLabelText('tuner address')).toHaveTextContent('10.0.0.5:5004')
  })

  it('follows the connection you choose until you type an address of your own', async () => {
    machine(ts, lan)
    mount()
    const user = userEvent.setup()
    await screen.findByRole('group', { name: 'Addresses for this way' })
    const connection = within(screen.getByRole('radiogroup', { name: 'Connection' }))

    await user.click(connection.getByRole('radio', { name: /Same network/ }))
    expect(await field()).toHaveValue('192.168.1.5')
    await user.click(connection.getByRole('radio', { name: /Tailscale/ }))
    expect(await field()).toHaveValue('100.64.0.9')

    await user.clear(await field())
    await user.type(await field(), 'mine.example.com')
    await user.click(connection.getByRole('radio', { name: /Same network/ }))
    expect(await field()).toHaveValue('mine.example.com') // what you typed is left alone
  })

  it('says so when Tailscale is not running on this machine, and falls back to the browser address', async () => {
    machine(lan)
    mount()
    expect(await screen.findByText(/Tailscale wasn't found running on this machine/)).toBeInTheDocument()
    expect(await field()).toHaveValue(window.location.hostname)
    expect(block('How will they reach it?')).toHaveTextContent('Install Tailscale') // so the longer steps are still there
  })

  it('does not show the long Tailscale steps while it is still checking', async () => {
    let release: (v: Response) => void = () => {}
    vi.stubGlobal('fetch', () => new Promise<Response>((r) => (release = r)))
    mount()
    await screen.findByLabelText(/Address viewers will use/)
    expect(block('How will they reach it?')).not.toHaveTextContent(/Install Tailscale|Tailscale is running/)
    release(new Response(JSON.stringify({ addresses: [ts] })))
    expect(await within(block('How will they reach it?')).findByText(/Tailscale is running on this machine/)).toBeInTheDocument()
  })

  it('does not claim anything when nothing could be read', async () => {
    vi.stubGlobal('fetch', stubFetch({ 'GET /network': () => ({ status: 500, body: { error: 'no' } }) }).fn)
    mount()
    expect(await field()).toHaveValue(window.location.hostname)
    await new Promise((r) => setTimeout(r, 50))
    expect(screen.queryByRole('group', { name: 'Addresses for this way' })).not.toBeInTheDocument()
    expect(screen.queryByText(/wasn't found running/)).not.toBeInTheDocument()
  })

  it('leaves the connection hint alone when there is no Tailscale here', async () => {
    machine(lan)
    mount()
    await screen.findByText(/Tailscale wasn't found running on this machine/)
    expect(screen.getByRole('radio', { name: /Tailscale/ })).not.toHaveTextContent('Running on this machine')
  })
})

describe("ViewerWizard with Yagi's own Tailscale", () => {
  const hostTs = { ip: '100.127.0.1', kind: 'tailscale', interface: 'tailscale0' }
  const hostLan = { ip: '192.168.1.5', kind: 'lan', interface: 'eth0' }
  const machine = (...addresses: unknown[]) => vi.stubGlobal('fetch', stubFetch({ 'GET /network': () => ({ body: { addresses } }) }).fn)
  const field = async () => (await screen.findByLabelText(/Address viewers will use/)) as HTMLInputElement
  const node = (over: Partial<NonNullable<State['tailscale']>> = {}, enabled = true) => {
    const s = makeState()
    s.settings.tailscale = { enabled, hostname: 'yagi', tailnetOnly: false, https: false }
    s.tailscale = { built: true, state: 'running', hostname: 'yagi', dnsName: 'yagi.tail1234.ts.net', addresses: ['100.100.0.7'], serving: ['5004'], ...over }
    return s
  }
  const ways = () => within(screen.getByRole('radiogroup', { name: 'Connection' }))

  it('is offered first, apart from the Tailscale on this machine, each saying which it is', async () => {
    machine(hostTs, hostLan)
    mount(node())
    await waitFor(() => expect(ways().getByRole('radio', { name: /Tailscale on this machine/ })).toHaveTextContent('Running on this machine (100.127.0.1)')) // once the machine's addresses are read
    const labels = ways().getAllByRole('radio').map((r) => r.querySelector('span')?.textContent)
    expect(labels).toEqual(['Yagi’s own Tailscale', 'Tailscale on this machine', 'Other VPN', 'Same network', 'Internet'])
    expect(ways().getByRole('radio', { name: /Yagi’s own Tailscale/ })).toHaveTextContent('Built in, no install needed here (yagi.tail1234.ts.net)')
    expect(ways().getByRole('radio', { name: /Tailscale on this machine/ })).toHaveTextContent('Running on this machine (100.127.0.1)')
  })

  it('is chosen when it is up, and its name is the address, with only its own addresses offered', async () => {
    machine(hostTs, hostLan)
    mount(node())
    expect(ways().getByRole('radio', { name: /Yagi’s own Tailscale/ })).toBeChecked()
    expect(await field()).toHaveValue('yagi.tail1234.ts.net')
    expect(screen.getByLabelText('tuner address')).toHaveTextContent('yagi.tail1234.ts.net:5004')
    const chips = within(screen.getByRole('group', { name: 'Addresses for this way' }))
    expect(screen.getByRole('group', { name: 'Addresses for this way' })).toHaveTextContent('Yagi’s own Tailscale is at')
    expect(chips.getAllByRole('button').map((b) => b.textContent)).toEqual(['yagi.tail1234.ts.netname', '100.100.0.7address'])
    // nothing from this machine's own Tailscale or network
    expect(chips.queryByRole('button', { name: /100\.127\.0\.1|192\.168\.1\.5/ })).not.toBeInTheDocument()
  })

  it('says what is needed from the viewer, and that nothing is needed here, and has no firewall to set up', async () => {
    machine(hostTs)
    mount(node())
    const steps = block('How will they reach it?')
    expect(steps).toHaveTextContent('Yagi is on your tailnet as yagi.tail1234.ts.net (100.100.0.7)')
    expect(steps).toHaveTextContent('nothing needs installing or opening on this machine')
    expect(steps).toHaveTextContent('Machines → yagi → Share')
    expect(steps).not.toHaveTextContent(/Install Tailscale|tailscale ip -4/)
    expect(screen.queryByRole('region', { name: 'On this machine' })).not.toBeInTheDocument()
    expect(screen.queryByRole('switch', { name: /has a firewall/ })).not.toBeInTheDocument()
  })

  it('goes back to the machine’s own Tailscale, and its firewall, when that is chosen', async () => {
    machine(hostTs, hostLan)
    mount(node())
    const user = userEvent.setup()
    await user.click(ways().getByRole('radio', { name: /Tailscale on this machine/ }))
    expect(await field()).toHaveValue('100.127.0.1')
    expect(block('How will they reach it?')).toHaveTextContent('Tailscale is running on this machine (100.127.0.1)')
    expect(screen.getByRole('switch', { name: /has a firewall/ })).toBeInTheDocument()
  })

  it('is not the default, and not offered as ready, until it is signed in', async () => {
    machine(hostTs)
    mount(node({ state: 'needs-login', dnsName: undefined, addresses: undefined, serving: undefined }))
    expect(ways().getByRole('radio', { name: /Yagi’s own Tailscale/ })).toHaveTextContent('Built in. Sign it in first: Settings → Access')
    expect(ways().getByRole('radio', { name: /Tailscale on this machine/ })).toBeChecked() // the machine's own is the default meanwhile
    await waitFor(async () => expect(await field()).toHaveValue('100.127.0.1'))
    const user = userEvent.setup()
    await user.click(ways().getByRole('radio', { name: /Yagi’s own Tailscale/ }))
    expect(block('How will they reach it?')).toHaveTextContent('isn’t signed in yet. Sign it in under Settings → Access')
    expect(await field()).toHaveValue('') // not the browser's address: that would be the wrong one to give
    expect(screen.queryByLabelText('tuner address')).not.toBeInTheDocument()
  })

  it('says when it is not answering on this tuner’s port yet', async () => {
    machine(hostTs)
    mount(node({ serving: ['5999'] }))
    expect(block('How will they reach it?')).toHaveTextContent('isn’t answering on port 5004 on the tailnet yet')
  })

  it('is not offered when its switch is off, or when this build does not have it', async () => {
    machine(hostTs)
    const { unmount } = renderWithApp(<ViewerWizard open onOpenChange={vi.fn()} devices={node({}, false).devices} viewer={john} />, node({ state: 'off' }, false))
    await screen.findByRole('radiogroup', { name: 'Connection' })
    expect(screen.queryByRole('radio', { name: /Yagi’s own Tailscale/ })).not.toBeInTheDocument()
    expect(ways().getByRole('radio', { name: /^Tailscale/ })).toBeInTheDocument() // unlabelled "on this machine": there is only one
    unmount()
    const s = node()
    s.tailscale = { built: false, state: 'unavailable', hostname: 'yagi' }
    renderWithApp(<ViewerWizard open onOpenChange={vi.fn()} devices={s.devices} viewer={john} />, s)
    await screen.findByRole('radiogroup', { name: 'Connection' })
    expect(screen.queryByRole('radio', { name: /Yagi’s own Tailscale/ })).not.toBeInTheDocument()
  })
})

describe('ViewerWizard firewall detection', () => {
  const network = (firewall: unknown) => vi.stubGlobal('fetch', stubFetch({ 'GET /network': () => ({ body: { addresses: [], firewall } }) }).fn)
  const gate = () => screen.getByRole('switch', { name: /has a firewall/ })

  it('opens the firewall section with the right tool when one is switched on', async () => {
    network({ tool: 'firewalld', active: true })
    mount()
    expect(await screen.findByText('Yagi found firewalld switched on here.')).toBeInTheDocument()
    expect(gate()).toHaveAttribute('aria-checked', 'true')
    expect(within(screen.getByRole('radiogroup', { name: 'Firewall' })).getByRole('radio', { name: 'firewalld' })).toBeChecked()
    expect(screen.getByLabelText('firewall rules')).toHaveTextContent('firewall-cmd')
  })

  it('goes by ufw when that is what is on', async () => {
    network({ tool: 'ufw', active: true })
    mount()
    expect(await screen.findByText('Yagi found ufw switched on here.')).toBeInTheDocument()
    expect(screen.getByLabelText('firewall rules')).toHaveTextContent('sudo ufw allow from')
  })

  it("does not claim iptables is blocking anything, since its rules can't be read, but offers it if you say there is a firewall", async () => {
    network({ tool: 'iptables', active: false })
    mount()
    expect(await screen.findByText(/iptables looks to be in use here, but Yagi can't see its rules/)).toBeInTheDocument()
    expect(gate()).toHaveAttribute('aria-checked', 'false')
    expect(screen.queryByLabelText('firewall rules')).not.toBeInTheDocument()
    await userEvent.setup().click(gate())
    expect(within(screen.getByRole('radiogroup', { name: 'Firewall' })).getByRole('radio', { name: 'iptables' })).toBeChecked()
    expect(screen.getByLabelText('firewall rules')).toHaveTextContent('sudo iptables -I INPUT')
  })

  it('says nothing and stays closed when no firewall was found', async () => {
    network({ tool: '', active: false })
    mount()
    await new Promise((r) => setTimeout(r, 50))
    expect(gate()).toHaveAttribute('aria-checked', 'false')
    expect(screen.queryByText(/switched on here|looks to be in use/)).not.toBeInTheDocument()
  })

  it('lets you overrule what it found', async () => {
    network({ tool: 'ufw', active: true })
    mount()
    const user = userEvent.setup()
    await screen.findByText('Yagi found ufw switched on here.')
    await user.click(within(screen.getByRole('radiogroup', { name: 'Firewall' })).getByRole('radio', { name: 'iptables' }))
    expect(screen.getByLabelText('firewall rules')).toHaveTextContent('sudo iptables')
    await user.click(gate())
    expect(screen.queryByLabelText('firewall rules')).not.toBeInTheDocument()
  })

  it('still works when the server cannot say (an older server, or a failed request)', async () => {
    vi.stubGlobal('fetch', stubFetch({ 'GET /network': () => ({ body: { addresses: [] } }) }).fn)
    mount()
    await new Promise((r) => setTimeout(r, 50))
    expect(gate()).toHaveAttribute('aria-checked', 'false')
  })
})

describe('ViewerWizard: their media server', () => {
  async function toMediaServer(address = 'box.lan') {
    mount()
    const user = await typeHost(address)
    await next(user)
    return user
  }
  const media = () => block('In their media server')

  it('shows Plex steps first and the others on their tabs, each with the address', async () => {
    const user = await toMediaServer()
    expect(within(media()).getByRole('tab', { name: 'Plex' })).toHaveAttribute('aria-selected', 'true')
    expect(media()).toHaveTextContent('Enter box.lan:5004')

    await user.click(within(media()).getByRole('tab', { name: 'Jellyfin' }))
    expect(media()).toHaveTextContent('Dashboard → Live TV')
    expect(media()).toHaveTextContent('box.lan:5004')
    await user.click(within(media()).getByRole('tab', { name: 'Emby' }))
    expect(media()).toHaveTextContent('Emby Premiere')
  })

  it('shows the check commands for the address', async () => {
    await toMediaServer()
    expect(screen.getByLabelText('check commands')).toHaveTextContent('curl -s http://box.lan:5004/discover.json')
  })

  it('holds back the address-dependent steps until the address is usable', async () => {
    await toMediaServer('http://bad')
    expect(screen.queryByLabelText('check commands')).not.toBeInTheDocument()
    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
    expect(media()).toHaveTextContent(/Fix the address/)
  })

  it('shows each media server with its own icon, which comes from the app and not a CDN', async () => {
    await toMediaServer()
    const icons = Object.fromEntries(screen.getAllByRole('tab').map((t) => [t.textContent, t.querySelector('img')?.getAttribute('src')]))
    expect(icons).toEqual({ Plex: '/plex-icon.svg', Jellyfin: '/jellyfin-icon.svg', Emby: '/emby-icon.svg' })
    for (const src of Object.values(icons)) expect(src).not.toMatch(/^https?:/)
  })

  it('marks which tab is selected, and moves the mark when you pick another', async () => {
    const user = await toMediaServer()
    const selected = () => screen.getAllByRole('tab').filter((t) => t.hasAttribute('data-active')).map((t) => t.textContent)
    expect(selected()).toEqual(['Plex'])
    await user.click(screen.getByRole('tab', { name: 'Emby' }))
    expect(selected()).toEqual(['Emby'])
    expect(screen.getByRole('tab', { name: 'Emby' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tab', { name: 'Plex' })).toHaveAttribute('aria-selected', 'false')
  })

  it('keeps the tab names readable without the icons', async () => {
    await toMediaServer()
    for (const name of ['Plex', 'Jellyfin', 'Emby']) expect(screen.getByRole('tab', { name })).toBeInTheDocument()
  })
})
