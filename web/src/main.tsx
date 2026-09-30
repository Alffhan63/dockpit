import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { registerSW } from 'virtual:pwa-register'
import { App } from './App'
import './index.css'

// autoUpdate: a new deploy's service worker activates on its own. What does not
// happen on its own is checking for updates while an installed app sits open on
// a phone, and reloading this tab once the new version takes over.
registerSW({
  immediate: true,
  onRegisteredSW(_url, registration) {
    if (!registration) return
    const check = () => registration.update().catch(() => {})
    check()
    setInterval(check, 60_000)
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'visible') check()
    })
  },
})

// controllerchange also fires the first time a worker claims the page; only
// reload when an existing worker is replaced, i.e. a real update.
const hadController = Boolean(navigator.serviceWorker?.controller)
let reloading = false
navigator.serviceWorker?.addEventListener('controllerchange', () => {
  if (!hadController || reloading) return
  reloading = true
  location.reload()
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)

// Keep the launch screen up briefly so it reads as intentional, then fade it.
setTimeout(() => {
  const splash = document.getElementById('app-splash')
  if (!splash) return
  splash.classList.add('app-splash-hide')
  setTimeout(() => splash.remove(), 300)
}, 500)
