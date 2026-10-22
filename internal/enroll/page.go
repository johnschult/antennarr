package enroll

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"net/http"
)

// plexWordmark is Plex's wordmark, the one the admin page's sign-in button uses, for the button here. It is drawn
// black on Plex's yellow-orange, so the page needs no image of its own to fetch.
//
//go:embed plex-wordmark.png
var plexWordmark []byte

var plexWordmarkURI = "data:image/png;base64," + base64.StdEncoding.EncodeToString(plexWordmark)

// plexButton is the button that starts the Plex sign-in: Plex's own yellow-orange with black lettering.
func plexButton(lead string) string {
	return fmt.Sprintf(`<button type="submit" class="plex" aria-label="%s Plex">%s <img src="%s" alt="" height="22"></button>`, esc(lead), esc(lead), plexWordmarkURI)
}

// mark is Yagi's own mark, the one in docs/brand/yagi-logo.svg: Lucide's antenna in teal on a dark tile. It is the icon
// of the pages where a person is joining Yagi.
const mark = `<svg class="mark" viewBox="0 0 256 256" aria-hidden="true"><rect width="256" height="256" rx="56" fill="#0f1b24"/><g transform="translate(28 28) scale(8.5)" fill="none" stroke="#2dd4bf" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12 7 2"/><path d="m7 12 5-10"/><path d="m12 12 5-10"/><path d="m17 12 5-10"/><path d="M4.5 7h15"/><path d="M12 16v6"/></g></svg>`

// plexLink is plexButton for something that goes to Plex's own page: a link, in the same place as the button.
func plexLink(lead, href string) string {
	return fmt.Sprintf(`<a class="button plex" href="%s" rel="noopener noreferrer" aria-label="%s Plex">%s <img src="%s" alt="" height="22"></a>`, esc(href), esc(lead), esc(lead), plexWordmarkURI)
}

// icons are Lucide's, drawn large at the top of each page. They are inline, so the page fetches nothing for them.
var icons = map[string]string{
	"user-plus":      `<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><line x1="19" x2="19" y1="8" y2="14"/><line x1="22" x2="16" y1="11" y2="11"/>`,
	"hourglass":      `<path d="M5 22h14"/><path d="M5 2h14"/><path d="M17 22v-4.172a2 2 0 0 0-.586-1.414L12 12l-4.414 4.414A2 2 0 0 0 7 17.828V22"/><path d="M7 2v4.172a2 2 0 0 0 .586 1.414L12 12l4.414-4.414A2 2 0 0 0 17 6.172V2"/>`,
	"server":         `<rect width="20" height="8" x="2" y="2" rx="2" ry="2"/><rect width="20" height="8" x="2" y="14" rx="2" ry="2"/><line x1="6" x2="6.01" y1="6" y2="6"/><line x1="6" x2="6.01" y1="18" y2="18"/>`,
	"circle-check":   `<circle cx="12" cy="12" r="10"/><path d="m16 9-5.5 5.5L8 12"/>`,
	"circle-x":       `<circle cx="12" cy="12" r="10"/><path d="m15 9-6 6"/><path d="m9 9 6 6"/>`,
	"server-off":     `<path d="M7 2h13a2 2 0 0 1 2 2v4a2 2 0 0 1-2 2h-5"/><path d="M10 10 2.5 2.5C2 2 2 2.5 2 5v3a2 2 0 0 0 2 2h6z"/><path d="M22 17v-1a2 2 0 0 0-2-2h-1"/><path d="M4 14a2 2 0 0 0-2 2v4a2 2 0 0 0 2 2h16.5l1-.5.5.5-8-8H4z"/><path d="M6 18h.01"/><path d="m2 2 20 20"/>`,
	"triangle-alert": `<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4"/><path d="M12 17h.01"/>`,
}

// tones colors an icon: green for going well, amber for something to look at, red for a no.
var tones = map[string]string{"circle-x": "#f87171", "triangle-alert": "#f5b942", "server-off": "#f5b942"}

