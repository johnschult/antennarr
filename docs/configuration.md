# Configuration

Antennarr is configured with a **settings file** (JSON), which you can edit by hand or change from the web page, and a few **command-line flags**.

- Changes made in the web page are saved to the settings file at once, and take effect at once (except the list of devices, see below).
- The file is read strictly: **a key it doesn't know is an error**, not silently ignored, so a typo can't quietly open access. If the file is wrong, Antennarr says why and doesn't start (or, if it is already running and you reload, keeps the settings it has).
- The file is saved atomically and with permissions that only its owner can read.
- Send the process `SIGHUP` (`docker kill -s HUP antennarr`) to read the file again after editing it by hand. Changing `devices` needs a restart; everything else is picked up by the reload.

## An example

```json
{
  "devices": [
    { "name": "antenna", "listen": ":5004", "tuner": "192.168.1.50", "id": "10C03B06" }
  ],
  "viewers": [
    { "name": "Mom", "addresses": ["100.64.0.7"], "limit": 1 }
  ],
  "sharedCap": 0,
  "ownerReserve": 1,
  "admin": ["100.64.0.1"],
  "labels": { "antenna": "Living room antenna" },
  "firmwareCheck": true,
  "guide": true,
  "plex": { "enabled": false, "admins": [] },
  "notifications": {
    "channels": [
      { "id": "c1a2b3c4d5", "name": "My phone", "type": "ntfy", "url": "https://ntfy.sh", "topic": "antennarr-x7k2q9" }
    ],
    "events": { "unknown": false }
  }
}
```

## Settings

### `devices`

The tuners to front. Each looks like its own HDHomeRun to media servers, on its own port. If you leave `devices` out, Antennarr runs one device described by the command-line flags.

| Key | Meaning |
|---|---|
| `name` | A stable key: 1–40 letters, digits, `.`, `_` or `-`. It is used in settings, limits and URLs, so don't change it. |
| `listen` | Where media servers reach this device: `:5004` (every address) or `127.0.0.1:5004`. Each device needs its own port. |
| `tuner` | The tuner's address, as a host or URL (`192.168.1.50`). Where to look first. |
| `id` | The tuner's DeviceID: exactly 8 hex digits. With it, Antennarr finds the tuner again if its address changes. If you leave it out, it is learned from the tuner the first time Antennarr reaches it. You need `tuner`, `id`, or both. |
| `streamPort` | The tuner's streaming port. Default `5004`. |
| `advertise` | The base URL handed to media servers, for example `http://antennarr.example.com:5004`. By default it is whatever host the client used to connect, which is right in almost every case. |
| `reportId` | A different DeviceID (8 hex digits) to show media servers instead of the tuner's own. Use it when a media server already knows the real tuner and would merge the two. |
| `ownerReserve`, `sharedCap` | Override the top-level values below for this device. |

### `viewers`

Who you share with. **With no viewers, nobody can use the tuner**: a new install is closed until you add one.

| Key | Meaning |
|---|---|
| `name` | Required, and unique. |
| `addresses` | At least one. Single addresses or ranges: `100.64.0.7`, `10.8.0.0/24`, `fd7a:115c::/48`. The address their requests come from, as Antennarr sees it. If ranges overlap, the most specific one wins. An address can't be listed for two viewers. |
| `limit` | How many tuners they may use at once, on each device. `0` pauses them. |
| `limits` | Overrides `limit` for particular devices, by device name: `{ "antenna": 2 }`. Needs `devices` to be declared in the file. |

### Everything else

