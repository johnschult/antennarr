#!/bin/sh
# Builds ./yagi from a checkout: the web page first (the program embeds it), then the program.
#
#   ./build.sh        builds ./yagi, then run it with ./yagi
#   ./build.sh -h     shows this
#
# It needs Go and Node 22.12 or newer. It checks for both and says what to install if one is missing or too old.
# (Without either, the Docker image needs neither: docker run ghcr.io/johnschult/yagi:latest.) On Windows, use WSL or Docker.
set -eu

case "${1:-}" in
-h | --help)
	sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//'
	exit 0
	;;
esac

cd "$(dirname "$0")"

NODE_MIN=22.12
GO_MIN=1.21 # the oldest Go that fetches the exact version go.mod asks for by itself

say() { printf '%s\n' "$*"; }
fail() {
	printf '\nyagi: %s\n' "$*" >&2
	exit 1
}

# quietly DESCRIPTION COMMAND...: runs a command without its output, which is only shown if it fails.
quietly() {
	what=$1
	shift
	log=$(mktemp)
	if "$@" >"$log" 2>&1; then
		rm -f "$log"
		return 0
	fi
	printf '\n' >&2
	cat "$log" >&2
	rm -f "$log"
	return 1
}

# at_least HAVE NEED: whether the version HAVE (such as 22.12.1) is NEED (such as 22.12) or newer.
at_least() {
	awk -v have="$1" -v need="$2" 'BEGIN {
		n = split(have, h, "."); m = split(need, w, ".")
		for (i = 1; i <= (n > m ? n : m); i++) {
			a = h[i] + 0; b = w[i] + 0
			if (a > b) exit 0
			if (a < b) exit 1
		}
		exit 0
	}'
}

# --- Go ---------------------------------------------------------------------------------------------------------
command -v go >/dev/null 2>&1 || fail "Go isn't installed.
  Install it from https://go.dev/dl/ (macOS: brew install go). Any Go $GO_MIN or newer will do: it fetches the exact
  version this needs by itself."
go_have=$(go env GOVERSION 2>/dev/null | sed 's/^go//; s/[^0-9.].*$//')
go_need=$(awk '$1 == "go" { print $2; exit }' go.mod)
[ -n "$go_have" ] || fail "Couldn't tell which version of Go you have. Run 'go version' and check it works."
at_least "$go_have" "$GO_MIN" || fail "Go $go_have is too old: this needs Go $GO_MIN or newer (and then fetches Go $go_need itself).
  Update it from https://go.dev/dl/ (macOS: brew upgrade go)."
if [ "${GOTOOLCHAIN:-}" = local ] && ! at_least "$go_have" "$go_need"; then
	fail "Go $go_have is too old for this, and GOTOOLCHAIN=local stops it fetching a newer one: it needs Go $go_need.
  Update Go, or unset GOTOOLCHAIN."
fi

# --- Node -------------------------------------------------------------------------------------------------------
command -v node >/dev/null 2>&1 || fail "Node.js isn't installed. Building the web page needs Node $NODE_MIN or newer.
  macOS: brew install node.  Anywhere: https://nodejs.org, or nvm (https://github.com/nvm-sh/nvm) then: nvm install
  (there is an .nvmrc).  Or skip building it yourself: docker run ghcr.io/johnschult/yagi:latest"
node_have=$(node -p 'process.versions.node' 2>/dev/null || true)
[ -n "$node_have" ] || fail "Couldn't tell which version of Node you have. Run 'node --version' and check it works."
at_least "$node_have" "$NODE_MIN" || fail "Node $node_have is too old: building the web page needs Node $NODE_MIN or newer.
  Update it with nvm (nvm install 22 && nvm use 22), brew upgrade node, or https://nodejs.org.
  Or skip building it yourself: docker run ghcr.io/johnschult/yagi:latest"
command -v npm >/dev/null 2>&1 || fail "npm isn't installed. It comes with Node: reinstall Node from https://nodejs.org."

say "Building yagi (Go $go_have, Node $node_have)"

# --- The web page -----------------------------------------------------------------------------------------------
if [ ! -d web/node_modules ] || [ web/package-lock.json -nt web/node_modules ]; then
	say "1/3  Installing the web page's packages (npm ci), which takes a minute the first time"
	quietly "npm ci" sh -c "cd web && npm ci" || fail "npm couldn't install the web page's packages.
  If it said EACCES for a folder in ~/.npm, the folder belongs to root from an old 'sudo npm': run
    sudo chown -R \$(id -u):\$(id -g) ~/.npm
  and run ./build.sh again."
else
	say "1/3  The web page's packages are installed already"
fi
say "2/3  Building the web page"
quietly "web build" sh -c "cd web && npm run build" || fail "The web page didn't build. The lines above say why."

# --- The program ------------------------------------------------------------------------------------------------
say "3/3  Building the program"
quietly "go build" go build -o yagi ./cmd/yagi || fail "The program didn't build. The lines above say why."

say ""
say "Done: ./yagi"
say "Run it with ./yagi, then open the address it prints."
