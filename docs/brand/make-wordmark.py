#!/usr/bin/env python3
"""Draws the README's logo with its wordmark: yagi-wordmark-light.png (dark lettering, for a light page) and
yagi-wordmark-dark.png (white lettering, for a dark one), as the app's top bar draws them: the mark, then "Yagi" in
Mozilla Headline Bold with its letters a little apart, lined up with the bottom of the mark's body.

  python3 docs/brand/make-wordmark.py

Needs Playwright for Python and a Chromium (pip install playwright && playwright install chromium), and
`npm ci` in web/ so the font is on disk (@fontsource-variable/mozilla-headline). The text colors are the app's
--foreground in each theme (web/src/index.css).
"""
import pathlib
import tempfile

from playwright.sync_api import sync_playwright

here = pathlib.Path(__file__).resolve().parent
root = here.parent.parent
font = root / 'web/node_modules/@fontsource-variable/mozilla-headline/files/mozilla-headline-latin-wght-normal.woff2'
mark = here / 'yagi-logo.png'
if not font.exists():
    raise SystemExit(f'{font} is missing: run `npm ci` in web/ first')

# The top bar draws the mark at 42px with 36px (text-4xl) lettering, 2.5 * 4px between them and 4px under the text.
# Everything here is that, times S.
S = 3.6
THEMES = {'light': 'oklch(0.2077 0.0398 265.7549)', 'dark': 'oklch(1 0 0)'}

PAGE = """<!doctype html><meta charset="utf-8">
<style>
@font-face {{ font-family: 'Mozilla Headline Variable'; src: url('file://{font}') format('woff2'); font-weight: 200 700; }}
html, body {{ margin: 0; background: transparent; }}
#w {{ display: inline-flex; align-items: flex-end; gap: {gap}px; padding: {pad}px; }}
#w img {{ width: {logo}px; height: {logo}px; display: block; }}
#w span {{ font-family: 'Mozilla Headline Variable'; font-weight: 700; font-size: {text}px; line-height: 1;
  letter-spacing: 0.02em; padding-bottom: {under}px; color: {color}; }}
</style>
<div id="w"><img src="file://{mark}"><span>Yagi</span></div>"""

with sync_playwright() as p:
    browser = p.chromium.launch()
    for name, color in THEMES.items():
        page = browser.new_page(device_scale_factor=2, viewport={'width': 1200, 'height': 400})
        # A file, not set_content: a page with no address may not load the font and the mark from disk.
        with tempfile.TemporaryDirectory() as tmp:
            html = pathlib.Path(tmp) / 'wordmark.html'
            html.write_text(PAGE.format(font=font, mark=mark, color=color, gap=10 * S, pad=4 * S, logo=42 * S, text=36 * S, under=4 * S))
            page.goto(html.as_uri())
            page.evaluate('document.fonts.ready')
            page.wait_for_function("document.fonts.check(\"700 20px 'Mozilla Headline Variable'\")")
            page.locator('#w').screenshot(path=str(here / f'yagi-wordmark-{name}.png'), omit_background=True)
        page.close()
    browser.close()
print('written to', here)
