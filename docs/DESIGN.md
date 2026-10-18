# Antennarr: design notes

Status: working. The proxy, discovery, limits, several devices, the JSON API and the web UI are built and tested against a real FLEX 4K; see "Where things stand" at the end. Last updated 2026-10-17.

Antennarr is a self-hosted app for sharing an HDHomeRun tuner with remote Plex, Jellyfin, Channels or Emby servers, and for keeping track of who uses it.

## Problem

The setup it replaces is a friend's HDHomeRun shared with the user's Plex. It's a socat relay container on the friend's box plus iptables DNAT rules on the viewer's Unraid box. It breaks in the following ways:

- `lineup.json` hands out the tuner's own LAN IP, so the viewer side needs DNAT and MASQUERADE hacks.
- The tuner's DHCP address changes (.98 → .132), which means manual scans and edits in two places.
- Firewall and iptables rules vanish on reboot unless separately persisted.
- There's no visibility; you find out when a channel won't tune.

## Name

**Antennarr**: over-the-air TV plus the -arr family. It's deliberately not "Tunarr", which is an existing, popular project that also emulates an HDHomeRun. A quick search found no other "Antennarr"; check GitHub, Docker Hub and Unraid Community Apps before committing.

Logo direction: rabbit-ear antennas on a TV, with dot tips as a nod to Tailscale, without copying Tailscale's mark. The logos are in `docs/brand/`.

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

**Chosen layout: "Control room"** (chosen from a throwaway HTML mockup, since removed):

- **Top bar:** a slim, sticky, translucent top bar with pill navigation (Live, Viewers, Tuners, Settings). On the right, the signed-in account and a Live / Reconnecting pill for the page's connection. It says nothing about the tuners: there can be any number, so each has a header on Live (its name, the model beside a name you chose, how many are live, its address and a firmware flag) and a row on Tuners, and an offline one is a banner.
- **Hero:** a short "On air now" summary ("2 of 4 tuners live", remote/local/reserved, total Mbps), with a 24h upload sparkline beside it.
- **One big tile per tuner:**
  - Live tiles show a large channel number, call sign and network, with a subtle scanline texture. Below that: who's watching and the bitrate. A red REC badge marks recordings.
  - Idle tiles are dashed and read "Idle" or "Held for your TVs" (owner reserve).
  - A badge on each tile says who holds it: viewer (cyan), local (violet), reserve (gray), free (green).
- **Adding a viewer is a short dialog of three steps** (Who, Connect, Media server), not a separate Connect page. The first step saves the viewer; the next two write out how their server reaches this one and what to do in their media server, with this machine's addresses filled in. **Connection steps** on a viewer's row reopens it at the second step. `/setup` redirects to Viewers.
- **Below the tiles:** a Viewers list with per-viewer tuner-slot pips, and a vertical Timeline of events (started recording, limit hit, IP changed, tuner offline).
- **Background:** a radial teal glow at the top over near-black.
- **Shared pages:** Viewers table, Setup guide wizard, Tuner details + lineup, Settings (network mode cards, advertised URL, owner reserve slider, Plex link).

## Status data

Tuner health and last-seen IP; tuners in use by viewer vs. local; current channel per stream; bandwidth (MPEG-TS is roughly 8–19 Mbps per stream on the sharer's upload); events for limit hits, IP changes and offline periods.

## Several devices

Built. Devices are declared in the settings file (`devices`), each looking like its own HDHomeRun to media servers on its own port, with its own monitor following the tuner by DeviceID. Viewers and the allowlist are shared; stream counts, the shared cap and the owner reserve are per device (with per-device overrides of the defaults), and a viewer's limit can be overridden per device. Running with just `-tuner` or `-tuner-id` is the same thing with one device. A pool mode (several devices behind one virtual tuner, with failover) is still a later option; it needs the lineups to match.

A device's `name` is a stable key (settings, limits, URLs); what people see on the web page is its **display name** (`labels`), which changes live, or the model the tuner reports (HDHomeRun FLEX 4K) when it has none, shown once. The display name is local: media servers are always told the device, so it stays the same whatever the tuner is called here, and the model always shows up in Plex (where a DVR can be renamed on Plex's side).

## Firmware

Built. Antennarr tells you when SiliconDust has released firmware for a tuner, and installs it on request (below).

- Versions are dates (`20260326`), so comparing is plain string comparison. The tuner reports its own version (`FirmwareVersion`) and, if it knows of a newer one, `UpgradeAvailable`; when present, the tuner's own word wins.
- Otherwise Antennarr reads SiliconDust's public changelog page (HTML only, no feed) once every 12 hours with a conditional request. Nothing about you or your tuners is sent. The check can be switched off in Settings, and nothing is fetched while it is off.
- Not every release is for every model, and notes are scoped in prose ("ATSC3 models:", "HDHR5-2DT/4DT:", "All models:"). A release counts as **update available** only when a note in it is plainly for this model; if the only match is a scope that isn't recognized it is a **possible update**; `TECH`/`DEV` narrow a scope. If the model isn't recognized at all, every note is shown. If the page's layout changes, the parser fails loudly and the status is "unknown" with the reason, never a guess.
- New releases are announced once in the device's timeline; the Tuner page shows what changed, when the page was last read, and a Check now button.

