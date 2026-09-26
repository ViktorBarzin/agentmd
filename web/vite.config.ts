/// <reference types="vitest/config" />
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { defineConfig } from 'vite'

// The Go binary embeds web/dist/ui and serves it from any path, so assets use
// relative URLs. The build owns only dist/ui, leaving the committed
// dist/placeholder.html alone. The dev server forwards API calls to a local
// `agentmd serve`.
export default defineConfig({
  base: './',
  plugins: [svelte()],
  build: {
    outDir: 'dist/ui',
    emptyOutDir: true,
    // The Graph view's chunk is cytoscape plus fcose, about 570 kB, and loads
    // only when that view opens. The first page load is about 100 kB.
    chunkSizeWarningLimit: 600,
  },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:7390',
    },
  },
  test: {
    include: ['src/**/*.test.ts'],
    environment: 'node',
  },
})
