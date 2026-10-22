#!/bin/sh
# Runs install.sh against a fake release served from this machine, and checks that it installs a good download and
# refuses a bad one. It needs python3 (to serve the files) and nothing from the internet.
#
#   sh scripts/install_test.sh
set -eu

here=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
server=
cleanup() {
	[ -z "$server" ] || kill "$server" 2>/dev/null || true
	rm -rf "$work"
}
trap cleanup EXIT INT TERM

version=2026.10.99
case $(uname -s) in Linux) os=linux ;; Darwin) os=darwin ;; *) echo "skipped: not Linux or macOS" && exit 0 ;; esac
case $(uname -m) in x86_64 | amd64) arch=amd64 ;; aarch64 | arm64) arch=arm64 ;; *) echo "skipped: unknown architecture" && exit 0 ;; esac
name=yagi_${version}_${os}_${arch}

sum() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{ print $1 }'; else shasum -a 256 "$1" | awk '{ print $1 }'; fi; }
pass() { printf 'ok   %s\n' "$1"; }
fail() {
	printf 'FAIL %s\n' "$1" >&2
	exit 1
}

# A fake release: the "program" only says its version.
good=$work/good
mkdir -p "$good/$name"
printf '#!/bin/sh\necho %s\n' "$version" >"$good/$name/yagi"
chmod +x "$good/$name/yagi"
echo license >"$good/$name/LICENSE"
echo readme >"$good/$name/README.md"
tar -C "$good" -czf "$good/$name.tar.gz" "$name"
echo "$(sum "$good/$name.tar.gz")  $name.tar.gz" >"$good/checksums.txt"

# One whose archive was changed after its checksum was written.
bad=$work/bad
mkdir -p "$bad"
cp "$good/checksums.txt" "$bad/"
printf 'not what was checksummed' >"$bad/$name.tar.gz"

# One whose archive holds a file it shouldn't, with a checksum that matches it.
extra=$work/extra
mkdir -p "$extra" "$work/x/$name"
cp "$good/$name/yagi" "$work/x/$name/yagi"
echo surprise >"$work/x/$name/surprise"
tar -C "$work/x" -czf "$extra/$name.tar.gz" "$name"
echo "$(sum "$extra/$name.tar.gz")  $name.tar.gz" >"$extra/checksums.txt"

# One with no download for this system.
other=$work/other
mkdir -p "$other"
echo "$(sum "$good/$name.tar.gz")  yagi_${version}_plan9_mips.tar.gz" >"$other/checksums.txt"

port=$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')
(cd "$work" && exec python3 -m http.server "$port" --bind 127.0.0.1 >/dev/null 2>&1) &
server=$!
i=0
until curl -fs "http://127.0.0.1:$port/good/checksums.txt" >/dev/null 2>&1; do
	i=$((i + 1))
	[ "$i" -lt 50 ] || fail "the test server didn't start"
	sleep 0.1
done

run() { # run RELEASE_DIR [ARG...]: install.sh against that fake release, into $work/bin
	rel=$1
	shift
	YAGI_VERSION=$version YAGI_DL="http://127.0.0.1:$port/$rel" YAGI_INSTALL_DIR="$work/bin" sh "$here/install.sh" "$@"
}

# -h shows the help, including when the script is piped to sh.
sh "$here/install.sh" -h | grep -q -- '--uninstall' || fail "-h didn't show the help"
sh -s -- -h <"$here/install.sh" | grep -q -- '--uninstall' || fail "-h didn't show the help when piped"
pass "-h shows the help, also when piped"

# A good release installs, and the program runs.
run good >"$work/out" 2>&1 || {
	cat "$work/out" >&2
	fail "a good release didn't install"
}
[ "$("$work/bin/yagi")" = "$version" ] || fail "the installed program isn't the one in the archive"
pass "a good release installs"

# Running it again (an update) works.
run good >/dev/null 2>&1 || fail "installing over an existing install failed"
pass "installing again works"

# Nothing is installed from a download that doesn't match, or that holds something unexpected.
for rel in bad extra; do
	rm -rf "$work/bin"
	if run "$rel" >"$work/out" 2>&1; then fail "$rel: it installed a bad download"; fi
	[ ! -e "$work/bin/yagi" ] || fail "$rel: it left a program behind"
done
pass "a changed archive, and one with an extra file, are refused and leave nothing"

# A release with nothing for this system says so.
if run other >"$work/out" 2>&1; then fail "it installed a release with no download for this system"; fi
grep -q "no download for $os/$arch" "$work/out" || fail "it didn't say there is no download for this system"
pass "a release with no download for this system is refused"

# A version that isn't a release number is refused before anything is downloaded.
if YAGI_VERSION=../../evil YAGI_DL="http://127.0.0.1:$port/good" YAGI_INSTALL_DIR="$work/bin" sh "$here/install.sh" >/dev/null 2>&1; then
	fail "it accepted a version that isn't a release number"
fi
pass "a version that isn't a release number is refused"

# --uninstall removes the program and leaves the settings.
run good >/dev/null 2>&1
echo '{}' >"$work/bin/yagi.json"
YAGI_INSTALL_DIR="$work/bin" HOME="$work/home" sh "$here/install.sh" --uninstall >/dev/null 2>&1 || fail "--uninstall failed"
[ ! -e "$work/bin/yagi" ] || fail "--uninstall left the program"
[ -e "$work/bin/yagi.json" ] || fail "--uninstall removed a settings file"
pass "--uninstall removes the program and leaves settings alone"

echo "all passed"
