# Changelog

<!-- Each section is also used as the text of a GitHub release, where a relative link would not work. Link to docs with full https://github.com/johnschult/antennarr/blob/main/... addresses. -->

What changed in each release, newest first. Releases are numbered `YYYY.MM.PATCH`; see [Releasing](https://github.com/johnschult/antennarr/blob/main/docs/RELEASING.md).
Each release's notes on GitHub are its section of this file.

## 2026.10.5 (2026-10-18)

### Read before updating

- **Media servers now always see the tuner's own name.** The name you gave a tuner on the *Tuners* page used to be sent to Plex and the others in place of its model, so renaming a tuner changed what Plex showed. Now they are always told what the tuner calls itself (for example *HDHomeRun FLEX 4K*), and the name you give it is only for Antennarr's own page. A DVR that is already set up in Plex may keep the old name until Plex reads the device again or you add the DVR again. A media server can still rename its own DVR.
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
