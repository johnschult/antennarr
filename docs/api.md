# API

Antennarr has two kinds of HTTP interface:

- the **tuner side**, which looks like an HDHomeRun to media servers (and is what viewers use);
- the **admin API**, a small JSON API under `/antennarr/api/` that the web page uses. You can script against it too.

The admin API is for Antennarr's own web page first, so it can change between releases. The release notes mention anything that does.

## The tuner side

Every device listens on its own port (`listen`) and answers, for each viewer, like an HDHomeRun:

| Path | Answer |
|---|---|
| `GET /discover.json` | The tuner's own details, with `BaseURL` and `LineupURL` pointing at Antennarr, `TunerCount` set to the viewer's number, and `FriendlyName` and `DeviceID` replaced if you set a display name or `reportId`. |
| `GET /lineup.json` | The channel list, with each channel's `URL` pointing at Antennarr. |
| `GET /status.json` | The tuner's own tuner status, passed through. |
| `GET /auto/vX.Y`, `/tunerN/vX.Y` | A stream, counted against the viewer's limits. |
| anything else | Passed through to the tuner. |

Requests from an address that isn't a viewer get `403` with `X-Antennarr-Reason: not-allowed`. A stream that is refused gets `503` with the reason in `X-Antennarr-Reason`:
`viewer-limit`, `shared-cap`, `owner-reserve` or `tuner-offline`. See [Configuration](configuration.md#how-the-limits-work).

## The admin API

All paths are under `/antennarr/api/` and need admin access (see [Security](security.md)). Without it they answer `403` with `{"error": "admin only"}`.
Replies are JSON and never cached. An error is `{"error": "a message safe to show a person"}` with a suitable status.

**Writes** (`POST`, `PUT`, `PATCH`, `DELETE`) must send `Content-Type: application/json` when they have a body, and an `Origin` header, if sent, must be the host you are talking to.
Bodies are limited to 64 KB, and a field the endpoint doesn't know is an error. If Antennarr couldn't create its settings file there is nowhere to save, so writes fail with a message saying so.

| Request | What it does |
|---|---|
| `GET state` | Everything the web page shows, in one document (below). |
| `GET events` | The same document, pushed as a server-sent event (`event: state`) each time something changes, with a comment line every 20 seconds to keep the connection open. |
| `GET viewers` | The viewers, with their current use and when they were last seen. |
| `POST viewers` | Add a viewer: `{"name": "Mom", "addresses": ["100.64.0.7"], "limit": 1}`, and optionally `"limits": {"device": 2}`. Replies `201` with the viewer. |
| `PUT viewers/{name}` | Replace a viewer (the same body). |
| `DELETE viewers/{name}` | Remove a viewer, `204`. Removing the last viewer leaves nobody able to use the tuner. The `mode` in the settings is then `no-viewers`. |
| `GET settings` | The general settings. |
| `PATCH settings` | Change any of `sharedCap`, `ownerReserve`, `admin` (a list), `firmwareCheck`, `guide`, `plexEnabled`. Only the fields you send change. |
| `POST discover` | Search the network for HDHomeRun tuners (takes about 3 seconds). Replies `{"tuners": [{"id", "address", "tuners", "added"}]}`; `added` is the device a tuner already is, or `""`. `429` while another search is running. |
| `POST devices` | Start sharing a tuner: `{"tuner": "192.168.1.50"}`, optionally with `"id"`, `"name"` and `"listen"`. The name (`tuner`, then `tuner-<id>`), the port and the DeviceID are chosen if left out. Replies `201` with the device. `422` if nothing answers at the address, `409` if that tuner is already added, the port can't be used, there is no settings file, or the tuner came from command-line flags. |
| `DELETE devices/{name}` | Stop sharing a tuner, `204`. Streams on it are cut, its port is freed, and its label and any per-device viewer limits are removed from the settings. `409` in the same cases as adding. |
| `PATCH devices/{name}` | Change a device's `label` (display name; `""` clears it), `ownerReserve` or `sharedCap`. |
| `GET devices/{name}/guide/{channel}/icon` | A channel's logo (a PNG), fetched by Antennarr, `404` when there isn't one. |
| `POST firmware/check` | Read the firmware changelog now. `429` if it was read a moment ago. |
| `GET network` | This machine's Tailscale and LAN addresses, and which firewall looks switched on, for the Connect page. |
| `DELETE plex/admins/{id}` | Stop a Plex account from signing in. |

### The state document

`GET state` returns:

```json
{
  "time": "2026-10-17T20:00:00Z",
  "devices": [ { "name": "antenna", "label": "…", "listen": ":5004", "tuner": { "online": true, "id": "10C03B06", "baseURL": "http://192.168.1.50", "tunerCount": 4, "slots": [ … ], "events": [ … ] }, "streams": [ … ], "summary": { … }, "firmware": { … }, "guide": { … } } ],
  "viewers": [ … ],
  "settings": { "mode": "enforced", "sharedCap": 0, "ownerReserve": 1, "admin": [], "firmwareCheck": true, "guide": true, "plex": { "enabled": false, "admins": [] } },
  "editable": true,
  "version": "2026.10.0",
  "versionUrl": "https://github.com/johnschult/antennarr/releases/tag/v2026.10.0"
}
```

`editable` is false when Antennarr couldn't create its settings file. For the exact fields, see `internal/api/api.go` and the TypeScript types in `web/src/lib/types.ts`, which are kept in step with it.
The tuner's guide key (`DeviceAuth`) and the Plex client ID are never in the state document.

## Signing in

These are under `/antennarr/auth/`, and are public, because you have to be able to sign in before you are an admin.

| Request | What it does |
|---|---|
| `GET status` | `{"plexEnabled": true, "signedIn": {"id": 7, "name": "you"} or null, "admin": false}`. |
| `POST plex/start` | Begin a Plex sign-in. Replies `{"pinId": …, "authUrl": "https://app.plex.tv/auth#…"}`. Open `authUrl` for the person to approve. |
| `POST plex/poll` | Body `{"pinId": …}`. Replies `{"status": "pending"}` until they finish, then `ok` (and a session cookie is set), `denied` (their account isn't allowed) or `expired`. |
| `POST plex/add/start`, `POST plex/add/poll` | The same flow, but to **add** a Plex account to the allowed list. Needs an admin. |
| `POST logout` | End the session. |

## Reading the log

Antennarr logs one line per request to the tuner side and per refusal, as `key=value` text on standard error: for example
`msg=refused device=antenna path=/discover.json client=100.64.0.9:51234 why="this address is not allowed to use this tuner"`.
It is meant for people, so don't build anything that depends on its wording.
