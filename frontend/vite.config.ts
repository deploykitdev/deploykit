import path from "node:path";
import tailwindcss from "@tailwindcss/vite";
import { reactRouter } from "@react-router/dev/vite";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [tailwindcss(), reactRouter()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./app"),
    },
  },
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: process.env.DEPLOYKIT_API_URL || "http://localhost:8080",
        changeOrigin: true,
        ws: true,
      },
    },
  },
});
