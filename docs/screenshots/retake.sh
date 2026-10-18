#!/bin/sh
# Retakes the README screenshots from a demo with a made-up tuner, guide and
# viewers (internal/server/demo_test.go), so no real tuner or network is needed.
#
#   docs/screenshots/retake.sh
#
# Needs Go, Python 3 with Playwright and a Chromium for it
# (pip install playwright && playwright install chromium). Run it from anywhere.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
work=$(mktemp -d)
trap 'kill "${pid:-}" 2>/dev/null || true; rm -rf "$work"' EXIT

(cd "$root" && go test -c -tags demo -o "$work/demo.test" ./internal/server)
DEMO_DIR="$work" DEMO_LOGOS="$here/demo" "$work/demo.test" -test.run '^TestDemo$' -test.timeout 1h >"$work/log" 2>&1 &
pid=$!
i=0
while [ ! -s "$work/addr" ]; do
  i=$((i + 1)); [ "$i" -gt 60 ] && { echo "the demo did not start:"; cat "$work/log"; exit 1; }
  sleep 1
done
sleep 4 # let the first stream and the guide show up
python3 "$here/shoot.py" "$(cat "$work/addr")" "$here"