const defaultTone = "#2dd4bf"

const pageStyle = `:root{color-scheme:dark}
*{box-sizing:border-box}
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:1rem;background:#121416;color:#e9ebee;font:16px/1.55 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
main{width:100%;max-width:28rem;padding:2.25rem 1.75rem 2rem;text-align:center;background:#1b1e21;border:1px solid #2c3035;border-radius:1rem;box-shadow:0 12px 40px #0009}
.icon{width:76px;height:76px;fill:none;stroke:var(--tone);stroke-width:1.6;stroke-linecap:round;stroke-linejoin:round}
.mark{width:96px;height:96px;display:block;margin:0 auto;border-radius:22px;box-shadow:0 8px 24px #0008}
h1{margin:.9rem 0 1rem;font-size:1.55rem;line-height:1.25}
p{margin:.8rem 0;color:#c4c9cf}
b{color:#fff}
a{color:#5eead4}
form{margin:1.4rem 0 0}
button,.button{display:inline-flex;align-items:center;justify-content:center;gap:.6rem;min-height:2.9rem;padding:.65rem 1.4rem;border:0;border-radius:.6rem;font:inherit;font-weight:600;text-decoration:none;color:#042f2a;background:#2dd4bf;cursor:pointer}
button:hover,.button:hover{filter:brightness(1.08)}
.button{background:#2c3035;color:#e9ebee}
.plex,.button.plex{background:#e5a00d;color:#000;padding:.65rem 1.6rem}
.plex:hover,.button.plex:hover{background:#f0ae21;filter:none}
.plex img{display:block;width:auto;filter:brightness(0)}
:focus-visible{outline:3px solid #ffffffb0;outline-offset:3px}
.row{display:block;margin:.6rem 0;padding:.75rem .9rem;text-align:left;background:#14171a;border:1px solid #2c3035;border-radius:.6rem;cursor:pointer}
.row:has(input:checked){border-color:#2dd4bf}
.row input{accent-color:#2dd4bf;margin-right:.35rem}
.steps{margin:.4rem 0;padding-left:1.4rem;text-align:left;color:#c4c9cf}
.steps li{margin:.35rem 0}
.addr,.note{color:#8f969e;font-size:.9rem}
.note{margin-top:1rem}
code{padding:.1rem .35rem;background:#ffffff14;border-radius:.3rem;font-size:.9em}
@media (prefers-reduced-motion:no-preference){main{animation:in .25s ease-out}@keyframes in{from{opacity:0;transform:translateY(6px)}}}`

// page writes a small standalone page: a large icon, the title, and the body. refresh, if set, is where the browser
// goes by itself in a few seconds.
func page(w http.ResponseWriter, status int, icon, title, body, refresh string) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	// The link is in the address, so nothing leaves the site. Not "no-referrer": that makes the browser send
	// "Origin: null" with a form.
	h.Set("Referrer-Policy", "same-origin")
	h.Set("X-Content-Type-Options", "nosniff")
	// Nothing but its own style, and the one image, which is in the page itself.
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; form-action 'self'; frame-ancestors 'none'")
	w.WriteHeader(status)
	meta := ""
	if refresh != "" {
		meta = fmt.Sprintf(`<meta http-equiv="refresh" content="3;url=%s">`, esc(refresh))
	}
	tone := defaultTone
	if t, ok := tones[icon]; ok {
		tone = t
	}
	fmt.Fprintf(w, `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark">%s
<title>%s</title>
<style>%s</style>
<main style="--tone:%s">
%s
<h1>%s</h1>
%s
</main>
</html>`, meta, esc(title), pageStyle, tone, iconMarkup(icon), esc(title), body)
}

// iconMarkup is the large icon at the top of a page: Yagi's mark, or one of Lucide's drawn in the page's tone.
func iconMarkup(icon string) string {
	if icon == "yagi" {
		return mark
	}
	return `<svg class="icon" viewBox="0 0 24 24" aria-hidden="true">` + icons[icon] + `</svg>`
}
