import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue2";
import { fileURLToPath, URL } from "node:url";

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url))
    }
  },
  server: {
    port: 8081,
    proxy: {
      "/api": "http://127.0.0.1:8080",
      "/uploads": "http://127.0.0.1:8080"
    }
  },
  build: {
    sourcemap: false,
    chunkSizeWarningLimit: 1200
  }
});
