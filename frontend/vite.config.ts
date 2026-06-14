import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 5174,
    // dev backend = air on :8091 (see .air.toml). Distinct from the systemd
    // wt-web instance on :8090 so `make run` exercises local backend code.
    proxy: { "/api": "http://localhost:8091" },
  },
  build: {
    outDir: "../internal/static/dist",
    emptyOutDir: true,
  },
});
