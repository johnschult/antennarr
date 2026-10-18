#!/usr/bin/env python3
"""Fetches real channel logos from a tuner's guide, for retake.sh.

usage: fetch-logos.py <tuner address> <output dir> [channel numbers, e.g. 2.1,5.1,11.1,46.1]

It reads the tuner's guide key from its discover.json, reads SiliconDust's guide
for it (the same request Antennarr makes), and saves the logos of the channels asked
for into <output dir>, with a channels.json the demo (internal/server/demo_test.go)
reads. The logos are the networks' marks, so retake.sh keeps them in a temporary
folder; only the finished screenshots are committed.

With no channel numbers it takes the first four main channels (x.1) that have a logo.
The guide key is never printed: it is passed on only to SiliconDust, as Antennarr does.
Standard library only.
"""
import json
import os
import re
import sys
import urllib.parse
import urllib.request
import xml.etree.ElementTree as ET

LOGO_HOST = 'img.hdhomerun.com'
# What is on, made up, so the screenshots don't show a real evening's listings.
TITLES = ['College Football', 'Evening News', 'Local Weather', 'Primetime Drama']


def fail(msg):
    # Never put a URL in a message: the guide's address has the key in it.
    sys.exit(f'fetch-logos: {msg}')


def get(url, what, limit=20_000_000):
    try:
        with urllib.request.urlopen(url, timeout=60) as r:
            return r.read(limit)
    except Exception as e:  # the exception text can quote the URL, so say only what failed
        fail(f"couldn't read {what} ({type(e).__name__})")


def main():
    if len(sys.argv) < 3:
        sys.exit(__doc__)
    tuner, out = sys.argv[1], sys.argv[2]
    wanted = [c.strip() for c in sys.argv[3].split(',')] if len(sys.argv) > 3 and sys.argv[3] else []
    base = tuner if '://' in tuner else f'http://{tuner}'

    auth = json.loads(get(f'{base}/discover.json', "the tuner's discover.json")).get('DeviceAuth', '')
    if not auth:
        fail("the tuner didn't give a guide key")
    guide = get('https://api.hdhomerun.com/api/xmltv?DeviceAuth=' + urllib.parse.quote(auth), "SiliconDust's guide")
    try:
        root = ET.fromstring(guide)
    except ET.ParseError:
        fail("the guide wasn't readable")

    channels = {}
    for c in root.findall('channel'):
        number = (c.findtext('lcn') or '').strip()
        icon = c.find('icon')
        src = icon.get('src', '') if icon is not None else ''
        if number and src and urllib.parse.urlparse(src).hostname == LOGO_HOST and src.startswith('https://'):
            channels[number] = ((c.findtext('display-name') or '').strip(), src)
    if not wanted:
        wanted = [n for n in sorted(channels, key=lambda n: float(n)) if n.endswith('.1')][:4]
    missing = [n for n in wanted if n not in channels]
    if missing:
        fail('no logo in the guide for ' + ', '.join(missing))

    os.makedirs(out, exist_ok=True)
    lineup = []
    for i, number in enumerate(wanted):
        name, src = channels[number]
        logo = os.path.basename(urllib.parse.urlparse(src).path)
        if not re.fullmatch(r'[A-Za-z0-9_.-]+\.png', logo):
            fail(f'an unexpected logo name for {number}')
        with open(os.path.join(out, logo), 'wb') as f:
            f.write(get(src, f'the logo for {number}', 2_000_000))
        # The lineup calls them WSB-HD where the guide says WSBDT.
        lineup.append({'number': number, 'name': re.sub(r'DT\d*$', '-HD', name) or name, 'logo': logo, 'title': TITLES[i % len(TITLES)]})
    with open(os.path.join(out, 'channels.json'), 'w') as f:
        json.dump(lineup, f, indent=2)
    print('logos for', ', '.join(f"{c['number']} {c['name']}" for c in lineup), '->', out)


main()
