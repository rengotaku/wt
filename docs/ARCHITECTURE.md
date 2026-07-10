# wt アーキテクチャ

git worktree 管理 CLI + Web UI。単一 Go バイナリ（React SPA を go:embed）で動く。

## 全体構成

```
wt (cobra CLI)
├── tree / repo / symlink / gc   worktree・リポジトリ管理
├── dev / serve / ports          dev サービス定義・起動・ポート管理
├── proxy                        ドメインルーティング (:8088)
└── web                          SPA + JSON API (:8090)
```

### ディレクトリマップ

| パス | 役割 |
|---|---|
| `cmd/` | cobra コマンド定義（`web.go` が SPA/API サーバ + pin 自動 serve + idle reaper の配線） |
| `internal/handler/` | `wt web` の JSON API（`handler.go` にルート一覧） |
| `internal/static/` | フロントビルド成果物の embed（`dist/`、`-tags dev` で vite proxy に切替） |
| `internal/wt/core/` | コンテナ（`<base>/<repo>/`）と `.worktrees.json` の読み書き |
| `internal/wt/tree/` | worktree の add / rm / update / list |
| `internal/wt/devserver/` | dev サービスの serve / down / logs / 実行状態（`running.json`） |
| `internal/wt/ports/` | ポートブロック割当・LISTEN/ESTABLISHED 検出（ss）・doctor |
| `internal/wt/procstats/` | `/proc` スキャンによるプロセスグループ単位のメモリ計測 |
| `internal/wt/autostart/` | pin 自動 serve（`ServePinned`）と idle reaper |
| `internal/wt/proxy/` | `<label>.<repo>.wt.localhost` → 割当ポートへの逆プロキシ |
| `internal/wt/settings/` | `~/.config/wt/settings.toml` の読み書き |
| `frontend/` | React + vite + Tailwind の SPA（`src/pages/TreesPage.tsx` が worktree 一覧） |

### 状態ファイル

| ファイル | 内容 |
|---|---|
| `<base>/<repo>/.worktrees.json` | worktree エントリ（type / created / branch / pinned / port_base / `_config.dev_services` 等） |
| `~/.config/wt/settings.toml` | `[dev_ports]`（既定 9000-9999）/ `[idle_reaper]` / `[process_stats]` |
| `~/.cache/wt/run/<worktree_key>/running.json` | serve 済みサービスの記録（name / pid / port / cmd） |
| `~/.cache/wt/run/<worktree_key>/<svc>.log` | サービスごとの stdout+stderr |

## dev サービスのライフサイクル

1. **定義**: worktree の `.wt/dev.toml` → repo 既定（`.worktrees.json` の `_config.dev_services`）の優先順で実効化（`devserver.EffectiveConfig`）
2. **ポート割当**: worktree ごとに band から `BlockSize=5` の連続ブロックを確保し `port_base` を `.worktrees.json` に永続化。service i は `base+i` を使う
3. **起動**: `devserver.Serve` が `sh -c` + `Setpgid` で各サービスを起動（leader PID = PGID）。`PORT` と全サービス分の `WT_PORT_<NAME>` を環境変数で共有。600ms の起動グレース後に生存確認し、`running.json` へ記録
4. **停止**: `devserver.Down` がプロセスグループごと SIGTERM
5. **pin 自動 serve**: `wt web` 起動時に pinned かつ未稼働の worktree を `autostart.ServePinned` が serve
6. **idle reaper**: pinned かつ稼働中で、dev ポート帯に ESTABLISHED 接続が TTL（既定30分）以上無い worktree を自動 down（手動 serve は対象外、#92/#93）

## Web API（`internal/handler/handler.go`）

- `GET/POST/DELETE /api/trees`、`/api/trees/{repo}/{wt}/update|pin`、`/api/trees/gc|merged-prs|issue-details`
- `GET /api/ports`（worktree 別の稼働/縮退/ドメイン）、`/api/ports/listeners|stale`、`POST /api/ports/prune`、`/api/ports/{repo}/{wt}/serve|down|devconfig|logs`
- `GET /api/process-stats`（後述）
- `GET/POST /api/proxy`、`GET/PUT /api/settings`、`GET/POST /api/repos` 系

