// Builds the setup guide: the address a viewer enters, the firewall rules for
// the machine running Antennarr, and the steps in each media server.
//
// Everything here is plain text for a person to copy. Anything that ends up
// inside a shell command is checked first (see validHost / validNetwork), so a
// typed address can never add a second command.

export type Transport = 'tailscale' | 'vpn' | 'lan' | 'internet'
export type Firewall = 'ufw' | 'firewalld' | 'iptables'
export type MediaServer = 'plex' | 'jellyfin' | 'emby'

export const TRANSPORTS: { id: Transport; label: string; hint: string }[] = [
  { id: 'tailscale', label: 'Tailscale', hint: 'Easiest: no ports opened to the internet' },
  { id: 'vpn', label: 'Other VPN', hint: 'WireGuard, ZeroTier, Netbird…' },
  { id: 'lan', label: 'Same network', hint: 'Both servers on one LAN' },
  { id: 'internet', label: 'Internet', hint: 'Not recommended' },
]

export const FIREWALLS: { id: Firewall; label: string }[] = [
  { id: 'ufw', label: 'ufw' },
  { id: 'firewalld', label: 'firewalld' },
  { id: 'iptables', label: 'iptables' },
]

/** `icon` is a file in the app's public folder. */
export const MEDIA_SERVERS: { id: MediaServer; label: string; icon: string }[] = [
  { id: 'plex', label: 'Plex', icon: 'plex-icon.svg' },
  { id: 'jellyfin', label: 'Jellyfin', icon: 'jellyfin-icon.svg' },
  { id: 'emby', label: 'Emby', icon: 'emby-icon.svg' },
]

/** The port from a listen address like ":5004" or "127.0.0.1:5004". */
export const portOf = (listen: string) => listen.slice(listen.lastIndexOf(':') + 1)

