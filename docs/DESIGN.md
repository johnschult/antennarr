# Antennarr: design notes

Status: design only, no code yet. Last updated 2026-10-17.

Antennarr is a self-hosted app for sharing an HDHomeRun tuner with remote Plex, Jellyfin, Channels or Emby servers, and for keeping track of who uses it.

## Problem

The setup it replaces is a friend's HDHomeRun shared with the user's Plex. It's a socat relay container on the friend's box plus iptables DNAT rules on the viewer's Unraid box. It breaks in the following ways:

- `lineup.json` hands out the tuner's own LAN IP, so the viewer side needs DNAT and MASQUERADE hacks.
- The tuner's DHCP address changes (.98 → .132), which means manual scans and edits in two places.
- Firewall and iptables rules vanish on reboot unless separately persisted.
- There's no visibility; you find out when a channel won't tune.

## Name

**Antennarr**: over-the-air TV plus the -arr family. It's deliberately not "Tunarr", which is an existing, popular project that also emulates an HDHomeRun. A quick search found no other "Antennarr"; check GitHub, Docker Hub and Unraid Community Apps before committing.

Logo direction: rabbit-ear antennas on a TV, with dot tips as a nod to Tailscale, without copying Tailscale's mark. The mockup has a placeholder.

## Core (works on any network)

- **Rewriting HTTP proxy.** It passes through `/discover.json`, `/lineup.json`, `/status.json` and the streams, rewriting `BaseURL`, `LineupURL` and each channel `URL` to point at Antennarr. Media servers only ever talk to Antennarr, so viewers need no iptables at all. The advertised address comes from the request's `Host` header by default, overridable with `ADVERTISE_URL`.
- **Auto-discovery by DeviceID.** UDP broadcast on 65001, falling back to a /24 `discover.json` scan. It follows the tuner when its DHCP address changes.
- **Any network.** Plain LAN, WireGuard or other VPNs, ZeroTier, Netbird, or Tailscale on the host. Networking is the user's job; Antennarr just listens on a port.
- **Optional built-in Tailscale (tsnet).** Set `TS_AUTHKEY` and Antennarr joins the tailnet as its own node. Then there are no host firewall rules, the sharer can share just that node, and viewer identity comes from WhoIs. The tsnet requirement makes the backend **Go**.

## Access control

- **Baseline: allowlist of IPs/CIDRs.** HDHomeRuns have no authentication of their own. Docs must warn against exposing Antennarr directly to the internet.
- **Tailscale mode:** tailnet ACLs plus WhoIs identity.
- **Plex (optional):**
  - Plex can't authenticate individual streams. Plex Media Server fetches tuner streams without a user token.
  - Dashboard sign-in can use the plex.tv PIN/OAuth flow.
  - **Plex-verified viewer enrollment:** the owner sends an invite link, and the viewer signs in with Plex and picks their server. Antennarr reads that server's addresses from plex.tv `resources` and allowlists them with the viewer's tuner limit, re-checking periodically.
  - Stretch: with consent, read the viewer's PMS `/status/sessions` to show which Plex user is watching.
- **Non-Plex viewers** use the IP or Tailscale allowlist.

## Tuner limits

- **Total shared cap.** At most N tuners in remote use across all viewers combined, regardless of how many tuners are idle. Viewer `TunerCount` = min(viewer cap, total cap). Settings shows "share up to N" or "keep N free" as the main choice, with the other under advanced.
- Limits work on tuner **counts, not specific tuners**. Local apps use `/auto/` and take any free tuner, so pinning viewers to `/tunerN/` isn't reliable.
- **Per-viewer maximum streams.** Over the limit, Antennarr refuses with HTTP 503, exactly what a busy HDHomeRun returns, so clients already handle it.
- **Per-viewer `TunerCount`** is rewritten in `discover.json`, so a DVR plans recordings around the limit.
- **Owner reserve:** N tuners always kept free for the sharer's own devices. It's checked against real usage in the tuner's `/status.json`, since local clients bypass the proxy.
- **Open question:** what happens when a remote recording collides with local use. Never cut off a running recording; the reserve handles most cases.

