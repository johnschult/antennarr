// Runs before the page paints, so a light or dark choice doesn't flash the
// wrong theme first. Kept in step with src/lib/theme.ts. It is a separate
// file (not inline) because the page's security policy forbids inline scripts.
;(function () {
  try {
    var pref = localStorage.getItem('antennarr-theme')
    var dark =
      pref === 'dark' || ((pref !== 'light') && window.matchMedia('(prefers-color-scheme: dark)').matches)
    document.documentElement.classList.toggle('dark', dark)
    document.documentElement.style.colorScheme = dark ? 'dark' : 'light'
  } catch {
    /* no storage: the provider sets the theme once the app loads */
  }
})()
