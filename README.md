<p align="center">
  <img src="docs/brand/antennarr-logo-512.png" alt="Antennarr" width="160">
</p>

<h1 align="center">Antennarr</h1>

<p align="center">
  Share an HDHomeRun tuner with Plex, Jellyfin or Emby servers on other networks, and keep track of who is using it.
</p>

---

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/screenshots/live-dark.jpg">
    <img src="docs/screenshots/live-light.jpg" alt="The Antennarr Live page: four tuners, with one being watched by a viewer and one by a local device, each showing its channel, logo, what's on, bitrate and signal, and a list of viewers" width="900">
  </picture>
</p>

An HDHomeRun's `lineup.json` hands out the tuner's own LAN address, so a media server on another network can't use it
without firewall tricks on both ends. Antennarr sits in front of the tuner and rewrites that address, so remote
media servers only ever talk to Antennarr. It also decides who may use the tuner, and how much of it.

- **Rewriting proxy.** `discover.json`, `lineup.json`, `status.json` and the streams all go through Antennarr. A viewer adds the
  tuner by `host:port` like any other HDHomeRun and needs no routing or NAT rules of their own.
- **Follows the tuner.** Finds the tuner by DeviceID (UDP broadcast, with a subnet scan as a fallback), so a DHCP address change doesn't break anything.
- **Limits.** An allowlist of viewer addresses, tuners per viewer, a cap on all viewers together, and tuners always kept free for you.
  Over the limit, a viewer gets the same HTTP 503 a busy tuner gives, so their software already knows what to do.
- **Several tuners.** Each looks like its own HDHomeRun on its own port.
- **A web UI.** Live tiles for each tuner (channel, logo, what's on with its picture and progress, who's watching, bitrate, signal), viewers, settings,
  a copy-paste setup guide, light and dark themes.
