import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { john, makeState, makeTwoDeviceState, mom } from '@/test/fixtures'
import { renderWithApp, stubFetch } from '@/test/render'
import { SetupPage } from './setup'

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

const block = (name: string) => screen.getByRole('region', { name })
const host = () => screen.getByLabelText(/Address viewers will use/)

async function typeHost(value: string) {
  const user = userEvent.setup()
  await user.clear(host())
  if (value) await user.type(host(), value)
  return user
}

describe('SetupPage', () => {
  it('starts from the address in the browser, and warns that a local one will not work for others', () => {
    renderWithApp(<SetupPage />, makeState())
    expect(host()).toHaveValue(window.location.hostname)
    expect(within(block('1. The address')).getByRole('alert')).toHaveTextContent(/only works on this machine|Enter/)
  })

  it('shows the address a viewer enters, with the tuner port', async () => {
    renderWithApp(<SetupPage />, makeState())
    await typeHost('antenna.example.com')
    expect(screen.getByLabelText('tuner address')).toHaveTextContent('antenna.example.com:5004')
    expect(within(block('1. The address')).queryByRole('alert')).not.toBeInTheDocument()
  })

  it('lets you pick the tuner when there are several, and uses its port and name', async () => {
    const s = makeTwoDeviceState()
    s.devices[0].label = 'Living room antenna'
    renderWithApp(<SetupPage />, s)
    const user = await typeHost('box.lan')
    expect(screen.getByLabelText('tuner address')).toHaveTextContent('box.lan:5004')
    const choice = within(screen.getByRole('radiogroup', { name: 'Tuner' }))
    expect(choice.getByRole('radio', { name: /Living room antenna/ })).toBeChecked()
    await user.click(choice.getByRole('radio', { name: /cable/ }))
    expect(screen.getByLabelText('tuner address')).toHaveTextContent('box.lan:5005')
  })

  it('has no tuner choice with one tuner', () => {
    renderWithApp(<SetupPage />, makeState())
    expect(screen.queryByRole('radiogroup', { name: 'Tuner' })).not.toBeInTheDocument()
  })

  it('asks you to add a viewer first when there are none', async () => {
    renderWithApp(<SetupPage />, makeState({ viewers: [] }))
    expect(within(block('2. Who is it for?')).getByRole('link', { name: 'Add one' })).toHaveAttribute('href', '/viewers')
    const user = userEvent.setup()
    await user.click(screen.getByRole('switch', { name: /has a firewall/ }))
    expect(within(block('4. On this machine')).getByText('Add a viewer to get their firewall rules.')).toBeInTheDocument()
  })

  it('shows nothing about firewalls until you say there is one, and hides it again when you say there is not', async () => {
    renderWithApp(<SetupPage />, makeState())
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

  it("writes the chosen viewer's firewall rules, and switches firewall and viewer", async () => {
    renderWithApp(<SetupPage />, makeState({ viewers: [john, mom] }))
    const user = await typeHost('box.lan')
    await user.click(screen.getByRole('switch', { name: /has a firewall/ }))
    const fw = () => screen.getByLabelText('firewall rules')

    expect(fw()).toHaveTextContent(`sudo ufw allow from ${john.addresses[0]} to any port 5004 proto tcp`)
    await user.click(within(screen.getByRole('radiogroup', { name: 'Firewall' })).getByRole('radio', { name: 'iptables' }))
    expect(fw()).toHaveTextContent(`sudo iptables -I INPUT -p tcp -s ${john.addresses[0]} --dport 5004 -j ACCEPT`)
    await user.click(within(screen.getByRole('radiogroup', { name: 'Viewer' })).getByRole('radio', { name: /Mom/ }))
    expect(fw()).toHaveTextContent(`-s ${mom.addresses[0]} `)
    expect(fw()).not.toHaveTextContent(john.addresses[0])
    expect(screen.getByLabelText('undo firewall rules')).toHaveTextContent('iptables -D INPUT')
  })

  it('changes the connection steps with the way they will connect', async () => {
    renderWithApp(<SetupPage />, makeState())
    const user = await typeHost('box.lan')
    const steps = () => block('3. How will they reach it?')
    expect(steps()).toHaveTextContent(/tailscale ip -4/)
    await user.click(within(steps()).getByRole('radio', { name: /Internet/ }))
    expect(steps()).toHaveTextContent(/no passwords/)
    await user.click(within(steps()).getByRole('radio', { name: /Same network/ }))
    expect(steps()).toHaveTextContent(/both servers are on the same network/)
  })

  it('shows Plex steps first and the others on their tabs, each with the address', async () => {
    renderWithApp(<SetupPage />, makeState())
    const user = await typeHost('box.lan')
    const media = () => block('5. In their media server')
    expect(within(media()).getByRole('tab', { name: 'Plex' })).toHaveAttribute('aria-selected', 'true')
    expect(media()).toHaveTextContent('Enter box.lan:5004')

    await user.click(within(media()).getByRole('tab', { name: 'Jellyfin' }))
    expect(media()).toHaveTextContent('Dashboard → Live TV')
    expect(media()).toHaveTextContent('box.lan:5004')
    await user.click(within(media()).getByRole('tab', { name: 'Emby' }))
    expect(media()).toHaveTextContent('Emby Premiere')
  })

  it('shows the check commands for the address', async () => {
    renderWithApp(<SetupPage />, makeState())
    await typeHost('box.lan')
    expect(screen.getByLabelText('check commands')).toHaveTextContent('curl -s http://box.lan:5004/discover.json')
  })

  it('holds back the address-dependent steps until the address is usable', async () => {
    renderWithApp(<SetupPage />, makeState())
    await typeHost('http://bad')
    expect(screen.queryByLabelText('tuner address')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('check commands')).not.toBeInTheDocument()
    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
    expect(block('5. In their media server')).toHaveTextContent(/Fix the address/)
  })

  it('does not repeat an unusable address in the connection steps', async () => {
    renderWithApp(<SetupPage />, makeState())
    await typeHost('localhost')
    expect(block('3. How will they reach it?')).not.toHaveTextContent('localhost')
  })

  it('never puts a typed address into a command', async () => {
    renderWithApp(<SetupPage />, makeState())
    await typeHost('box.lan; rm -rf ~')
    const where = [...document.querySelectorAll('*')].filter((e) => e.children.length === 0 && e.textContent?.includes('rm -rf')).map((e) => e.outerHTML)
    expect(where).toEqual([])
  })

  it('copies a command block', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    renderWithApp(<SetupPage />, makeState())
    const user = await typeHost('box.lan')
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true }) // after setup(), which installs its own
    await user.click(screen.getByRole('button', { name: 'Copy tuner address' }))
    expect(writeText).toHaveBeenCalledWith('box.lan:5004')
    expect(await screen.findByText('Copied')).toBeInTheDocument()
  })

  it('says so when copying is not allowed', async () => {
    renderWithApp(<SetupPage />, makeState())
    const user = await typeHost('box.lan')
    Object.defineProperty(navigator, 'clipboard', { value: { writeText: vi.fn().mockRejectedValue(new Error('no')) }, configurable: true })
    await user.click(screen.getByRole('button', { name: 'Copy tuner address' }))
    expect(await screen.findByText(/Couldn't copy/)).toBeInTheDocument()
  })
})

describe('SetupPage on a machine that knows its own addresses', () => {
  const ts = { ip: '100.64.0.9', kind: 'tailscale', interface: 'tailscale0' }
  const lan = { ip: '192.168.1.5', kind: 'lan', interface: 'eth0' }
  const machine = (...addresses: unknown[]) => vi.stubGlobal('fetch', stubFetch({ 'GET /network': () => ({ body: { addresses } }) }).fn)
  const field = async () => (await screen.findByLabelText(/Address viewers will use/)) as HTMLInputElement

  it("fills in the machine's Tailscale address, and says Tailscale is already running", async () => {
    machine(ts, lan)
    renderWithApp(<SetupPage />, makeState())
    await screen.findByRole('group', { name: 'Addresses of this machine' })
    expect(await field()).toHaveValue('100.64.0.9')
    expect(screen.getByLabelText('tuner address')).toHaveTextContent('100.64.0.9:5004')
    expect(within(block('1. The address')).queryByRole('alert')).not.toBeInTheDocument()
    const steps = block('3. How will they reach it?')
    expect(steps).toHaveTextContent('Tailscale is running on this machine (100.64.0.9)')
    expect(steps).not.toHaveTextContent(/Install Tailscale|tailscale ip -4|tailscale ping/) // nothing to install or look up: we know it
    expect(within(steps).getByRole('radio', { name: /Tailscale/ })).toHaveTextContent('Running on this machine (100.64.0.9)')
  })

  it('offers each address, and clicking one uses it', async () => {
    machine(ts, lan)
    renderWithApp(<SetupPage />, makeState())
    const user = userEvent.setup()
    const chips = within(await screen.findByRole('group', { name: 'Addresses of this machine' }))
    expect(chips.getByRole('button', { name: /100\.64\.0\.9.*Tailscale/ })).toHaveAttribute('aria-pressed', 'true')
    expect(chips.getByRole('button', { name: /192\.168\.1\.5.*LAN/ })).toHaveAttribute('aria-pressed', 'false')

    await user.click(chips.getByRole('button', { name: /192\.168\.1\.5/ }))
    expect(await field()).toHaveValue('192.168.1.5')
    expect(chips.getByRole('button', { name: /192\.168\.1\.5/ })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByLabelText('tuner address')).toHaveTextContent('192.168.1.5:5004')
  })

  it('follows the connection you choose until you type an address of your own', async () => {
    machine(ts, lan)
    renderWithApp(<SetupPage />, makeState())
    const user = userEvent.setup()
    await screen.findByRole('group', { name: 'Addresses of this machine' })
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
    renderWithApp(<SetupPage />, makeState())
    expect(await screen.findByText(/Tailscale wasn't found running on this machine/)).toBeInTheDocument()
    expect(await field()).toHaveValue(window.location.hostname)
    expect(block('3. How will they reach it?')).toHaveTextContent('Install Tailscale') // so the longer steps are still there
  })

  it('does not show the long Tailscale steps while it is still checking', async () => {
    let release: (v: Response) => void = () => {}
    vi.stubGlobal('fetch', () => new Promise<Response>((r) => (release = r)))
    renderWithApp(<SetupPage />, makeState())
    await screen.findByLabelText(/Address viewers will use/)
    expect(block('3. How will they reach it?')).not.toHaveTextContent(/Install Tailscale|Tailscale is running/)
    release(new Response(JSON.stringify({ addresses: [ts] })))
    expect(await within(block('3. How will they reach it?')).findByText(/Tailscale is running on this machine/)).toBeInTheDocument()
  })

  it('does not claim anything when nothing could be read', async () => {
    vi.stubGlobal('fetch', stubFetch({ 'GET /network': () => ({ status: 500, body: { error: 'no' } }) }).fn)
    renderWithApp(<SetupPage />, makeState())
    expect(await field()).toHaveValue(window.location.hostname)
    await new Promise((r) => setTimeout(r, 50))
    expect(screen.queryByRole('group', { name: 'Addresses of this machine' })).not.toBeInTheDocument()
    expect(screen.queryByText(/wasn't found running/)).not.toBeInTheDocument()
  })

  it('leaves the connection hint alone when there is no Tailscale here', async () => {
    machine(lan)
    renderWithApp(<SetupPage />, makeState())
    await screen.findByRole('group', { name: 'Addresses of this machine' })
    expect(screen.getByRole('radio', { name: /Tailscale/ })).not.toHaveTextContent('Running on this machine')
  })
})
