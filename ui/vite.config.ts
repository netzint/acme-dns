import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// The Go binary serves the built app from its own root, so the API lives on the
// same origin in production. In dev, proxy it to a locally running acme-dns.
const backend = process.env.ACMEDNS_BACKEND ?? 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    proxy: {
      '/api': { target: backend, changeOrigin: true },
      '/health': { target: backend, changeOrigin: true },
      '/register': { target: backend, changeOrigin: true },
      '/update': { target: backend, changeOrigin: true },
    },
  },
})
