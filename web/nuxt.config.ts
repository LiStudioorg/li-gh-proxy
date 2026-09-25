import tailwindcss from '@tailwindcss/vite'
import { resolve } from 'node:path'

export default defineNuxtConfig({
  compatibilityDate: '2025-07-15',

  ssr: false,
  devtools: { enabled: false },
  telemetry: false,

  typescript: {
    strict: true,
  },

  app: {
    // Go 侧注册的路由：/、/images、/search 固定回源 dist/index.html，
    // 因此必须输出纯 SPA（客户端路由），且静态资源位于 /assets/ 下
    buildAssetsDir: 'assets/',
    head: {
      htmlAttrs: { lang: 'zh-CN' },
      meta: [
        { name: 'description', content: 'li-gh-proxy - GitHub 加速、Docker 镜像加速与离线下载' },
      ],
      link: [{ rel: 'icon', href: '/favicon.ico' }],
      script: [
        {
          // 挂载前应用主题，避免暗色模式下闪白
          innerHTML: `(function(){try{var t=localStorage.getItem('theme');var d=t==='dark'||(!t&&window.matchMedia('(prefers-color-scheme: dark)').matches);if(d)document.documentElement.classList.add('dark')}catch(e){}})()`,
        },
      ],
    },
    pageTransition: { name: 'page', mode: 'out-in' },
  },

  css: ['~/assets/css/main.css'],

  vite: {
    plugins: [tailwindcss()],
  },

  nitro: {
    // go:embed 约束：产物输出到 ../src/dist
    output: {
      publicDir: resolve(process.cwd(), '../src/dist'),
    },
    prerender: {
      crawlLinks: false,
    },
    // 开发期将 /api 代理到本地 Go 后端
    devProxy: {
      '/api': { target: 'http://127.0.0.1:5000', changeOrigin: true },
    },
  },
})
