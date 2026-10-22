# Yagi web UI

React 19 + Vite + TypeScript, Tailwind v4 and shadcn/ui. It is built into
`../internal/ui/dist`, which the Go binary embeds and serves at `/`.

```sh
npm install
npm run dev        # http://localhost:5173/, proxying the API to a running Yagi
npm test           # vitest
npm run typecheck
npm run build      # into ../internal/ui/dist; then `go build ./cmd/yagi`
```

## Running it for development

`npm run dev` only serves the page. The API behind it is the Go program, so you run **two things**, each in its own terminal:

```sh
# 1. The Go server, from the repository root. It is the API, on the admin port, 9244.
go run ./cmd/yagi -config /tmp/yagi-dev.json

# 2. The UI with hot reload, from web/.
npm run dev        # then open http://localhost:5173/
```

- **No tuner needed.** With no `-tuner`, Yagi looks for HDHomeRuns for about ten seconds, and if it finds none it starts anyway with just the web page. That is enough to work on the UI (you can add a tuner from the Tuners page). To use your own, add `-tuner 192.168.1.50`.
- **Keep the settings file out of the repository.** `-config` names a file that Yagi creates and saves to, and it can hold notification tokens, so use a path outside the repo as above. (If you do put one in the repo, `yagi*.json` is ignored by git.) Delete the file to start again from the defaults.
- **If the page says 502, or the terminal says `http proxy error` and `ECONNREFUSED 127.0.0.1:9244`,** the Go server isn't running (or is on another port). Start step 1. Without `-config` nothing can be saved, so adding a viewer or a channel is refused.
- **Another port or machine:** `YAGI_URL=http://host:port npm run dev`. Loopback is always allowed to use the admin page; for anything else, add its address under `admin` in the settings file.
- **The embedded copy.** The Go server also serves the last *built* UI at `http://localhost:9244/`. That one only changes when you run `npm run build` (the Go program embeds it), so use port 5173 while you are editing.

The UI talks only to the JSON API in `internal/api`. Its types are in
`src/lib/types.ts` and must be kept in step with `internal/api/api.go`.

## Install warnings

- **Install scripts.** npm 12 asks you to review packages that run scripts when installed. `package.json`'s `allowScripts` records the answers; `fsevents` (macOS file watching) is denied, since it ships its prebuilt binary. Review new ones with `npm install-scripts ls`.
- **`npm audit` is clean on purpose.** The `shadcn` CLI package pulled in `braces`, which has an unpatched advisory and no maintained release, so `shadcn` is not a dependency. The one file the build used from it is copied to `src/shadcn.css`. Add components with `npx shadcn@latest add <name>`; if that wants newer base styles, copy them into that file.

## Third-party assets

- `public/plex-wordmark.png` is a resized copy of the Plex wordmark from
  [homarr-labs/dashboard-icons](https://github.com/homarr-labs/dashboard-icons) and
  [selfhst/icons](https://github.com/selfhst/icons). Plex is a trademark of Plex, Inc.; it is used only to
  label the optional "Sign in with Plex" button. It is bundled rather than loaded from a CDN, so the page makes no
  outside requests.
- `public/plex-icon.svg`, `public/jellyfin-icon.svg`, `public/emby-icon.svg` and `public/hdhomerun-icon.svg` are the
  SVG icons from [homarr-labs/dashboard-icons](https://github.com/homarr-labs/dashboard-icons) (dashboardicons.com),
  shown beside the media server names on the Setup page and on each tuner. Plex, Jellyfin, Emby and HDHomeRun are
  trademarks of their owners; the icons only label the matching instructions and tuners and don't imply any
  affiliation. They are bundled, not loaded from a CDN.
- Fonts (Open Sans, Mozilla Headline, JetBrains Mono) come from Google Fonts via the Fontsource npm packages, also
  bundled.

