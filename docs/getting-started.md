# Getting started

This takes you from nothing to a viewer watching your tuner. It assumes you have an HDHomeRun tuner on your network
and a machine that is always on to run Antennarr (a small server, a NAS, a Raspberry Pi: anything that runs Linux, macOS or Docker).

## 1. Install

### Docker (recommended)

Images are published for `amd64` and `arm64`, tagged by release (`2026.10.0`) and `latest`.

Make a folder for Antennarr and put the compose file in it:

```yaml
services:
  antennarr:
    image: ghcr.io/johnschult/antennarr:latest   # or pin a release, e.g. :2026.10.0
    container_name: antennarr
    network_mode: host
    restart: unless-stopped
    user: "1000:1000"          # a user that owns ./config
    command: ["-config", "/config/antennarr.json"]
    volumes:
      - ./config:/config
```

Then `mkdir config && chown 1000:1000 config` (use the same numbers as `user:`), write a first settings file
(step 2), and run `docker compose up -d`.

Use **host networking** (`network_mode: host`). Antennarr tells its own streams from the ones your own TVs open by comparing addresses,
looks for the tuner with a broadcast on your network, and offers this machine's Tailscale and LAN addresses on the Setup page. All three need to see the machine's real network, which a container on its own bridge network can't.
(With host networking you can't also remap the port: set `listen` in the settings file instead.)

### A program, without Docker

You need Go (see `go.mod` for the version) and Node 22.12 or newer. The web UI is built first, because the Go program embeds it.

```sh
git clone https://github.com/johnschult/antennarr
cd antennarr
(cd web && npm ci && npm run build)
go build -o antennarr ./cmd/antennarr
```

Run it with `./antennarr -config antennarr.json`. To start it on boot, use whatever your system uses (a systemd unit, launchd, an
Unraid User Script). `./antennarr -version` prints the version.

## 2. Tell it about your tuner

Find the tuner on your network:

```sh
antennarr discover          # in Docker: docker exec antennarr antennarr discover
```

It prints one line for each HDHomeRun it finds: its DeviceID, its address and how many tuners it has, for example `10C03B06  http://192.168.1.50  4 tuners`. Then write the settings file. The smallest useful one is:

```json
{
  "devices": [
    { "name": "antenna", "listen": ":5004", "tuner": "192.168.1.50", "id": "10C03B06" }
  ]
}
```

- `name` is a short key (letters, digits, `.`, `_`, `-`). It appears in URLs and settings, so pick one you won't change. You can give the tuner a friendlier display name later, in the web page.
- `listen` is the port viewers' servers connect to. `:5004` means every address on this machine.
- `tuner` is where the tuner is now. `id` is its DeviceID: with it, Antennarr finds the tuner again if its address changes (DHCP). You can leave `id` out and Antennarr learns it the first time it reaches the tuner, but then it can't search for a tuner it has never seen, so adding it now is better.

If the file doesn't exist, Antennarr starts with defaults and creates it the first time you change something in the web page.
You can also run one tuner straight from the command line with no file at all (`antennarr -tuner 192.168.1.50`), but then settings you change in the web page can't be saved.
The full list of settings is in [Configuration](configuration.md).

## 3. Open the web page

Go to `http://<this machine>:5004/antennarr/`.

**Right now, anyone who can reach that address can open it and change anything**, because there are no viewers yet. That is why the next step is to add one.
(Antennarr writes "no viewers configured: anyone who can reach these ports can use the tuners" to its log when it starts like this.)

## 4. Add a viewer

A *viewer* is a person or a server you share the tuner with. Open **Viewers → Add viewer** and fill in:

- **Name:** whoever it is.
- **Addresses:** the address their media server will connect *from*, as your Antennarr sees it. Single addresses (`100.64.0.7`) or ranges (`10.8.0.0/24`). On Tailscale it is the viewer machine's Tailscale address. [Networking](networking.md) explains how to find it.
- **Tuners:** how many tuners they may use at once.

As soon as the first viewer exists, Antennarr locks down: only listed viewers can use the tuner, and only loopback (this machine),
the addresses under `admin` in the settings, and signed-in Plex accounts can open the admin pages.
So **before you add your first viewer, decide how you will get back in**. The easiest ways:

- **Open it from the machine Antennarr runs on** (`http://localhost:5004/antennarr/`), or through an SSH tunnel: `ssh -L 5004:127.0.0.1:5004 yourserver`, then browse to `http://localhost:5004/antennarr/`.
- **Add your own computer's address to `admin`** in the settings (in the web page under *Settings → Access*, or in the file).
- **Turn on Sign in with Plex** (*Settings → Access*), then add your Plex account. After that you can open the page from any address that can reach the port. See [Security](security.md).

## 5. Point a media server at it

Antennarr looks like an ordinary HDHomeRun at `<address>:<port>`. The **Setup** page in the web app writes out the steps for the viewer you pick, with your real addresses filled in.
For the full walk-through, see [Media servers](media-servers.md).

## 6. Check it works

On the viewer's server: `curl http://<address>:5004/discover.json` should print JSON that mentions your tuner's name and an address that is
**Antennarr's, not the tuner's own**. Then watch a channel. In the web page's **Live** view you will see the tuner's tile with the channel, who is watching, the bitrate and the signal.

If something doesn't work, [Troubleshooting](troubleshooting.md) covers the usual causes.

## Updating

- **Docker:** `docker compose pull && docker compose up -d`. If you use Watchtower or something like it, `latest` updates itself. To stay on one release, pin the tag.
- **From source:** pull, rebuild, restart.

Releases are numbered by calendar, `YYYY.MM.PATCH` (for example `2026.10.0`). The version is at the bottom of the web page, linked to its release notes.
Your settings file carries over between versions. Read the release notes before updating: if a release changes the settings, they say so.
