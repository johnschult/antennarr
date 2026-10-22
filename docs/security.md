# Security

## What protects what

**HDHomeRun tuners have no passwords, and neither does the tuner side of Yagi.** The protection is the *address a request comes from*.

| Who | Can do | How they are recognized |
|---|---|---|
| A **viewer** | Use the tuner: `discover.json`, `lineup.json`, `status.json`, `lineup_status.json` and the streams, within their limits. Read-only: nothing else the tuner offers (rescans, firmware upgrade, its web pages) is reachable and answers `404`. | Their address is listed under *Viewers*. |
| An **admin** | Open the web page and change settings: add viewers, change limits, and so on. | They connect from this machine (loopback), from an address listed under `admin`, or they are signed in with an allowed Plex account. |
| Anyone else | Nothing. The tuner side answers `403`. The admin pages answer `403` (the page itself loads so you can sign in, but shows nothing). | |

**A new install is closed.** With no viewers nobody can use the tuner, and only this machine can open the admin page. Nothing is open to the network until you decide.
To let you in from another computer, Yagi writes a **one-time setup link** to its log while nobody is an admin yet (see [Getting started](getting-started.md)). The token in it is random (256 bits), kept only in memory, works once, and changes at every start; it can only be used from a private network (LAN, Tailscale, a VPN), five wrong tries lock an address out for ten minutes, and opening the link only shows a confirmation page: the address is saved as an admin only when you press the button. Because the link is in the log, anyone who can read the log can claim it until you do.
A viewer is not an admin: being allowed to watch doesn't let you change anything. The exception is a viewer whose address falls inside an `admin` range: they are an admin too, and Yagi logs a warning. Keep `admin` to the addresses that are really yours.

If a media server needs a tuner page that isn't on the list, Yagi logs `path not served` with the path. Please open an issue with that line.

## Keep it off the open internet

