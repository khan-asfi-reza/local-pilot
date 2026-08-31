import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The app talks to the backend through this proxy: fetch('/api/...') is forwarded
// to the API on http://localhost:8000. Do NOT hardcode the backend URL in the app.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    strictPort: false,
    proxy: {
      '/api': { target: 'http://localhost:8010', changeOrigin: true },
    },
  },
})
