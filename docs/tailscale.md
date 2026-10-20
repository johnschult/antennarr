# Built-in Tailscale

Yagi can join your Tailscale network by itself, as its own device, with no Tailscale installed on the machine it runs on. The idea is that you share that one device with the people who need it, and nothing is opened on your router.

> **Where this is.** Yagi can join a tailnet, keep its sign-in, show you where it is, and **answer on it**: every port it serves (each tuner's, and the admin port when it has its own) is also served at the device's tailnet name and addresses. Still to come, see [the plan](DESIGN.md#built-in-tailscale-plan): a *tailnet only* switch that turns the LAN ports off, recognising viewers by their Tailscale identity, and HTTPS.

## Which image

The Tailscale code is in a separate image, so the normal one stays free of it. In your compose file:

```yaml
image: ghcr.io/johnschult/yagi:tailscale        # the latest release, with Tailscale
# image: ghcr.io/johnschult/yagi:2026.10.13-tailscale   # or pin a release
```

`ghcr.io/johnschult/yagi:latest` and `:<version>` are the same program without it. In a build without it, **Settings → Access** says so and shows no switch. Building it yourself: `go build -tags tsnet ./cmd/yagi`, or `docker build --build-arg TAGS=tsnet .`

## Turning it on

1. Open **Settings → Access** and switch on **Join your Tailscale network**. The name on the tailnet is `yagi` unless you change it (it must be unique among your devices).
2. Press **Sign in to Tailscale** and approve the new device. Or, for an unattended start, give the container a Tailscale [auth key](https://tailscale.com/kb/1085/auth-keys) and no link is needed:

   ```yaml
   environment:
     TS_AUTHKEY: tskey-auth-…   # ideally a tagged, reusable key
   ```
3. The status row shows the device's tailnet name and addresses once it is connected.

Yagi keeps the device's identity in a `tailscale` folder beside the settings file, so it comes back after a restart without signing in again. Without a settings file (`-config`) there is nowhere to keep it, and the page says so.

## Reaching it from the tailnet

Once the status row says *Connected, and answering on port …*, a viewer on the tailnet adds the tuner in their media server at the device's name and the tuner's port, such as `yagi:5004`, like any other HDHomeRun (see [Media servers](media-servers.md)). The tuner ports answer exactly as they do on the LAN: the same limits, the same allowlist, the same log.

- **The viewer's setup steps know about it.** In *Viewers → Connection steps* (and when adding a viewer), **Yagi's own Tailscale** is its own choice, first, with the device's tailnet name as the address to give them, and it is kept apart from **Tailscale on this machine** (the Tailscale the host runs, if it runs one): they are two devices with two addresses, and each choice shows only its own. For Yagi's own there is no firewall step, because its traffic doesn't come through the machine's network.
- **The viewer is still matched by address.** On the tailnet that is their Tailscale address (`100.x.y.z`), so list that in the viewer's addresses. Recognising them by their Tailscale login instead is a later part.
- **The admin page is on the tuner's port too**, and is still only for an admin: an address in *Settings → Access → Admin addresses* (add `100.64.0.0/10` to allow your whole tailnet, but only if none of your viewers is on it), or a Plex account.
- **The LAN ports are unchanged.** Yagi still answers on the machine's own address as before; turning that off is the *tailnet only* switch, which comes next.
- If a port is changed, or a tuner added or removed, the tailnet follows within a couple of seconds.

## Good to know

- It is a separate device from anything else Tailscale on the machine. It doesn't change the machine's network.
- In the Tailscale admin console, consider **Disable key expiry** for the device, or it will need signing in again when the key expires.
- To let someone on another tailnet reach it, share the device from the admin console (**Share…**).
- Switching it off stops the node. Its entry stays in your admin console until you delete it there.
