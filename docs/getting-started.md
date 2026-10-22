# Getting started

This takes you from nothing to a viewer watching your tuner. It assumes you have an HDHomeRun tuner on your network
and a machine that is always on to run Yagi (a small server, a NAS, a Raspberry Pi: anything that runs Linux, macOS or Docker).

## 1. Install

### Docker (recommended)

Images are published for `amd64` and `arm64`, tagged by release (`2026.10.0`) and `latest`.

Make a folder for Yagi and put the compose file in it:

```yaml
services:
  yagi:
    image: ghcr.io/johnschult/yagi:latest   # or pin a release, e.g. :2026.10.0
    container_name: yagi
    network_mode: host
    restart: unless-stopped
    user: "1000:1000"          # a user that owns ./config
    command: ["-config", "/config/yagi.json"]
    volumes:
      - ./config:/config
```

Then `mkdir config && chown 1000:1000 config` (use the same numbers as `user:`), write a first settings file
(step 2), and run `docker compose up -d`.

Use **host networking** (`network_mode: host`). Yagi tells its own streams from the ones your own TVs open by comparing addresses,
looks for the tuner with a broadcast on your network, and offers this machine's Tailscale and LAN addresses when you add a viewer. All three need to see the machine's real network, which a container on its own bridge network can't.
(With host networking you can't also remap the port: set `listen` in the settings file instead.)

### A program, without Docker

You need Go (see `go.mod` for the version) and Node 22.12 or newer. The web UI is built first, because the Go program embeds it.

```sh
git clone https://github.com/johnschult/yagi
cd yagi
(cd web && npm ci && npm run build)
go build -o yagi ./cmd/yagi
```

Run it with `./yagi -config yagi.json`. To start it on boot, use whatever your system uses (a systemd unit, launchd, an
Unraid User Script). `./yagi -version` prints the version.

## 2. Your tuner

**There is nothing to set up here.** Start Yagi without naming a tuner (`./yagi`, or the compose file above). It starts with only the web page, on its own port, `9244`, and looks for HDHomeRuns on your network while the page is open (every few seconds, for a few minutes). Open the page (step 3): each tuner it finds is listed with a name filled in, which you can change, and an **Add** button (or **Add all**). **Nothing is added until you press it**, because a tuner on a shared network may not be yours. If it stops looking before yours is on, press **Look again**.

Tuners you add are saved in the settings file. Each gets a port of its own, starting at `5004` and going up (`5005`, …), which is the port a viewer's media server connects to. The web page stays on `9244`, and no tuner is ever on it, so a test of port `5004` fails until you have added your first tuner. You can add more, change a tuner's port, or remove one later on the *Tuners* page.

**If your tuner isn't listed**, Yagi can't see its broadcast. That happens when the tuner is on another network or VLAN than Yagi, or when Yagi runs in Docker without host networking (which is why the compose file uses it). Add it by its address on the same page (**Add tuner**), or, for an install nobody opens a page on, set `YAGI_TUNER` (for example `YAGI_TUNER=192.168.1.50`).

To see what is on your network from the command line:

```sh
yagi discover          # in Docker: docker exec yagi yagi discover
```

It prints one line for each HDHomeRun it finds: its DeviceID, its address and how many tuners it has, for example `10C03B06  http://192.168.1.50  4 tuners`.

The settings file is created with the defaults when Yagi starts, if it doesn't exist, and the changes you make in the web page are saved to it. If you would rather write the tuners in the file yourself, [Configuration](configuration.md) has its format.

## 3. Open the web page

A new install is closed: **nobody can use the tuner yet, and only the machine Yagi runs on can open the web page.**
To open it from your own computer, use the **setup link** Yagi writes to its log when it starts for the first time:

```
FIRST RUN: nobody is an admin yet. To set up, open one of these links from the device you want to use as the admin. ...
  setup link url=http://100.64.0.5:9244/claim?token=...
  setup link url=http://192.168.1.5:9244/claim?token=...
```

(`docker logs yagi`, or wherever you run it.) Open the link that matches how you reach the machine, from the computer you want to use as your admin, and press **Make this device an admin**. Yagi saves that computer's address under `admin` in the settings file and sends you to the web page, `http://<this machine>:9244/`.

- The link works **once**, only from a private network (your LAN, Tailscale, a VPN), and only until Yagi restarts. If you restart before using it, a new link is written to the log. Anyone who can read the log can use it, so treat the log like a password until you have.
- Five wrong tries from one address lock that address out for ten minutes.
- If you open the web page on the machine itself (`http://localhost:9244/`) you don't need the link: that machine is always an admin.
- If your computer's address changes (a new DHCP lease, say), you are locked out. **Reserve its address in your router** (a DHCP reservation, usually under *LAN* or *DHCP* settings) so it stays the same. Or add the new one under *Settings → Access* from the machine Yagi runs on, or, if you use Plex, turn on Sign in with Plex (see [Security](security.md)).
- If the settings file can't be saved there is nothing to claim into, and no link is offered: fix where the file lives (`-config`) first.

## 4. Add a viewer

A *viewer* is a person or a server you share the tuner with. Open **Viewers → Add viewer**. It is a short set of steps: who they are, how their server reaches yours, and what to do in their media server (steps 4, 5 and 6 here). Start by filling in:

<img src="screenshots/viewers.jpg" alt="The Viewers page: who can use the tuner, from which addresses, and how many tuners each gets" width="800">

- **Name:** whoever it is.
- **Addresses:** the address their media server will connect *from*, as your Yagi sees it. Single addresses (`100.64.0.7`) or ranges (`10.8.0.0/24`). On Tailscale it is the viewer machine's Tailscale address. [Networking](networking.md) explains how to find it.
- **Tuners:** how many tuners they may use at once.

Only listed viewers can use the tuner, and only loopback (this machine), the addresses under `admin` in the settings, and signed-in Plex accounts can open the admin pages.
If you lose access to the web page, you can always get back in by one of these:

- **Open it from the machine Yagi runs on** (`http://localhost:9244/`), or through an SSH tunnel: `ssh -L 9244:127.0.0.1:9244 yourserver`, then browse to `http://localhost:9244/`.
- **Add your own computer's address to `admin`** in the settings file, then send Yagi `SIGHUP` (or restart it).
- **Turn on Sign in with Plex** (*Settings → Access*) while you still have access, then add your Plex account. After that you can open the page from any address that can reach the port. See [Security](security.md).

## 5. Point a media server at it

<img src="screenshots/setup.jpg" alt="The connect step of adding a viewer, offering this machine's Tailscale and LAN addresses and the steps for the chosen connection" width="600">


Yagi looks like an ordinary HDHomeRun at `<address>:<port>`. The Add viewer steps write this out for you, with your real addresses filled in; for a viewer you have already added, use **Connection steps** on their row on the Viewers page.
For the full walk-through, see [Media servers](media-servers.md).

## 6. Check it works

On the viewer's server: `curl http://<address>:5004/discover.json` should print JSON that mentions your tuner's name and an address that is
**Yagi's, not the tuner's own**. Then watch a channel. In the web page's **Live** view you will see the tuner's tile with the channel, who is watching, the bitrate and the signal.

If something doesn't work, [Troubleshooting](troubleshooting.md) covers the usual causes.

## Updating

- **Docker:** `docker compose pull && docker compose up -d`. If you use Watchtower or something like it, `latest` updates itself. To stay on one release, pin the tag.
- **From source:** pull, rebuild, restart.

Releases are numbered by calendar, `YYYY.MM.PATCH` (for example `2026.10.0`). The version is at the bottom of the web page, linked to its release notes.
Your settings file carries over between versions. Read the release notes before updating: if a release changes the settings, they say so.
