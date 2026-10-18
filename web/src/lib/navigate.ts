/**
 * Opens this page again on another port of the same host. After a tuner is moved
 * to a new port the old one stops answering, and the page was served from it.
 */
export function moveToPort(port: number) {
  const url = new URL(window.location.href)
  if (url.port === String(port)) return
  url.port = String(port)
  window.location.assign(url.toString())
}
