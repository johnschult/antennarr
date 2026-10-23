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

# The same release's build with the built-in Tailscale node, whose program says so when asked its version.
mkdir -p "$good/${name}_tailscale"
printf '#!/bin/sh\necho "%s (built-in Tailscale)"\n' "$version" >"$good/${name}_tailscale/yagi"
chmod +x "$good/${name}_tailscale/yagi"
echo license >"$good/${name}_tailscale/LICENSE"
echo readme >"$good/${name}_tailscale/README.md"
tar -C "$good" -czf "$good/${name}_tailscale.tar.gz" "${name}_tailscale"
echo "$(sum "$good/${name}_tailscale.tar.gz")  ${name}_tailscale.tar.gz" >>"$good/checksums.txt"

# What a signed release has beside them (the shim below doesn't read it, only whether it is there).
echo "{}" >"$good/checksums.txt.sigstore.json"

# A release from before there was one: only the plain archive.
oldrel=$work/oldrel
mkdir -p "$oldrel"
cp "$good/$name.tar.gz" "$good/checksums.txt.sigstore.json" "$oldrel/"
echo "$(sum "$good/$name.tar.gz")  $name.tar.gz" >"$oldrel/checksums.txt"

# One whose archive was changed after its checksum was written.
bad=$work/bad
mkdir -p "$bad"
cp "$good/checksums.txt" "$good/checksums.txt.sigstore.json" "$bad/"
printf 'not what was checksummed' >"$bad/$name.tar.gz"

# One whose archive holds a file it shouldn't, with a checksum that matches it.
extra=$work/extra
mkdir -p "$extra" "$work/x/$name"
cp "$good/$name/yagi" "$work/x/$name/yagi"
echo surprise >"$work/x/$name/surprise"
tar -C "$work/x" -czf "$extra/$name.tar.gz" "$name"
echo "$(sum "$extra/$name.tar.gz")  $name.tar.gz" >"$extra/checksums.txt"
echo '{}' >"$extra/checksums.txt.sigstore.json"

# One with no download for this system.
other=$work/other
mkdir -p "$other"
echo "$(sum "$good/$name.tar.gz")  yagi_${version}_plan9_mips.tar.gz" >"$other/checksums.txt"
echo '{}' >"$other/checksums.txt.sigstore.json"

# A good release with no signature.
unsigned=$work/unsigned
mkdir -p "$unsigned"
cp "$good/$name.tar.gz" "$good/checksums.txt" "$unsigned/"

# A stand-in for cosign, which says whether the signature is good as FAKE_COSIGN says, and keeps what it was asked.
shim=$work/shim
mkdir -p "$shim"
cat >"$shim/cosign" <<'SHIM'
#!/bin/sh
echo "$@" >"$FAKE_COSIGN_LOG"
[ "${FAKE_COSIGN:-ok}" = ok ] || { echo "no matching signature" >&2; exit 1; }
SHIM
chmod +x "$shim/cosign"
real_cosign=0
command -v cosign >/dev/null 2>&1 && real_cosign=1

port=$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')
(cd "$work" && exec python3 -m http.server "$port" --bind 127.0.0.1 >/dev/null 2>&1) &
server=$!
i=0
until curl -fs "http://127.0.0.1:$port/good/checksums.txt" >/dev/null 2>&1; do
	i=$((i + 1))
	[ "$i" -lt 50 ] || fail "the test server didn't start"
	sleep 0.1
done

