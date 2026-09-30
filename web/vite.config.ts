import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { VitePWA } from 'vite-plugin-pwa'

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    VitePWA({
      registerType: 'autoUpdate',
      // Registered from main.tsx (virtual:pwa-register) rather than an injected
      // inline script, which the controller's CSP would block.
      injectRegister: false,
      includeAssets: ['favicon.svg', 'apple-touch-icon.png', 'theme-init.js'],
      manifest: {
        name: 'Docker Cockpit',
        short_name: 'Cockpit',
        description: 'Monitor and manage your Docker hosts.',
        theme_color: '#0B0F14',
        background_color: '#0B0F14',
        display: 'standalone',
        start_url: '/',
        scope: '/',
        icons: [
          { src: '/icon.svg', sizes: 'any', type: 'image/svg+xml', purpose: 'any' },
          { src: '/icon-192.png', sizes: '192x192', type: 'image/png', purpose: 'any' },
          { src: '/icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'any' },
          { src: '/icon-maskable-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
        ],
      },
      workbox: {
        // Only the app shell is cached. API responses (containers, logs, tokens)
        // are never stored on the device: there is no runtimeCaching.
        globPatterns: ['**/*.{js,css,html,svg,png,woff2}'],
        cleanupOutdatedCaches: true,
        navigateFallback: '/index.html',
        // Let these reach the network even when typed into the address bar.
        navigateFallbackDenylist: [/^\/api\//, /^\/agent\//, /^\/healthz$/, /^\/install\.sh$/],
      },
    }),
  ],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  // In development the controller runs separately; proxy API calls to it.
  server: {
    // Explicit IPv4 loopback: "localhost" may bind to ::1 only, which some
    // browsers cannot reach. Loopback-only also keeps it off the LAN.
    host: '127.0.0.1',
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': { target: 'http://127.0.0.1:8080', ws: true },
      '/agent': { target: 'http://127.0.0.1:8080', ws: true },
    },
  },
})
