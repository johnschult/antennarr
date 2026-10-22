# Networking

**Yagi serves private networks only:** this machine, your own network, a VPN, Tailscale. A connection from a public internet address is dropped as it arrives, with one line in the log saying why, so a port forwarded by mistake shows scanners nothing. There is no switch to turn that off. Sharing a tuner is for people you trust enough to put on your network, and the way to reach someone elsewhere is a VPN or Tailscale.

How a viewer's media server reaches the port is up to you, and there are three common ways.
Adding a viewer in the web app writes out the steps for each, with your addresses filled in. For a viewer you have already added, use **Connection steps** on their row on the Viewers page.

Two ideas to keep in mind:

- **Viewers are identified by the address their requests come from**, as Yagi sees it. That is what you list in *Viewers*. After Tailscale or a VPN, it is the viewer's address on that network, not their home address.
- **HDHomeRun tuners have no passwords**, and neither does Yagi's tuner side: the list of allowed addresses is the protection, on top of the network itself being private. See [Security](security.md).

## Tailscale (recommended)

Yagi can also **join your tailnet by itself**, with no Tailscale on the machine, in the `-tailscale` image: see [Built-in Tailscale](tailscale.md). The steps below are for a Tailscale you run on the machine yourself.

[Tailscale](https://tailscale.com) is the easiest way: nothing is opened to the internet, and the addresses don't change.

1. Put Tailscale on the machine running Yagi and on the viewer's media server, signed in to the same tailnet. If they are on different tailnets, share the Yagi machine with the viewer's tailnet from the Tailscale admin console (*Machines → ⋯ → Share*).
2. On the Yagi machine, `tailscale ip -4` prints its Tailscale address (it looks like `100.x.y.z`). The connect step shows it for you when Yagi is running on the machine itself. In Docker, use host networking, or it can't see it.
3. In Yagi, add the **viewer's** Tailscale address under *Viewers*. (On the viewer's machine, `tailscale ip -4`.)
4. The viewer adds the tuner in their media server as `<Yagi's Tailscale address>:5004`. See [Media servers](media-servers.md).

If you can reach it with `tailscale ping` but not with a web browser or `curl`, check two things: your Tailscale access controls might not allow that port, and the machine's firewall (below) may be blocking it.

## Another VPN (WireGuard, ZeroTier, Netbird…)

The same idea. Both machines join the VPN. The viewer adds the Yagi machine's address *on the VPN*, and you list the viewer's address on the VPN under *Viewers*.

## The same network

If both servers are on one LAN, there is nothing to set up. Use the Yagi machine's LAN address, and list the viewer's LAN address.

## Not over the open internet

Yagi refuses connections from public internet addresses, and won't take a public address in a viewer's list or the admin list (the page and API say so). A viewer who isn't on your network, a VPN or your tailnet joins one of those: Tailscale is the easiest (see above), and the viewer's address you list is the one Yagi sees from there.

- **Your own network on public IPv6 addresses** is fine: a source on a network the machine is directly connected to counts as local, even when its address looks public.
- **A reverse proxy** in front of Yagi on the same machine looks local to Yagi, so the proxy decides who gets through. Exposing Yagi that way is outside what Yagi supports.

## The machine's firewall

If the machine running Yagi has a firewall that blocks the port, viewers' connections time out. They don't get a "refused" message, they just hang. Allow each viewer's address to reach Yagi's port, and only that.
The connect step shows the commands. It offers `ufw`, `firewalld` and `iptables`, and it opens the firewall step by itself when it can tell `ufw` or `firewalld` is switched on.

```sh
# ufw
sudo ufw allow from 100.64.0.7 to any port 5004 proto tcp
# firewalld (add each without --permanent too to apply it immediately, with no reload)
sudo firewall-cmd --permanent --zone=public --add-rich-rule='rule family="ipv4" source address="100.64.0.7/32" port port="5004" protocol="tcp" accept'
sudo firewall-cmd --reload
# iptables (lost on reboot unless you save it)
sudo iptables -I INPUT -p tcp -s 100.64.0.7 --dport 5004 -j ACCEPT
```

Yagi can't read your firewall's rules (that takes root), and testing from the machine to itself says nothing, so it can only tell which firewall is switched on. It cannot tell you whether the port is blocked.
To test a rule, use `curl` from the viewer's machine.

On firewalld, a Tailscale interface normally falls in the default zone, so the rule above applies to it. If the machine runs Docker, adding rules without `--permanent` and without `--reload` avoids a reload that can disturb Docker's own rules.

## If Yagi runs in Docker

Use host networking (`network_mode: host`). Without it, Yagi:

- sees every request as coming from Docker's gateway address, so the allowlist can't tell viewers apart;
- can't find the tuner with a broadcast;
- can't tell its own streams from your own TVs' (it compares addresses with the tuner's idea of who is watching).

A consequence of host networking: published ports (`ports:`) don't apply, so set the listening port with `listen` in the settings file.
Yagi inside a container can't see the host's firewall either, so the connect step's firewall detection stays off.

## Checking each step

From the viewer's machine, in this order:

```sh
tailscale ping <yagi address>            # is the machine reachable? (Tailscale only)
curl -i http://<yagi address>:5004/discover.json
```

- A timeout means the connection never arrived: a firewall, or the wrong network or address.
- `403` with `X-Yagi-Reason: not-allowed` means it arrived and Yagi doesn't recognize the sender's address. The log shows the address it saw. Add that.
- `502 tuner unreachable` means it got through and the tuner is offline.
- `200` with your tuner's name and Yagi's address means it works.

## The admin port and the tuner ports

The admin pages have a port of their own, `9244` unless you change it (see [Configuration](configuration.md#the-admin-port)), and no tuner is ever on it. So allow your viewers to the tuner ports (`5004`, `5005`, …) only, and yourself to the admin port.