run() { # run RELEASE_DIR [ARG...]: install.sh against that fake release, into $work/bin, with the cosign stand-in
	rel=$1
	shift
	PATH="$shim:$PATH" FAKE_COSIGN_LOG="$work/cosign.args" YAGI_VERSION=$version YAGI_DL="http://127.0.0.1:$port/$rel" \
		YAGI_INSTALL_DIR="$work/bin" sh "$here/install.sh" "$@"
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

# --tailscale installs the build with the built-in Tailscale node; updating without a flag keeps it; --plain switches back.
rm -rf "${work:?}/bin"
run good >/dev/null 2>&1
"$work/bin/yagi" -version | grep -qi tailscale && fail "a first install with no flag wasn't the plain build"
run good --tailscale >"$work/out" 2>&1 || { cat "$work/out" >&2; fail "--tailscale didn't install"; }
"$work/bin/yagi" -version | grep -qi tailscale || fail "--tailscale didn't install the Tailscale build"
grep -q "built-in Tailscale node" "$work/out" || fail "it didn't say it was installing the Tailscale build"
run good >"$work/out" 2>&1 || fail "updating over the Tailscale build failed"
"$work/bin/yagi" -version | grep -qi tailscale || fail "updating with no flag dropped the Tailscale build"
grep -q "That is the build you have" "$work/out" || fail "it didn't say it was keeping the Tailscale build"
run good --plain >/dev/null 2>&1 || fail "--plain didn't install"
"$work/bin/yagi" -version | grep -qi tailscale && fail "--plain left the Tailscale build"
(YAGI_FLAVOR=tailscale && export YAGI_FLAVOR && run good) >/dev/null 2>&1 || fail "YAGI_FLAVOR=tailscale didn't install"
"$work/bin/yagi" -version | grep -qi tailscale || fail "YAGI_FLAVOR=tailscale didn't install the Tailscale build"
if (YAGI_FLAVOR=other && export YAGI_FLAVOR && run good) >"$work/out" 2>&1; then fail "it accepted YAGI_FLAVOR=other"; fi
if run oldrel --tailscale >"$work/out" 2>&1; then fail "it installed a Tailscale build from a release that has none"; fi
grep -q "no build with Tailscale" "$work/out" || fail "it didn't say that release has no Tailscale build"
pass "--tailscale installs the Tailscale build, updating keeps it, --plain switches back"

# Nothing is installed from a download that doesn't match, or that holds something unexpected.
for rel in bad extra; do
	rm -rf "${work:?}/bin"
	if run "$rel" >"$work/out" 2>&1; then fail "$rel: it installed a bad download"; fi
	[ ! -e "$work/bin/yagi" ] || fail "$rel: it left a program behind"
done
pass "a changed archive, and one with an extra file, are refused and leave nothing"

# With cosign, the signature is checked, for this release's tag and this repository's workflow.
grep -q -- "--certificate-identity https://github.com/johnschult/yagi/.github/workflows/release.yml@refs/tags/v$version" "$work/cosign.args" ||
	fail "cosign wasn't asked to check for this release's tag and workflow: $(cat "$work/cosign.args")"
grep -q -- "--certificate-oidc-issuer https://token.actions.githubusercontent.com" "$work/cosign.args" || fail "cosign wasn't asked for GitHub's issuer"
pass "with cosign, the signature is checked for this release's tag and workflow"

# A signature that doesn't check out, or none at all, is refused, and nothing is installed.
rm -rf "${work:?}/bin"
if (FAKE_COSIGN=bad && export FAKE_COSIGN && run good) >"$work/out" 2>&1; then fail "it installed a release whose signature failed"; fi
grep -q "signature on this release's checksums is not valid" "$work/out" || fail "it didn't say the signature failed"
[ ! -e "$work/bin/yagi" ] || fail "it left a program behind after a bad signature"
if run unsigned >"$work/out" 2>&1; then fail "it installed an unsigned release while cosign was there to check"; fi
grep -q "has no signature" "$work/out" || fail "it didn't say the release has no signature"
pass "a bad signature, and a missing one, are refused when cosign is there"

# Without cosign it says so and installs, on the checksums alone (not tried if this machine has the real cosign).
if [ "$real_cosign" -eq 0 ]; then
	rm -rf "${work:?}/bin"
	YAGI_VERSION=$version YAGI_DL="http://127.0.0.1:$port/unsigned" YAGI_INSTALL_DIR="$work/bin" sh "$here/install.sh" >"$work/out" 2>&1 ||
		fail "it didn't install without cosign"
	grep -q "cosign isn't installed" "$work/out" || fail "it didn't say it wasn't checking the signature"
	pass "without cosign it says so, and installs on the checksums"
fi

# A release with nothing for this system says so.
if run other >"$work/out" 2>&1; then fail "it installed a release with no download for this system"; fi
grep -q "no download for $os/$arch" "$work/out" || { cat "$work/out" >&2; fail "it didn't say there is no download for this system"; }
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
