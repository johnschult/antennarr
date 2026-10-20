# Built-in Tailscale

Yagi can join your Tailscale network by itself, as its own device, with no Tailscale installed on the machine it runs on. The idea is that you share that one device with the people who need it, and nothing is opened on your router.

> **Where this is.** This is the first part. Yagi can join a tailnet, show you where it is, and keep its sign-in. **It does not yet answer on the tailnet**: serving the admin page and the tuners there, recognising viewers by their Tailscale identity, and HTTPS come next (see [the plan](DESIGN.md#built-in-tailscale-plan)). Until then, use Tailscale on the machine, or [another way](networking.md) to reach it.

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

## Good to know

- It is a separate device from anything else Tailscale on the machine. It doesn't change the machine's network.
- In the Tailscale admin console, consider **Disable key expiry** for the device, or it will need signing in again when the key expires.
- To let someone on another tailnet reach it, share the device from the admin console (**Share…**).
- Switching it off stops the node. Its entry stays in your admin console until you delete it there.
