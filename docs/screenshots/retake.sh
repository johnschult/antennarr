#!/bin/sh
# Retakes the README screenshots from a demo with a made-up tuner, guide and
# viewers (internal/server/demo_test.go), so no real tuner or network is needed.
#
#   docs/screenshots/retake.sh
#
# By default the channel logos are plain badges (docs/screenshots/demo). To show
# real ones, name a tuner on your network and the channels to show:
#
#   TUNER=192.168.1.50 CHANNELS=2.1,5.1,11.1,46.1 docs/screenshots/retake.sh
#
# That reads the tuner's guide (fetch-logos.py: the same request Antennarr makes) and
# keeps the logos in a temporary folder that is deleted afterwards, so only the
# screenshots are committed, not the networks' marks. Leave CHANNELS out to take the
# first four main channels.
#
# Or, if you already have a folder of logos with a channels.json (as fetch-logos.py writes), use it:
#
#   LOGOS=/path/to/logos docs/screenshots/retake.sh
#
# Needs Go, Python 3 with Playwright and a Chromium for it
# (pip install playwright && playwright install chromium). Run it from anywhere.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
work=$(mktemp -d)
trap 'kill "${pid:-}" 2>/dev/null || true; rm -rf "$work"' EXIT

logos="$here/demo"
if [ -n "${LOGOS:-}" ]; then
  logos="$LOGOS"
elif [ -n "${TUNER:-}" ]; then
  logos="$work/logos"
  python3 "$here/fetch-logos.py" "$TUNER" "$logos" "${CHANNELS:-}"
fi

(cd "$root" && go test -c -tags demo -o "$work/demo.test" ./internal/server)
# DEMO_SIGNAL makes the signal quality drift and dip a little, so its trend line has something to draw.
DEMO_SIGNAL=1 DEMO_DIR="$work" DEMO_LOGOS="$logos" "$work/demo.test" -test.run '^TestDemo$' -test.timeout 1h >"$work/log" 2>&1 &
pid=$!
i=0
while [ ! -s "$work/addr" ]; do
  i=$((i + 1)); [ "$i" -gt 60 ] && { echo "the demo did not start:"; cat "$work/log"; exit 1; }
  sleep 1
done
sleep 4 # let the first stream and the guide show up
python3 "$here/shoot.py" "$(cat "$work/addr")" "$here"
