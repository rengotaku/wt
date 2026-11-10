import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";
import { mkdirSync, writeFileSync } from "node:fs";

const DIST_DIR = path.resolve(__dirname, "../internal/static/dist");

// `emptyOutDir` は成果物ディレクトリを丸ごと空にするため、追跡下の
// `internal/static/dist/.gitkeep` も消える。残すと作業ツリーが dirty になり、
// rebase 設定の clone では次の `git pull` が未コミット変更で失敗する。
// `//go:embed all:dist` は空ディレクトリを embed できないので、ビルド後に書き戻す。
// See: https://github.com/rengotaku/wt/issues/163
function keepDistGitkeep(): Plugin {
  return {
    name: "wt-keep-dist-gitkeep",
    apply: "build",
    closeBundle() {
      mkdirSync(DIST_DIR, { recursive: true });
      writeFileSync(path.join(DIST_DIR, ".gitkeep"), "");
    },
  };
}

export default defineConfig({
  plugins: [react(), tailwindcss(), keepDistGitkeep()],
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
