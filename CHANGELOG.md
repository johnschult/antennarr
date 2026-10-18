# Changelog

<!-- Each section is also used as the text of a GitHub release, where a relative link would not work. Link to docs with full https://github.com/johnschult/antennarr/blob/main/... addresses. -->

What changed in each release, newest first. Releases are numbered `YYYY.MM.PATCH`; see [Releasing](https://github.com/johnschult/antennarr/blob/main/docs/RELEASING.md).
Each release's notes on GitHub are its section of this file.

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
