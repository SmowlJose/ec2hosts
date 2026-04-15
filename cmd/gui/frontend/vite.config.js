import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Wails expects the built frontend at frontend/dist — the //go:embed
// directive in cmd/gui/main.go pulls from there.
export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
