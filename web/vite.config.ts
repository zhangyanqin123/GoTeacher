import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// dev proxy：后端 handicap-service :8080（CORS 虽已放开，走 proxy 同源更干净）
export default defineConfig({
  plugins: [react()],
  resolve: { alias: { '@': '/src' } },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: true },
      '/guyuzhoudb': { target: 'http://localhost:8080', changeOrigin: true },
    },
  },
})
