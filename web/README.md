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
- **`npm audit` reports 7 high findings** in `braces`, reached through the `shadcn` CLI (`shadcn` → `fast-glob` → `micromatch` → `braces`). `npm audit --omit=dev` reports none, and nothing from that chain is in the built UI: `shadcn` is a dev dependency and only its CSS is imported. There is no patched `braces` yet, and the `npm audit fix --force` suggestion would downgrade `shadcn` to 1.0.0, which breaks the build. Re-check after `npm update`.

## Third-party assets

- `public/plex-wordmark.png` and `public/plex-icon.png` are resized copies of the Plex logos from
  [homarr-labs/dashboard-icons](https://github.com/homarr-labs/dashboard-icons) and
  [selfhst/icons](https://github.com/selfhst/icons). Plex is a trademark of Plex, Inc.; they are used only to
  label the optional "Sign in with Plex" button. They are bundled rather than loaded from a CDN, so the page makes no
  outside requests.
- `public/jellyfin-icon.png` and `public/emby-icon.png` are resized copies of the app icons from
  [selfhst/icons](https://github.com/selfhst/icons), shown beside the media server names on the Setup page
  (`plex-icon.png` above is from the same set). Jellyfin and Emby are trademarks of their owners; the icons only label
  the matching instructions and don't imply any affiliation. They are bundled, not loaded from a CDN.
- Fonts (Open Sans, Mozilla Headline, JetBrains Mono) come from Google Fonts via the Fontsource npm packages, also
  bundled.

