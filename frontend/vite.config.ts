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
    // API proxy target:
    //  - `wt serve` injects WT_PORT_API（このフロントと同じブロックの api ポート）
    //  - 無い場合は air の既定 :8091（.air.toml）にフォールバック
    proxy: { "/api": `http://localhost:${process.env.WT_PORT_API ?? "8091"}` },
  },
  build: {
    outDir: "../internal/static/dist",
    emptyOutDir: true,
  },
});
