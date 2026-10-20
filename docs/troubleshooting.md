# Troubleshooting

Start with the log, it usually says what is wrong. In Docker: `docker logs yagi`. Every request that reaches Yagi is logged with the address it came from, and every refusal says why.
A request that never shows up in the log never arrived, which points at the network or a firewall, not at Yagi.

- [A page or the tuner address just hangs, then times out](#it-just-hangs-then-times-out)
- ["This address can't open the admin page"](#this-address-cant-open-the-admin-page)
- [The tuner address shows "this address is not allowed to use this tuner"](#the-bare-address-says-this-address-is-not-allowed-to-use-this-tuner)
- [The tuner shows as offline](#the-tuner-shows-as-offline)
- [A viewer's media server can't add the tuner, or a channel won't play](#a-viewers-media-server-cant-add-the-tuner-or-a-channel-wont-play)
- [A 503 when watching](#a-503-when-watching)
- [Plex shows the tuner twice, or the wrong channels](#plex-shows-the-tuner-twice-or-with-the-wrong-channels)
- [Settings I change don't stick](#settings-i-change-dont-stick)
- [Yagi won't start](#yagi-wont-start)
- [I'm signed out after a restart, or the page says "Reconnecting…"](#im-signed-out-after-a-restart-or-the-page-says-reconnecting)
- [No channel logos or "what's on"](#no-channel-logos-or-whats-on)
- [Docker problems](#docker-problems)
- [Asking for help](#asking-for-help)

## It just hangs, then times out

Nothing came back at all, not even a refusal. The connection is being dropped before it reaches Yagi.

1. **The machine's firewall** is the usual cause. Allow the viewer's address (and yours, for the admin page) to reach Yagi's port. See [Networking](networking.md#the-machines-firewall).
2. **The wrong address, or the wrong network.** Are both machines on the same Tailscale, VPN or LAN? `tailscale ping <address>` works at the network level even when a port is blocked, so use `curl -i http://<address>:5004/discover.json` to test the port itself.
3. **Tailscale access controls** may not allow that port between those two machines.
4. **Safari or another browser changed the address**, for example to `https://`. Yagi speaks plain `http://`. Type it in full, with `:5004`.

Check the log: if the request appears there, it arrived and this section isn't your problem.

## "This address can't open the admin page"

The page loaded (it is public so you can sign in), but Yagi doesn't consider your address an admin. Once you have a viewer, only these can open the admin pages:

- this machine itself (`http://localhost:5004/`, or through `ssh -L 5004:127.0.0.1:5004 yourserver`);
- addresses listed under `admin` in the settings;
- a Plex account you have added, if *Sign in with Plex* is on (the page then shows a **Sign in with Plex** button).

To fix it, add your address to `admin` (the log line for your request shows the address Yagi saw), or turn on Plex sign-in from an address that is already an admin. If you have locked yourself out completely, edit the settings file on the machine, add your address to `admin`, and send the process `SIGHUP` (or restart it).

Your address might not be the one you expect. Behind Docker's bridge network, a reverse proxy, or after NAT, Yagi sees the gateway's address. See [Networking](networking.md#if-yagi-runs-in-docker).

## The bare address says "this address is not allowed to use this tuner"

The address itself is fine, but whatever opened it (a script, a media server, `curl`) isn't one of your viewers, so it is refused. A browser at `http://host:5004/` gets the web page, where you can sign in. If a browser shows this message, it is on an older version (before 2026.10.13 the page was at `/yagi/`), or the address is not on your admin list: see above.

If you open Yagi by a name like `mini.local`, your phone or laptop may connect over IPv6 using a link-local address (`fe80::…`). Yagi now treats those as ordinary addresses, so you can list `fe80::/10` under `admin` if you want the whole local network, but it is usually easier to use the machine's IPv4 address.

## The tuner shows as offline

Yagi checks the tuner every 5 seconds. After 3 failures in a row it shows it offline, and then searches for it by its DeviceID (a broadcast, and a scan of the nearby subnet) every 30 seconds.

- **Is the tuner on?** Power, the Ethernet cable (an extender or switch can fail), and the router. Can you reach `http://<tuner address>/discover.json` from the Yagi machine?
- **Is it listed at all?** The *Tuners* page lists the tuners being shared . Press *Add tuner* to search the network again or add one by its address. In Docker, searching needs host networking.
- **Did its address change?** If you gave the tuner's `id` in the settings (or Yagi has reached it before), it finds the new address by itself, usually within a minute, and the Live page's timeline records "Tuner moved … (found by DeviceID …)". If you didn't, run `yagi discover` and put the address and `id` in the settings.
- **A restart starts at the address in the settings file** and then searches, so it can take half a minute to come back. Put the tuner's current address in `tuner`, and reserve it in your router, to avoid the wait.
- **Restarting or updating is not an outage.** While Yagi restarts a tuner or installs firmware on it, the page says *Restarting…* or *Updating firmware…*, and no offline message is sent. If it stays that way for more than a few minutes, check the tuner's power and cable: the result on the tuner's *Firmware* or *Device* tab says what Yagi saw.
- **A broadcast can't cross networks.** If the tuner is on a different subnet or VLAN from Yagi, the search can't find it. Give its address in `tuner`.
- **In Docker**, the search needs host networking.

## A viewer's media server can't add the tuner, or a channel won't play

From the **viewer's server** (not their TV), try `curl -i http://<address>:5004/discover.json`:

| You get | It means |
|---|---|
| Nothing, then a timeout | The connection never arrived. A firewall, or the wrong address. See [it just hangs](#it-just-hangs-then-times-out). |
| `403` and `X-Yagi-Reason: not-allowed` | It arrived, but the sender's address isn't a viewer. The log shows the address Yagi saw. Add that exact address under *Viewers*. After Tailscale or a VPN it is the viewer's address on that network. |
| `502 tuner unreachable` | It got through, but the tuner is offline. See [the tuner shows as offline](#the-tuner-shows-as-offline). |
| `200` with Yagi's address in `BaseURL` | Yagi is fine. The problem is in the media server. |

Also: a viewer with a **limit of 0** is paused and every stream is refused. And **the server itself** must reach the address; a TV or phone being able to reach it is no help.

## A 503 when watching

A `503` is Yagi deliberately saying "no tuner for you right now", the same answer a busy tuner gives. The `X-Yagi-Reason` header says why (`curl -i`, or look at the Live page's timeline):

| Reason | What happened | What to change |
|---|---|---|
| `viewer-limit` | This viewer is already using all their tuners. | Raise their limit under *Viewers*, or stop a stream. |
| `shared-cap` | All viewers together are using the shared cap. | Raise *Max tuners for viewers* in *Settings → Access → Limits*. |
| `owner-reserve` | The tuners left are being held for your own devices. | Lower *Tuners reserved for you*. Remember the reserve is **in addition to** what your own TVs are using right now. |
| `tuner-offline` | The tuner is offline. | See above. |

## Plex shows the tuner twice, or with the wrong channels

Media servers treat devices with the **same DeviceID as the same device**. If the viewer's Plex already has this tuner from an older setup (a relay, or by its own address), adding it again by Yagi's address can merge with, or fight, the old entry.

- Remove the old DVR/tuner in the viewer's Plex first, then add `<address>:5004`.
- Or give Yagi's copy its own ID with `reportId` (8 hex digits) in the settings, which makes it a separate device. Restart afterwards, since it is a device setting.
- Live TV in Plex needs Plex Pass, and the guide data comes from Plex, not from Yagi.

## Settings I change don't stick

- **"Start Yagi with -config to edit settings"**: it could not create its settings file where it was started (a read-only directory, usually), so there is nowhere to save. The log says so at start-up. Start it with `-config /path/to/yagi.json` somewhere writable.
- **Changes to `devices` don't apply**: the list of devices only changes on restart. The log says "restart Yagi to apply that". Everything else reloads (`SIGHUP`, or any change made in the web page).
- **Hand edits are ignored**: tell it to reload (`SIGHUP`), or restart. A mistake in the file keeps the old settings and logs the error ("settings reload failed, keeping the current ones").
- **Saving fails with a permissions error**: Yagi needs to be able to create files in the settings file's folder (it saves by writing a temporary file and renaming it). In Docker, make `./config` owned by the user in `user:`.

## Yagi won't start

The message says why. The common ones:

- **An unknown key**: the settings file is strict, so a typo is an error: `json: unknown field "viewrs"`.
- **A bad value**: a device name with a space, a `listen` without a port, an `id` that isn't 8 hex digits, a viewer with no addresses, the same address listed for two viewers, two devices on one port.
- **"no tuner is set up yet"** is not an error: Yagi is running with only the web page. Open it and add a tuner from the list it finds, or by address.
- **"… can't be combined with devices declared in the settings file"**: you passed `-tuner`/`-listen`/… and also have `devices` in the file. Choose one.
- **"address already in use"**: something else has that port. Pick another `listen`.

## I'm signed out after a restart, or the page says "Reconnecting…"

- Sign-ins are kept in a file beside the settings file, `<settings>.sessions.json`, so a restart shouldn't sign you out. If it does, Yagi couldn't create its settings file, or it couldn't write the sessions file (check the log), or the sign-in had expired (12 hours idle, 7 days at most).
- A page left open while Yagi restarts shows **Reconnecting…**. It opens a new connection by itself, so it recovers on its own. If the server no longer recognizes you, it shows the sign-in screen.

## No channel logos or "what's on"

- It is on by default: check *Settings → Privacy → Show what's on*.
- It needs the machine to reach `api.hdhomerun.com` and `img.hdhomerun.com`, and the tuner has to have reported its guide key. The log shows `guide fetch failed` with the reason.
- It only appears for channels being watched right now.
- The picture behind a tile comes from the same guide, through Yagi, so it needs the same two addresses. A tile without a picture is normal when the guide has none for that program.

## Installing firmware or restarting says it didn't work

- **"Didn't take" or "still running …"**: the tuner accepted the file and came back on the old version. Yagi never tries a second time by itself. Check what the tuner's own page says, and try again from the tuner's *Firmware* tab when nothing is using it.
- **"SiliconDust has no firmware file …"**: Yagi builds the download address from the tuner's firmware family and the release the check found, and SiliconDust doesn't document those addresses. The firmware check keeps working; open an issue with the model and version.
- **"The tuner has not come back"**: give it a few minutes, then check power and the Ethernet cable and unplug it for ten seconds. Nothing more is sent to it.
- **Restart says it kept answering**: the tuner didn't appear to restart. Try again, or restart it from its own page or by unplugging it.
- Neither runs while something is using the tuner unless you confirm that you want to interrupt it.

## Testing a channel's signal says it can't

- The test needs a tuner nothing is using, and never interrupts a stream. With all of the tuner's tuners in use the button is greyed out; try again when one is free.
- It can't run while the tuner is scanning, restarting or being updated, or while another test is running.
- *No signal* means the tuner tuned to the channel but couldn't lock on to it within a few seconds: the antenna doesn't get it, or it is too weak. It is a real answer, not an error.
- If the tuner won't tune to a channel at all, the message says what it replied.

## The Activity page is empty or missing

- It is switched on by default under *Settings → Privacy → Keep activity history*, and the *Activity* item is hidden while it is off.
- It records what is in use as it happens, so it starts empty, and a session shorter than 15 seconds is not kept.
- Without a settings file it is kept in memory and starts over with Yagi. Otherwise it is in `<settings file>.history.jsonl` beside it.

## Docker problems

- **It sees every request as coming from `172.x.x.x`, can't find the tuner, or adding a viewer offers no Tailscale address**: use `network_mode: host`.
- **Can't write its settings**: the `./config` folder isn't owned by the user in `user:`.
- **A build fails with a DNS error** (`dl-cdn.alpinelinux.org`): the default Docker network on some firewalld machines can't reach the internet during a build. Use the published image instead of building, or build with `docker build --network host`.
- **The firewall's rules aren't visible from inside the container**, so the Add viewer steps don't detect it. That is expected.

## Asking for help

[Open an issue](https://github.com/johnschult/yagi/issues) and include:

- the **version** (the bottom of the web page, or `yagi -version`) and how you run it (Docker or not, and the network setup);
- what you did and what you expected;
- the relevant lines of the **log**, with any addresses you'd rather not share replaced;
- your settings file, with addresses changed if you prefer. It has no passwords or tokens in it.

Please don't post your tuner's `DeviceAuth` or a Plex token anywhere.
