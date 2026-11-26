import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    host: '127.0.0.1',
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': { target: 'http://127.0.0.1:6767', changeOrigin: false },
      '/health': { target: 'http://127.0.0.1:6767', changeOrigin: false },
    },
  },
  build: {
    outDir: '../web/dist',
    emptyOutDir: true,
  },
})
