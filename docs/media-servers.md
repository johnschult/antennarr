# Media servers

Yagi looks like an ordinary HDHomeRun to a media server, at `<address>:<port>` (for example `100.109.224.87:5004`). The viewer adds it the way they would add any HDHomeRun, by entering that address.

Before you start:

1. The viewer's address must be listed under *Viewers* in Yagi, with a number of tuners above zero.
2. The viewer's server must be able to reach the address. The server itself has to reach it, not the TV or phone that is watching. Run `curl http://<address>:5004/discover.json` from the *server* to check.

Adding a viewer in the web app (**Viewers → Add viewer**) writes these steps out with your real address filled in. For a viewer you have already added, use **Connection steps** on their row.

Plex is the one Yagi has been tried with. The Jellyfin and Emby steps follow those apps' own menus and haven't been tried yet; if a menu has moved, please [open an issue](https://github.com/johnschult/yagi/issues).

## What the media server will see

- **The tuner's name** is the display name you set in Yagi (*Tuners →* your tuner *→ Settings → Name*), or the device (such as HDHomeRun FLEX 4K) if you haven't set one. It changes live. Pick something that tells it apart from a tuner already in the viewer's server.
- **The number of tuners** is *their* number, not the tuner's: the least of the tuners the device really has minus your reserve, your shared cap, and that viewer's limit. A DVR plans recordings around it.
- **The DeviceID** is the tuner's own, unless you set `reportId`. Media servers treat devices with the same ID as the same device. If the viewer already has this tuner under another address, for example from an older setup, they should remove the old one first, or give Yagi's copy its own ID with `reportId` (8 hex digits, for example `10ABCD89`).
- **The guide data and logos** come from the media server's own guide source. Yagi also shows channel logos and what's on in *its* web page, and you can turn that off.

## Plex

Live TV and DVR need **Plex Pass**.

1. In Plex, open *Settings → Live TV & DVR → Set Up Plex DVR*.
2. Click *Don't see your HDHomeRun device? Enter its network address manually*.
3. Enter `<address>:5004` and continue. Scan for channels and match the guide.
4. If Plex already has this tuner under an older address, remove that DVR first so channels aren't doubled.

If you share a tuner with a friend, note that Plex only shares Live TV with people in your Plex Home. That is the problem Yagi solves: their Plex sees your tuner as if it were on their own network.

## Jellyfin

1. Open *Dashboard → Live TV → Tuner Devices* and click **+**.
2. Choose **HDHomeRun** as the tuner type.
3. Enter `<address>:5004` as the tuner address and save.
4. Add a guide data provider on the same page (XMLTV or Schedules Direct). The channels appear without one but have no program listings.

## Emby

Live TV needs **Emby Premiere**.

1. Open *Settings → Live TV* and add a tuner device.
2. Choose **HDHomeRun**.
3. Enter `<address>:5004` as the tuner address and save, then pick a guide source.

## Not listed here

Anything else that can add an HDHomeRun by its address (Channels DVR, for example) should work the same way, but hasn't been tried. Reports are welcome.

## Limits, and what a viewer sees at the limit

When a viewer is already using all their tuners, their next request for a channel gets a **503**, the same answer a busy tuner gives, so media servers handle it cleanly: a recording "couldn't start", or a channel that won't play.
The reason is in an `X-Yagi-Reason` header, and the Live page's timeline records the refusal. See [Configuration](configuration.md#how-the-limits-work).
