#!/bin/sh
# Installs Yagi from a release: downloads the archive for this machine, checks it against the release's checksums, and
# puts the program in a folder. Nothing is built, so Go, Node and Docker aren't needed.
#
#   curl -fsSL https://github.com/johnschult/yagi/releases/latest/download/install.sh | sh
#
#   sh install.sh               installs the latest release into ~/.local/bin (no root needed)
#   sh install.sh --service     also runs it in the background, as a systemd service (Linux; asks sudo for that)
#   sh install.sh --uninstall   removes the program, and the service if there is one. Your settings are left alone.
#   sh install.sh -h            shows this
#
# Settings, each optional:
#   YAGI_VERSION      a release to install instead of the latest, such as 2026.10.17
#   YAGI_INSTALL_DIR  where to put the program (default ~/.local/bin, or /usr/local/bin with --service)
#   YAGI_DL           where the archives are downloaded from (default: the release's page on GitHub), for a mirror or a test
#
# It reads the release's checksums.txt, checks the archive against it before unpacking anything, and unpacks only the
# program. If cosign is installed it also checks the signature on checksums.txt, which says the release was built by
# this project's own release workflow, and installs nothing if that fails. It is for Linux and macOS, on amd64 and
# arm64. On Windows, use WSL or Docker.
#
# All of it is inside main, which runs on the last line, so a download cut short can't run half of it.

set -eu

REPO=johnschult/yagi
SERVICE_NAME=yagi
SERVICE_USER=yagi
STATE_DIR=/var/lib/yagi
UNIT=/etc/systemd/system/yagi.service

say() { printf '%s\n' "$*"; }
fail() {
	printf '\nyagi: %s\n' "$*" >&2
	exit 1
}

usage() {
	cat <<'EOF'
Installs Yagi from a release, with no building.

  sh install.sh               installs the latest release into ~/.local/bin (no root needed)
  sh install.sh --service     also runs it in the background, as a systemd service (Linux; asks sudo for that)
  sh install.sh --uninstall   removes the program, and the service if there is one. Your settings are left alone.
  sh install.sh -h            shows this

Settings, each optional:
  YAGI_VERSION      a release to install instead of the latest, such as 2026.10.17
  YAGI_INSTALL_DIR  where to put the program (default ~/.local/bin, or /usr/local/bin with --service)
  YAGI_DL           where the archives are downloaded from (default: the release's page on GitHub)
EOF
}

# have COMMAND: whether it is installed.
have() { command -v "$1" >/dev/null 2>&1; }

# fetch URL FILE: downloads a file, over HTTPS unless YAGI_DL says otherwise.
fetch() {
	if have curl; then
		case $1 in
		https://*) curl -fsSL --proto '=https' --proto-redir '=https' --tlsv1.2 --retry 2 --max-time 300 -o "$2" "$1" ;;
		*) curl -fsSL --retry 2 --max-time 300 -o "$2" "$1" ;;
		esac
	elif have wget; then
		wget -q -O "$2" "$1"
	else
		fail "Neither curl nor wget is installed, and one of them is needed to download Yagi."
	fi
}

# sha256 FILE: the file's SHA-256, in lowercase hex.
sha256() {
	if have sha256sum; then
		sha256sum "$1" | awk '{ print $1 }'
	elif have shasum; then
		shasum -a 256 "$1" | awk '{ print $1 }'
	else
		fail "Neither sha256sum nor shasum is installed, so the download can't be checked. Install coreutils, or use Docker."
	fi
}

# verify_signature BASE DIR VERSION: if cosign is installed, checks that checksums.txt was signed by this project's
# release workflow for this release (a keyless Sigstore signature, so there is no key to look after or to forge). Without
# cosign it says so and carries on: the checksums still catch a damaged download, though not a replaced release.
verify_signature() {
	if ! have cosign; then
		say "Not checking the release's signature, because cosign isn't installed (https://docs.sigstore.dev/cosign/). To check it, install cosign and run this again."
		return 0
	fi
	fetch "$1/checksums.txt.sigstore.json" "$2/checksums.txt.sigstore.json" ||
		fail "Release $3 has no signature ($1/checksums.txt.sigstore.json), and cosign is here to check it, so nothing was installed."
	cosign verify-blob --bundle "$2/checksums.txt.sigstore.json" \
		--certificate-identity "https://github.com/$REPO/.github/workflows/release.yml@refs/tags/v$3" \
		--certificate-oidc-issuer https://token.actions.githubusercontent.com \
		"$2/checksums.txt" >"$2/cosign.log" 2>&1 || {
		cat "$2/cosign.log" >&2
		fail "The signature on this release's checksums is not valid, so nothing was installed."
	}
	say "Checked the release's signature: built by $REPO's release workflow"
}

