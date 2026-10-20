#!/usr/bin/env python3
"""Takes the README screenshots of a running demo (see retake.sh).

usage: shoot.py <demo address, e.g. http://127.0.0.1:5004> <output dir>

Needs Playwright for Python and a Chromium it can find
(`pip install playwright && playwright install chromium`).
"""
import json
import re
import sys
import time

from playwright.sync_api import sync_playwright

base, out = sys.argv[1].rstrip('/'), sys.argv[2]

# The demo runs on this machine, so the page would show its own address and no
# Tailscale one. Show what a real install looks like instead.
NETWORK = {
    'addresses': [
        {'ip': '100.127.237.109', 'kind': 'tailscale', 'interface': 'tailscale0'},
        {'ip': '192.168.68.72', 'kind': 'lan', 'interface': 'en0'},
        {'ip': '192.168.68.55', 'kind': 'lan', 'interface': 'en1'},
    ],
    'firewall': {'tool': '', 'active': False},
}
SHOW_LAN_ADDRESS = """() => {
  const re = /127\\.0\\.0\\.1:\\d+/
  const w = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
  let n
  while ((n = w.nextNode())) if (re.test(n.nodeValue)) n.nodeValue = n.nodeValue.replace(re, '192.168.68.143')
}"""


def shot(page, name, **kw):
    page.screenshot(path=f'{out}/{name}.jpg', type='jpeg', quality=90, **kw)


with sync_playwright() as p:
    browser = p.chromium.launch()

    def context(scheme, width=1280, height=834, scale=1.25, phone=False):
        return browser.new_context(viewport={'width': width, 'height': height}, device_scale_factor=scale,
                                   color_scheme=scheme, is_mobile=phone, has_touch=phone)

    def open_page(ctx, path, wait_for):
        page = ctx.new_page()
        page.route('**/api/network', lambda r: r.fulfill(status=200, content_type='application/json', body=json.dumps(NETWORK)))
        # The live feed never goes idle, so don't wait for the network to.
        page.goto(base + path, wait_until='domcontentloaded')
        page.wait_for_selector(wait_for, timeout=15000)
        return page

    for scheme in ('light', 'dark'):
        ctx = context(scheme)
        page = open_page(ctx, '/', '[data-testid="tuner-tile"]')
        time.sleep(14)  # let the sparklines gather some samples
        page.evaluate(SHOW_LAN_ADDRESS)
        time.sleep(0.3)
        # Just short of the footer, which would otherwise show as a sliver.
        shot(page, f'live-{scheme}', clip={'x': 0, 'y': 0, 'width': 1280, 'height': 826})
        ctx.close()

    ctx = context('dark')
    page = open_page(ctx, '/viewers', 'table')
    page.evaluate(SHOW_LAN_ADDRESS)
    time.sleep(0.3)
    shot(page, 'viewers', clip={'x': 0, 'y': 0, 'width': 1280, 'height': 418})
    ctx.close()

    ctx = context('dark')
    page = open_page(ctx, '/viewers', 'table')
    # How to connect is a step of adding a viewer; this is the same steps, opened from a viewer's row.
    page.get_by_role('button', name='Connection steps for Alex').click()
    dialog = page.get_by_role('dialog')
    dialog.get_by_label('Address viewers will use to reach this machine').wait_for(timeout=15000)
    time.sleep(1)
    page.evaluate(SHOW_LAN_ADDRESS)
    time.sleep(0.3)
    dialog.screenshot(path=f'{out}/setup.jpg', type='jpeg', quality=90)
    ctx.close()

    # Who watched what: the demo seeds two weeks of made-up sessions, and Alex's stream is recorded for real.
    ctx = context('dark', height=980)
    page = open_page(ctx, '/activity', '[data-testid="session"]')
    time.sleep(1)
    page.evaluate(SHOW_LAN_ADDRESS)
    shot(page, 'activity')
    ctx.close()

    # A tuner's page, on its Overview (opened directly; /tuners is the list).
    ctx = context('dark', height=600)
    page = open_page(ctx, '/tuners/antenna', '[data-testid="firmware-line"]')
    time.sleep(1)
    page.evaluate(SHOW_LAN_ADDRESS)
    time.sleep(0.3)
    shot(page, 'tuners')
    ctx.close()

    # A tuner's Channels tab.
    ctx = context('dark', height=860)
    page = open_page(ctx, '/tuners/antenna/channels', '[data-testid="channel"]')
    time.sleep(1)
    page.evaluate(SHOW_LAN_ADDRESS)
    shot(page, 'channels')
    ctx.close()

    # What is known about a channel: the card the i opens, on a channel the guide has things to say about.
    ctx = context('dark', height=760)
    page = open_page(ctx, '/tuners/antenna/channels', '[data-testid="channel"]')
    time.sleep(1)
    page.get_by_role('button', name=re.compile('^About .* Classic TV')).click()
    page.wait_for_selector('[data-testid="channel-about"]', timeout=15000)
    time.sleep(0.6)
    page.evaluate(SHOW_LAN_ADDRESS)
    shot(page, 'channel-info')
    ctx.close()

    # Filtering the channels: the panel open, with a category chosen.
    ctx = context('dark', height=760)
    page = open_page(ctx, '/tuners/antenna/channels', '[data-testid="channel"]')
    time.sleep(1)
    page.get_by_role('button', name='Filters').click()
    page.wait_for_selector('[data-testid="filters"]', timeout=15000)
    page.get_by_role('region', name='Categories').get_by_role('button', name=re.compile('^Shopping')).click()
    page.evaluate("document.querySelector('[data-testid=filters]').parentElement.scrollTop = 0")  # clicking scrolled the panel
    time.sleep(0.6)
    page.evaluate(SHOW_LAN_ADDRESS)
    shot(page, 'channel-filters')
    ctx.close()

    # Settings, on its Logs tab.
    ctx = context('dark', height=720)
    # The demo records at debug; show the default, info, where the note about ordinary requests is the useful one.
    ctx.request.patch(base + '/api/settings', data=json.dumps({'logLevel': 'info'}), headers={'Content-Type': 'application/json'})
    page = open_page(ctx, '/settings/logs', '[data-testid="log-line"]')
    time.sleep(1)
    page.evaluate(SHOW_LAN_ADDRESS)
    shot(page, 'logs')
    ctx.close()

    # Settings, on its Privacy tab, where the activity history switch is.
    ctx = context('dark', height=1100)
    page = open_page(ctx, '/settings/privacy', 'text=Keep activity history')
    time.sleep(0.5)
    page.evaluate(SHOW_LAN_ADDRESS)
    shot(page, 'settings')
    ctx.close()

    # Settings, the Access tab: Tailscale in its own section (the demo's node is made up, and connected).
    ctx = context('dark', height=1300)
    page = open_page(ctx, '/settings/access', 'section[aria-label="Tailscale"]')
    time.sleep(0.8)
    page.evaluate(SHOW_LAN_ADDRESS)
    page.locator('section[aria-label="Tailscale"]').screenshot(path=f'{out}/tailscale.jpg', type='jpeg', quality=90)
    ctx.close()

    ctx = context('dark', 390, 844, 2, phone=True)
    page = open_page(ctx, '/', '[data-testid="tuner-tile"]')
    time.sleep(2)
    page.evaluate(SHOW_LAN_ADDRESS)
    time.sleep(0.3)
    shot(page, 'phone-live')
    page.get_by_role('link', name='Viewers').click()
    time.sleep(1)
    shot(page, 'phone-viewers')
    ctx.close()
    browser.close()
print('screenshots written to', out)
