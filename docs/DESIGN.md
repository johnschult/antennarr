# Yagi: design notes

Status: working. The proxy, discovery, limits, several devices, the JSON API and the web UI are built and tested against a real FLEX 4K; see "Where things stand" at the end. Last updated 2026-10-17.

Yagi is a self-hosted app for sharing an HDHomeRun tuner with remote Plex, Jellyfin, Channels or Emby servers, and for keeping track of who uses it.

## Problem

The setup it replaces is a friend's HDHomeRun shared with the user's Plex. It's a socat relay container on the friend's box plus iptables DNAT rules on the viewer's Unraid box. It breaks in the following ways:

- `lineup.json` hands out the tuner's own LAN IP, so the viewer side needs DNAT and MASQUERADE hacks.
- The tuner's DHCP address changes (.98 → .132), which means manual scans and edits in two places.
- Firewall and iptables rules vanish on reboot unless separately persisted.
- There's no visibility; you find out when a channel won't tune.

## Name

**Yagi**, after the Yagi–Uda antenna: the rooftop TV antenna, a boom with a row of rods across it, named for Hidetsugu Yagi and Shintaro Uda, who developed it in the 1920s. The program does something similar for a tuner: it takes a signal that only reaches one place and points it at the people you choose. It was called Antennarr until October 2026; [the FAQ](faq.md#why-is-it-called-yagi) tells the story and how to say it.

Logo: Lucide's `antenna` icon (ISC licence) in teal on a dark tile; the SVG and PNGs are in `docs/brand/`. It replaced an earlier retro-TV-with-rabbit-ears mark.

## Core (works on any network)

- **Rewriting HTTP proxy.** It passes through `/discover.json`, `/lineup.json`, `/status.json` and the streams, rewriting `BaseURL`, `LineupURL` and each channel `URL` to point at Yagi. Media servers only ever talk to Yagi, so viewers need no iptables at all. The advertised address comes from the request's `Host` header by default, overridable with `ADVERTISE_URL`.
- **Auto-discovery by DeviceID.** UDP broadcast on 65001, falling back to a /24 `discover.json` scan. It follows the tuner when its DHCP address changes.
- **Any network.** Plain LAN, WireGuard or other VPNs, ZeroTier, Netbird, or Tailscale on the host. Networking is the user's job; Yagi just listens on a port.
- **Optional built-in Tailscale (tsnet).** Set `TS_AUTHKEY` and Yagi joins the tailnet as its own node. Then there are no host firewall rules, the sharer can share just that node, and viewer identity comes from WhoIs. The tsnet requirement makes the backend **Go**.

## Access control

- **Baseline: allowlist of IPs/CIDRs.** HDHomeRuns have no authentication of their own. Docs must warn against exposing Yagi directly to the internet.
- **Tailscale mode:** tailnet ACLs plus WhoIs identity.
- **Plex (optional):**
  - Plex can't authenticate individual streams. Plex Media Server fetches tuner streams without a user token.
  - Dashboard sign-in can use the plex.tv PIN/OAuth flow.
  - **Plex-verified viewer enrollment:** the owner sends an invite link, and the viewer signs in with Plex and picks their server. Yagi reads that server's addresses from plex.tv `resources` and allowlists them with the viewer's tuner limit, re-checking periodically.
  - Stretch: with consent, read the viewer's PMS `/status/sessions` to show which Plex user is watching.
- **Non-Plex viewers** use the IP or Tailscale allowlist.

## Tuner limits

- **Total shared cap.** At most N tuners in remote use across all viewers combined, regardless of how many tuners are idle. Viewer `TunerCount` = min(viewer cap, total cap). Settings shows "share up to N" or "keep N free" as the main choice, with the other under advanced.
- Limits work on tuner **counts, not specific tuners**. Local apps use `/auto/` and take any free tuner, so pinning viewers to `/tunerN/` isn't reliable.
- **Per-viewer maximum streams.** Over the limit, Yagi refuses with HTTP 503, exactly what a busy HDHomeRun returns, so clients already handle it.
- **Per-viewer `TunerCount`** is rewritten in `discover.json`, so a DVR plans recordings around the limit.
- **Owner reserve:** N tuners always kept free for the sharer's own devices. It's checked against real usage in the tuner's `/status.json`, since local clients bypass the proxy.
- **Open question:** what happens when a remote recording collides with local use. Never cut off a running recording; the reserve handles most cases.

## Setup guide generator

Yagi generates copy-paste commands for each viewer, tailored to their setup. It never runs anything on a viewer's machine.

- **Inputs:**
  - Viewer OS: Unraid, Debian/Ubuntu (ufw), RHEL/Fedora (firewalld), Synology.
  - Transport: built-in Tailscale, Tailscale on host, WireGuard/VPN, same LAN.
  - Media server: Plex, Jellyfin, Channels, Emby.
  - Whether an existing DVR is saved with the tuner's raw LAN IP.
- **Outputs, as tabbed steps with copy buttons:**
  1. **Your server.** Firewall allow rule for the viewer's address to the Yagi port (firewalld rich rule or ufw), persistent. Skipped in tsnet mode, where you share the node instead.
  2. **Viewer's server.** Connectivity only: `tailscale ping`, WireGuard `AllowedIPs`, or nothing on a LAN. A **legacy DNAT fallback** appears only when the existing-DVR box is ticked. It's an idempotent iptables script plus a persistence method for each OS (Unraid User Script, iptables-persistent, a systemd oneshot unit, Synology Task Scheduler). The preferred fix is re-adding the tuner by the Yagi address.
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

**The layout: "Control room"** (chosen from a throwaway HTML mockup, since removed):

- **Top bar:** a slim, sticky, translucent top bar. The logo and name link to Live (the version is in the footer, linked to its release notes, since the wordmark is big enough to cover it), and then pill navigation (Live, Viewers, Tuners, Activity, Settings; Activity is left out while the activity history is switched off). On the right, the signed-in account and a Live / Reconnecting pill for the page's connection, with a dot that pulses while connected. It says nothing about the tuners: there can be any number, so each has a header on Live (its name, the model beside a name you chose, how many are live, its address and a firmware flag) and a row on Tuners, and an offline one is a banner.
- **Summary:** "On the air now": how many tuners are live, then remote, local and held-for-you counts and the total Mbps.
- **One tile per tuner:**
  - A live tile has the program's picture faded behind its top, with the tuner's number, who holds it (a pill with an icon), the large channel number, its name and logo. Below: the program's title, category, episode and description, and a progress bar with how long is left. Then two small charts, the bitrate (with its rate) and the signal quality (with its percentage), and a footer with where the stream is going and the signal strength drawn as bars.
  - Idle tiles are dashed and read "Idle" or "Held for your TVs" (owner reserve).
  - The pill says who holds the tile: a viewer (cyan, with a globe), a local device (violet, with a house), the reserve (amber, with a crown), or free (green).
  - Logos are outlined only where the artwork needs it to stay visible (a dark outline around white artwork in the light theme, a light one around dark artwork in the dark theme), decided from the logo's own pixels. There is no box around them.
- **Adding a viewer is a short dialog of three steps** (Who, Connect, Media server), not a separate Connect page. The first step saves the viewer; the next two write out how their server reaches this one and what to do in their media server, with this machine's addresses filled in. **Connection steps** on a viewer's row reopens it at the second step. `/setup` redirects to Viewers.
- **Below the tiles:** a Viewers list with per-viewer tuner-slot pips, and a vertical Timeline of events (started recording, limit hit, IP changed, tuner offline).
- **Theme:** the ExploreMiam theme from tweakcn (indigo and emerald), light and dark, with the theme's own page background. The fonts are our own and are bundled with the app (Open Sans, Mozilla Headline for the wordmark, JetBrains Mono), so the page loads nothing from other sites.
- **Other pages:** Viewers (table, each name opening that viewer's page), Tuners (a list of rows, each opening that tuner's page: Overview, Channels, Device, Firmware and Settings as tabs with their own addresses, `/tuners/<name>/<tab>`; a single tuner opens its page directly, and the page's own *Tuners* link asks for the list with `?list` so another can be added), Activity (below), Settings (four tabs with their own addresses, `/settings/<tab>`: Access, with the limits, admin addresses and Plex sign-in; Notifications; Privacy, the switches for what it reads and keeps; and Logs).

## Status data

Tuner health and last-seen IP; tuners in use by viewer vs. local; current channel per stream; bandwidth (MPEG-TS is roughly 8–19 Mbps per stream on the sharer's upload); events for limit hits, IP changes and offline periods.

## Several devices

Built. Devices are declared in the settings file (`devices`), each looking like its own HDHomeRun to media servers on its own port, with its own monitor following the tuner by DeviceID. Viewers and the allowlist are shared; stream counts, the shared cap and the owner reserve are per device (with per-device overrides of the defaults), and a viewer's limit can be overridden per device. Running with just `-tuner` or `-tuner-id` is the same thing with one device. A pool mode (several devices behind one virtual tuner, with failover) is still a later option; it needs the lineups to match.

A device's `name` is a stable key (settings, limits, URLs); what people see on the web page is its **display name** (`labels`), which changes live, or the model the tuner reports (HDHomeRun FLEX 4K) when it has none, shown once. The display name is local: media servers are always told the device, so it stays the same whatever the tuner is called here, and the model always shows up in Plex (where a DVR can be renamed on Plex's side).

## Firmware

Built. Yagi tells you when SiliconDust has released firmware for a tuner, and installs it on request (below).

- Versions are dates (`20260326`), so comparing is plain string comparison. The tuner reports its own version (`FirmwareVersion`) and, if it knows of a newer one, `UpgradeAvailable`; when present, the tuner's own word wins.
- Otherwise Yagi reads SiliconDust's public changelog page (HTML only, no feed) once every 12 hours with a conditional request. Nothing about you or your tuners is sent. The check can be switched off in Settings, and nothing is fetched while it is off.
- Not every release is for every model, and notes are scoped in prose ("ATSC3 models:", "HDHR5-2DT/4DT:", "All models:"). A release counts as **update available** only when a note in it is plainly for this model; if the only match is a scope that isn't recognized it is a **possible update**; `TECH`/`DEV` narrow a scope. If the model isn't recognized at all, every note is shown. If the page's layout changes, the parser fails loudly and the status is "unknown" with the reason, never a guess.
- New releases are announced once in the device's timeline; the Tuner page shows what changed, when the page was last read, and a Check now button.

### Installing firmware

Built. A tuner's Firmware tab has an Install button when the check is certain there is a newer release (**update available**, not a possible one).

- `internal/hdhr` is a native client of the tuner's control protocol (TCP 65001), the one Silicondust's `hdhomerun_config` speaks, so nothing is shipped or run besides Yagi. Frames are `type | length | payload | CRC-32`; an upgrade frees the tuners, sends the image in 1 KB frames with their offsets, then an end marker, and waits up to 40 seconds for the answer. `conformance_test.go` (opt in with `HDHOMERUN_CONFIG=<path to the official tool>`; needs port 65001 free) runs the official tool and this client against a recording fake and requires identical bytes. `live_test.go` (opt in with `HDHR_TUNER=<address>`) only reads from a real tuner.
- `internal/firmware` downloads the file from `https://download.silicondust.com/hdhomerun/<FirmwareName>_firmware_<date>.bin`, built from the tuner's own firmware family and the release the check found, never from anything a user types. That URL pattern is not documented by Silicondust. There is no published checksum or signature, so the checks are HTTPS, no redirects, status 200, `application/octet-stream`, and a size of 256 KiB to 16 MiB. A 404 is an error; no other address is guessed.
- `internal/upgrade` runs one install at a time per tuner in the background: downloading, sending, restarting, verifying. The server re-checks everything when the button is pressed (the version is the current release, the tuner is online, the check is on) and refuses with `409` if anything is in use unless the request says `interrupt`. It never retries a flash. It is done only when the tuner's control port reports the new version; if it comes back on the old one, or doesn't come back, it says so and tells you to power-cycle it.
- While an install runs the monitor does not call the tuner offline, send an offline message or start searching for it elsewhere (`Monitor.SetUpdating`, which expires after 30 minutes).
- The viewer proxy still answers `404` for the tuner's own upgrade path: this is an admin-only route.

## Device panel

Built. A tuner's Device tab shows what it says about itself: hardware model, firmware, ATSC 3.0 DRM version, device ID, MAC, IP, subnet, channel maps and modulations.

- `internal/tunerinfo` reads a fixed list of values (`/sys/model`, `/sys/hwmodel`, `/sys/version`, `/sys/features`) over the control port, and the tuner's System Menu page (`/system.html`) for what the control protocol doesn't have (DRM, MAC, subnet). It never sends a value to set, and no name comes from a request. The page is a plain HTML table with no JSON behind it, so a layout change leaves those fields out and the panel says why: it never guesses.
- It reads when the panel opens and when someone presses Refresh, not on a timer, and remembers each answer for 30 seconds.
- Admin-only, like the rest of the API.

### Channels and scanning

Built. A tuner's Channels tab lists the channels it has found and sets each one to enabled, favorite or disabled, and scans for channels, the same things the tuner's own Channel Lineup page does.

- `internal/lineup` reads `lineup.json?show=found` and `lineup_status.json` and writes through `POST /lineup.post`: `favorite=` with `-`, `+` or `x` and the channel number (normal, favorite, disabled) and `scan=start` (with a source) or `abort`. A channel with no `Enabled` flag is enabled; `Subscribed: 0` is one the tuner has but can't play, which can't be changed.
- Nothing odd is sent: a channel number has to look like one (digits, perhaps a dot and more), the state has to be one of the three, a scan source has to be one the tuner lists, and the API only changes a channel the tuner itself lists. After a change it asks the tuner again and says so if it didn't take. The viewer proxy still answers `404` for `lineup.post`: this is an admin-only route.
- A scan takes all of the tuner's tuners, so the API refuses while anything is in use unless the request says `interrupt`, as for a restart. The page asks again every two seconds while a scan runs, for its progress, and says once when it ends.
- Where a channel has been watched, the page shows its average signal quality from the activity history (30 days, weighted by how long). The tuner's lineup has no signal information; only a tuner that is tuned reports it.
- **Testing a channel's signal** (the gauge button on a channel) tunes a free tuner to it for a few seconds and reads its status (`ss`, `snq`, `seq`, `lock`), the one thing the lineup doesn't have. It is the only place Yagi tunes a tuner, so it is careful: it only uses a tuner that has no lock, no channel and no target (checking again at the tuner, not just from the status page), takes it with a lock key of its own the way `hdhomerun_config` does (a random key, sent with every change from then on) so nothing else can retune it, picks the highest free tuner so a new stream still gets the lowest, gives the channel and the lock back however the test ends (including when the request is cancelled), and runs one at a time per tuner and never while it scans, restarts or updates. It never interrupts a stream: with every tuner in use it simply says so. While it runs the tuner shows briefly as in use; a session that short isn't kept in the activity history.

### Restarting a tuner

Built. A Restart row in the Device panel, behind a confirmation that says what it will cut off.

- `internal/restart` sends `set /sys/restart self` (the command SiliconDust documents for `hdhomerun_config`; a tuner lists `/sys/restart` in `get help`). It then checks that the tuner stopped answering and that it came back, because a tuner that kept answering did not restart. A tuner that never returns is reported with what to check. It never repeats a restart by itself.
- The server refuses when the tuner is offline, being updated or already restarting, and, if anything is in use, unless the request says `interrupt`. While it runs the monitor does not call the tuner offline (`Monitor.SetRestarting`, which expires after 10 minutes).
- Not yet: rescanning channels, which is deferred.

## Guide

Built. Live tiles and the Tuner page's table show each watched channel's logo, and the tiles show what is on.

- Source: SiliconDust's guide for the tuner, `https://api.hdhomerun.com/api/xmltv?DeviceAuth=<the tuner's own DeviceAuth>`, which the tuner reports in `discover.json`. About 3.5 MB, ~85 channels and three days of listings for a FLEX 4K. Read every 6 hours (conditional requests, size-capped, kept if a refresh fails). Devices that share a tuner share one fetch.
- Channels are matched to the tuner's lineup by `<lcn>` (the channel number, `11.1`), not by the guide's own ids.
- The DeviceAuth stays on the server: it is never in the API output or the logs (tested), and error text from failed fetches has the URL stripped.
- Logos are fetched by Yagi, only from `img.hdhomerun.com`, only as plain images (SVG is refused), cached in memory, and served from `/api/devices/{name}/guide/{channel}/icon`. The browser never contacts SiliconDust, so the strict CSP stays as it is.
- The switch is Settings → Privacy → Show what's on (default on). While off, nothing is sent to SiliconDust. Turning it on, or a tuner appearing, fetches a guide that has never been read or has gone stale, and nothing else.
- **What's on, in more detail.** Each program also carries its description (cut to 400 characters), its first category, its on-screen episode number (`S01E32`, never the guide's own program ID) and a picture. The tile draws the picture faded behind its top part, with how far into the program it is and how long is left. In the guide a picture is 16:9 (the feed says 360x270, but the files are 480x270), and every program in a real FLEX 4K's guide had one; a tile still works without.
- **Pictures** are fetched the same way as logos, only from `img.hdhomerun.com`, only as plain images, up to 1 MB each, with at most 120 kept in memory. A request names a picture by a `token` (the picture's file name from the guide) and Yagi looks the program up by it, so no address from a request is ever fetched. The picture a token stands for never changes, which is why the browser may keep it.
- **About a channel.** The guide also says things about a channel as a whole, for every channel it lists, watched or not, and the info popover on a tuner's Channels tab shows them: the **network** (a channel's names run call sign, number and call sign, number and name, number, name and, only when it differs from the name, the network; it is read from after the number's own entry, never by position alone), the **languages** of its programs (the `<language>` of each; those with at least a tenth of the programs, so a stray one doesn't make a channel bilingual) and its **categories** (its programs' `<category>` words, counted without regard to case, top five, "Series" left out since it says how a show is made, not what it is about). Nothing is inferred and nothing is taken from a country: what the guide doesn't say is left out. The popover credits SiliconDust. The flag beside a language is only a picture to make it quick to see.
- Not done: a full guide page, and serving the guide to viewers (Jellyfin and Channels can read XMLTV; Plex gets its guide from Plex).

## Activity

Built. The Activity page shows who watched what and for how long, for the last 7 or 30 days: totals, a bar for each day, and the sessions grouped by day.

- **A session** is one tuner on one channel for one viewer, from when it was first seen to when it was last seen. It is recorded (`internal/history`) by looking at the tuners every poll: a stream through Yagi is paired with its viewer by channel, as the Live page does, and anything else in use is the owner's own device straight from the tuner, which never passes through Yagi. A change of channel or viewer starts a new session. Anything shorter than 15 seconds is flicking through channels and isn't kept; the program titles the guide named while it ran are kept in order (up to six).
- **Data and reception.** Each session also keeps about how much data the tuner sent (the bitrate it reported, over the time it was watched, counting only gaps of a minute or less) and the average signal quality it reported. Data through Yagi is what went out over the sharer's upload; an owner's own device streams straight from the tuner, so its data is shown as staying on their network. The page totals both by viewer and by channel (with the average reception, colored when it is fair or poor), and each viewer has a page of their own (`/viewers/<name>`, from the Viewers page or the Activity page) with their data and time for each day, what they watched and their sessions.
- **Kept for 30 days**, one JSON line per session, in `<settings>.history.jsonl` beside the settings file (and in memory only when there isn't one). It is written as each session ends, and a session going when Yagi stops is closed at that moment; one lost to a crash is lost. Old ones are dropped on start and hourly.
- **What it stores:** the viewer's name, the channel and the show titles. It does not store a viewer's address; for the owner's own device it stores the address on the owner's own network, so two TVs can be told apart.
- **It can be switched off** (Settings → Privacy → Keep activity history, default on). Off stops recording at once (a session going ends there), hides the Activity item in the nav, and closes the API route. What was kept stays until someone presses *Clear activity history*, which forgets it all, on or off.
- Not yet: filtering by viewer or tuner, and exporting.

## Notifications

Built. Settings > Notifications. Messages to ntfy, Pushover or a webhook about firmware, tuner and viewer events, each switched on or off. Off until a channel is added, and nothing is contacted until then.

- `internal/notify` takes events from the places that already notice them and sends them from one goroutine through a small queue, so the code that noticed never waits on a network. There is no scheduler: firmware hangs off the existing 12-hour check (`announceFirmware`, which already announced once per release), tuner state off the monitor's online/offline change, and refusals off `Policy.note` and the unknown-address refusal. A viewer starting to watch (`watching`, off by default) comes from `Policy.Stream` when a tuner is given, at most once every half hour per viewer and tuner; it says the channel number when the request names one, not a channel's name, which isn't known until the tuner has tuned.
- The settings are read on every event, so adding a channel or switching an event needs no restart.
- **Offline** is sent only if the tuner is still down after two minutes, and **back online** only if the offline message was sent, so a reboot, a blip, or Yagi's own start says nothing. A restart doesn't repeat a firmware message either: the release last announced per tuner is kept in `<settings>.notified.json`.
- **Unknown addresses** are the noisy one and are off by default: once per address per day, at most six an hour in all, and an address held back by the hourly limit isn't marked as reported.
- Nothing is retried. A late, out-of-order message is worse than a missing one, and the log says what failed.
- **Secrets are write-only.** Tokens, user keys and webhook addresses are in the settings file (0600) and never in the API's output, the logs or error text (Go's own errors repeat the URL, so they are rewritten). A webhook is shown as its host. Redirects aren't followed, so a token can't be sent somewhere else.
- A channel can be tested before it is saved: Test sends the form's values, or a saved channel's values with the form's changes, and saves nothing.
- Not done: routing a kind of event to particular channels (every channel gets every enabled event), quiet hours, a daily digest, and more kinds of event. They would want a small scheduler of their own.

## Logs

Built. The **Logs** page shows Yagi's own log, for when something is wrong and you don't have a shell.

- `internal/logbuf` wraps the logger Yagi already has. Every line still goes to standard error as before, and a copy (time, level, message, and its attributes flattened to `key=value`, groups as dotted keys) goes into a ring of the last 2,000 lines in memory. Nothing is written to disk, so a restart starts it over; `docker logs` is still the history.
- **Secrets are hidden before a line is kept**: an attribute whose name looks like a token, secret, password, key, authorization, cookie or lock key is replaced, and `token=…`, `DeviceAuth=…`, `key=…` and bearer tokens inside a message or value are too. Lines and values are cut at 2,000 and 500 characters. The rest of the code already keeps secrets out of its logs (tested); this is a second line of defence for the one place where logs leave the process.
- `GET logs` is admin only like the rest of the API and takes a level, a search, a number to continue from and a limit. The page asks again every few seconds while Live is on; it does not stream.
- **The level is a setting** (`logLevel`, default `info`, not written when it is the default): `slog.LevelVar` is the logger's level, and the policy tells a callback each new set of settings (now, on an edit, and on a reload of the file), which moves it. So changing it on the page, editing the file and reloading, and starting with it set all do the same thing. Debug lines are few: the guide fetch, a stream being given to a viewer, a tuner check failing, a device being built.

## What's new

Built. After an update, a dialog shows the release notes, like the "What's new" of any app, and *What's new* in the footer opens the current release's notes at any time.

- **Where the text comes from:** `CHANGELOG.md`, bundled into the web page by Vite (`?raw`, as a separate chunk that is only fetched when the dialog is needed), so the notes always match the version that is running, work offline, and nothing leaves the network. The `Dockerfile` copies the file into the web build stage; without it the build fails rather than shipping an empty dialog.
- **When it opens by itself:** the build is a release (not `dev`) and the browser last saw an older one, which it remembers in `localStorage` (`yagi.lastSeenVersion`). It shows every release since, newest first, at most five. *Got it!*, the X and Escape all count as seen. A browser that has never been here records the version and shows nothing (a new install has no use for the history), and one that won't let the page remember anything never opens it by itself rather than at every visit. Going back to an older build is not an update.
- **How it is drawn:** a small parser for the changelog's own format (`lib/changelog.ts`) and a renderer for the few inline marks it uses (`components/inline-markdown.tsx`). The text becomes React elements, never HTML, and only `http` and `https` links are made, so nothing in a changelog can add markup or a `javascript:` link.
- It is per browser, not per install: each browser that opens the page after an update sees it once.

## Sign in with Plex

Built. Off by default; switched on in Settings > Access. An admin can open the web UI from a listed address (loopback or `admin` CIDRs) or by signing in with an allowed Plex account, so it works from anywhere the port is reachable.

- The server runs the plex.tv PIN flow. The Plex token never reaches the browser and is thrown away as soon as the account is identified. Only the numeric account ID and name are kept (`plex.admins` in the settings file).
- Sessions (idle 12h, at most 7 days) are tracked by an `yagi_session` cookie: HttpOnly, SameSite=Strict, Secure when served over https. They are kept in a file beside the settings file (`<settings>.sessions.json`, mode 0600) so a restart doesn't sign anyone out; only a hash of each session ID is written, never the ID or the Plex token. Without a settings file they live in memory and a restart signs everyone out. Removing an account, or turning sign-in off, signs it out at once.
- Starting a sign-in is rate-limited per address, and pending sign-ins are capped. Nothing contacts plex.tv until someone signs in.
- The client ID is sent to plex.tv and appears in the sign-in link, so it isn't secret; it's what makes Yagi show up under the account's authorised devices in Plex.
- Bootstrapping: from an admin address, turn the switch on, then "Add a Plex account" and sign in with it.

## Where things stand

Built and tested (Go, with the race detector; UI, with Vitest):

- the rewriting proxy, DeviceID discovery (UDP broadcast and an HTTP scan), the monitor and its events;
- the allowlist, per-viewer and per-device limits, the shared cap and the owner reserve, advertised `TunerCount`;
- several devices, display names, firmware checking and installing, a Device panel, restarting a tuner;
- the settings file (atomic saves, reload on SIGHUP), the JSON API and the live event stream;
- the web UI: Live (with the program guide's pictures and details), Viewers, Tuners, Activity, Settings (grouped sections and rows), light and dark themes, the add-viewer wizard;
- notifications, the What's new popup, and the activity history (below).

Not built yet: rescanning channels from the UI, filtering and exporting the activity history, Plex-verified viewer enrollment, built-in Tailscale, and editing a tuner's address from the UI.

- sign in with Plex (below).

Next, roughly in this order. Each is a real piece of work, not a tweak:

1. **Plex-verified viewer enrollment** (later): an invite link leads to a Plex sign-in, and the viewer's server addresses are read from plex.tv and allowlisted.
2. **Tailscale, set up from the web UI.** An optional built-in tailnet node (tsnet). The plan is in [Built-in Tailscale](#built-in-tailscale-plan) below.
3. **Manage tuners from the UI.** Renaming, adding, removing and changing the port are done, live. Still to do: changing a tuner's address or DeviceID.
4. **The setup guide generator** (above): per-viewer commands for firewall, routing and the media server, with verify and rollback steps, replacing the placeholder Setup page.
5. **Cutover:** replace the socat relay and the viewer-side DNAT that serve a friend's tuner today.

## Built-in Tailscale (plan)

An optional node inside Yagi (Tailscale's `tsnet` package), so the machine needs no Tailscale install and no host firewall rules, the sharer can share just that node, and viewers can be recognised by their tailnet identity.

**Decisions made**

- **Two images, chosen in compose.** The Tailscale code sits behind a Go build tag (`tsnet`), so the normal binary and image stay free of third-party dependencies. The release builds `ghcr.io/johnschult/yagi:<version>` as now and `…:<version>-tailscale`, plus a rolling `:tailscale`. Each has its own CI build and vulnerability scan.
- **Signing in.** Settings shows the Tailscale sign-in link while the node isn't enrolled, and a `TS_AUTHKEY` (ideally tagged) also works for unattended setups. State is kept in `/config/tailscale`.
- **LAN ports.** The tailnet listeners are added beside the LAN ones. A *tailnet only* switch turns the LAN ones off, leaving loopback so the machine's own browser still reaches the admin page.

**Phases**

1. **Spike.** Add the dependency behind the build tag. Measure the binary size, the vulnerability scan and the Go version it needs, and decide whether to go on.
2. **The node.** Settings (on/off, hostname, default `yagi`, optional tag), starting and stopping it, the sign-in link, and its name and addresses in the state.
3. **Listeners.** The admin port and each tuner port are also served on the tailnet, which means a device's HTTP server takes more than one listener. The *tailnet only* switch. The separate admin port lets the admin pages be tailnet-only while the tuner ports are shared. **Done** (3a): a listener on the node for each port, answered by the same handler as the LAN one (`internal/server/tailserve.go`), kept in line every two seconds with the node (started, stopped or replaced), and with tuners added, moved and removed; the settings row says which ports. The Connect step in viewer setup offers the node as its own way, apart from a Tailscale on the host, with only that way's addresses listed. **Done** (3b): *Only on the tailnet*. It is a gate on the LAN listeners (`internal/server/lanonly.go`), not closed ports: a connection from anywhere but loopback is dropped as it arrives while the setting is on **and** the node is connected. Ports stay bound, so a tuner keeps its port and the switch is instant either way. It can only be turned on while the node is connected, it fails open when the node is down (an outage never takes the admin page with it), and `-allow-lan` / `YAGI_ALLOW_LAN` is the way back in. The page asks first and warns if it was opened at a LAN address. Not done: cutting connections already open when it goes on.
4. **Identity.** Ask Tailscale who a caller is (their login and device) and let a viewer be matched by tailnet user or device as well as by address. An admin can be a listed tailnet user.
5. **TLS.** HTTPS for the admin page when the tailnet has HTTPS certificates enabled; opt-in, because the machine name then appears in a public certificate log. Tuner ports stay HTTP: Plex expects it, and the tailnet already encrypts the traffic.
6. **Docs.** Setup, sharing the node with a viewer's tailnet, turning off key expiry for the node, tags and access controls, a compose example for the `-tailscale` image.

**Risks**

- Whether a viewer on another tailnet can use the HTTPS name of a shared node is not covered by Tailscale's documentation. Test it in the spike.
- Our tests use a fake listener provider, not a tailnet. Real behaviour needs a manual test with a real viewer.
- A large dependency tree means a vulnerability in it becomes ours to update. Only the Tailscale build carries it.

## Notes

- Viewers add the tuner by address manually; Plex likely won't auto-discover it across networks.
- First target: replace the existing socat relay and viewer-side DNAT for the friend's tuner. Generalize after that.
