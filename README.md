<p align="center">
  <img src="docs/brand/antennarr-logo-512.png" alt="Antennarr" width="160">
</p>

<h1 align="center">Antennarr</h1>

<p align="center">
  Share an HDHomeRun tuner with Plex, Jellyfin or Emby servers on other networks, and keep track of who is using it.
</p>

---

An HDHomeRun's `lineup.json` hands out the tuner's own LAN address, so a media server on another network can't use it
without firewall tricks on both ends. Antennarr sits in front of the tuner and rewrites that address, so remote
media servers only ever talk to Antennarr. It also decides who may use the tuner, and how much of it.

- **Rewriting proxy.** `discover.json`, `lineup.json`, `status.json` and the streams all go through Antennarr. A viewer adds the
  tuner by `host:port` like any other HDHomeRun and needs no routing or NAT rules of their own.
- **Follows the tuner.** Finds the tuner by DeviceID (UDP broadcast, with a subnet scan as a fallback), so a DHCP address change doesn't break anything.
- **Limits.** An allowlist of viewer addresses, tuners per viewer, a cap on all viewers together, and tuners always kept free for you.
  Over the limit, a viewer gets the same HTTP 503 a busy tuner gives, so their software already knows what to do.
- **Several tuners.** Each looks like its own HDHomeRun on its own port.
- **A web UI.** Live tiles for each tuner (channel, logo, what's on, who's watching, bitrate, signal), viewers, settings,
  a copy-paste setup guide, light and dark themes.
- **Firmware news.** Tells you when SiliconDust releases firmware for your model. It never installs anything.
- **Any network.** Plain LAN, Tailscale, WireGuard, ZeroTier… Antennarr just listens on a port. Getting the two servers to reach each other is up to you.

> **Status: early.** It runs a real FLEX 4K and is tested with the Go race detector and a UI test suite, but it is young software.
> Expect rough edges, and please read [Security](#security) before exposing it to anything beyond a network you trust.

## Quick start

You need Go (see `go.mod` for the version) and Node 22.12 or newer. The web UI is built first, because the Go binary embeds it.

```sh
git clone https://github.com/johnschult/antennarr
cd antennarr
(cd web && npm ci && npm run build)    # builds into internal/ui/dist
go build -o antennarr ./cmd/antennarr
```

Find your tuner, then run Antennarr in front of it:

```sh
./antennarr discover                   # lists HDHomeRun tuners on your network
./antennarr -tuner 192.168.1.50 -config antennarr.json
```

Open `http://<this machine>:5004/antennarr/`. The Setup page in the UI writes out the steps for a viewer, with your addresses filled in.

A viewer then adds the tuner to their media server as `<this machine>:5004`, for example in Plex under
*Settings → Live TV & DVR → Set Up Plex DVR → enter its network address manually*.

Without `-config`, Antennarr runs with defaults and can't save changes made in the UI. With it, the file is created the first time you change something, and from then on changes are saved to it. It is read again when the process gets `SIGHUP`. The tuner on the command line stays a flag until you move it into the file's `devices` (see below).

### Docker

```sh
docker build -t antennarr https://github.com/johnschult/antennarr.git#main
```

Or copy [docker-compose.example.yml](docker-compose.example.yml) next to your `antennarr.json` and run `docker compose up -d --build`.
It uses host networking, which Antennarr needs to recognise its own streams and to offer your Tailscale and LAN addresses on the Setup page.

## Security

**HDHomeRun tuners have no authentication, and neither do their streams.** Antennarr's protection is the address a request comes from.
Please keep that in mind:

- **With no viewers configured, everything is open**, including the admin page and its settings, to anyone who can reach the port.
  Add a viewer first (Viewers page, or `viewers` in the settings file). Once there is one, only listed addresses can use the tuner, and
  only loopback and your `admin` addresses (or a signed-in Plex account) can use the admin pages.
- **Don't put Antennarr on the open internet.** Use Tailscale, a VPN, or a private network. If you must, allow only the viewer's address in both Antennarr and your firewall.
- **Admin by address** trusts the address a request arrives from. If you run Antennarr behind a reverse proxy, every request looks like it came from the proxy.
- The admin API refuses cross-site writes and only accepts JSON. The web page is served with a strict Content-Security-Policy.

### Sign in with Plex (optional)

Turn it on under *Settings → Access*, then add a Plex account. That account can open the admin pages from any address that can reach the port.
It uses the plex.tv sign-in flow, done by the server. Your Plex token is never sent to the browser and is thrown away as soon as Antennarr knows who you are;
only the account ID and name are kept. Sessions are saved beside the settings file (hashed, readable only by the owner) so a restart doesn't sign you out.
Nothing contacts plex.tv until someone signs in.

## What Antennarr contacts

Only your tuners, plus these, each of which can be switched off in Settings:

| What | Where | How often | Sends |
|---|---|---|---|
| Firmware changelog | `silicondust.com` | every 12 hours | nothing about you or your tuners |
| Programme guide and channel logos | `api.hdhomerun.com`, `img.hdhomerun.com` | every 6 hours | your tuner's guide key (its `DeviceAuth`), which SiliconDust uses to know which channels to list |
| Sign in with Plex | `plex.tv` | only when someone signs in | the sign-in request |

Logos are fetched by Antennarr, not by your browser, and the web page itself loads nothing from outside.

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
  "admin": ["100.64.0.0/10"],
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
| `labels` | Display names by device name; also what media servers are told the tuner is called. |
| `firmwareCheck`, `guide` | Turn the firmware changelog check and the programme guide off. On by default. |
| `plex` | Sign in with Plex: `enabled`, and the allowed `admins` (`id`, `name`). |

With no `devices`, Antennarr runs one device from the command line: `-tuner` (an address) or `-tuner-id` (a DeviceID to search for), with `-listen`, `-stream-port`, `-advertise`. You can't mix the two.
Run `antennarr -h` for every flag.

## Development

```sh
go test -race ./...               # the server
cd web && npm test                # the UI (Vitest)
cd web && npm run dev             # hot-reloading UI against a running Antennarr; see web/README.md
```

Everything is in the standard library on the Go side (no third-party Go dependencies). The tests use fake tuners and fake plex.tv / SiliconDust servers and never touch the real network.
The design notes, including what is built and what is planned, are in [docs/DESIGN.md](docs/DESIGN.md).

## Roadmap

Planned, roughly in this order: a published container image, Plex-verified viewer enrollment, built-in Tailscale (set up from the UI), adding and removing tuners from the UI, and a redesigned Tuner page. Contributions and bug reports are welcome.

## License

[MIT](LICENSE). Antennarr is not affiliated with or endorsed by SiliconDust, Plex, Jellyfin or Emby. HDHomeRun is a trademark of SiliconDust; Plex is a trademark of Plex, Inc.
See [web/README.md](web/README.md) for the third-party logos and fonts the web UI bundles.
