// Applies the saved theme before first paint to avoid a flash. Loaded as a
// file (not inline) so the controller's Content-Security-Policy allows it.
;(function () {
  var t = 'system'
  try {
    t = localStorage.getItem('cockpit-theme') || 'system'
  } catch (e) {}
  var dark = t === 'dark' || (t === 'system' && matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', dark)
  var meta = document.querySelector('meta[name="theme-color"]')
  if (meta) meta.setAttribute('content', dark ? '#0B0F14' : '#F6F8FA')
})()
