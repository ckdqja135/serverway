import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react()],
  server: { host: '127.0.0.1', port: 7880, strictPort: true, proxy: { '/api': 'http://127.0.0.1:7800', '/ws': { target: 'http://127.0.0.1:7800', ws: true } } },
})