### Installing firmware

Built. The Tuners page has an Install button when the check is certain there is a newer release (**update available**, not a possible one).

- `internal/hdhr` is a native client of the tuner's control protocol (TCP 65001), the one Silicondust's `hdhomerun_config` speaks, so nothing is shipped or run besides Antennarr. Frames are `type | length | payload | CRC-32`; an upgrade frees the tuners, sends the image in 1 KB frames with their offsets, then an end marker, and waits up to 40 seconds for the answer. `conformance_test.go` (opt in with `HDHOMERUN_CONFIG=<path to the official tool>`; needs port 65001 free) runs the official tool and this client against a recording fake and requires identical bytes. `live_test.go` (opt in with `HDHR_TUNER=<address>`) only reads from a real tuner.
- `internal/firmware` downloads the file from `https://download.silicondust.com/hdhomerun/<FirmwareName>_firmware_<date>.bin`, built from the tuner's own firmware family and the release the check found, never from anything a user types. That URL pattern is not documented by Silicondust. There is no published checksum or signature, so the checks are HTTPS, no redirects, status 200, `application/octet-stream`, and a size of 256 KiB to 16 MiB. A 404 is an error; no other address is guessed.
- `internal/upgrade` runs one install at a time per tuner in the background: downloading, sending, restarting, verifying. The server re-checks everything when the button is pressed (the version is the current release, the tuner is online, the check is on) and refuses with `409` if anything is in use unless the request says `interrupt`. It never retries a flash. It is done only when the tuner's control port reports the new version; if it comes back on the old one, or doesn't come back, it says so and tells you to power-cycle it.
- While an install runs the monitor does not call the tuner offline, send an offline message or start searching for it elsewhere (`Monitor.SetUpdating`, which expires after 30 minutes).
- The viewer proxy still answers `404` for the tuner's own upgrade path: this is an admin-only route.

## Device panel

Built. The Tuners page shows what each tuner says about itself: hardware model, firmware, ATSC 3.0 DRM version, device ID, MAC, IP, subnet, channel maps and modulations.

- `internal/tunerinfo` reads a fixed list of values (`/sys/model`, `/sys/hwmodel`, `/sys/version`, `/sys/features`) over the control port, and the tuner's System Menu page (`/system.html`) for what the control protocol doesn't have (DRM, MAC, subnet). It never sends a value to set, and no name comes from a request. The page is a plain HTML table with no JSON behind it, so a layout change leaves those fields out and the panel says why: it never guesses.
- It reads when the panel opens and when someone presses Refresh, not on a timer, and remembers each answer for 30 seconds.
- Admin-only, like the rest of the API. Not yet: reboot and rescan, which change the tuner and wait for their own confirmation and tests.

## Guide

Built. Live tiles and the Tuner page's table show each watched channel's logo, and the tiles show what is on.