| Key | Meaning |
|---|---|
| `sharedCap` | The most tuners *all* viewers together may use on a device. `0` means no cap. |
| `ownerReserve` | Tuners always kept free for your own devices (default `1`), **in addition to** any your own devices are using at the moment. |
| `admin` | Extra addresses (single or ranges) allowed to open the admin pages. Loopback is always allowed once a viewer exists. Keep this narrow, and keep viewers' addresses out of it: a viewer inside an admin range is an admin too (Antennarr logs a warning). |
| `labels` | Display names, by device name: `{ "antenna": "Living room antenna" }`, up to 60 characters. It is for the web page only, and changes live. Media servers are always told the tuner's own name (its model, such as `HDHomeRun FLEX 4K`), so a name you give it here never reaches Plex: renaming doesn't change what Plex shows, and a media server can name its own DVR as it likes. |
| `firmwareCheck` | Look at SiliconDust's firmware changelog every 12 hours (default `true`). |
| `guide` | Read SiliconDust's programme guide so Live tiles can show the channel logo and what's on (default `true`). |
| `notifications` | Messages to your phone or another service. `channels` is a list (at most 8); `events` switches kinds of event on or off. See [Notifications](#notifications) below. |
| `plex` | Sign in to the admin pages with a Plex account. `enabled`, the allowed `admins` (each with an `id` and a `name`, at most 50), and a `clientId` Antennarr creates and keeps for you. See [Security](security.md). |

## Notifications

Antennarr can message you about things that are worth knowing. Add a channel under **Settings → Notifications** (and press **Test** to check it works), or write it in the file. Nothing is sent, and nothing is contacted, until there is a channel.

Each channel has an `id` (made for you, and never changed), a `name`, and a `type`:

| `type` | Fields | Notes |
|---|---|---|
| `ntfy` | `url` (the server, such as `https://ntfy.sh`), `topic`, and optionally `token` | Sent as JSON to the server. On `ntfy.sh` anyone who knows the topic can read it. |
| `pushover` | `token` (the application token) and `user` (your user key) | Sent to `api.pushover.net`. |
| `webhook` | `url`, and optionally `token` | A JSON `POST` with `event`, `title`, `message`, `device` and `time`. A `token` is sent as `Authorization: Bearer …`. |

`url` must be `http://` or `https://`, without a username or password in it (use `token`). A private or LAN address, or this machine, is fine; a link-local one (such as `169.254.169.254`), multicast and `0.0.0.0` are refused.

`events` switches each kind on or off. Every channel gets every message that is on.

| Event | Default | When |
|---|---|---|
| `firmware` | on | A tuner has newer firmware. Once per release, kept across restarts in `<settings file name without .json>.notified.json`. It does nothing while `firmwareCheck` is off, so the page won't let you switch it on until the check is on (and the API refuses). |
| `tuner` | on | A tuner has been unreachable for two minutes, and when it comes back. A reboot or a brief drop sends nothing. |
| `viewer` | on | A viewer was turned away because they were at their limit or the tuners were taken. The same viewer and reason is reported at most every half hour. |
| `unknown` | **off** | An address that isn't a viewer tried to use a tuner. A tuner port that can be reached from the internet is scanned all day, so each address is reported once a day, and at most six an hour in all. |

A message is sent once and not retried; if a channel is down you'll find `notification not sent` in the log, with the reason and without the token. The settings are read for every message, so changes need no restart.

## How the limits work

Limits count **tuners, not particular tuners**. Your own TVs take whichever tuner is free, so Antennarr can't pin a viewer to one.

For a viewer on a device (call the tuner's real number of tuners *N*):

- **What the viewer is told** (`TunerCount` in `discover.json`): the least of *N − ownerReserve*, the `sharedCap` (if set) and the viewer's `limit`, never below zero. A DVR plans recordings around it.
- **When a stream is refused:** a request for a channel gets a **503** if any of these is true:
  - the viewer is already using their `limit` (`viewer-limit`);
  - the viewers together are already using the `sharedCap` (`shared-cap`);
  - the tuners in use by all viewers, plus this one, plus the ones your own devices are using (read from the tuner's own status), would leave fewer than `ownerReserve` free (`owner-reserve`);
  - the tuner is offline (`tuner-offline`).
- A request from an address that isn't a viewer gets a **403** (`not-allowed`).

The reason is in the `X-Antennarr-Reason` response header, and each refusal is noted (at most once a minute per viewer, device and reason) in the Live page's timeline. A stream already running is never cut off to make room.

Example, with a 4-tuner device, `ownerReserve: 1` and one of *your* TVs watching: viewers can have at most 4 − 1 − 1 = 2 tuners at that moment. The Live page shows the tuner being held for you as **Reserved**.

## Command-line flags

| Flag | Meaning |
|---|---|
| `-config PATH` | The settings file, `antennarr.json` in the current directory by default (`/config/antennarr.json` in the Docker image). If it doesn't exist it is created with the defaults at start-up. If the default one can't be created, Antennarr runs without saving; a `-config` you name that can't be created is an error. |
| `-tuner HOST` | A tuner's address, to run one device without a settings file. |
| `-tuner-id ID` | A tuner's DeviceID to search for if its address changes. |
| `-listen ADDR` | Where to listen (default `:5004`). |
| `-stream-port PORT` | The tuner's streaming port (default `5004`). |
| `-advertise URL` | The base URL handed to clients (default: the host they connected with). |
| `-report-id ID` | Report this DeviceID to clients instead of the tuner's own. |
| `-poll DURATION` | How often to check the tuners (default `5s`). |
| `-firmware-url URL` | Read the firmware changelog from here instead of SiliconDust's page (for a mirror, or testing). |
| `-version` | Print the version and exit. |

`-tuner`, `-tuner-id`, `-listen`, `-stream-port`, `-advertise` and `-report-id` describe a device, so they can't be combined with `devices` in the settings file. Put those settings in the file.

`antennarr discover [-wait 3s] [host …]` lists the HDHomeRun tuners it can find (a broadcast on your network, and a scan of the nearby subnet), one per line: DeviceID, address, number of tuners.
Add host addresses to look at those instead.

## Files Antennarr keeps

- **The settings file** (the one you pass to `-config`): everything above.
- **`<settings file name without .json>.sessions.json`**, beside it: who is signed in with Plex, so a restart doesn't sign you out. Only a hash of each session is stored, never the session itself or your Plex token. It is created only if Plex sign-in is used, and only when there is a settings file.

- **`<settings file name without .json>.notified.json`**, beside it: which firmware release each tuner was last messaged about, so a restart doesn't send it again. It is only written when notifications are used and there is a settings file.

Back them up with your other configuration. There is no database, and nothing else is kept. Logs go to standard error (`docker logs antennarr`).

## What it checks on its own

| What | How often | Notes |
|---|---|---|
| The tuners | every 5 seconds (`-poll`) | After 3 failures in a row a tuner is shown offline, and Antennarr starts searching for it by DeviceID (at most every 30 seconds), using a broadcast and a scan of the nearby subnet. |
| Firmware changelog | every 12 hours | Only when `firmwareCheck` is on. |
| Programme guide | every 6 hours | Only when `guide` is on. About 3.5 MB for a FLEX 4K. |

Notifications aren't on a schedule of their own: they are sent when the firmware check, the tuner check or a viewer's request notices something.

A restart begins again at the address in the settings file, and finds the tuner by its DeviceID. Antennarr doesn't write a new address back into the file. Update `tuner` yourself, and reserve the address in your router, to skip the search.