const HOST = /^(?=.{1,253}$)[A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?$/ // a name or an IPv4 address
const PORT = /^\d{1,5}$/
// An IP or CIDR as Antennarr stores it: hex digits, dots, colons and one slash.
const NETWORK = /^[0-9A-Fa-f:.]+(?:\/\d{1,3})?$/

export const validHost = (h: string) => HOST.test(h)
export const validPort = (p: string) => PORT.test(p) && Number(p) >= 1 && Number(p) <= 65535
export const validNetwork = (n: string) => NETWORK.test(n)

/** Why a host can't be used as the address viewers enter, or null if it can. */
export function hostProblem(host: string): string | null {
  const h = host.trim()
  if (h === '') return 'Enter the address viewers will use to reach this machine.'
  if (/^[a-z]+:\/\//i.test(h) || h.includes('/') || /\s/.test(h)) return 'Enter just the host name or IP address, without http:// or a path.'
  if (h.includes(':')) return 'Enter just the host name or IPv4 address; the port is added for you.'
  if (!validHost(h)) return "That doesn't look like a host name or IP address."
  if (h.toLowerCase() === 'localhost' || /^127\./.test(h) || h === '0.0.0.0') return `“${h}” only works on this machine. Use the address another computer would use to reach it.`
  return null
}

export const tunerAddress = (host: string, port: string) => `${host.trim()}:${port}`

export interface Rules {
  add: string[]
  undo: string[]
  /** Addresses left out because they weren't a plain IP or network. */
  skipped: string[]
}

/** Firewall rules letting the viewer's addresses reach the Antennarr port. */
export function firewallRules(fw: Firewall, port: string, networks: string[]): Rules {
  const rules: Rules = { add: [], undo: [], skipped: [] }
  if (!validPort(port)) return rules
  const ok = networks.filter((n) => {
    const good = validNetwork(n)
    if (!good) rules.skipped.push(n)
    return good
  })
  for (const n of ok) {
    const v6 = n.includes(':')
    switch (fw) {
      case 'ufw':
        rules.add.push(`sudo ufw allow from ${n} to any port ${port} proto tcp`)
        rules.undo.push(`sudo ufw delete allow from ${n} to any port ${port} proto tcp`)
        break
      case 'firewalld': {
        const rule = `rule family="${v6 ? 'ipv6' : 'ipv4'}" source address="${n}" port port="${port}" protocol="tcp" accept`
        rules.add.push(`sudo firewall-cmd --permanent --add-rich-rule='${rule}'`)
        rules.undo.push(`sudo firewall-cmd --permanent --remove-rich-rule='${rule}'`)
        break
      }
      case 'iptables': {
        const bin = v6 ? 'ip6tables' : 'iptables'
        rules.add.push(`sudo ${bin} -I INPUT -p tcp -s ${n} --dport ${port} -j ACCEPT`)
        rules.undo.push(`sudo ${bin} -D INPUT -p tcp -s ${n} --dport ${port} -j ACCEPT`)
        break
      }
    }
  }
  if (ok.length > 0) {
    if (fw === 'firewalld') {
      rules.add.push('sudo firewall-cmd --reload')
      rules.undo.push('sudo firewall-cmd --reload')
    }
    if (fw === 'iptables') {
      const save = '# These rules are lost on reboot unless you save them, e.g.: sudo netfilter-persistent save'
      rules.add.push(save)
      rules.undo.push(save)
    }
  }
  return rules
}

/**
 * What the viewer's side needs for the way the two servers will reach each other.
 * `tailscaleIP` is this machine's Tailscale address, if Antennarr found one:
 * then there is nothing to install on this side.
 */
export function transportSteps(t: Transport, tailscaleIP?: string): string[] {
  switch (t) {
    case 'tailscale':
      if (tailscaleIP) {
        return [
          `Tailscale is running on this machine (${tailscaleIP}). The viewer\u2019s media server needs Tailscale too, signed in to the same tailnet, or share this machine with their tailnet (Tailscale admin console → Machines → ⋯ → Share).`,
          'Make sure the viewer\u2019s Tailscale address (100.x.y.z) is one of their addresses in Antennarr (Viewers → Edit).',
        ]
      }
      return [
        'Install Tailscale on this machine and on the viewer\u2019s media server, signed in to the same tailnet. Or share this machine with their tailnet (Tailscale admin console → Machines → ⋯ → Share).',
        'On this machine, `tailscale ip -4` shows the address to give them.',
        'Make sure the viewer\u2019s Tailscale address (100.x.y.z) is one of their addresses in Antennarr (Viewers → Edit).',
      ]
    case 'vpn':
      return [
        'Connect the machine running Antennarr and the viewer\u2019s media server to the same VPN.',
        'Give the viewer this machine\u2019s address on the VPN (not its LAN address).',
        'The viewer\u2019s address on the VPN must be one of their addresses in Antennarr (Viewers → Edit).',
      ]
    case 'lan':
      return ['Nothing to set up: both servers are on the same network.', 'Give the viewer this machine\u2019s LAN address, and make sure the viewer\u2019s LAN address is one of their addresses in Antennarr (Viewers → Edit).']
    case 'internet':
      return [
        'Antennarr has no passwords: it only checks the address a request comes from. Anyone who can reach the port and is on the allowlist can use your tuner. A VPN or Tailscale is much safer.',
        'If you do this anyway, forward only the Antennarr port on your router to this machine, and allow only the viewer\u2019s public address in your firewall (see the firewall section below).',
        'Use your public address or a dynamic-DNS name as the address, and check it from outside your network.',
      ]
  }
}

/** The steps in a media server, with the address filled in. */
export function mediaServerSteps(m: MediaServer, address: string): string[] {
  switch (m) {
    case 'plex':
      return [
        'In Plex, open Settings → Live TV & DVR → Set Up Plex DVR. (Live TV needs Plex Pass.)',
        'Click “Don’t see your HDHomeRun device? Enter its network address manually”.',
        `Enter ${address} and continue, then scan for channels and match the guide.`,
        'If Plex already has this tuner under an older address, remove that DVR first so channels aren’t doubled.',
        'This must be reachable from the Plex server itself, not from the TV or phone watching it.',
      ]
    case 'jellyfin':
      return [
        'In Jellyfin, open Dashboard → Live TV → Tuner Devices → + (add).',
        'Choose HDHomeRun as the tuner type.',
        `Enter ${address} as the tuner address and save.`,
        'Add a guide data provider on the same page (XMLTV or Schedules Direct); the channels appear without one but have no program listings.',
      ]
    case 'emby':
      return [
        'In Emby, open Settings → Live TV → add a tuner device. (Live TV needs Emby Premiere.)',
        'Choose HDHomeRun as the device type.',
        `Enter ${address} as the tuner address and save, then pick a guide source.`,
      ]
  }
}

/** Commands to check the setup from the viewer's side. */
export function verifyCommands(address: string): string[] {
  return [`curl -s http://${address}/discover.json`, `curl -s http://${address}/lineup.json | head -c 400`]
}
