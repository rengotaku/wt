# wt アーキテクチャ

git worktree 管理 CLI + Web UI。単一 Go バイナリ（React SPA を go:embed）で動く。

## 全体構成

```
wt (cobra CLI)
├── tree / repo / symlink / gc   worktree・リポジトリ管理
├── dev / serve / ports          dev サービス定義・起動・ポート管理
├── proxy                        ドメインルーティング (:8088、`wt web` に内蔵。単独起動も可)
└── web                          SPA + JSON API (:8090、内蔵 proxy を既定 ON で同時起動)
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
| `internal/wt/autostart/` | AutoStart 自動 serve（`ServeAutoStart`）/ idle reaper（`Reaper`）/ 幽霊ポート reaper（`PortReaper`）/ クラッシュ検知＆自動復帰 reaper（`HealthReaper`） |
| `internal/wt/proxy/` | `<label>.<repo>.wt.localhost` → 割当ポートへの逆プロキシ |
| `internal/wt/settings/` | `~/.config/wt/settings.toml` の読み書き |
| `frontend/` | React + vite + Tailwind の SPA（`src/pages/TreesPage.tsx` が worktree 一覧） |

### 状態ファイル

| ファイル | 内容 |
|---|---|
| `<base>/<repo>/.worktrees.json` | worktree エントリ（type / created / branch / pinned / auto_start / port_base / `_config.dev_services` 等） |
| `<base>/<repo>/.worktrees.json::_config.hidden（core.EntryConfig.Hidden）` | repo 単位の表示/非表示フラグ（ローカル設定、コミットしない） |
| `~/.config/wt/settings.toml` | `[dev_ports]`（既定 9000-9999）/ `[idle_reaper]` / `[port_reaper]`（既定 1440分=1日）/ `[health_reaper]`（既定 interval=2分 / cooldown=10分 / max_retries=3）/ `[process_stats]` / `[proxy]`（`enabled` 既定 true / `port` 既定 8088） |
| `~/.cache/wt/run/<worktree_key>/running.json` | serve 済みサービスの記録（name / pid / port / cmd） |
| `~/.cache/wt/run/<worktree_key>/<svc>.log` | サービスごとの stdout+stderr |

## dev サービスのライフサイクル

1. **定義**: worktree の `.wt/dev.toml` → repo 既定（`.worktrees.json` の `_config.dev_services`）の優先順で実効化（`devserver.EffectiveConfig`）
2. **ポート割当**: worktree ごとに band から `BlockSize=5` の連続ブロックを確保し `port_base` を `.worktrees.json` に永続化。service i は `base+i` を使う
3. **起動**: `devserver.Serve` が各サービスを `Setpgid` 付きで起動（leader PID = PGID）。systemd が使える環境では `systemd-run --user --scope --slice=wt-dev.slice` でラップし、`wt-dev-<worktree>-<svc>-<suffix>.scope` として **wt-web.service の cgroup 外**に分離する（#122。unit の stop/restart が dev サービスと headless chrome を巻き添えにしない）。`systemd-run` 不在・`WT_NO_SYSTEMD_RUN=1` の環境では従来どおり `sh -c` 直接 spawn にフォールバック。`PORT` と全サービス分の `WT_PORT_<NAME>` を環境変数で共有。600ms の起動グレース後に生存確認し、`running.json` へ記録。稼働 scope は `systemctl --user list-units 'wt-dev-*'` で列挙できる。**`IsRunning` は `running.json` の PID プロセスグループが空でも `wt-dev-<worktree>-*.scope` が active なら true を返す**（python worker 等が setsid で detach するとプロセス group プローブでは死んで見えるため、systemd の scope 有無を fallback にする。#131）
4. **停止**: `devserver.Down` がプロセスグループごと SIGTERM（カレント worktree の記録済みサービスのみが対象。wt-web 本体・他 worktree には影響しない。scope は全メンバー終了で自動消滅）
5. **AutoStart 自動 serve**: `wt web` 起動時に `auto_start=true` かつ未稼働の worktree を `autostart.ServeAutoStart` が serve。既に稼働中の worktree は再起動しない
6. **idle reaper**: `auto_start=true` かつ稼働中で、dev ポート帯に ESTABLISHED 接続が TTL（既定30分）以上無い worktree を自動 down（手動 serve は対象外、#92/#93）。ただし dev 設定に `headless=true` のサービス（ポート listen しない worker/scheduler）を **1つでも含む worktree は除外**する（#129。接続数ベース活動判定で常駐バックグラウンド処理を誤停止しないため）
7. **幽霊ポート reaper**: 削除済み worktree の残骸（port_base だけ残る registry エントリ）を定期 prune（既定 1 日 1 回、`autostart.PortReaper`）。起動時と定期スケジュールで実行
8. **HealthReaper（クラッシュ検知＆自動復帰、#137）**: `devserver.Recorded` が非空（serve 済みで一度も明示的に `Down` されていない）にもかかわらず `devserver.IsRunning == false`（全サービス死亡）な worktree を「クラッシュ」とみなし、`devserver.Serve` で再起動する。`Reaper`（idle 判定で `Down` のみ）とは責務が逆（起動を試みる側）のため別 struct として実装。cooldown ウィンドウ内（既定10分）で試行回数が上限（既定3回）に達すると以降そのウィンドウでは再起動を試みず `slog.Warn` のみ出す（壊れた dev 設定が systemd scope を無限に生成し続ける新たな障害増幅器にならないためのガード）。復帰に成功した worktree は試行カウンタをリセットする。degraded（部分死）は対象外、全滅のみ対象

## 内蔵 proxy（#125）

`wt web` は起動時に built-in reverse proxy を **既定で同時起動**する（`cmd/web.go`）。`<label>.<repo>.wt.localhost:<port>` 形式で各 worktree の domain-exposed dev サービスに名前でアクセスできる。

- 実装は `internal/handler/proxy.go` の `proxyController`。`wt web` プロセス内で goroutine として動作
- 設定は `settings.toml` の `[proxy]` セクション（`enabled`, `port`）と、CLI フラグ（`--proxy-port` / `--no-proxy`）で行う。CLI フラグは settings を上書きする
- 起動失敗（ポート衝突等）は warn ログとして出力し、`wt web` 本体（:8090）は起動を続行する
- 既存の `wt proxy` 独立コマンドは維持（別プロセスで proxy だけ動かしたい場合や、内蔵をオフにしたい場合に使う）
- Web UI（`SettingsPage`）から `POST /api/proxy/start|stop` で稼働状態を切り替えられる。この操作は現行 wt web セッション内でのみ有効（`settings.toml` を書き換えないため永続化しない）

## ピン留めと自動起動の分離（#103）

worktree 一覧の先頭固定（ピン留め）と `wt web` 起動時の自動 serve を独立したフラグで制御する。

| フラグ | 役割 | 格納先 |
|---|---|---|
| `pinned` | 一覧を先頭固定（UI 表示順のみ） | `.worktrees.json::pinned` |
| `auto_start` | `wt web` 起動時に自動 serve、idle reaper の対象 | `.worktrees.json::auto_start` |

- worktree 詳細パネル（`WorktreeDetailPanel`）で「自動起動 ON/OFF」スイッチで個別トグル
- API: `PUT /api/trees/{repo}/{wt}/autostart`（body: `{"auto_start": bool}`）
- 起動時に `autostart.ServeAutoStart` が `auto_start=true` の全 worktree を確認し、未稼働なら serve（既稼働は再起動しない）
- idle reaper は `auto_start=true` の worktree のみ対象。加えて `headless=true` サービスを含む worktree はスキップ（#129）

## repo 単位の表示/非表示（#104）

repos ページで repo ごとに表示/非表示をトグル。非表示 repo 配下の worktree は worktree 一覧から除外。

- 非表示状態は `.worktrees.json` の `_config.hidden（core.EntryConfig.Hidden）` にサーバ側で永続化（ローカル設定のため `.gitignore` で除外）
- ListRepos レスポンスに `hidden` フィールド追加
- ListTrees は `hidden=true` の repo を事前に除外してから worktree を返す
- API: `PUT /api/repos/{name}/hidden`（body: `{"hidden": bool}`）

## Web API（`internal/handler/handler.go`）

- `GET/POST/DELETE /api/trees`、`/api/trees/{repo}/{wt}/update|pin|autostart`、`/api/trees/gc|merged-prs|issue-details`
  - `/api/trees/{repo}/{wt}/autostart`: `PUT` で worktree 単位の AutoStart フラグ設定（`wt web` 起動時の自動 serve と idle reaper の対象）
- `GET /api/repos`、`PUT /api/repos/{name}/hidden`
  - `/api/repos/{name}/hidden`: repo 単位の表示/非表示をトグル。非表示 repo 配下の worktree は ListTrees から除外
  - ListRepos レスポンスに `hidden` フィールド追加
- `GET /api/ports`（worktree 別の稼働/縮退/ドメイン）、`/api/ports/listeners|stale`、`POST /api/ports/prune`、`/api/ports/{repo}/{wt}/serve|down|devconfig|logs`
- `GET /api/process-stats`（後述）
- `GET/POST /api/proxy`、`GET/PUT /api/settings`
- `GET /api/build-info`（後述: バイナリ鮮度表示）

## バイナリ鮮度表示（#133）

`wt web` は systemd unit として常駐する（ホットリロード無し）。ソース repo に新しい commit が入っても再ビルド・差し替え・再起動しない限り古いコードで動き続けるため、画面ヘッダに「⚠ バイナリ古い可能性」バッジを出して気づけるようにする。

- **判定**: `HEAD の commit 時刻 > バイナリ起動時刻` なら stale。動的ビルド（`-tags dev` / air）と source repo path が取れないケースはバッジを出さない（fail-closed）
- **backend** (`internal/buildinfo/`, `internal/handler/buildinfo.go`):
  - `buildinfo.Commit` / `.CommitTime` / `.SourceRepo` は Makefile の `ldflags` で埋め込む（`make build` 経由）。`buildinfo.StartTime` は `init()` で `time.Now()`
  - `buildinfo.IsDev` は build tag（`mode_dev.go` / `mode_prod.go`）で切替
  - `GET /api/build-info` が build 情報 + `git -C <SourceRepo> log -1 HEAD` の結果 + `is_stale` を返す。git 呼び出しは 5s の TTL キャッシュ、2s のタイムアウト
- **frontend** (`Layout.tsx` の `StaleBinaryBadge`): 60s 間隔で fetch、`is_dev=false && is_stale=true` の時だけ黄色バッジ + hover で build/head commit 情報を表示

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

### ポート稼働状態の判定（#116）

`GET /api/ports` のレスポンス（`internal/handler/ports.go::portItem`）は「稼働」と「wt 管理下か」を分けて返す:

- `running` = **`alive > 0 || anyListening`** — `alive` は `devserver.RunStatus`（`running.json` 記録の生存 PID 数）、`anyListening` は `ports.Status` が `ss` から拾った LISTEN の有無。**LISTEN があれば `wt serve` 経由でなくても「稼働中」扱い**にして CLI (`internal/wt/ports/list.go::liveCell` の `Listening || Running`) と揃える
- `unmanaged` = `alive == 0 && anyListening` — LISTEN しているが wt の `running.json` に記録が無い状態（外部から手動起動された dev サーバ等）。wt 側に PID を握っていないので **`down` できない**
- `degraded` = `alive > 0 && alive < total || any unhealthy` — wt 記録の一部だけが死んでいる「縮退稼働」。外部 LISTEN だけで `running=true` になった状態を縮退と誤解しないように `alive > 0` でガードする
- UI（`WorktreeCard.tsx` / `TreesPage.tsx` / `WorktreeDetailPanel.tsx`）は `port.running` で「稼働/停止」を出し、`port.unmanaged` のとき「wt管理外」バッジを並置。詳細パネルの起動/停止トグルは `unmanaged` のとき `disabled`（wt からは停止できない旨のツールチップ）

### テーブル UI 刷新（#106）

`TreesPage.tsx` のテーブル レイアウト改善:

- **sticky header**: `TableHeader className="sticky top-0 z-10 bg-background shadow-[0_1px_2px_rgba(0,0,0,0.1)]"` で行リストのみ縦スクロール可能に
- **wrapperClassName**: `Table` コンポーネントに `max-h-[calc(100vh-250px)]` を指定し、ビューポート内での高さ制限
- **列トグルドロップダウン**: 「表示列 ▾」ドロップダウンに Issue / PR 列の表示/非表示を集約
- **表示変更**: tmux 列・親 issue 列は UI から削除（tmux 概念は #127 でバックエンド/CLI からも完全撤去）

## wt tree gc の3群フラグ（#127）

`wt tree gc` のフラグは意味論で3群に整列されている（`cmd/tree.go` の `treeGcCmd` と `internal/wt/gc/gc.go` の `Options`）:

- **Filter**（複数指定は AND）: `--done`（PR merged/closed または issue closed）/ `--merged`（`--done` の後方互換 alias）/ `--older-than=STR`（30d / 24h）
- **Retention**: `--keep-branch`（ブランチを残す）
- **Safety**: `--dry-run`（列挙のみ）/ `-y, --yes`（確認省略）/ `--force`（dirty も対象）

`--merged` / `--closed` / `--include-dirty` の分離は撤廃され、`--done` と `--force` に統合された。tmux セッションの管理（`--no-tmux` / `--keep-tmux` / `killTmuxSession`）は撤去されており、`wt tree rm` / `wt tree gc` は tmux を一切参照しない。

## どこを触れば何が変わるか

| 変えたいこと | 触る場所 |
|---|---|
| dev サービスの起動・記録の仕組み | `internal/wt/devserver/run.go` |
| ポート帯・割当ロジック | `internal/wt/ports/` + `settings.toml [dev_ports]` |
| AutoStart 自動 serve / idle 停止の方針 | `internal/wt/autostart/` + `[idle_reaper]` |
| 幽霊ポート自動 prune | `internal/wt/autostart/port_reaper.go` + `[port_reaper]` |
| クラッシュ検知・自動復帰・リトライ上限 | `internal/wt/autostart/health_reaper.go` + `[health_reaper]` |
| devserver Down/Serve 呼び出しの監査ログ（trigger 種別） | `internal/handler/devserver.go` / `internal/handler/devrestart.go` / `internal/wt/autostart/reaper.go` / `internal/wt/autostart/autostart.go` / `internal/wt/autostart/health_reaper.go` / `cmd/serve.go`（`slog.Info("devserver action", ...)`） |
| worktree の AutoStart フラグ（起動時自動 serve）| `.worktrees.json::auto_start` + `internal/handler/trees.go::SetTreeAutoStart` |
| repo 単位の表示/非表示 | `.worktrees.json::_config.hidden（core.EntryConfig.Hidden）` + `internal/handler/repos.go::SetRepoHidden` |
| メモリ計測・危険判定 | `internal/wt/procstats/` + `internal/handler/stats.go` + `[process_stats]` |
| テーブル header sticky 化・列トグル | `frontend/src/pages/TreesPage.tsx` の `Table` コンポーネント + `wrapperClassName` |
| worktree 一覧の非表示フィルタ | `frontend/src/pages/TreesPage.tsx::ListTrees` の hidden repos チェック |
| API の追加 | `internal/handler/` + `frontend/src/api/` |
| ポート稼働/管理外/縮退の判定 | `internal/handler/ports.go::ListPorts`（`running` / `unmanaged` / `degraded` 導出）+ `frontend/src/api/ports.ts::PortItem` + `WorktreeCard.tsx` / `TreesPage.tsx` / `WorktreeDetailPanel.tsx` の「稼働」列 |

## 検証ゲート

- `make ci` = golangci-lint + go mod diff-check + `go test -cover ./...`
- CI はさらに `cd frontend && npm ci && npm run build`（tsc 込み）を回す — push 前に `make build` も緑にすること
- フロントテストは `cd frontend && npx vitest run`（CI 対象外だがローカルで回す）
- ローカルで `TestProxyController_StartStop` が落ちる場合は稼働中 proxy の :8088 占有が原因（環境起因）
