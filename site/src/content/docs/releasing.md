---
title: "Releasing"
---

Releases are numbered by calendar: **`YYYY.MM.PATCH`**. `2026.10.0` is the first release of
October 2026, and `2026.10.1` a fix to it. `PATCH` counts up within a month and starts again at 0 in the next.
The git tag is the same number with a `v` in front: `v2026.10.0`.

The running app shows its version in the footer, first among the links, linked to the release's page on GitHub. A build from a
checkout says `dev` (with the commit, e.g. `dev+3553344`); `yagi -version` prints it too.

## To release

`main` is protected, so changes arrive by pull request. First write the release's entry in [`CHANGELOG.md`](../CHANGELOG.md), in a pull request like any other change: a `## 2026.10.2 (2026-10-18)` heading, then what changed. Put anything that changes what an existing install does under *Read before updating*. Link to the docs with full `https://johnschult.github.io/yagi/...` addresses, because the section is shown on a GitHub release page where relative links break. The release's notes on GitHub are that section, and a tag with no section fails before anything is published.

The entry is also what people see in the app: after an update, Yagi pops up the notes for the releases since the last one that browser saw (and *What's new* in the footer opens them on request). So write it for them, and keep to the format: a `## YYYY.MM.PATCH (YYYY-MM-DD)` heading, then `### Group` headings (*Read before updating* first, which is drawn as a notice) with `- ` bullets under them. Inside a bullet, `**bold**`, `*italic*`, `` `code` `` and `[links](https://…)` are drawn; anything else is shown as text. A test reads the real `CHANGELOG.md` and fails if an entry isn't in that shape. The notes are bundled into the web page when it is built (the `Dockerfile` copies the file into the web stage), so they are always the ones for the version running, and nothing is fetched from anywhere.

Once the entry is on `main`:

```sh
git checkout main && git pull
git tag v2026.10.2
git push origin v2026.10.2
```

The **Release** workflow then:

1. checks the tag looks like `vYYYY.MM.PATCH` and that `CHANGELOG.md` has a section for it (otherwise it fails, so a typo or a forgotten entry can't publish);
2. builds the image for `linux/amd64` and `linux/arm64`, with the version built in;
3. pushes it to `ghcr.io/johnschult/yagi` as `2026.10.0` and `latest`;
4. builds and pushes the same program with the built-in Tailscale node (`-tags tsnet`, with the Tailscale version stamped in) as `2026.10.0-tailscale` and `tailscale`;
5. creates the GitHub release: the changelog section first, then the list of merged pull requests that GitHub generates;
6. builds the program for `linux` and `darwin` (macOS), each on `amd64` and `arm64`, with the web page and the version built in, and attaches `yagi_2026.10.0_<os>_<arch>.tar.gz` for each (the program, `LICENSE` and `README.md`) and a `checksums.txt` of their SHA-256 sums to the release, along with `install.sh`, so that `https://github.com/johnschult/yagi/releases/latest/download/install.sh` is always the script that goes with the newest release.

7. signs the images (by digest) and the archives' `checksums.txt` with Sigstore, keylessly, as the workflow itself: the signature is `checksums.txt.sigstore.json` on the release, and it is checked straight away. It also records GitHub build provenance for the archives. There is no key to keep or rotate; see [Verifying a download](security.md#verifying-a-download).
8. updates the Homebrew formula in the tap, [johnschult/homebrew-tap](https://github.com/johnschult/homebrew-tap): it fills in `packaging/homebrew/yagi.rb` with the version and the SHA-256 of each archive (`scripts/render_formula.sh`) and pushes the result as `Formula/yagi.rb`. The tap's own checks then install the formula on Linux and macOS and run its test. This needs the repository secret `HOMEBREW_TAP_TOKEN`: a fine-grained personal access token with *Contents: read and write* on `johnschult/homebrew-tap` only. If the job fails the release itself is fine; run the job again once it is fixed.

Pull requests that change the `Dockerfile` or the workflow build the image and the archives without publishing them, and sign and check the checksums when the pull request is from this repository, so the whole path is tried before a release.

## After a release

Check that the image can be pulled without logging in, from a machine that isn't signed in to GitHub:

```sh
docker pull ghcr.io/johnschult/yagi:2026.10.0
docker pull ghcr.io/johnschult/yagi:2026.10.0-tailscale
```

The `-tailscale` image is a package version of the same package, so it is public with it.

The first release (2026.10.0) was public straight away. If a package ever starts out private, open its settings
(github.com/johnschult?tab=packages → *yagi* → *Package settings*) and change its visibility to **Public**.
