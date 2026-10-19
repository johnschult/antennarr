# Releasing

Releases are numbered by calendar: **`YYYY.MM.PATCH`**. `2026.10.0` is the first release of
October 2026, and `2026.10.1` a fix to it. `PATCH` counts up within a month and starts again at 0 in the next.
The git tag is the same number with a `v` in front: `v2026.10.0`.

The running app shows its version in the top bar, under its name, linked to the release's page on GitHub. A build from a
checkout says `dev` (with the commit, e.g. `dev+3553344`); `antennarr -version` prints it too.

## To release

`main` is protected, so changes arrive by pull request. First write the release's entry in [`CHANGELOG.md`](../CHANGELOG.md), in a pull request like any other change: a `## 2026.10.2 (2026-10-18)` heading, then what changed. Put anything that changes what an existing install does under *Read before updating*. Link to the docs with full `https://github.com/...` addresses, because the section is shown on a GitHub release page where relative links break. The release's notes on GitHub are that section, and a tag with no section fails before anything is published.

The entry is also what people see in the app: after an update, Antennarr pops up the notes for the releases since the last one that browser saw (and *What's new* in the footer opens them on request). So write it for them, and keep to the format: a `## YYYY.MM.PATCH (YYYY-MM-DD)` heading, then `### Group` headings (*Read before updating* first, which is drawn as a notice) with `- ` bullets under them. Inside a bullet, `**bold**`, `*italic*`, `` `code` `` and `[links](https://…)` are drawn; anything else is shown as text. A test reads the real `CHANGELOG.md` and fails if an entry isn't in that shape. The notes are bundled into the web page when it is built (the `Dockerfile` copies the file into the web stage), so they are always the ones for the version running, and nothing is fetched from anywhere.

Once the entry is on `main`:

```sh
git checkout main && git pull
git tag v2026.10.2
git push origin v2026.10.2
```

The **Release** workflow then:

1. checks the tag looks like `vYYYY.MM.PATCH` and that `CHANGELOG.md` has a section for it (otherwise it fails, so a typo or a forgotten entry can't publish);
2. builds the image for `linux/amd64` and `linux/arm64`, with the version built in;
3. pushes it to `ghcr.io/johnschult/antennarr` as `2026.10.0` and `latest`;
4. creates the GitHub release: the changelog section first, then the list of merged pull requests that GitHub generates.

Pull requests that change the `Dockerfile` or the workflow build the image without pushing it.

## After a release

Check that the image can be pulled without logging in, from a machine that isn't signed in to GitHub:

```sh
docker pull ghcr.io/johnschult/antennarr:2026.10.0
```

The first release (2026.10.0) was public straight away. If a package ever starts out private, open its settings
(github.com/johnschult?tab=packages → *antennarr* → *Package settings*) and change its visibility to **Public**.
