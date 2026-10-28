/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  server: {
    // In development, proxy the API like nginx does in the container.
    proxy: { '/api': process.env.API_URL ?? 'http://localhost:8080' },
  },
  test: {
    environment: 'jsdom',
  },
})
