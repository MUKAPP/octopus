import path from 'node:path';
import react from '@vitejs/plugin-react';
import { compression, defineAlgorithm } from 'vite-plugin-compression2';
import { defineConfig } from 'vite';

export default defineConfig({
  base: './',
  plugins: [
    react({
      babel: {
        plugins: ['babel-plugin-react-compiler'],
      },
    }),
    compression({
      algorithms: [defineAlgorithm('gzip', { level: 9 })],
      include: /\.(html|css|js|mjs|json|svg|txt|xml)$/,
      deleteOriginalAssets: true,
    }),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    outDir: path.resolve(__dirname, '../static/out'),
    emptyOutDir: true,
  },
  server: {
    hmr: process.env.DISABLE_HMR !== 'true',
    watch: process.env.DISABLE_HMR === 'true' ? null : {},
    proxy: {
      '/api': {
        target: process.env.VITE_PROXY_TARGET || 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
});
