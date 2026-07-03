import { defineConfig } from "vite";
import solid from "vite-plugin-solid";

export default defineConfig({
  plugins: [solid()],
  build: {
    assetsDir: "client-assets",
  },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:8080",
      "/assets": "http://127.0.0.1:8080",
    },
  },
});