# latest_version: the newest release's number, from where GitHub's "latest" page sends you (so no API, no rate limit).
latest_version() {
	have curl || fail "Finding the latest release needs curl. Install it, or say which release to install with YAGI_VERSION=2026.10.17."
	url=$(curl -fsSIL --proto '=https' --proto-redir '=https' --tlsv1.2 --max-time 60 -o /dev/null -w '%{url_effective}' \
		"https://github.com/$REPO/releases/latest") || fail "Couldn't reach GitHub to find the latest release."
	case $url in
	*/releases/tag/v*) printf '%s\n' "${url##*/releases/tag/v}" ;;
	*) fail "GitHub didn't say which release is the latest (it sent $url). Is there a release yet? Or say which with YAGI_VERSION=2026.10.17." ;;
	esac
}

# as_root COMMAND...: runs a command as root, with sudo unless already root.
as_root() {
	if [ "$(id -u)" -eq 0 ]; then
		"$@"
	elif have sudo; then
		sudo "$@"
	else
		fail "That needs root, and sudo isn't installed. Run this as root, or without --service."
	fi
}

# find_system sets os and arch, in the names the archives use.
find_system() {
	case $(uname -s) in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) fail "This installs on Linux and macOS (it is $(uname -s)). On Windows, use WSL or Docker: docker run ghcr.io/$REPO:latest" ;;
	esac
	case $(uname -m) in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) fail "There is no download for a $(uname -m) machine; Yagi is built for amd64 and arm64. Docker or building from source may work: https://github.com/$REPO#quick-start" ;;
	esac
}

install_service() {
	if [ "$(uname -s)" != Linux ] || ! have systemctl || [ ! -d /run/systemd/system ]; then
		fail "--service needs Linux with systemd. Run yagi yourself, or use Docker."
	fi
	bin=$1
	say "Setting up the service (this asks sudo for a few things):"
	say "  a user '$SERVICE_USER' with no login, its settings in $STATE_DIR, and $UNIT"
	if ! id "$SERVICE_USER" >/dev/null 2>&1; then
		nologin=/usr/sbin/nologin
		[ -x "$nologin" ] || nologin=/sbin/nologin
		as_root useradd --system --home-dir "$STATE_DIR" --shell "$nologin" "$SERVICE_USER"
	fi
	as_root install -d -m 0750 -o "$SERVICE_USER" -g "$SERVICE_USER" "$STATE_DIR"
	tmp=$(mktemp)
	cat >"$tmp" <<EOF
[Unit]
Description=Yagi, sharing an HDHomeRun tuner with your family and friends
After=network-online.target
Wants=network-online.target

[Service]
User=$SERVICE_USER
WorkingDirectory=$STATE_DIR
ExecStart=$bin -config $STATE_DIR/yagi.json
Restart=on-failure
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true

[Install]
WantedBy=multi-user.target
EOF
	as_root install -m 0644 "$tmp" "$UNIT"
	rm -f "$tmp"
	as_root systemctl daemon-reload
	as_root systemctl enable "$SERVICE_NAME"
	# restart, not just start, so that running this again to update runs the new program
	as_root systemctl restart "$SERVICE_NAME"
	say ""
	say "Yagi is running as a service. Its log, which has the one-time setup link, is: journalctl -u $SERVICE_NAME"
	say "Its settings file is $STATE_DIR/yagi.json."
}

uninstall() {
	removed=0
	if [ -f "$UNIT" ]; then
		say "Removing the service"
		as_root systemctl disable --now "$SERVICE_NAME" 2>/dev/null || true
		as_root rm -f "$UNIT"
		as_root systemctl daemon-reload || true
		removed=1
	fi
	for dir in "${YAGI_INSTALL_DIR:-}" "$HOME/.local/bin" /usr/local/bin; do
		[ -n "$dir" ] || continue
		[ -f "$dir/yagi" ] || continue
		say "Removing $dir/yagi"
		if [ -w "$dir" ]; then rm -f "$dir/yagi"; else as_root rm -f "$dir/yagi"; fi
		removed=1
	done
	[ "$removed" -eq 1 ] || say "Nothing to remove: there is no yagi in ~/.local/bin or /usr/local/bin, and no service."
	say "Your settings were left alone (yagi.json, and $STATE_DIR if you used --service); delete them yourself if you want them gone."
}