## プロセス状態可視化（process-stats）仕様（#99 / PR #100）

worktree 一覧の「状態」列に dev サービスの合計メモリを表示し、しきい値超過を一覧で警告する。

### 計測方式（`internal/wt/procstats`）

- dev サービスは Setpgid 起動のため **leader PID = PGID**。`/proc/<pid>/stat` を1回スキャンし pgrp（第5フィールド）でグルーピングすると、サービス配下の全プロセス（air の子 go プロセス・node watcher 等）を外部コマンド無しで集計できる
- 集計値: RSS 合計（第24フィールドのページ数 × ページサイズ）・プロセス数・グループ最古プロセスの経過秒（第22フィールド starttime、USER_HZ=100 前提）
- comm はスペース・括弧を含みうるため、行の**最後の `)`** より後ろをフィールド分割する
- **注意**: RSS はプロセス間の共有ページを重複カウントするため実メモリよりやや大きめに出る。Linux 専用

### API

`GET /api/process-stats` — `running.json` に記録がある worktree のみ返す:

```json
{
  "warn_bytes": 2147483648,
  "danger_bytes": 4294967296,
  "items": [{
    "repo": "myrepo", "wt_name": "myrepo--feat-x",
    "total_rss_bytes": 123456789, "level": "ok",
    "services": [{"name": "web", "pid": 1234, "port": 9001, "alive": true,
                  "procs": 4, "rss_bytes": 123456, "uptime_sec": 3600}]
  }]
}
```

- `wt_name` は生の worktree 名（trees/ports API とキー互換。CLI 表示ラベルではない）
- `level`: 合計 RSS が danger 以上 → `danger`、warn 以上 → `warn`、それ以外 `ok`
- 全プロセスが消えたサービスは `alive: false`（procs/rss/uptime は 0）

### しきい値設定

`settings.toml`（MB 単位、`Load()` で 0 以下と warn>=danger を既定値に補正）:

```toml
[process_stats]
warn_mb = 2048    # 既定 2GiB
danger_mb = 4096  # 既定 4GiB（#92 実測: 5 worktree で 21.5GiB ≒ 1worktree ~4GiB を危険域とした）
```

### UI（`TreesPage.tsx` / `WorktreeCard.tsx` / `ProcessStatsOverlay.tsx`）

- 一覧「状態」列: 合計 RSS（`formatBytes`、例 `320.9M`）。ok=muted / warn=アンバー / danger=赤字 + **行背景赤**（`bg-red-500/10`）。モバイルカードも同様（danger はカード枠も赤）
- 状態リンククリックでオーバーレイ: サービス別の PID / ポート / 稼働・停止 / プロセス数 / メモリ / 稼働時間 + しきい値の注記
- 10秒間隔でポーリング（稼働 worktree が無い間は停止）

## どこを触れば何が変わるか

| 変えたいこと | 触る場所 |
|---|---|
| dev サービスの起動・記録の仕組み | `internal/wt/devserver/run.go` |
| ポート帯・割当ロジック | `internal/wt/ports/` + `settings.toml [dev_ports]` |
| pin 自動 serve / idle 停止の方針 | `internal/wt/autostart/` + `[idle_reaper]` |
| メモリ計測・危険判定 | `internal/wt/procstats/` + `internal/handler/stats.go` + `[process_stats]` |
| 一覧の列・行の表示 | `frontend/src/pages/TreesPage.tsx`（カードは `WorktreeCard.tsx`） |
| API の追加 | `internal/handler/`（ルートは `handler.go`）+ `frontend/src/api/` |

## 検証ゲート

- `make ci` = golangci-lint + go mod diff-check + `go test -cover ./...`
- CI はさらに `cd frontend && npm ci && npm run build`（tsc 込み）を回す — push 前に `make build` も緑にすること
- フロントテストは `cd frontend && npx vitest run`（CI 対象外だがローカルで回す）
- ローカルで `TestProxyController_StartStop` が落ちる場合は稼働中 proxy の :8088 占有が原因（環境起因）
