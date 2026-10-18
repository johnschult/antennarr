# Antennarr web UI

React 19 + Vite + TypeScript, Tailwind v4 and shadcn/ui. It is built into
`../internal/ui/dist`, which the Go binary embeds and serves at `/antennarr/`.

```sh
npm install
npm run dev        # http://localhost:5173/antennarr/, proxying the API to a running Antennarr
npm test           # vitest
npm run typecheck
npm run build      # into ../internal/ui/dist; then `go build ./cmd/antennarr`
```

`npm run dev` proxies `/antennarr/api` to `http://127.0.0.1:5004`; point
`ANTENNARR_URL` at another instance to develop against it. The dev server's
address must be allowed to use the admin page (loopback always is).

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

