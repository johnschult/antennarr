# Releasing

Releases are numbered by calendar: **`YYYY.MM.PATCH`**. `2026.10.0` is the first release of
October 2026, and `2026.10.1` a fix to it. `PATCH` counts up within a month and starts again at 0 in the next.
The git tag is the same number with a `v` in front: `v2026.10.0`.

The running app shows its version in the footer, linked to the release's page on GitHub. A build from a
checkout says `dev` (with the commit, e.g. `dev+3553344`); `antennarr -version` prints it too.

## To release

`main` is protected, so changes arrive by pull request. Once what you want is on `main`:

```sh
git checkout main && git pull
git tag v2026.10.0
git push origin v2026.10.0
```

The **Release** workflow then:

1. checks the tag looks like `vYYYY.MM.PATCH` (anything else fails, so a typo can't publish);
2. builds the image for `linux/amd64` and `linux/arm64`, with the version built in;
3. pushes it to `ghcr.io/johnschult/antennarr` as `2026.10.0` and `latest`;
4. creates the GitHub release, with notes generated from the merged pull requests.

Pull requests that change the `Dockerfile` or the workflow build the image without pushing it.

## After a release

Check that the image can be pulled without logging in, from a machine that isn't signed in to GitHub:

```sh
docker pull ghcr.io/johnschult/antennarr:2026.10.0
```

The first release (2026.10.0) was public straight away. If a package ever starts out private, open its settings
(github.com/johnschult?tab=packages → *antennarr* → *Package settings*) and change its visibility to **Public**.
