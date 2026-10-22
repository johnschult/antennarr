# Changelog

<!-- Each section is also used as the text of a GitHub release, where a relative link would not work. Link to docs with full https://github.com/johnschult/yagi/blob/main/... addresses. -->

What changed in each release, newest first. Releases are numbered `YYYY.MM.PATCH`; see [Releasing](https://github.com/johnschult/yagi/blob/main/docs/RELEASING.md).
Each release's notes on GitHub are its section of this file.

## 2026.10.14 (2026-10-22)

### Read before updating

- **The web page now has a port of its own: `9244`.** It used to share a tuner's port, so it moved when you added a tuner, and a viewer who could reach the tuner could reach the sign-in page. Now `http://<this machine>:9244/` is always the page, and no tuner is ever on that port: tuners keep `5004`, `5005`, … exactly as before, so **nothing changes for your viewers or their media servers**. What changes for you: open `:9244` instead of `:5004`, **sign in with Plex once more** (the cookie is for the page's new address), re-add the page to your Home Screen, and allow `9244` in the firewall for the computers you use as admins. Change it with `-admin-listen`, `YAGI_ADMIN_LISTEN` or `adminListen` in the settings file; see [Configuration](https://github.com/johnschult/yagi/blob/main/docs/configuration.md#the-admin-port). The `-admin-listen` option from the last release is now simply how you move it.

### Added

- **Plain startup messages.** Yagi now prints where the admin page is (with your machine's addresses), says that a tuner's port opens only once a tuner is added, and points to the troubleshooting page. `-admin-listen:9244` (no space) gets a message saying what to type instead of an "unknown flag", and a port that is already in use names the port and how to change it.
- **A Yagi antenna, made of the letters of its name, when you start it from a terminal.** In green where the terminal can show it (`NO_COLOR` keeps the letters and drops the color), and never in a log: `docker logs`, systemd and files get nothing.
- **Notices on the page that can be closed.** A warning explains that your way into the page is your computer's address, which can change, and suggests reserving it in your router (or Sign in with Plex). The note on the Viewers page about limits can be closed too. The offline and "nobody can use your tuner" notices link to troubleshooting.

## 2026.10.13 (2026-10-20)

### Read before updating

- **Yagi no longer adds a tuner by itself.** On a first start with no tuner named, Yagi used to look for ten seconds and save whatever answered. Now it starts with only the web page, and the page looks for tuners while it is open and **lists what it finds**, with a name filled in and an **Add** button (or **Add all**). Nothing is added until you press it. Tuners already in your settings file, and `-tuner`, work as before. If Yagi runs in Docker **without host networking**, it can't see the broadcast: add the tuner by its address on the page, or set the new `YAGI_TUNER`. See [Getting started](https://github.com/johnschult/yagi/blob/main/docs/getting-started.md#2-your-tuner).
- **The web page is at the address itself, not under `/yagi/`.** `http://<this machine>:5004/` is the page (and `http://<this machine>:5000/` with an admin port), and its API is `/api/`. Old links and bookmarks to `/yagi/…` (and `/antennarr/…`) are sent on to the same page, including the first-run setup link. On a tuner's port only the page's own paths go to the page: the tuner's own addresses (`/discover.json`, `/lineup.json`, `/auto/…`) are exactly as before. If you installed the page to your Home Screen, add it again. **You will need to sign in with Plex once more**, because the sign-in cookie now covers the whole site.

### Added

- **Yagi can join your Tailscale network by itself.** A new `-tailscale` image (`ghcr.io/johnschult/yagi:tailscale`, or `<version>-tailscale`) has a Tailscale node built in: switch it on in *Settings → Access → Tailscale*, sign in with the link (or set `TS_AUTHKEY`), and Yagi appears on your tailnet as its own device, with no Tailscale on the machine, **and answers there**: every tuner's port (and the admin port when it has its own) is also served at the device's tailnet name, so a viewer adds the tuner at `yagi:5004`, with the same limits, allowlist and log as on the LAN. The viewer setup steps offer it as its own way to connect. The normal image is unchanged. See [Built-in Tailscale](https://github.com/johnschult/yagi/blob/main/docs/tailscale.md).
- **Only on the tailnet** (`-tailscale` image). Once Yagi's own Tailscale is connected, a switch refuses everything that comes in over the machine's network address, leaving the tailnet and the machine itself, and closes the connections already open from the LAN when it goes on. It asks first, can only be turned on while the node is connected, stays out of force while the tailnet is down so you are never locked out, and `-allow-lan` (`YAGI_ALLOW_LAN`) is the way back in. See [Only on the tailnet](https://github.com/johnschult/yagi/blob/main/docs/tailscale.md#only-on-the-tailnet).
- **Viewers and admins can be matched by their Tailscale login or device** (`-tailscale` image). Over Yagi's own Tailscale, an entry in a viewer's addresses or in the admin addresses can be `tailscale:alice@example.com` (anyone signed in as that login, on any device) or `tailscale:laptop.tail1234.ts.net` (that device), instead of a `100.x` address that changes when a device is re-registered. The node says who is on the other end; a connection from the LAN or a header can't claim one. See [Who is on the other end](https://github.com/johnschult/yagi/blob/main/docs/tailscale.md#who-is-on-the-other-end).
- **HTTPS for the admin page on the tailnet** (`-tailscale` image). A switch also serves the admin pages over HTTPS at the device's tailnet name (`https://yagi.tail1234.ts.net/`), with a certificate Tailscale gets for it (fetched as soon as it is switched on, so the first visit isn't slow). It needs HTTPS certificates turned on for your tailnet, and the name then appears in a public certificate log, so it is off until you turn it on. The tuner ports stay plain HTTP. See [HTTPS for the admin page](https://github.com/johnschult/yagi/blob/main/docs/tailscale.md#https-for-the-admin-page).
- **The admin pages can have a port of their own.** Start Yagi with `-admin-listen :9244` (or set `YAGI_ADMIN_LISTEN`, which is easiest in Docker) and the web page, its API, sign-in and the first-run setup link are on that port only. The tuner ports then carry nothing but the tuners, so you can open them to your viewers and keep the admin port to yourself. Adding or removing a tuner never moves the page, and a tuner can't be given the admin port. Nothing changes unless you set it: see [Configuration](https://github.com/johnschult/yagi/blob/main/docs/configuration.md#giving-the-admin-pages-their-own-port).

### Changed

- **The tuner pages are at `/tuners`** (was `/tuner`), so the address matches the menu: `/tuners`, `/tuners/<name>/channels`. Old `/tuner/…` links and bookmarks are sent on to the new address.
- **Settings, Access, is tidier.** Tailscale has its own section, and what was *Admins and viewers* is now *Admin access* (the admin addresses and Sign in with Plex) and *Viewers*.
- **A whole row on the Tuners page opens the tuner**, not only its name.
- **The channel card and HD.** The card the **i** opens shows the channel's number and name with the status as one line under it, and HD channels are marked with the HD icon there and in the channel list.
- **Routine requests are logged at the Debug level.** The `discover.json` and `lineup.json` a media server asks for over and over were filling the log at Info and hiding the lines that matter. Streams, refusals and warnings are still shown; switch the log to *Debug* in *Settings → Logs* to see every request.

## 2026.10.12 (2026-10-20)

### Changed

- **A tab bar along the bottom on phones**, like an iPhone app, instead of the menu button. Sign-out is in the footer on a phone; the connection status stays in the header.
- **A phone turned sideways keeps the phone layout** (the tab bar and sheets) instead of switching to the crowded desktop top bar. A touch screen under 500px tall counts as a phone; a tablet or a short desktop window does not.
- **Dialogs are bottom sheets on phones** (using [Vaul](https://vaul.emilkowal.ski/)), and so is a channel's info button. Wide screens keep dialogs and the popover.
- **The *Tuners* link always shows the list of tuners**, even with just one, so another can be added from there. It used to jump to the tuner's page.
- The version in the footer starts with a **v**, like the release it links to.
- The Home Screen icon has more room around the logo, so iOS rounding no longer crowds it.

## 2026.10.11 (2026-10-19)

### Changed

- **The version is back in the footer**, first among the links and still linked to its release notes. The wordmark is bigger now and covered it under the logo in the top bar.
- **A tagline in the footer:** *Point your antenna **anywhere**.* The page's description, which a link preview shows, says the same; the tab title is still *Yagi*.
- The name, model and address on a tuner's header line on the *Live* page are centered on one line. They were lined up by their text baselines, which put the smaller ones a little low.

## 2026.10.10 (2026-10-19)

### Read before updating

- **Antennarr is now Yagi**, after the Yagi–Uda antenna (see the new [FAQ](https://github.com/johnschult/yagi/blob/main/docs/faq.md)). Nothing needs doing right away:
  - Old `/antennarr/` addresses and bookmarks redirect to `/yagi/`.
  - Started with no `-config`, Yagi still uses an existing `antennarr.json` when there is no `yagi.json`, and says so in the log. Rename the file when convenient.
  - The container image is `ghcr.io/johnschult/yagi`. For a few releases the same tags are also pushed to `ghcr.io/johnschult/antennarr`, so existing compose files keep updating.
  - **You will be signed out of the admin pages once** if you use Plex sign-in, and Plex's list of authorized devices will show the new name. The program is now `yagi` rather than `antennarr`, and the Go module is `github.com/johnschult/yagi`.
  - The `viaAntennarr` field in the tuner state of the [API](https://github.com/johnschult/yagi/blob/main/docs/api.md) is now `viaProxy`.

### Changed

- **A new logo:** an antenna on a dark tile, in place of the TV with rabbit ears. The antenna icon is no longer used elsewhere in the page: tuners are shown with a receiver icon.
- The README screenshots are retaken.

### Added

- A [FAQ](https://github.com/johnschult/yagi/blob/main/docs/faq.md), starting with why it is called Yagi.

## 2026.10.9 (2026-10-19)

### Added

- **A log viewer.** *Settings → Logs* shows what Antennarr has been doing, newest last: each line has its time, a level with its own icon and color, the message and its details. **Show** picks which of the kept lines are listed (everything, info and above, warnings and errors, or errors only), the search box looks in the messages and their details, **Live** keeps it up to date and following the newest line until you scroll up to read, **Download** saves what is shown as a text file, and **Clear** forgets what is kept. The last 2,000 lines are kept in memory, not in a file, so they start over when Antennarr restarts; they are still written to standard error as before (`docker logs antennarr`). Anything that looks like a secret (a token, a key, the guide key) is hidden before a line is kept, and only admins can open the page. It is in the footer too, and an offline tuner's *Overview* says *See why in the logs* and opens the log already searched for that tuner.
- **A log level setting.** **Record** on the *Logs* tab sets how much Antennarr writes down from now on: *Info* is the default, *Debug* adds more detail for when something is wrong, and *Warnings and up* or *Errors* is quieter. It is the `logLevel` setting, it takes effect at once, and it follows the settings file when that is reloaded. See [Configuration](https://github.com/johnschult/antennarr/blob/main/docs/configuration.md).

### Changed

- **Settings is in tabs**, like a tuner's page: **Access** (how much of each tuner viewers may use, who may open the page, Sign in with Plex), **Notifications**, **Privacy** (the firmware check, the program guide and the activity history) and **Logs**. Each tab has its own address, and on a phone the tabs share the width with the icon above the name. Nothing about how a setting works changed.
- **Icons.** Settings uses the sliders everywhere (the cog is gone), a tuner's *Overview* tab is a dashboard so the dial is only for limits and testing a signal, and its *Device* tab is a router.

## 2026.10.8 (2026-10-19)

### Added

- **A page for each tuner**, with tabs: *Overview*, *Channels*, *Device*, *Firmware* and *Settings*. The *Tuners* page is now a list of your tuners, and each opens its own page (with one tuner, *Tuners* goes straight to it). The *Overview* leads with how the tuner is now (online, when it was last seen, how many tuners it has), then what is on each tuner, a line about its firmware and its details, and anything with more behind it links to the tab that has it. On a phone the five tabs share the width, so they never scroll.
- **A *Channels* tab, so you rarely need the tuner's own web page.** It lists every channel the tuner has found, and each can be **enabled**, made a **favorite** or **disabled**, the same as on the tuner's page: a disabled channel is left out of the lineup that Plex, Jellyfin and Emby get. Only channels the tuner itself lists can be changed, and each change is checked by asking the tuner again. Tick channels, or **select all** of those shown, to change many at once. **Scan for channels** starts the tuner's own scan (Antenna or Cable) after asking how many streams it would cut off, with progress and a Stop button.
- **Test a channel's signal.** The gauge button on a channel tunes a tuner that nothing is using to it for about ten seconds and shows its signal strength and quality (or *No signal*), colored when it is fair or poor. It never interrupts anything: it only uses a tuner that is free, takes its lock with a key of its own, and gives the tuner back however the test ends. It is greyed out, with the reason, when every tuner is in use or the tuner is scanning.
- **What is known about a channel.** The **i** beside a channel's name opens a card with its number, name and whether it is in the lineup, and, from SiliconDust's program guide, the network it carries, its language (with a flag) and its categories (each with an icon). Anything the guide doesn't say is left out, and nothing is guessed. This reads the guide Antennarr already fetches, so nothing new leaves your network.
- **Filter channels** by quality (HD only), by category and by language, from a **Filters** button next to the search box, with the filters in use shown as chips above the list. The search box has a clear button.
- **Reception on watched channels.** Where a channel has been watched, the *Channels* tab shows its average signal quality over the last 30 days (marked with a Σ), colored when it is fair or poor.
- **More on *Activity*:** how much data each viewer used, the average reception of what they watched, and a page for each viewer (click a name) with their own history.
- **A notification when a viewer starts watching**, off by default (*Settings → Notifications → A viewer starts watching*). It says who, which channel and which tuner, and a viewer is reported at most once every half hour per tuner, so changing channels or a media server connecting again is not a message each time.
- **The logo links to *Live***, and the version sits under the name in the top bar instead of in the footer.

### Changed

- The *Device* and *Firmware* panels and a tuner's own settings (its name and port) moved from the *Tuners* page into the tuner's tabs. Nothing about how they work changed.
- *Channels* and *Test signal* have been tried on a real FLEX 4K: changing a channel, and testing one on a free tuner. Starting a channel scan has not been run on a real tuner.
- Settings switches stay on their title's line on a phone, with the switch at the right.

### Fixed

- **Test signal on a real tuner.** A real tuner answers a lock with the address that holds it, not the key that was set, so Test signal called a free tuner busy and left it locked. It now treats a tuner that refuses the lock as busy and one that takes it as its own, and always gives it back.
- Adding a tuner on the port the admin pages were using no longer fails on systems where the old listener hadn't let go of it yet (it showed up on macOS).

## 2026.10.7 (2026-10-19)

### Added

- **Channel logos never vanish.** Most logos have no background of their own, so white lettering was lost on the light theme and dark artwork on the dark one. Antennarr looks at each logo once in your browser and gives it a thin outline only where it needs one: dark around light artwork in the light theme, light around dark artwork in the dark theme. Logos with a mix of colors are left alone, and there is no box around any of them.
- **Signal strength is drawn as bars** in each tile's footer: four from 80%, then three, two, one, and none under 20%. It is still colored only when reception is fair or poor.
- **Icons on the *Viewers* and *Timeline* headings** on the *Live* page.
- **A tour of the web page** in the docs, with a picture of each page, and new pictures of *Activity*, *Tuners* and *Settings*. The README is shorter: it has a table of the docs, and the details it repeated are in the pages that already had them. See [A tour of the web page](https://github.com/johnschult/antennarr/blob/main/docs/web-ui.md).

### Changed

- The *Live* page's heading says **On the air now**.
- Restarting a tuner from the *Device* panel has now been tried on a real FLEX 4K: it went away and came back in about seven seconds, with no offline message in between. Installing firmware still has not run end to end on a real tuner, since it needs a release newer than the one the tuner is on.
- The docs match what is built: the design notes describe the current layout, the API's state document shows the restart and upgrade fields, and *Troubleshooting* covers what Install and Restart say when something goes wrong and an empty *Activity* page.

## 2026.10.6 (2026-10-19)

### Read before updating

- **Antennarr can now install tuner firmware, when you press *Install*.** On the *Tuners* page, a tuner with a newer firmware release (one the changelog is sure applies to it) has an **Install** button. It asks first, and says how many streams or recordings it will cut off. Antennarr then downloads that release from `download.silicondust.com` and sends it to the tuner over its control port, the way SiliconDust's own tool does, waits for the tuner to restart, and checks that it is running the new release. It only ever installs the latest release for the tuner's own firmware family, never an older one and never by itself, and it never tries a flash twice. This adds one place Antennarr contacts, and only when you press the button: see [what leaves your network](https://github.com/johnschult/antennarr/blob/main/docs/security.md#what-leaves-your-network). SiliconDust doesn't document where its firmware files are, so the address is inferred; if it ever changes, *Install* says so and the firmware check keeps working. The protocol was compared byte for byte with SiliconDust's tool and a real FLEX 4K accepted a re-flash of the firmware it already runs, but the first real update through *Install* is the one to watch.
- **Antennarr now keeps a 30-day history of who watched what.** It is shown on the new *Activity* page and kept in `<settings file>.history.jsonl` beside the settings file: the viewer's name (or *Local* for your own devices), the tuner, the channel, the show titles and when it began and ended. A viewer's address is not stored. It is on by default: switch it off under *Settings → Keep activity history* (that stops recording and hides the page, and what was kept stays), and *Clear activity history* forgets it all. Without a settings file it is kept in memory only. See [Files Antennarr keeps](https://github.com/johnschult/antennarr/blob/main/docs/configuration.md#files-antennarr-keeps).

### Added

- **Install firmware from Antennarr** (see above), with progress, one message when it ends, and the tuner shown as *Updating firmware…* instead of offline while it restarts.
- **A Device panel** on the *Tuners* page: hardware model, firmware, ATSC 3.0 DRM version, device ID, MAC, IP, subnet, channel maps and modulations, read from the tuner when you open it and on **Refresh**. Read-only.
- **Restart a tuner** from the Device panel, behind a confirmation that says what it will cut off. Antennarr checks that the tuner went away and came back, and says so if it didn't.
- **An Activity page**: totals, a bar for each day and the sessions by day, for the last 7 or 30 days, with a switch to turn the history off and a button to clear it, both in *Settings*.
- **What's on, in more detail, on each Live tile**: the program's picture drawn faded behind the top of the tile, its category, episode and description, and how far into it you are. Antennarr fetches the pictures itself, only from SiliconDust's image host, so your browser still contacts nothing outside.
- **A signal quality trend** on each tuner in use, beside the bitrate line. Each line says what it is and its current value.
- **Icons on the owner pills**: a house for your own devices and a globe for viewers. The *Live* pill has a dot that pulses while connected.

### Changed

- **A new theme**, ExploreMiam from tweakcn (indigo and emerald), with the page background from the theme. The fonts are the same. One adjustment: the light theme's secondary text is dark, since white on that green is hard to read.
- **Live tiles are redesigned** and taller. The *Signal* and *Quality* bars are gone: signal quality is a percentage on its trend line, and signal strength is in the footer next to where the stream is going, colored only when reception is fair or poor.
- The *Tuners* page says media servers always see the device (for example *HDHomeRun FLEX 4K*), which is what it has done since the last release.
- US spelling throughout: *program guide*, *color*, *recognize*.

## 2026.10.5 (2026-10-18)

### Read before updating

- **Media servers now always see the tuner's own name.** The name you gave a tuner on the *Tuners* page used to be sent to Plex and the others in place of its model, so renaming a tuner changed what Plex showed. Now they are always told what the tuner calls itself (for example *HDHomeRun FLEX 4K*), and the name you give it is only for Antennarr's own page. A DVR that is already set up in Plex picks up the change by itself, so there is nothing to re-add. A media server can still rename its own DVR.
- **The Connect page is part of adding a viewer.** *Viewers → Add viewer* now walks through three steps: who they are, how their server reaches yours, and what to do in their media server. For a viewer you already have, use the new *Connection steps* button on their row. The old `/setup` link goes to *Viewers*.
- **The top bar no longer shows the tuner.** It has only the *Live* connection pill. Each tuner now has its own header on the *Live* page (see below), so nothing is lost, and with several tuners the top bar can't run out of room.

### Added

- **Notifications to ntfy, Pushover or a webhook**, under *Settings → Notifications*. Switch each kind on or off: new tuner firmware (once per release, not repeated after a restart), a tuner offline for two minutes and then back, a viewer turned away, and an address that isn't a viewer connecting (off by default, and limited, since a tuner port on the internet is scanned all day). Every channel has a **Test** button, and the dialog's **Send test** tries what is in the form before you save it. Nothing is sent, and nothing is contacted, until you add a channel. Tokens and keys are saved in the settings file and are never shown again. Notifications won't go to link-local addresses such as the cloud metadata service. See [Notifications](https://github.com/johnschult/antennarr/blob/main/docs/configuration.md#notifications) and [what leaves your network](https://github.com/johnschult/antennarr/blob/main/docs/security.md#what-leaves-your-network). A small `<settings file>.notified.json` beside the settings file remembers which firmware release was announced.
- **The firmware *Check now* button shows it is working**, and the message says **New firmware found** or **No new firmware**.
- **Each tuner has a header on the *Live* page**: its name, the model beside a name you chose, how many of its tuners are live, its address and a firmware flag. A tuner with no name of its own is titled by its model, shown once.
- **The HDHomeRun logo on each tuner**, and icons on the section headings in *Settings* and on *Sign in with Plex*.
- **Automated accessibility checks** (axe) over every page and dialog, which found and fixed a blank column header on the *Viewers* table.
- **How to run the Go server for development** in [web/README.md](https://github.com/johnschult/antennarr/blob/main/web/README.md). A settings file made by running from the repository is git-ignored.

### Changed

- A viewer's tuner count is a row of small antennas, on *Viewers* and *Live*, and the note under the viewers table is an info box. The first column has more room.
- An idle tuner on *Live* says **Idle** (it said "Ready") and has no antenna icon; one held for you has a TV. The station logo no longer sits against the owner's pill, and the cards are a little taller to make room.
- The Plex, Jellyfin, Emby and HDHomeRun icons are SVGs: sharper, and smaller.
- Text fields ask password managers and the browser not to offer to fill them.
- The README screenshots are retaken with real channel logos. `docs/screenshots/retake.sh` can read the logos from your own tuner's guide when it takes them (the logos are not kept in the repository).

## 2026.10.4 (2026-10-18)

### Changed

- **The Tuners page is denser.** Each tuner is one row: status, name, model, address, port, how many of its tuners are in use, and a flag when firmware is out of date. It opens to its details, a strip showing what is on each of its tuners, and *Manage* and *Firmware* side by side. A single tuner starts open; with several they start folded, so they are easy to scan. The tuner table is replaced by the strip (the Live page still has the full view).

## 2026.10.3 (2026-10-18)

### Read before updating

- **The first-run setup link in 2026.10.2 could not be used from a browser.** Pressing *Make this device an admin* answered "cross-origin requests are not allowed". It is fixed here; update first, then use the link in the log (a new one is written each start).
- **Tuners moved.** Adding, finding, renaming and removing tuners is now on the *Tuners* page (it was in *Settings*). *Settings → Limits* keeps each tuner's limits, and *Settings → Access* and *Privacy and updates* hold the rest.

### Added

- **Change a tuner's port** from *Tuners → Manage*, without a restart. The page follows to the new port. A port that is taken or not valid is refused and the tuner stays where it was. The [API](https://github.com/johnschult/antennarr/blob/main/docs/api.md) takes `listen` on `PATCH devices/{name}`.
- **Icons on every button**, and on the current page in the navigation (always shown in the phone menu).

### Changed

- **Navigation is now Live, Viewers, Tuners, Connect, Settings.** The page you reach from *Connect* is headed "Connect a viewer", and the Tuners page is always headed "Tuners".
- **The two limits are named for what they do:** *Tuners reserved for you* and *Max tuners for viewers*.
- *Remove* on a tuner is red. It already asked you to confirm.
- On *Settings → Access*, the *Who can use your tuners* line links to *Viewers*.
- The README screenshots are retaken with made-up data, and `docs/screenshots/retake.sh` makes them again.
- The web page's build no longer pulls in the `shadcn` package, so `npm audit` is clean.

## 2026.10.2 (2026-10-18)

### Read before updating

- **A new install is closed.** With no viewers, nobody can use the tuner, and only this machine (and any address under `admin`) can open the web page. Before, an install with no viewers was open to anyone who could reach its port. If you rely on that, add a viewer (*Viewers → Add viewer*).
- **First-run setup link.** While nobody is an admin (no address under `admin` and no signed-in Plex account), Antennarr writes a one-time setup link to its log. Open it from your own computer, press *Make this device an admin*, and that address is saved under `admin`. It works once, only from a private network, and only until Antennarr restarts. If you already open the web page from the machine itself you can ignore it. See [Getting started](https://github.com/johnschult/antennarr/blob/main/docs/getting-started.md#3-open-the-web-page).
- **Viewers can only reach what a media server needs.** `discover.json`, `lineup.json`, `status.json` and `lineup_status.json` (read-only) and the streams. Before, every other path was passed to the tuner, so a viewer could reach its rescan, firmware upgrade and own web pages. Those now answer `404` and the log says `path not served` with the path. If a media server needs one that is missing, please [open an issue](https://github.com/johnschult/antennarr/issues) with that line.
- **A viewer inside an `admin` range is an admin too.** That was always so; Antennarr now logs a warning when it happens.

### Added

- **Add, find and remove tuners without editing a file or restarting.** *Settings → Tuners* lists them, with *Add tuner* (it searches the network, or you give an address) and *Remove* (streams on it are cut, and you are told how many first). The same is in the [API](https://github.com/johnschult/antennarr/blob/main/docs/api.md): `POST discover`, `POST devices`, `DELETE devices/{name}`.
- **Start with no tuner.** With none named, Antennarr looks for one for about ten seconds and saves what it finds. If none answers it starts anyway and serves the web page, where you can find or add one. The first tuner added takes over the page's port.
- **The settings file is created when Antennarr starts**, with the defaults, instead of the first time you change something. The default is `antennarr.json` in the directory you run from, and `/config` in the Docker image.

### Changed

- Removing your last viewer no longer asks you to confirm: it just closes the tuner.
- The web page says *No tuner yet* where it used to assume there was one.

## 2026.10.1 (2026-10-17)

- Documentation, screenshots and a *Help* link in the app.
- The container package is public from the first release.

## 2026.10.0 (2026-10-17)

The first release.

- The version, and a link to the project, in the app. Container images are published for `amd64` and `arm64` and numbered by calendar.
- A browser at the bare address is sent to the admin page.
- Fixed matching of IPv6 link-local addresses.
- A phone layout: a navigation menu, and cards instead of sideways-scrolling tables.
