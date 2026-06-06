import { defineConfig } from 'vite';
import { resolve } from 'node:path';

// Vite is set up but not wired into the Capacitor sync step yet. The
// follow-up integration PR will switch index.html to import modules
// directly and drop the per-file <script> tags. For now this config
// exists so `vite build` works locally for sanity checks.
export default defineConfig({
  root: resolve(__dirname, 'src'),
  build: {
    outDir: resolve(__dirname, 'www'),
    emptyOutDir: true,
    rollupOptions: {
      input: {
        main: resolve(__dirname, 'src/index.html'),
      },
    },
  },
  server: {
    port: 5173,
    strictPort: false,
  },
});
