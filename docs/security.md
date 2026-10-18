# Security

## What protects what

**HDHomeRun tuners have no passwords, and neither does the tuner side of Antennarr.** The protection is the *address a request comes from*.

| Who | Can do | How they are recognised |
|---|---|---|
| A **viewer** | Use the tuner: `discover.json`, `lineup.json`, `status.json`, `lineup_status.json` and the streams, within their limits. Read-only: nothing else the tuner offers (rescans, firmware upgrade, its web pages) is reachable and answers `404`. | Their address is listed under *Viewers*. |
| An **admin** | Open the web page and change settings: add viewers, change limits, and so on. | They connect from this machine (loopback), from an address listed under `admin`, or they are signed in with an allowed Plex account. |
| Anyone else | Nothing. The tuner side answers `403`. The admin pages answer `403` (the page itself loads so you can sign in, but shows nothing). | |

**A new install is closed.** With no viewers nobody can use the tuner, and only this machine can open the admin page. Nothing is open to the network until you decide.
To let you in from another computer, Antennarr writes a **one-time setup link** to its log while nobody is an admin yet (see [Getting started](getting-started.md)). The token in it is random (256 bits), kept only in memory, works once, and changes at every start; it can only be used from a private network (LAN, Tailscale, a VPN), five wrong tries lock an address out for ten minutes, and opening the link only shows a confirmation page: the address is saved as an admin only when you press the button. Because the link is in the log, anyone who can read the log can claim it until you do.
A viewer is not an admin: being allowed to watch doesn't let you change anything. The exception is a viewer whose address falls inside an `admin` range: they are an admin too, and Antennarr logs a warning. Keep `admin` to the addresses that are really yours.

If a media server needs a tuner page that isn't on the list, Antennarr logs `path not served` with the path. Please open an issue with that line.

## Keep it off the open internet

Use Tailscale, a VPN or a private network ([Networking](networking.md)). If you must expose the port, list only the viewer's public address in Antennarr *and* in your firewall.
Antennarr speaks plain HTTP: it has no TLS of its own, so anything sent to it can be read by anyone on the path. On Tailscale or a VPN the traffic is already encrypted.

If you put Antennarr **behind a reverse proxy**, every request looks like it came from the proxy's address, so the allowlist and admin-by-address can't tell people apart. Either don't proxy the tuner side, or make sure the proxy is the only thing that can reach it and handle access there.
Set `X-Forwarded-Proto: https` on proxied requests so the sign-in cookie is marked `Secure`.

## Sign in with Plex

Optional, off by default, and switched on under *Settings → Access*. It lets chosen Plex accounts open the admin pages from any address that can reach the port, without listing their addresses.

- Antennarr uses plex.tv's sign-in flow **on the server**. Your Plex token never reaches the browser, and it is thrown away as soon as Antennarr knows who you are. Only the numeric account ID and name are kept, in the settings file.
- Only accounts you have added (by ID, which never changes, not by name) can sign in. Removing an account, or turning the feature off, signs it out on the very next request.
- Sessions last up to 12 hours idle or 7 days in all. The cookie (`antennarr_session`) is `HttpOnly`, `SameSite=Strict`, limited to `/antennarr/`, and `Secure` when the request came over HTTPS.
- Starting a sign-in is rate-limited per address and the number in progress is capped.
- Nothing contacts plex.tv until someone signs in. It appears in your Plex account's authorised devices as "Antennarr" (the `clientId` in the settings file).
- To add the first account you need to be an admin another way (this machine, or an `admin` address): turn the switch on, then *Add a Plex account*.

## The web page and its API

- The admin API refuses cross-site writes: they must be JSON, and an `Origin` header, if there is one, must be this host. A page you happen to visit can't change Antennarr's settings from your browser.
- Request bodies are limited to 64 KB and unknown fields are rejected.
- The web page is served with a strict Content-Security-Policy: it loads nothing from other sites, so channel logos come through Antennarr, not straight from SiliconDust.
- Settings are validated before they are saved.

## What leaves your network

Only your tuners, plus these. Each can be switched off or left unset, and Antennarr contacts nothing else.

| What | Where | How often | What is sent |
|---|---|---|---|
| Firmware changelog | `silicondust.com` | every 12 hours | Nothing about you or your tuners. |
| Programme guide, channel logos | `api.hdhomerun.com`, `img.hdhomerun.com` | every 6 hours | Your tuner's guide key (its `DeviceAuth`), which is how SiliconDust knows which channels to list. |
| Sign in with Plex | `plex.tv` | when someone signs in | The sign-in request. |
| Notifications | the ntfy server, `api.pushover.net`, or the webhook address **you** set up | when something you switched on happens, and when you press Test | A short message: a title and a sentence naming a tuner, a viewer or an address, for example `Mom was refused a tuner`, `192.0.2.9 tried to use Living room`, or `Living room is offline`. Nothing is sent, and nothing is contacted, until you add a channel. |


Notifications go only where you point them. If you use `ntfy.sh` (or any server you don't run), what is in the message is visible to that service, and on `ntfy.sh` to anyone who knows your topic, so pick a topic that is hard to guess or run your own server. Antennarr doesn't follow redirects when it sends one, and it will send to a private or LAN address on purpose, since a server on your own network is the usual case.
The tokens, user keys and webhook addresses are saved in the settings file (readable only by its owner) and are never in the admin API's output, the logs or error messages. A webhook address often has a secret in it, so the page shows only its host; the way to change it is to enter a new one.

The guide key stays on the server: it isn't in the admin API's output or the logs, and error messages from fetching the guide have it stripped out.
One thing to know: `discover.json` passes the tuner's own `DeviceAuth` through to viewers, the same as the tuner would to anyone on its network. It lets someone fetch that tuner's guide data from SiliconDust, which is low risk, but it is there.

## Reporting a problem

If you find a security problem, please report it privately with **Report a vulnerability** on the repository's *Security* tab, not in a public issue.
