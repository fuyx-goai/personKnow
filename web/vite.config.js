import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 构建产物直接落进 Go 的 embed 目录：internal/gateway/web/dist
// 这样 go build 时前端已经"长"在二进制里，运行时不需要 Node，也不依赖工作目录。
const outDir = fileURLToPath(new URL('../internal/gateway/web/dist', import.meta.url))

export default defineConfig({
  plugins: [vue()],

  // 页面由 Go 服务挂在根路径 / 上，静态资源统一走 /assets
  base: '/',

  build: {
    outDir,
    emptyOutDir: true,
    assetsDir: 'assets',
    sourcemap: false,
    // 这是本机自用的界面，不必为"首屏体积"过度拆包
    chunkSizeWarningLimit: 1200,
  },

  server: {
    port: 5173,
    // 开发模式下前端跑 Vite（热更新），接口转发给 Go 网关，两边各管各的
    proxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: true },
    },
  },
})