- Source: SiliconDust's guide for the tuner, `https://api.hdhomerun.com/api/xmltv?DeviceAuth=<the tuner's own DeviceAuth>`, which the tuner reports in `discover.json`. About 3.5 MB, ~85 channels and three days of listings for a FLEX 4K. Read every 6 hours (conditional requests, size-capped, kept if a refresh fails). Devices that share a tuner share one fetch.
- Channels are matched to the tuner's lineup by `<lcn>` (the channel number, `11.1`), not by the guide's own ids.
- The DeviceAuth stays on the server: it is never in the API output or the logs (tested), and error text from failed fetches has the URL stripped.
- Logos are fetched by Antennarr, only from `img.hdhomerun.com`, only as plain images (SVG is refused), cached in memory, and served from `/antennarr/api/devices/{name}/guide/{channel}/icon`. The browser never contacts SiliconDust, so the strict CSP stays as it is.
- The switch is Settings > Updates > Show what's on (default on). While off, nothing is sent to SiliconDust. Turning it on, or a tuner appearing, fetches a guide that has never been read or has gone stale, and nothing else.
- Not done: a full guide page, and serving the guide to viewers (Jellyfin and Channels can read XMLTV; Plex gets its guide from Plex).

## Notifications

Built. Settings > Notifications. Messages to ntfy, Pushover or a webhook about firmware, tuner and viewer events, each switched on or off. Off until a channel is added, and nothing is contacted until then.

- `internal/notify` takes events from the places that already notice them and sends them from one goroutine through a small queue, so the code that noticed never waits on a network. There is no scheduler: firmware hangs off the existing 12-hour check (`announceFirmware`, which already announced once per release), tuner state off the monitor's online/offline change, and refusals off `Policy.note` and the unknown-address refusal.
- The settings are read on every event, so adding a channel or switching an event needs no restart.
- **Offline** is sent only if the tuner is still down after two minutes, and **back online** only if the offline message was sent, so a reboot, a blip, or Antennarr's own start says nothing. A restart doesn't repeat a firmware message either: the release last announced per tuner is kept in `<settings>.notified.json`.
- **Unknown addresses** are the noisy one and are off by default: once per address per day, at most six an hour in all, and an address held back by the hourly limit isn't marked as reported.
- Nothing is retried. A late, out-of-order message is worse than a missing one, and the log says what failed.
- **Secrets are write-only.** Tokens, user keys and webhook addresses are in the settings file (0600) and never in the API's output, the logs or error text (Go's own errors repeat the URL, so they are rewritten). A webhook is shown as its host. Redirects aren't followed, so a token can't be sent somewhere else.
- A channel can be tested before it is saved: Test sends the form's values, or a saved channel's values with the form's changes, and saves nothing.
- Not done: routing a kind of event to particular channels (every channel gets every enabled event), quiet hours, a daily digest, and more kinds of event. They would want a small scheduler of their own.

## What's new

Built. After an update, a dialog shows the release notes, like the "What's new" of any app, and *What's new* in the footer opens the current release's notes at any time.

- **Where the text comes from:** `CHANGELOG.md`, bundled into the web page by Vite (`?raw`, as a separate chunk that is only fetched when the dialog is needed), so the notes always match the version that is running, work offline, and nothing leaves the network. The `Dockerfile` copies the file into the web build stage; without it the build fails rather than shipping an empty dialog.
- **When it opens by itself:** the build is a release (not `dev`) and the browser last saw an older one, which it remembers in `localStorage` (`antennarr.lastSeenVersion`). It shows every release since, newest first, at most five. *Got it!*, the X and Escape all count as seen. A browser that has never been here records the version and shows nothing (a new install has no use for the history), and one that won't let the page remember anything never opens it by itself rather than at every visit. Going back to an older build is not an update.
- **How it is drawn:** a small parser for the changelog's own format (`lib/changelog.ts`) and a renderer for the few inline marks it uses (`components/inline-markdown.tsx`). The text becomes React elements, never HTML, and only `http` and `https` links are made, so nothing in a changelog can add markup or a `javascript:` link.
- It is per browser, not per install: each browser that opens the page after an update sees it once.

## Sign in with Plex

Built. Off by default; switched on in Settings > Access. An admin can open the web UI from a listed address (loopback or `admin` CIDRs) or by signing in with an allowed Plex account, so it works from anywhere the port is reachable.

- The server runs the plex.tv PIN flow. The Plex token never reaches the browser and is thrown away as soon as the account is identified. Only the numeric account ID and name are kept (`plex.admins` in the settings file).
- Sessions (idle 12h, at most 7 days) are tracked by an `antennarr_session` cookie: HttpOnly, SameSite=Strict, Secure when served over https. They are kept in a file beside the settings file (`<settings>.sessions.json`, mode 0600) so a restart doesn't sign anyone out; only a hash of each session ID is written, never the ID or the Plex token. Without a settings file they live in memory and a restart signs everyone out. Removing an account, or turning sign-in off, signs it out at once.
- Starting a sign-in is rate-limited per address, and pending sign-ins are capped. Nothing contacts plex.tv until someone signs in.
- The client ID is sent to plex.tv and appears in the sign-in link, so it isn't secret; it's what makes Antennarr show up under the account's authorised devices in Plex.
- Bootstrapping: from an admin address, turn the switch on, then "Add a Plex account" and sign in with it.

## Where things stand

Built and tested (Go, with the race detector; UI, with Vitest):

- the rewriting proxy, DeviceID discovery (UDP broadcast and an HTTP scan), the monitor and its events;
- the allowlist, per-viewer and per-device limits, the shared cap and the owner reserve, advertised `TunerCount`;
- several devices, display names, firmware checking;
- the settings file (atomic saves, reload on SIGHUP), the JSON API and the live event stream;
- the web UI: Live, Viewers, Tuner, Settings (grouped sections and rows), a light / dark / system theme, and a placeholder Setup page.

- sign in with Plex (below).

Next, roughly in this order. Each is a real piece of work, not a tweak:

1. **Plex-verified viewer enrollment** (later): an invite link leads to a Plex sign-in, and the viewer's server addresses are read from plex.tv and allowlisted.
2. **Tailscale, set up from the web UI.** An optional built-in tailnet node (tsnet): enable it, sign in with a link or an auth key, see the node's name and address, and have viewers identified by their tailnet identity instead of an IP. Needs no firewall rules on the host.
3. **Manage tuners from the UI.** Renaming, adding, removing and changing the port are done, live. Still to do: changing a tuner's address or DeviceID.
4. **The setup guide generator** (above): per-viewer commands for firewall, routing and the media server, with verify and rollback steps, replacing the placeholder Setup page.
5. **Cutover:** replace the socat relay and the viewer-side DNAT that serve a friend's tuner today.

## Notes

- Viewers add the tuner by address manually; Plex likely won't auto-discover it across networks.
- First target: replace the existing socat relay and viewer-side DNAT for the friend's tuner. Generalize after that.