Use Tailscale, a VPN or a private network ([Networking](networking.md)). If you must expose the port, list only the viewer's public address in Yagi *and* in your firewall.
Yagi speaks plain HTTP: it has no TLS of its own, so anything sent to it can be read by anyone on the path. On Tailscale or a VPN the traffic is already encrypted. The one exception is the `-tailscale` image, which can also serve the admin pages over HTTPS at its tailnet name, with a certificate from Tailscale ([Built-in Tailscale](tailscale.md#https-for-the-admin-page)); that name then appears in a public certificate log.

If you put Yagi **behind a reverse proxy**, every request looks like it came from the proxy's address, so the allowlist and admin-by-address can't tell people apart. Either don't proxy the tuner side, or make sure the proxy is the only thing that can reach it and handle access there.
Set `X-Forwarded-Proto: https` on proxied requests so the sign-in cookie is marked `Secure`.

## Built-in Tailscale

In the `-tailscale` image, Yagi can join your tailnet as its own device ([Built-in Tailscale](tailscale.md)). What that changes for security:

- **Who is on the other end is the node's word.** A viewer or admin listed as `tailscale:name@example.com` (or a device) is matched only when Yagi's own node says that is who a connection came from; a connection from the LAN, or a header that claims a login, never counts. Removing someone from your tailnet takes effect on their next connection.
- **Only on the tailnet** refuses everything that arrives over the machine's own address except the machine itself, and closes the connections already open from the LAN when it goes on. It can only be switched on while the node is connected, and it is not in force while the node is down, so an outage doesn't lock you out; `-allow-lan` is the way back in. It is a gate in Yagi, not a firewall: the port is still open to a scan.
- **A tagged device has no login**, so name it by its tailnet name. A tailnet-wide admin entry (`100.64.0.0/10`) makes every device on it an admin, so don't put your viewers on it.
- The node's identity (the device's key) is kept in a `tailscale` folder beside the settings file. Treat it like the settings file: anyone who can read it can be that device.

## Sign in with Plex

Optional, off by default, and switched on under *Settings → Access*. It lets chosen Plex accounts open the admin pages from any address that can reach the port, without listing their addresses.

- Yagi uses plex.tv's sign-in flow **on the server**. Your Plex token never reaches the browser, and it is thrown away as soon as Yagi knows who you are. Only the numeric account ID and name are kept, in the settings file.
- Only accounts you have added (by ID, which never changes, not by name) can sign in. Removing an account, or turning the feature off, signs it out on the very next request.
- Sessions last up to 12 hours idle or 7 days in all. The cookie (`yagi_session`) is `HttpOnly`, `SameSite=Strict`, limited to `/`, and `Secure` when the request came over HTTPS.
- Starting a sign-in is rate-limited per address and the number in progress is capped.
- Nothing contacts plex.tv until someone signs in. It appears in your Plex account's authorised devices as "Yagi" (the `clientId` in the settings file).
- To add the first account you need to be an admin another way (this machine, or an `admin` address): turn the switch on, then *Add a Plex account*.

## The web page and its API

- The admin API refuses cross-site writes: they must be JSON, and an `Origin` header, if there is one, must be this host. A page you happen to visit can't change Yagi's settings from your browser.
- Request bodies are limited to 64 KB and unknown fields are rejected.
- The web page is served with a strict Content-Security-Policy: it loads nothing from other sites, so channel logos and program pictures come through Yagi, not straight from SiliconDust.
- Settings are validated before they are saved.

## What leaves your network

Only your tuners, plus these. Each can be switched off or left unset, and Yagi contacts nothing else.

| What | Where | How often | What is sent |
|---|---|---|---|
| Firmware changelog | `silicondust.com` | every 12 hours | Nothing about you or your tuners. |
| Firmware file | `download.silicondust.com` | only when an admin presses Install | Nothing about you or your tuners. |
| Program guide, channel logos and program pictures | `api.hdhomerun.com`, `img.hdhomerun.com` | every 6 hours | Your tuner's guide key (its `DeviceAuth`), which is how SiliconDust knows which channels to list. |
| Sign in with Plex | `plex.tv` | when someone signs in | The sign-in request. |
| Enrolling with an invite | `plex.tv` | when a person signs in on an invite link | The sign-in request, and then the person's Plex token, once, to ask who signed in and which servers they have. The token is not kept. |
| Notifications | the ntfy server, `api.pushover.net`, or the webhook address **you** set up | when something you switched on happens, and when you press Test | A short message: a title and a sentence naming a tuner, a viewer or an address, for example `Mom was refused a tuner`, `192.0.2.9 tried to use Living room`, or `Living room is offline`. Nothing is sent, and nothing is contacted, until you add a channel. |


Notifications go only where you point them. If you use `ntfy.sh` (or any server you don't run), what is in the message is visible to that service, and on `ntfy.sh` to anyone who knows your topic, so pick a topic that is hard to guess or run your own server. Yagi doesn't follow redirects when it sends one. It will send to this machine or a private or LAN address on purpose, since a server on your own network is the usual case, but it refuses link-local addresses (`169.254.0.0/16`, which includes the cloud metadata service at `169.254.169.254`, and `fe80::/10`), multicast and the unspecified address. That is checked when you save the channel and again when the message is sent, after any name has been looked up. Only an admin can set where messages go.
The tokens, user keys and webhook addresses are saved in the settings file (readable only by its owner) and are never in the admin API's output, the logs or error messages. A webhook address often has a secret in it, so the page shows only its host; the way to change it is to enter a new one.

The guide key stays on the server: it isn't in the admin API's output or the logs, and error messages from fetching the guide have it stripped out.
One thing to know: `discover.json` passes the tuner's own `DeviceAuth` through to viewers, the same as the tuner would to anyone on its network. It lets someone fetch that tuner's guide data from SiliconDust, which is low risk, but it is there.

## Invite links

An invite is a link the owner sends so a person can add themselves as a viewer. They open it, sign in with Plex, pick one of the Plex servers they own, and Yagi allows that server's public internet address with the tuner limit the invite carries. It is off until you turn on *Invite viewers with Plex* in *Settings → Plex*, which is separate from signing in to the admin pages with Plex.

- **The link is a password.** It carries a random 256-bit token, shown once when the invite is made. Only a hash of it is saved, so the settings file can't be used to enroll anyone. A wrong, expired or revoked link, and a link while enrollment is off, are all the same plain `404`, and wrong links from one address are limited to five every ten minutes.
- **It expires.** An invite nobody has used stops working after seven days. Once used it keeps working until you revoke it, because opening it again is how a person brings their address up to date when their internet address changes; the account that enrolled is the only one that can.
- **The browser never supplies an address.** Yagi asks plex.tv which servers the account **owns**, and offers only those with a public internet address: not a server that is only shared with them, not a relay, not a private or Tailscale address. What the person picks is a server; the addresses come from plex.tv. An address that another viewer already covers is refused, with a message that doesn't say who.
- **The Plex token is used once.** It is held only while two questions go to plex.tv (who is this, and what servers do they have) and is in no log, file or reply. The sign-in is its own: the page that does it can't sign anyone in to the admin pages or add an admin, and the admin sign-in can't finish one of these.
- **The page is public.** It is on the admin port, so it is reachable wherever you already let people reach that port (the *Tailnet only* switch still applies). It is plain HTML with no scripts and a content security policy that allows nothing else.

**What this does and doesn't tell you.** Plex does not authenticate the streams a server fetches from a tuner, so this verifies who **owns** the server, not which Plex user is watching. And an address is shared by everything behind it: if a person's server is behind a connection that other people also use, all of them are allowed with it.

## What is checked automatically

Every change, and the main branch each week, is checked in the open on GitHub:

- **Tests** with the Go race detector, the web page's tests and its type check, and **golangci-lint** (settings in `.golangci.yml`).
- **Known vulnerabilities**: `govulncheck` for the Go code (it only reports a vulnerability the code can reach) and `npm audit` for the web page's packages, which fail a change; Dependabot proposes updates and raises an alert for a vulnerable dependency.
- **CodeQL**, GitHub's code scanning (its default setup, switched on in the repository's settings), for the Go and TypeScript code and the workflows themselves.
- **Trivy** on the container image, reported to the *Security* tab (it does not fail a change, since a problem in the base image is not what the change did).
- The **OpenSSF Scorecard**, which checks the repository's own practices and publishes the score; the actions the workflows use are pinned to an exact commit.

What these find is on the repository's *Security* tab.

## Reporting a problem

If you find a security problem, please report it privately with **Report a vulnerability** on the repository's *Security* tab, not in a public issue.