main() {
	service=0
	for arg in "$@"; do
		case $arg in
		-h | --help)
			usage
			exit 0
			;;
		--service) service=1 ;;
		--uninstall)
			uninstall
			exit 0
			;;
		*) fail "I don't know '$arg'. Try: sh install.sh -h" ;;
		esac
	done

	find_system

	version=${YAGI_VERSION:-}
	version=${version#v}
	[ -n "$version" ] || version=$(latest_version) || exit 1
	printf '%s\n' "$version" | grep -Eq '^[0-9]{4}\.[0-9]{1,2}\.[0-9]+$' ||
		fail "'$version' isn't a release number; they look like 2026.10.17."

	base=${YAGI_DL:-https://github.com/$REPO/releases/download/v$version}
	case $base in
	https://*) ;;
	*) say "Warning: downloading from $base, which isn't HTTPS." ;;
	esac

	if [ -n "${YAGI_INSTALL_DIR:-}" ]; then
		dir=$YAGI_INSTALL_DIR
	elif [ "$service" -eq 1 ]; then
		dir=/usr/local/bin
	else
		dir=$HOME/.local/bin
	fi

	name=yagi_${version}_${os}_${arch}
	file=$name.tar.gz

	work=$(mktemp -d)
	trap 'rm -rf "$work"' EXIT INT TERM

	say "Installing Yagi $version for $os/$arch"
	fetch "$base/checksums.txt" "$work/checksums.txt" || fail "Couldn't download $base/checksums.txt. Is $version a release, and is there one for it yet?"
	verify_signature "$base" "$work" "$version"
	want=$(awk -v f="$file" '$2 == f { print $1; exit }' "$work/checksums.txt")
	[ -n "$want" ] || fail "Release $version has no download for $os/$arch ($file isn't in its checksums.txt)."
	case $want in *[!0-9a-f]*) fail "The checksum for $file isn't a SHA-256 ($want)." ;; esac
	[ ${#want} -eq 64 ] || fail "The checksum for $file isn't a SHA-256 ($want)."

	fetch "$base/$file" "$work/$file" || fail "Couldn't download $base/$file."
	got=$(sha256 "$work/$file") || exit 1
	[ "$got" = "$want" ] || fail "The download doesn't match its checksum, so nothing was installed.
  wanted $want
  got    $got"

	# Only the program is unpacked, and only if the archive holds what it should: a folder of the same name with the
	# program, LICENSE and README.md in it, and nothing that points outside it.
	tar -tzf "$work/$file" >"$work/list" || fail "The download isn't a readable archive, so nothing was installed."
	while IFS= read -r entry; do
		case $entry in
		"$name/yagi" | "$name/LICENSE" | "$name/README.md" | "$name/") ;;
		*) fail "The archive holds something unexpected ($entry), so nothing was installed." ;;
		esac
	done <"$work/list"
	tar -xzf "$work/$file" -C "$work" "$name/yagi"

	mkdir -p "$dir" 2>/dev/null || true
	if [ -w "$dir" ]; then
		install -m 0755 "$work/$name/yagi" "$dir/yagi"
	else
		say "$dir needs root to write to; asking sudo"
		as_root install -d -m 0755 "$dir"
		as_root install -m 0755 "$work/$name/yagi" "$dir/yagi"
	fi

	say "Installed $("$dir/yagi" -version 2>/dev/null || printf 'yagi') in $dir"
	case ":$PATH:" in
	*":$dir:"*) ;;
	*)
		say ""
		say "$dir isn't on your PATH. Add it (to ~/.profile, ~/.zshrc or similar):"
		say "  export PATH=\"$dir:\$PATH\""
		;;
	esac

	if [ "$service" -eq 1 ]; then
		say ""
		install_service "$dir/yagi"
	else
		say ""
		say "Run it with: yagi"
		say "Then open the setup link it prints in its log (or http://localhost:9244/ on this machine) to become the admin."
		say "To keep it running in the background on Linux, run this again with --service."
	fi
}

main "$@"
