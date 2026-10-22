# API

Yagi has two kinds of HTTP interface:

- the **tuner side**, which looks like an HDHomeRun to media servers (and is what viewers use);
- the **admin API**, a small JSON API under `/api/` that the web page uses. You can script against it too.

The admin API is for Yagi's own web page first, so it can change between releases. The release notes mention anything that does.

## The tuner side

Every device listens on its own port (`listen`) and answers, for each viewer, like an HDHomeRun:

| Path | Answer |
|---|---|
| `GET /discover.json` | The tuner's own details, with `BaseURL` and `LineupURL` pointing at Yagi, `TunerCount` set to the viewer's number, and `FriendlyName` and `DeviceID` replaced if you set a display name or `reportId`. |
| `GET /lineup.json` | The channel list, with each channel's `URL` pointing at Yagi. |
| `GET /status.json` | The tuner's own tuner status, passed through. |
| `GET /auto/vX.Y`, `/tunerN/vX.Y` | A stream, counted against the viewer's limits. |
| anything else | Passed through to the tuner. |

Requests from an address that isn't a viewer get `403` with `X-Yagi-Reason: not-allowed`. A stream that is refused gets `503` with the reason in `X-Yagi-Reason`:
`viewer-limit`, `shared-cap`, `owner-reserve` or `tuner-offline`. See [Configuration](configuration.md#how-the-limits-work).

## The admin API

All paths are under `/api/` and need admin access (see [Security](security.md)). Without it they answer `403` with `{"error": "admin only"}`.
Replies are JSON and never cached. An error is `{"error": "a message safe to show a person"}` with a suitable status.

**Writes** (`POST`, `PUT`, `PATCH`, `DELETE`) must send `Content-Type: application/json` when they have a body, and an `Origin` header, if sent, must be the host you are talking to.
Bodies are limited to 64 KB, and a field the endpoint doesn't know is an error. If Yagi couldn't create its settings file there is nowhere to save, so writes fail with a message saying so.

| Request | What it does |
|---|---|
| `GET state` | Everything the web page shows, in one document (below). |
| `GET events` | The same document, pushed as a server-sent event (`event: state`) each time something changes, with a comment line every 20 seconds to keep the connection open. |
| `GET viewers` | The viewers, with their current use and when they were last seen. |
| `POST viewers` | Add a viewer (`400` if an address is a public internet address: Yagi serves private networks only): `{"name": "Mom", "addresses": ["100.64.0.7"], "limit": 1}`, and optionally `"limits": {"device": 2}`. Replies `201` with the viewer. |
| `PUT viewers/{name}` | Replace a viewer (the same body). |
| `DELETE viewers/{name}` | Remove a viewer, `204`. Removing the last viewer leaves nobody able to use the tuner. The `mode` in the settings is then `no-viewers`. |
| `GET invites` | The invites, without their links: `[{"id", "name", "limit", "limits", "created", "expires", "used", "viewer", "expired"}]`. |
| `POST invites` | Make an invite for a person to enroll as a viewer by signing in with Plex: `{"name": "Dad", "limit": 2}`, and optionally `"limits": {"device": 1}`. Replies `201` with the invite and `path` and `url`, **the only time its link is shown**: it is not saved, only a hash of it. `409` unless inviting is on (`plexEnrollment`), if there are 50 invites already, or if a viewer or another invite has the name; `400` for a bad name, limit or device. |
| `DELETE invites/{id}` | Revoke an invite, `204`. Its link stops working at once. A viewer made from it stays, but can no longer bring their addresses up to date by opening it again. Deleting a viewer also revokes the invite they came in by. `404` if there is none. |
| `GET settings` | The general settings. |
| `PATCH settings` | Change any of `sharedCap`, `ownerReserve`, `admin` (a list), `firmwareCheck`, `guide`, `history`, `logLevel` (`debug`, `info`, `warn` or `error`), `plexEnabled`, `plexEnrollment` (people may enroll with an invite link; needs `plexEnabled` too), `tailscaleEnabled`, `tailscaleHostname` (`""` is the default, `yagi`), `tailnetOnly` (refuse the LAN while the node is connected; `409` unless it is), `tailscaleHttps`, `notifyEvents` (`{"firmware": false, "unknown": true}`: any of `firmware`, `tuner`, `viewer`, `watching`, `unknown`; `firmware: true` is refused with `400` while `firmwareCheck` is off). Only the fields you send change. |
| `POST notifications/channels` | Add a channel: `{"name", "type": "ntfy"\|"pushover"\|"webhook", "url", "topic", "token", "user"}`. Replies `201` with the channel, without its secrets. `400` with the reason if it isn't valid; `409` without a settings file. |
| `PUT notifications/channels/{id}` | Change a channel. A `token`, `user` or (for a webhook) `url` left empty keeps the saved one; `"clearToken": true` removes the token. Secrets aren't carried to a different `type`. `404` if there is no such channel. |
| `DELETE notifications/channels/{id}` | Remove a channel, `204`. |
| `POST notifications/test` | Send one message to a channel and say whether it got through: `{"id": "…"}` for a saved one, `{"channel": {…}}` for one that isn't saved yet, or both (the saved channel with those changes, saving nothing). Replies `{"ok": true}`, or `502` with why not. `429` if that channel was tested a moment ago. |
| `POST discover` | Search the network for HDHomeRun tuners (takes about 3 seconds). Replies `{"tuners": [{"id", "address", "tuners", "added", "label", "name"}]}`; `added` is the device a tuner already is, or `""`, and `label` and `name` are a display name and a device name to offer for it (such as `HDHomeRun FLEX 4K` and `flex-4k-87`), made from what the tuner calls itself. `429` while another search is running. |
| `POST devices` | Start sharing a tuner: `{"tuner": "192.168.1.50"}`, optionally with `"id"`, `"name"`, `"label"` (the display name, saved with it) and `"listen"`. The name (`tuner`, then `tuner-<id>`), the port and the DeviceID are chosen if left out. Replies `201` with the device. `422` if nothing answers at the address, `409` if that tuner is already added, the port can't be used, there is no settings file, or the tuner came from command-line flags. |
| `DELETE devices/{name}` | Stop sharing a tuner, `204`. Streams on it are cut, its port is freed, and its label and any per-device viewer limits are removed from the settings. `409` in the same cases as adding. |
| `PATCH devices/{name}` | Change a device's `label` (display name; `""` clears it), `listen` (move it to another port, e.g. `":5010"`; the new port is opened first, so one that is taken is refused with `409` and nothing changes; streams on the old port are cut and the page itself is no longer served there), `ownerReserve` or `sharedCap`. |
| `GET devices/{name}/guide/{channel}/icon` | A channel's logo (a PNG), fetched by Yagi, `404` when there isn't one. |
| `GET devices/{name}/guide/{channel}/poster/{token}` | A program's picture, fetched by Yagi. `token` is the `poster` of a program in the guide (`devices[].guide[channel].now`): it only says which of the channel's programs is meant, and the address comes from the guide, never the request. `404` when there is no such picture or the guide is off. The picture at a token doesn't change, so it is cached for a day. |
| `POST firmware/check` | Read the firmware changelog now. `429` if it was read a moment ago. |
| `GET devices/{name}/channels` | The channels the tuner has found, and how its scan stands: `{channels: [{number, name, hd, video, audio, drm, state, subscribed, about}], scan: {inProgress, possible, source, sources, progress, found}}`. `state` is `enabled`, `favorite` or `disabled`; `subscribed: false` is a channel the tuner has but can't play. `about` is what the program guide says about the channel as a whole, and is left out when the guide is off or doesn't have the channel: `{network, languages, categories}`. `network` is the network it carries when that differs from its name (ABC for WSB-HD); `languages` are the guide's codes (`en`, `es`) for languages of at least a tenth of its programs, most first; `categories` are up to five of what its programs are, most common first, without "Series". Anything the guide doesn't say is left out, and nothing is taken from a country. Read from the tuner each time (the guide is kept). `409` offline, `502` the tuner couldn't be read. |
| `PUT devices/{name}/channels/{number}` | Set a channel's state: `{"state": "disabled"}` (or `enabled`, `favorite`). Only a channel the tuner lists, and only one it can play (`404`, `409`); a bad number or state is `400`. The change is checked by asking the tuner again (`502` if it didn't take). A disabled channel is left out of the lineup viewers get. |
| `POST devices/{name}/channels/{number}/test` | Test a channel's signal: tunes a free tuner to it for a few seconds and reads how well it is received, then gives the tuner back. Answers `200` with `{tuner, channel, locked, lock, strength, quality, symbol}` (percentages; `locked: false` is "no signal"), after about ten seconds. It never interrupts anything: `409` when every tuner is in use (or turned out to be), while a test is already running, while the tuner scans, restarts or updates, and for a channel the tuner can't play; `404` for a channel it doesn't have. |
| `POST devices/{name}/channel-scan` | Scan for channels: `{"source": "Antenna", "interrupt": false}` (`source` defaults to the tuner's current one). `202`. A scan takes all the tuners, so `409` while any is in use unless `interrupt` is `true`, and while a scan is running, a restart or an install is under way, or the tuner can't scan. `DELETE` on the same path stops a scan (`204`). |
| `GET logs` | The recent log lines, for the Logs page: `{entries, latest, kept, capacity, level}`, oldest first. An entry is `{seq, time, level, message, attrs: [{key, value}]}`; `seq` counts up for as long as Yagi runs. `?level=` (`debug`, `info`, `warn`, `error`) keeps that level and above, `?q=` keeps lines containing the text (in the message or a detail, any case), `?since=N` keeps lines after number N, and `?limit=` (default 500, at most 2000) keeps the newest that many. `level` is the level being logged at. Admin only, and anything that looks like a secret is hidden before a line is kept. `DELETE logs` forgets what is kept (`204`). `404` when there is no log. |
| `GET history` | Who watched what, for the Activity page: `{days, now, sessions}`. `?days=N` (1 to 30, default 7) says how far back. `404` while the `history` setting is off. `DELETE history` forgets everything that has been kept (`204`), and works while it is off. A session is one tuner on one channel for one viewer: `device`, `tuner`, `channel`, `channelName`, `kind` (`viewer`, `remote` or `local`), `viewer`, `client` (a local device's address), `start`, `end`, `titles` (the programs that were on), `bytes` (about how much the tuner sent for it, from the bitrate it reported), `quality` (the average signal quality it reported, when it did), and `open` for one still going. Oldest first. |
| `GET devices/{name}/info` | What the tuner says about itself, for the Device panel: `model`, `hardwareModel`, `version`, `deviceId`, `drm`, `mac`, `ip`, `subnet`, `channelMaps`, `modulations`, `readAt`, and `problems` (what couldn't be read, in words). Read-only, from a fixed list of values on the tuner's control port and its System Menu page; remembered for 30 seconds, `?refresh=1` reads again. `404` no such tuner, `409` offline, `502` nothing could be read. |
| `POST devices/{name}/restart` | Restart the tuner (`set /sys/restart self`, as SiliconDust documents): `{"interrupt": false}`. Answers `202` and runs in the background; progress is `devices[].restart` (`phase`: `restarting`, `done`, `failed`; `error`). Refused with `404` (no such device) or `409` (offline, being updated, already restarting, or streams or recordings in use and `interrupt` is not `true`). While it runs the tuner's `restarting` is `true` and it is not reported offline. `DELETE` on the same path dismisses a finished result. |
| `POST devices/{name}/firmware/upgrade` | Install the firmware release the check found: `{"version": "20260326", "interrupt": false}`. Answers `202` and runs in the background; progress is `devices[].upgrade` (`phase`: `downloading`, `sending`, `restarting`, `verifying`, `done`, `failed`; `percent`; `version`; `error`). Refused with `404` (no such device), `400` (nothing installable), or `409` (no update available, the check is off, already running, tuner offline, `version` is not the current release, or streams or recordings are in use and `interrupt` is not `true`). While it runs the tuner's `updating` is `true` and it is not reported offline. |
| `DELETE devices/{name}/firmware/upgrade` | Dismiss a finished install's result. |
| `GET network` | This machine's Tailscale and LAN addresses, and which firewall looks switched on, for the connect step of adding a viewer. |
| `DELETE plex/admins/{id}` | Stop a Plex account from signing in. |

### The state document

`GET state` returns:

```json
{
  "time": "2026-10-17T20:00:00Z",
  "devices": [ { "name": "antenna", "label": "…", "listen": ":5004", "tuner": { "online": true, "id": "10C03B06", "baseURL": "http://192.168.1.50", "tunerCount": 4, "slots": [ … ], "events": [ … ], "updating": false, "restarting": false }, "streams": [ … ], "summary": { … }, "firmware": { … }, "upgrade": { … }, "restart": { … }, "guide": { … } } ],
  "viewers": [ … ],
  "settings": { "mode": "enforced", "sharedCap": 0, "ownerReserve": 1, "admin": [], "firmwareCheck": true, "guide": true, "history": true, "logLevel": "info", "plex": { "enabled": false, "admins": [] }, "tailscale": { "enabled": false, "hostname": "yagi", "tailnetOnly": false, "https": false }, "notifications": { "channels": [ … ], "events": { "firmware": true, "tuner": true, "viewer": true, "watching": false, "unknown": false } } },
  "editable": true,
  "version": "2026.10.0",
  "versionUrl": "https://github.com/johnschult/yagi/releases/tag/v2026.10.0"
}
```

`editable` is false when Yagi couldn't create its settings file. `tailscale` (next to `settings`, which has the switches) is where the built-in Tailscale node is: `built`, `state` (`off`, `unavailable`, `no-store`, `starting`, `needs-login`, `running` or `error`), `hostname`, `dnsName`, `loginUrl`, `addresses`, the `serving` ports, `lanClosed` (the LAN is being refused), `adminReachable`, and, when HTTPS is on, `https` (its address) or `httpsError`. For the exact fields, see `internal/api/api.go` and the TypeScript types in `web/src/lib/types.ts`, which are kept in step with it.
The tuner's guide key (`DeviceAuth`), the Plex client ID and the secrets in notification channels (tokens, user keys, and a webhook's address, of which only the host is shown) are never in the state document.

## Signing in

These are under `/auth/`, and are public, because you have to be able to sign in before you are an admin.

| Request | What it does |
|---|---|
| `GET status` | `{"plexEnabled": true, "signedIn": {"id": 7, "name": "you"} or null, "admin": false}`. |
| `POST plex/start` | Begin a Plex sign-in. Replies `{"pinId": …, "authUrl": "https://app.plex.tv/auth#…"}`. Open `authUrl` for the person to approve. |
| `POST plex/poll` | Body `{"pinId": …}`. Replies `{"status": "pending"}` until they finish, then `ok` (and a session cookie is set), `denied` (their account isn't allowed) or `expired`. |
| `POST plex/add/start`, `POST plex/add/poll` | The same flow, but to **add** a Plex account to the allowed list. Needs an admin. |
| `POST logout` | End the session. |

## Reading the log

Yagi logs a stream opening and closing, and every refusal, as `key=value` text on standard error: for example
`msg=refused device=antenna path=/discover.json client=100.64.0.9:51234 why="this address is not allowed to use this tuner"`.
The routine requests a media server makes again and again (`discover.json`, `lineup.json` and the like) are logged at the `debug` level only: set the log level to *Debug* (*Settings → Logs*) to see them. The log is meant for people, so don't build anything that depends on its wording.