- **Firmware news.** Tells you when SiliconDust releases firmware for your model, and installs it when you press Install on the Tuners page: the latest release for that tuner only, never an older one, and never on its own.
- **Notifications.** Optional messages to ntfy, Pushover or a webhook when there is new firmware, a tuner is offline for a couple of minutes (and when it's back), or a viewer is turned away. Each kind can be switched off, and each channel has a Test button.
- **Any network.** Plain LAN, Tailscale, WireGuard, ZeroTier… Antennarr just listens on a port. Getting the two servers to reach each other is up to you.

> [!WARNING]
> **Status: early.** It runs a real FLEX 4K and is tested with the Go race detector and a UI test suite, but it is young software.
> Expect rough edges, and please read [Security](#security) before exposing it to anything beyond a network you trust.

## Screenshots

<table>
  <tr>
    <td width="50%"><img src="docs/screenshots/viewers.jpg" alt="The Viewers page: who can use the tuner, from which addresses, and how many tuners each gets"><br><sub><b>Viewers:</b> who can use the tuner, from where, and how many tuners each gets.</sub></td>
    <td width="50%"><img src="docs/screenshots/setup.jpg" alt="The connect step of adding a viewer, offering this machine's Tailscale and LAN addresses and the steps for the chosen connection"><br><sub><b>Connect guide:</b> a step of adding a viewer. It spots this machine's Tailscale address and writes out the steps with your real addresses.</sub></td>
  </tr>
</table>

<p>
  <img src="docs/screenshots/phone-live.jpg" alt="The Live page on a phone" width="220">
  &nbsp;
  <img src="docs/screenshots/phone-menu.jpg" alt="The navigation menu on a phone" width="220">
  <br><sub><b>On a phone:</b> the same pages, with a menu and cards instead of wide tables.</sub>
</p>

## Quick start

You need Go (see `go.mod` for the version) and Node 22.12 or newer. The web UI is built first, because the Go binary embeds it.

```sh
git clone https://github.com/johnschult/antennarr
cd antennarr
(cd web && npm ci && npm run build)    # builds into internal/ui/dist
go build -o antennarr ./cmd/antennarr
```

Run it. With no `-tuner`, Antennarr finds your HDHomeRun on the network and remembers it in `antennarr.json`. If none answers it starts anyway, and you add one from the web page (*Find tuners*, or by its address):

```sh
./antennarr
```

To name a tuner yourself (say it's on another network), find it and pass its address:

```sh
./antennarr discover                   # lists HDHomeRun tuners on your network
./antennarr -tuner 192.168.1.50
```

Look in the log for the first-run **setup link** and open it from your own computer to become the admin (or open `http://localhost:5004/antennarr/` on the machine itself). Adding a viewer in the UI walks through the steps to connect them, with your addresses filled in.

A viewer then adds the tuner to their media server as `<this machine>:5004`, for example in Plex under
*Settings → Live TV & DVR → Set Up Plex DVR → enter its network address manually*.

The settings file is `antennarr.json` in the directory you run from, or wherever `-config` says. If it doesn't exist, Antennarr creates it with the defaults when it starts, and from then on changes made in the UI are saved to it. It is read again when the process gets `SIGHUP`. If the file can't be created in the default place (a read-only directory), Antennarr still runs, but changes can't be saved; use `-config` to put it somewhere writable. The tuner on the command line stays a flag until you move it into the file's `devices` (see below).

### Docker

```sh
docker pull ghcr.io/johnschult/antennarr:latest
```

Or copy [docker-compose.example.yml](docker-compose.example.yml) next to your `antennarr.json` and run `docker compose up -d`.
It uses host networking, which Antennarr needs to recognize its own streams and to offer your Tailscale and LAN addresses in the Add viewer steps.
Images are published for `amd64` and `arm64`. To build one yourself: `docker build -t antennarr https://github.com/johnschult/antennarr.git#main`.

### Versions

Releases are numbered by calendar, `YYYY.MM.PATCH` (for example `2026.10.0`), and the image is tagged with that number as well as `latest`.
The app shows its version in the footer, linked to the release notes; `antennarr -version` prints it. See [docs/RELEASING.md](docs/RELEASING.md).

## Documentation

The full documentation is in [docs/](docs/README.md):
[getting started](docs/getting-started.md),
[networking](docs/networking.md) (Tailscale, a VPN, the same network, firewalls),
[media servers](docs/media-servers.md) (Plex, Jellyfin, Emby),
[configuration](docs/configuration.md),
[security](docs/security.md),
[troubleshooting](docs/troubleshooting.md) and the [API](docs/api.md).
The app's **Help** link, at the bottom of its web page, opens the docs for the version you are running.

## Security

**HDHomeRun tuners have no authentication, and neither do their streams.** Antennarr's protection is the address a request comes from.
Please keep that in mind:

- **A new install is closed.** With no viewers nobody can use the tuner, and only this machine can open the admin page. On first start Antennarr writes a one-time **setup link** to its log;
  open it from your own computer (LAN, Tailscale or VPN) and confirm to make that computer an admin. Then add a viewer (Viewers page, or `viewers` in the settings file).
  Only listed addresses can use the tuner, and only loopback, your `admin` addresses and a signed-in Plex account can use the admin pages.
- **Viewers only get what a media server needs.** Besides `discover.json`, `lineup.json` and the streams, only `status.json` and `lineup_status.json` are passed to the tuner, read-only.
  Everything else the tuner serves (rescans, firmware upgrade, its own web pages) answers `404`.
- **Keep viewers out of your `admin` range.** A viewer whose address is inside an `admin` range is an admin too. Antennarr logs a warning if that happens.
- **Don't put Antennarr on the open internet.** Use Tailscale, a VPN, or a private network. If you must, allow only the viewer's address in both Antennarr and your firewall.
- **Admin by address** trusts the address a request arrives from. If you run Antennarr behind a reverse proxy, every request looks like it came from the proxy.
- The admin API refuses cross-site writes and only accepts JSON. The web page is served with a strict Content-Security-Policy.

### Sign in with Plex (optional)

Turn it on under *Settings → Access*, then add a Plex account. That account can open the admin pages from any address that can reach the port.
It uses the plex.tv sign-in flow, done by the server. Your Plex token is never sent to the browser and is thrown away as soon as Antennarr knows who you are;
only the account ID and name are kept. Sessions are saved beside the settings file (hashed, readable only by the owner) so a restart doesn't sign you out.
Nothing contacts plex.tv until someone signs in.

## What Antennarr contacts

Only your tuners, plus these, each of which can be switched off or left unset in Settings:

| What | Where | How often | Sends |
|---|---|---|---|
| Firmware changelog | `silicondust.com` | every 12 hours | nothing about you or your tuners |
| Firmware file | `download.silicondust.com` | only when you press Install | nothing about you or your tuners |
| Program guide, channel logos and program pictures | `api.hdhomerun.com`, `img.hdhomerun.com` | every 6 hours | your tuner's guide key (its `DeviceAuth`), which SiliconDust uses to know which channels to list |
| Sign in with Plex | `plex.tv` | only when someone signs in | the sign-in request |
| Notifications | the ntfy server, `api.pushover.net` or the webhook address you set up | when something you switched on happens, and on Test | a short message naming a tuner, viewer or address. Nothing is contacted until you add a channel |

Logos and program pictures are fetched by Antennarr, not by your browser, and the web page itself loads nothing from outside.

## Settings file

All of this can be changed in the UI. The file is JSON, written atomically, and strict: unknown fields are an error.

```json
{
  "devices": [
    { "name": "antenna", "listen": ":5004", "tuner": "192.168.1.50" }
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
  "plex": { "enabled": false, "admins": [] }
}
```

| Field | Meaning |
|---|---|
| `devices[]` | Tuners to front. `name` (letters, digits, `.` `_` `-`) is a stable key; `listen` is where media servers reach it; `tuner` is its address and `id` its DeviceID to follow if it moves. Optional: `streamPort`, `advertise`, `reportId`, and per-device `ownerReserve` and `sharedCap`. |
| `viewers[]` | `addresses` (IPs or CIDRs) their media server connects from, `limit` (tuners at once; 0 pauses them), and `limits` to override that per device. |
| `sharedCap` | Most tuners all viewers together may use on a device. 0 means no cap. |
| `ownerReserve` | Tuners kept free for your own devices, in addition to any you are using. |
| `admin` | Extra addresses (IPs or CIDRs) allowed to use the admin pages. Loopback always is, once a viewer exists. |
| `labels` | Display names by device name, for the web page only. Media servers are always told the device (such as HDHomeRun FLEX 4K), so renaming doesn't change what Plex shows. |
| `firmwareCheck`, `guide` | Turn the firmware changelog check and the program guide off. On by default. |
| `plex` | Sign in with Plex: `enabled`, and the allowed `admins` (`id`, `name`). |
| `notifications` | `channels` (ntfy, Pushover or webhook, with their tokens) and which `events` to send. See [Configuration](docs/configuration.md#notifications). |

With no `devices`, Antennarr runs one device from the command line: `-tuner` (an address) or `-tuner-id` (a DeviceID to search for), with `-listen`, `-stream-port`, `-advertise`. You can't mix the two.
Run `antennarr -h` for every flag.

## Development

```sh
go test -race ./...               # the server
cd web && npm test                # the UI (Vitest)
go run ./cmd/antennarr -config /tmp/antennarr-dev.json   # the Go server, in one terminal
cd web && npm run dev             # hot-reloading UI in another, using that server; see web/README.md
```

Everything is in the standard library on the Go side (no third-party Go dependencies). The tests use fake tuners and fake plex.tv / SiliconDust servers and never touch the real network.
The design notes, including what is built and what is planned, are in [docs/DESIGN.md](docs/DESIGN.md).

## Roadmap

Planned, roughly in this order: Plex-verified viewer enrollment, built-in Tailscale (set up from the UI), and editing a tuner's address from the UI. Contributions and bug reports are welcome: see [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE). Antennarr is not affiliated with or endorsed by SiliconDust, Plex, Jellyfin or Emby. HDHomeRun is a trademark of SiliconDust; Plex is a trademark of Plex, Inc.
See [web/README.md](web/README.md) for the third-party logos and fonts the web UI bundles.
