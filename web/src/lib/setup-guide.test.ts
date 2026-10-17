import { describe, expect, it } from 'vitest'
import {
  firewallRules,
  hostProblem,
  mediaServerSteps,
  portOf,
  transportSteps,
  tunerAddress,
  validHost,
  validNetwork,
  validPort,
  verifyCommands,
} from './setup-guide'

describe('addresses', () => {
  it('takes the port from a listen address', () => {
    expect(portOf(':5004')).toBe('5004')
    expect(portOf('127.0.0.1:5005')).toBe('5005')
  })

  it('joins host and port, trimming the host', () => {
    expect(tunerAddress('  antenna.example.com ', '5004')).toBe('antenna.example.com:5004')
  })

  it('accepts names and IPv4 addresses', () => {
    for (const h of ['192.168.68.72', 'antenna', 'antenna.tail1234.ts.net', 'my-box.lan']) expect(hostProblem(h)).toBeNull()
  })

  it('explains what is wrong with an address', () => {
    expect(hostProblem('')).toMatch(/Enter the address/)
    expect(hostProblem('http://antenna')).toMatch(/without http/)
    expect(hostProblem('antenna/path')).toMatch(/without http/)
    expect(hostProblem('two words')).toMatch(/without http/)
    expect(hostProblem('antenna:5004')).toMatch(/port is added/)
    expect(hostProblem('-bad-')).toMatch(/doesn't look like/)
    for (const h of ['localhost', 'LOCALHOST', '127.0.0.1', '127.1.2.3', '0.0.0.0']) expect(hostProblem(h)).toMatch(/only works on this machine/)
  })

  it('validates ports and networks', () => {
    expect(['1', '5004', '65535'].every(validPort)).toBe(true)
    expect(['', '0', '65536', '50x4', '-1', '5004; rm'].some(validPort)).toBe(false)
    expect(['10.0.0.1', '10.8.0.0/24', '100.64.0.0/10', 'fd7a:115c::1', 'fe80::/64'].every(validNetwork)).toBe(true)
    expect(['', '10.0.0.1; reboot', '$(id)', '10.0.0.1 -j DROP', "1.2.3.4'"].some(validNetwork)).toBe(false)
    expect(validHost('a.b')).toBe(true)
  })
})

describe('firewall rules', () => {
  it('ufw: one rule per address, with a matching undo', () => {
    const r = firewallRules('ufw', '5004', ['10.0.0.5', '10.8.0.0/24'])
    expect(r.add).toEqual(['sudo ufw allow from 10.0.0.5 to any port 5004 proto tcp', 'sudo ufw allow from 10.8.0.0/24 to any port 5004 proto tcp'])
    expect(r.undo).toEqual(['sudo ufw delete allow from 10.0.0.5 to any port 5004 proto tcp', 'sudo ufw delete allow from 10.8.0.0/24 to any port 5004 proto tcp'])
  })

  it('firewalld: permanent rich rules, then a reload, in both directions', () => {
    const r = firewallRules('firewalld', '5004', ['10.0.0.5', 'fd7a::1'])
    expect(r.add[0]).toBe(`sudo firewall-cmd --permanent --add-rich-rule='rule family="ipv4" source address="10.0.0.5" port port="5004" protocol="tcp" accept'`)
    expect(r.add[1]).toContain('family="ipv6"')
    expect(r.add.at(-1)).toBe('sudo firewall-cmd --reload')
    expect(r.undo[0]).toContain('--remove-rich-rule')
    expect(r.undo.at(-1)).toBe('sudo firewall-cmd --reload')
  })

  it('iptables: uses ip6tables for IPv6, inserts and deletes the same rule, and warns about reboots', () => {
    const r = firewallRules('iptables', '5004', ['10.0.0.5', 'fd7a::1'])
    expect(r.add[0]).toBe('sudo iptables -I INPUT -p tcp -s 10.0.0.5 --dport 5004 -j ACCEPT')
    expect(r.add[1]).toBe('sudo ip6tables -I INPUT -p tcp -s fd7a::1 --dport 5004 -j ACCEPT')
    expect(r.undo[0]).toBe('sudo iptables -D INPUT -p tcp -s 10.0.0.5 --dport 5004 -j ACCEPT')
    expect(r.add.at(-1)).toMatch(/lost on reboot/)
  })

  it('never turns an odd address into a command', () => {
    const r = firewallRules('ufw', '5004', ['10.0.0.5', '1.2.3.4; reboot', '$(id)'])
    expect(r.add).toEqual(['sudo ufw allow from 10.0.0.5 to any port 5004 proto tcp'])
    expect(r.skipped).toEqual(['1.2.3.4; reboot', '$(id)'])
    expect(r.add.join('\n')).not.toMatch(/reboot|\$\(/)
  })

  it('writes nothing without addresses or with a bad port', () => {
    expect(firewallRules('ufw', '5004', [])).toEqual({ add: [], undo: [], skipped: [] })
    expect(firewallRules('firewalld', '5004', []).add).toEqual([]) // not even a lone reload
    expect(firewallRules('ufw', '5004; x', ['10.0.0.5']).add).toEqual([])
  })
})

describe('steps', () => {
  it('puts the address in every media server', () => {
    for (const m of ['plex', 'jellyfin', 'emby'] as const) {
      expect(mediaServerSteps(m, 'antenna:5004').join(' ')).toContain('antenna:5004')
    }
  })

  it("tells Plex users the server, not the TV, must reach it, and that Live TV needs Plex Pass", () => {
    const s = mediaServerSteps('plex', 'a:1').join(' ')
    expect(s).toMatch(/Plex server itself/)
    expect(s).toMatch(/Plex Pass/)
  })

  it('says each transport differently, and warns about the internet one', () => {
    const all = (['tailscale', 'vpn', 'lan', 'internet'] as const).map((t) => transportSteps(t, 'a:1').join(' '))
    expect(new Set(all).size).toBe(4)
    expect(all[0]).toMatch(/Tailscale/)
    expect(all[3]).toMatch(/no passwords/)
  })

  it('checks the address with curl', () => {
    expect(verifyCommands('a:1')).toEqual(['curl -s http://a:1/discover.json', 'curl -s http://a:1/lineup.json | head -c 400'])
  })
})
