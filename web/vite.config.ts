import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// 링크 미리보기 이미지는 전체 주소여야 한다. Vercel에서는 배포 주소를 자동으로 쓴다.
process.env.VITE_SITE_URL ||= `https://${process.env.VERCEL_PROJECT_PRODUCTION_URL || 'web-nine-rho-62.vercel.app'}`

export default defineConfig({
  plugins: [react()],
  server: { host: '127.0.0.1', port: 7880, strictPort: true, proxy: { '/api': 'http://127.0.0.1:7800', '/ws': { target: 'http://127.0.0.1:7800', ws: true } } },
})
