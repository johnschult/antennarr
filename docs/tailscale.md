# Built-in Tailscale

Yagi can join your Tailscale network by itself, as its own device, with no Tailscale installed on the machine it runs on. The idea is that you share that one device with the people who need it, and nothing is opened on your router.

> **Where this is.** Yagi can join a tailnet, keep its sign-in, show you where it is, and **answer on it**: every port it serves (each tuner's, and the admin port when it has its own) is also served at the device's tailnet name and addresses. See [the plan](DESIGN.md#built-in-tailscale-plan) for how it was built.

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
- **A viewer can be matched by who they are** (see [Who is on the other end](#who-is-on-the-other-end)), or by address: their Tailscale address (`100.x.y.z`), which changes if they re-register the device.
- **The admin page is on the tuner's port too**, and is still only for an admin: an address in *Settings → Access → Admin addresses* (add `100.64.0.0/10` to allow your whole tailnet, but only if none of your viewers is on it), a Plex account, or a tailnet login (`tailscale:you@example.com`).
- **The LAN ports stay open unless you close them.** Yagi still answers on the machine's own address as before. See [Only on the tailnet](#only-on-the-tailnet) to turn that off.
- If a port is changed, or a tuner added or removed, the tailnet follows within a couple of seconds.

## Only on the tailnet

Once the node is connected, *Settings → Access → Only on the tailnet* closes Yagi to everything that comes in over the machine's own network address. What is left is the tailnet and the machine itself (a browser on the same machine at `127.0.0.1`). Media servers and viewers on your LAN, and the admin page opened at the LAN address, are refused; the log says so, once a minute at most.

- **It is a gate, not a closed port.** The ports stay bound (a tuner keeps its port, and switching the setting off is immediate), but a connection from anywhere but the machine itself is dropped as it arrives. A scan of the LAN still sees the port open. When you switch it on, connections already open from the LAN are closed within a couple of seconds, a stream in progress included.
- **It can only be switched on while the node is connected**, so there is always a way in. Before it saves, the page tells you if you are using it at the LAN address, and where to open it afterwards (`<name>.<tailnet>.ts.net:<port>`).
- **It is not in force while the node is not connected.** If the tailnet is down, or signed out, the LAN stays open and the page says *On, but not in force*, so a Tailscale outage never takes the admin page with it.
- **Docker:** a browser on the Docker host reaches a published port from the bridge address, not from loopback, so with the switch on it is refused too. Use the tailnet name from that machine as well.
- **Getting back in.** Switch it off from the page over the tailnet, or start Yagi with `-allow-lan` (`YAGI_ALLOW_LAN=1`), which keeps the LAN open whatever the setting says.
- Tailscale running on the machine itself (not Yagi's node) connects from a `100.x` address on the machine's own network interface, so it is refused too. Use Yagi's own node with this switch.

## Who is on the other end

A connection that comes in over Yagi's own Tailscale arrives with a name, not only an address: the node knows which login and which device it came from. So a viewer's address list (Viewers → Edit) and the admin addresses can name them directly:

- `tailscale:alice@example.com` is anyone signed in to the tailnet as that login, on any of their devices, at whatever address. Use it for a person.
- `tailscale:laptop.tail1234.ts.net` is that one device (its full tailnet name; the *Machines* page of the Tailscale admin console shows it). Use it for a tagged server, or one device.

The rules:

- **It only counts when the node says so.** Yagi asks its own Tailscale node who is on the other end of each connection that came in over the tailnet. A connection from the LAN, or a request header that claims a login, is never taken as one.
- **A login is not a device, and a tagged device has no login.** Name a tagged device by its tailnet name.
- **It beats the address.** If a connection matches a name, that viewer is used; otherwise the address decides, as before. The same name can't be listed for two viewers.
- **Shared devices work the same way.** Someone on another tailnet who is given access to Yagi's device by sharing arrives with their own login.
- **It is checked against what Tailscale reports now**, so removing someone from the tailnet, or their login changing, takes effect for their next connection.

## HTTPS for the admin page

*Settings → Tailscale → HTTPS for the admin page* also serves the admin pages (and only those) over HTTPS at the device's tailnet name, such as `https://yagi.tail1234.ts.net/`, with no port. Yagi's node gets and renews the certificate itself from Tailscale; nothing on the machine changes.

- **Your tailnet needs HTTPS certificates turned on** (Tailscale admin console → DNS: MagicDNS, then *Enable HTTPS*). Without it the switch shows Tailscale's own reason instead of an address.
- **The name goes into a public certificate log.** Certificates are issued by Let's Encrypt, which publishes them, so the device name and your tailnet's name become public. That is why it is off until you turn it on.
- **Only the admin pages are on it.** The tuner ports stay plain HTTP: media servers expect it, and the traffic is already encrypted by the tailnet. Port 443 is on the node, not on the machine, so it doesn't clash with anything the machine runs.
- **Sign-in cookies are marked secure** over HTTPS, so signing in with Plex works there too. Yagi asks Tailscale for the certificate as soon as the HTTPS listener starts, so it is usually ready before the first visit; if you are quicker than that, the first visit waits a few seconds.
- Whether someone on **another tailnet** can use the HTTPS name of a shared device is not covered by Tailscale's documentation.

## Good to know

- It is a separate device from anything else Tailscale on the machine. It doesn't change the machine's network.
- In the Tailscale admin console, consider **Disable key expiry** for the device, or it will need signing in again when the key expires.
- To let someone on another tailnet reach it, share the device from the admin console (**Share…**).
- Switching it off stops the node. Its entry stays in your admin console until you delete it there.
