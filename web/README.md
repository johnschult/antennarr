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
