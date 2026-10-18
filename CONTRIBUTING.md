# Contributing

Thank you for wanting to help. Bug reports, ideas and pull requests are all welcome.

## Reporting a problem or asking for something

[Open an issue](https://github.com/johnschult/antennarr/issues). For a bug, include the version (shown at the bottom of the web page, or `antennarr -version`), how you run Antennarr, what you did and what you expected, and the relevant lines of its log.
[Troubleshooting](docs/troubleshooting.md) says what else is useful. Security problems go through [SECURITY.md](SECURITY.md), not a public issue.

## Changing the code

`main` is protected: every change arrives by pull request, and the checks must pass (**Go**, **Web UI**, and the image build when the `Dockerfile` changes).

```sh
git checkout -b my-change
# …make the change, with tests…
git push -u origin my-change     # then open a pull request
```

Keep a pull request to one thing. Say what changed and why, and how you tested it. If it changes what people see or do, update the docs in `docs/` in the same pull request. Releases get their notes from [`CHANGELOG.md`](CHANGELOG.md), which is written when a release is made, not in every pull request.

### Setting up

You need Go (see `go.mod` for the version) and Node 22.12 or newer.

```sh
go test -race ./...                       # the server
cd web && npm ci && npm test              # the web UI (Vitest)
cd web && npx tsc -b                      # type-check the UI
cd web && npm run dev                     # the UI with hot reload, against a running Antennarr (see web/README.md)
(cd web && npm run build) && go build -o antennarr ./cmd/antennarr    # the real thing: the UI is built first, because the program embeds it
```

### How the code is laid out

| Folder | What is in it |
|---|---|
| `cmd/antennarr` | The command: flags, the `discover` command, reloading on `SIGHUP`. |
| `internal/proxy` | The rewriting proxy for the tuner's own address. |
| `internal/tuner` | Watches a tuner: online or offline, who is using it, finding it again by DeviceID. |
| `internal/discovery` | Finding HDHomeRuns: a UDP broadcast, and a scan of the nearby subnet. |
| `internal/access` | Who may use the tuners and how many: the settings, viewers, limits, admins. |
| `internal/api` | The admin JSON API the web page uses. |
| `internal/server` | Puts it together: one listener per device, the admin pages, the sign-in, the checks. |
| `internal/plexauth` | Sign in with Plex (server side), with sessions kept across restarts. |
| `internal/firmware`, `internal/guide` | The firmware changelog and the programme guide. |
| `internal/netinfo` | This machine's addresses and firewall, for the Setup page. |
| `internal/live`, `internal/httpx`, `internal/version`, `internal/ui` | Event notifications, shared HTTP helpers, the version, the embedded web page. |
| `web` | The web UI: React, TypeScript, Vite, Tailwind and shadcn/ui. See `web/README.md`. |
| `docs` | The documentation, and the design notes in `docs/DESIGN.md`. |

### What a good change looks like

- **It has tests.** The Go side uses the standard library only (there are no third-party Go dependencies, and that is on purpose), and tests use fake tuners and fake plex.tv and SiliconDust servers. **A test must never reach the real network.** Run Go tests with `-race`.
- **The UI tests** use Vitest and Testing Library. The admin API's types in `web/src/lib/types.ts` must be kept in step with `internal/api/api.go`.
- **Tests don't depend on the date they run.** Use an injected clock or build times relative to now.
- **Nothing leaves the network without a setting** that turns it off, and it is listed in [Security](docs/security.md#what-leaves-your-network).
- **It fails loudly.** The settings file rejects unknown keys on purpose. Prefer a clear error to a guess.
- **It reads like the code around it**: naming, comments, and level of detail.
- **Look at it in a browser** if you change the UI, in both the light and dark themes and at a phone width (about 390 px).

### Commit messages

A short summary line in the imperative ("Show the version in the footer"), then a paragraph about *why* if it isn't obvious.

## Releases

Releases are numbered by calendar, `YYYY.MM.PATCH`, and are made by tagging `main`. See [docs/RELEASING.md](docs/RELEASING.md).

## License

By contributing you agree that your work is released under the [MIT License](LICENSE).
