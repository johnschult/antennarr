# Contributing

Thank you for wanting to help. Bug reports, ideas and pull requests are all welcome.

## Reporting a problem or asking for something

[Open an issue](https://github.com/johnschult/yagi/issues). For a bug, include the version (shown at the bottom of the web page, or `yagi -version`), how you run Yagi, what you did and what you expected, and the relevant lines of its log.
[Troubleshooting](https://johnschult.github.io/yagi/troubleshooting/) says what else is useful. Security problems go through [SECURITY.md](SECURITY.md), not a public issue.

## Changing the code

`main` is protected: every change arrives by pull request, and the checks must pass (**Go**, **Web UI**, and the image build when the `Dockerfile` changes).

```sh
git checkout -b my-change
# …make the change, with tests…
git push -u origin my-change     # then open a pull request
```

Keep a pull request to one thing. Say what changed and why, and how you tested it. If it changes what people see or do, update the docs in `site/src/content/docs/` in the same pull request. Releases get their notes from [`CHANGELOG.md`](CHANGELOG.md), which is written when a release is made, not in every pull request.

### Setting up

You need Go (see `go.mod` for the version) and Node 22.12 or newer.

```sh
go test -race ./...                       # the server
golangci-lint run ./...                   # the linters CI runs (settings in .golangci.yml); https://golangci-lint.run/welcome/install/
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...   # known vulnerabilities the code can reach
cd web && npm ci && npm test              # the web UI (Vitest)
cd web && npx tsc -b                      # type-check the UI
go run ./cmd/yagi -config /tmp/yagi-dev.json   # the Go server (the API), on :9244; start this first
cd web && npm run dev                     # the UI with hot reload, in a second terminal; it proxies to the server above (see web/README.md)
./build.sh                                # the real thing: builds the UI, then the program that embeds it
```

### How the code is laid out

| Folder | What is in it |
|---|---|
| `cmd/yagi` | The command: flags, the `discover` command, reloading on `SIGHUP`. |
| `internal/proxy` | The rewriting proxy for the tuner's own address. |
| `internal/tuner` | Watches a tuner: online or offline, who is using it, finding it again by DeviceID. |
| `internal/discovery` | Finding HDHomeRuns: a UDP broadcast, and a scan of the nearby subnet. |
| `internal/access` | Who may use the tuners and how many: the settings, viewers, limits, admins. |
| `internal/api` | The admin JSON API the web page uses. |
| `internal/server` | Puts it together: one listener per device, the admin pages, the sign-in, the checks. |
| `internal/plexauth` | Sign in with Plex (server side), with sessions kept across restarts. |
| `internal/firmware`, `internal/guide` | The firmware changelog and the program guide. |
| `internal/netinfo` | This machine's addresses and firewall, for the Connect page. |
| `internal/live`, `internal/httpx`, `internal/version`, `internal/ui` | Event notifications, shared HTTP helpers, the version, the embedded web page. |
| `web` | The web UI: React, TypeScript, Vite, Tailwind and shadcn/ui. See `web/README.md`. |
| `site` | The documentation site (Astro and Starlight): the pages are in `site/src/content/docs/`. |

### What a good change looks like

- **It has tests.** The Go side uses the standard library only (there are no third-party Go dependencies, and that is on purpose), and tests use fake tuners and fake plex.tv and SiliconDust servers. **A test must never reach the real network.** Run Go tests with `-race`.
- **The UI tests** use Vitest and Testing Library. The admin API's types in `web/src/lib/types.ts` must be kept in step with `internal/api/api.go`.
- **Accessibility is checked with axe** in `web/src/a11y.test.tsx`: every page in each of its states, the phone menu, and every dialog. Add a new page or dialog there. jsdom has no layout, so color contrast and focus order still need a look in a browser.
- **Tests don't depend on the date they run.** Use an injected clock or build times relative to now.
- **Nothing leaves the network without a setting** that turns it off, and it is listed in [Security](https://johnschult.github.io/yagi/security/#what-leaves-your-network).
- **It fails loudly.** The settings file rejects unknown keys on purpose. Prefer a clear error to a guess.
- **It reads like the code around it**: naming, comments, and level of detail.
- **Look at it in a browser** if you change the UI, in both the light and dark themes and at a phone width (about 390 px).

### Commit messages

A short summary line in the imperative ("Show the version in the footer"), then a paragraph about *why* if it isn't obvious.

## Releases

Releases are numbered by calendar, `YYYY.MM.PATCH`, and are made by tagging `main`. See [Releasing](https://johnschult.github.io/yagi/releasing/).

## License

By contributing you agree that your work is released under the [MIT License](LICENSE).

## Retaking the README screenshots

`scripts/screenshots/retake.sh` regenerates every image in `site/public/screenshots/`. It runs the demo harness (`internal/server/demo_test.go`, build tag `demo`), which serves the real UI against a fake HDHomeRun, a fake guide and made-up viewers, then drives Chromium with `shoot.py` (needs Python with Playwright). Nothing real appears in the images. Build the web UI first (`npm run build` in `web/`), then run the script from the repo root. The demo also has switches for trying things by hand: `DEMO_SIGNAL=1` makes the signal quality drift, `DEMO_SIGNAL=sweep` also sweeps the signal strength through every icon, `DEMO_UPDATE=1` makes the tuner a release behind so firmware *Install* can be tried, and `DEMO_LOGOS=<folder>` uses your own logos (see `retake.sh`).
