import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { john, makeState, makeTwoDeviceState, mom } from '@/test/fixtures'
import { renderWithApp } from '@/test/render'
import { SetupPage } from './setup'

afterEach(() => vi.restoreAllMocks())

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
    expect(block('3. How will they reach it?')).toHaveTextContent('<address>')
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