## Setup guide generator

Antennarr generates copy-paste commands for each viewer, tailored to their setup. It never runs anything on a viewer's machine.

- **Inputs:**
  - Viewer OS: Unraid, Debian/Ubuntu (ufw), RHEL/Fedora (firewalld), Synology.
  - Transport: built-in Tailscale, Tailscale on host, WireGuard/VPN, same LAN.
  - Media server: Plex, Jellyfin, Channels, Emby.
  - Whether an existing DVR is saved with the tuner's raw LAN IP.
- **Outputs, as tabbed steps with copy buttons:**
  1. **Your server.** Firewall allow rule for the viewer's address to the Antennarr port (firewalld rich rule or ufw), persistent. Skipped in tsnet mode, where you share the node instead.
  2. **Viewer's server.** Connectivity only: `tailscale ping`, WireGuard `AllowedIPs`, or nothing on a LAN. A **legacy DNAT fallback** appears only when the existing-DVR box is ticked. It's an idempotent iptables script plus a persistence method for each OS (Unraid User Script, iptables-persistent, a systemd oneshot unit, Synology Task Scheduler). The preferred fix is re-adding the tuner by the Antennarr address.
  3. **Media server.** Where to add the tuner by address in Plex, Jellyfin, Channels or Emby.
  4. **Verify.** `curl …/discover.json` and a short stream fetch, with the expected output shown.
  5. **Rollback.** Undo everything above.
- **"Viewer connected" check:** turns green once `discover.json` has been fetched from the viewer's allowlisted address.

## UI

Polish target: as good as Tracearr. We use the same kind of stack but write our own code and copy none of Tracearr's.

- React 19 + Vite + TypeScript; Tailwind v4 + shadcn/ui (Radix, cva, tailwind-merge); `lucide-react`, `sonner`, `next-themes`.
- Dark mode is the default.
- TanStack Query + Table; `react-hook-form` + `zod`.
- Live updates over WebSocket or SSE from the Go backend.
- Charts: shadcn charts (Recharts), not Highcharts, because of Highcharts' licensing.
- The SPA is embedded in the Go binary with `embed`, giving one container and one port.

**Chosen layout: "Control room"** (variant B in `mockup/index.html`):

- **Top bar:** a slim, sticky, translucent top bar with pill navigation (Live, Viewers, Setup, Tuner, Settings). Tuner name, IP and a live dot on the right.
- **Hero:** a short "On air now" summary ("2 of 4 tuners live", remote/local/reserved, total Mbps), with a 24h upload sparkline beside it.
- **One big tile per tuner:**
  - Live tiles show a large channel number, call sign and network, with a subtle scanline texture. Below that: who's watching and the bitrate. A red REC badge marks recordings.
  - Idle tiles are dashed and read "Ready" or "Held for your TVs" (owner reserve).
  - A badge on each tile says who holds it: viewer (cyan), local (violet), reserve (grey), free (green).
- **Below the tiles:** a Viewers list with per-viewer tuner-slot pips, and a vertical Timeline of events (started recording, limit hit, IP changed, tuner offline).
- **Background:** a radial teal glow at the top over near-black.
- **Shared pages:** Viewers table, Setup guide wizard, Tuner details + lineup, Settings (network mode cards, advertised URL, owner reserve slider, Plex link).

## Status data

Tuner health and last-seen IP; tuners in use by viewer vs. local; current channel per stream; bandwidth (MPEG-TS is roughly 8–19 Mbps per stream on the sharer's upload); events for limit hits, IP changes and offline periods.

## Notes

- Viewers add the tuner by address manually; Plex likely won't auto-discover it across networks.
- First target: replace the existing socat relay and viewer-side DNAT for the friend's tuner. Generalize after that.
