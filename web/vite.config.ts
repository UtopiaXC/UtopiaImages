import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
    },
  },
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:2983',
        changeOrigin: true,
      },
      '/upload': {
        target: 'http://localhost:2983',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    assetsDir: 'ast',
    emptyOutDir: true,
    chunkSizeWarningLimit: 2000,
    rollupOptions: {
      output: {
        manualChunks: {
          'naive-ui': ['naive-ui'],
          'icons': ['@vicons/ionicons5'],
        },
        chunkFileNames: (chunkInfo) => {
          let name = chunkInfo.name || 'chunk'
          if (name.startsWith('_')) {
            name = name.replace(/^_+/, '')
          }
          return `ast/${name}-[hash].js`
        },
      },
    },
  },
})
