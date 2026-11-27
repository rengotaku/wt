# wt アーキテクチャ

git worktree 管理 CLI。単一 Go バイナリで動く。Web UI と HTTP API は持たない（#178、ADR 0004）。

出力の方針: 操作するのは主に AI なので、既定の出力は機械が読みやすい形（色・罫線なし、列固定）にし、人向けの見た目は `--rich` などのオプションで出す。

## 全体構成

```
wt (cobra CLI。名詞 → 動詞。#188)
├── repo      ls [--fresh] / show / add / init / rm / sync / config（set・add・rm）   リポジトリ
├── worktree  ls / show / add / rm / gc / set / update                    worktree
├── service   up / down / logs                                            worktree の dev サービス
├── daemon    run [--proxy-only] / status                                 常駐: 内蔵 proxy + AutoStart 自動起動 + idle / port / health reaper
├── policy    show / path / init                                          リポジトリごとの AI 作業方針の解決
└── doctor    [--fix]                                                     リポジトリ・worktree・ポートをまたぐ不整合の検出と修復
```

コマンドは `cmd/views.go`（見る）、`cmd/actions.go`（worktree・service を変える）、`cmd/repo.go` / `cmd/repo_config.go`（リポジトリ）、`cmd/daemon.go`、`cmd/doctor.go`、`cmd/policy.go` に分かれる。

### ディレクトリマップ

| パス | 役割 |
|---|---|
| `cmd/` | cobra コマンド定義（`daemon.go` が内蔵 proxy + AutoStart 自動 serve + 各 reaper の配線） |
| `internal/wt/core/` | コンテナ（`<base>/<repo>/`）と `.worktrees.json` の読み書き。リポジトリ名は `ValidateRepoName` で 1 階層の名前に限る（#180） |
| `internal/wt/tree/` | worktree の add / rm（`RemoveAt`）/ update / AutoStart 切替（`SetAutoStart`） |
| `internal/wt/devserver/` | dev サービスの serve / down / logs / 実行状態（`running.json`） |
| `internal/wt/ports/` | ポートブロック割当・LISTEN/ESTABLISHED 検出（Linux は `ss`、無ければ `lsof`。macOS は `lsof`） |
| `internal/wt/repo/` | リポジトリの add / init / rm（`RmAt`）/ sync / main の退避（`RescueMain`。`doctor --fix` が呼ぶ）と設定（`config.go`: branch_pattern・symlink 候補）。`Sync` は結果を返し、終了コードは呼び出し側が決める |
| `internal/wt/doctor/` | `wt doctor` の検査（main_branch / stale_port / unallocated_port / service_config / metadata）と `--fix` の修復。修復は `repo.RescueMain`・`core.DeleteEntry`・`ports.EnsureBase` を呼ぶ |
| `internal/wt/autostart/` | AutoStart 自動 serve（`ServeAutoStart`）/ idle reaper（`Reaper`）/ 幽霊ポート reaper（`PortReaper`）/ クラッシュ検知＆自動復帰 reaper（`HealthReaper`） |
| `internal/cli/out/` | 見る系の出力の共通部品（既定のタブ区切り・`--json`・`--rich`） |
| `internal/cli/clierr/` | エラーの種類と終了コード（0 成功 / 1 予期しない / 2 使い方 / 3 対象が無い / 4 前提を満たさない / 5 外部コマンド）。`main.go` が出力と終了コードを決める |
| `internal/wt/ref/` | `<repo>/<worktree>` の解決。省略時は今いる worktree |
| `internal/wt/view/` | 見る系（`repo ls` / `show`、`worktree ls` / `show`）が出す中身の組み立て |
| `internal/wt/status/` | worktree の稼働・ポート・AutoStart・proxy URL の集計（`Collect` / `Build`）。`worktree ls` の列順 `TSVHeader` |
| `internal/wt/daemon/` | `wt daemon` の二重起動防止の lock（PID を書く）と、動いているかの確認（`Probe`） |
| `internal/wt/listencache/` | LISTEN ソケットの走査結果のキャッシュ（`listeners.json`）の読み書きと無効化（#205） |
| `internal/wt/proxy/` | `<label>.<repo>.wt.localhost` → 割当ポートへの逆プロキシ |
| `internal/wt/settings/` | `settings.toml`（`$WT_CONFIG_DIR/wt/`、未指定ならユーザー設定ディレクトリの `wt/`。macOS は `~/Library/Application Support/wt/`）の読み書き |

### 状態ファイル

| ファイル | 内容 |
|---|---|
| `<base>/<repo>/.worktrees.json` | worktree エントリ（type / created / branch / pinned / auto_start / port_base / `_config.dev_services` 等） |
| `<base>/<repo>/.worktrees.json::_config.hidden` / `pinned` | 廃止した Web UI の表示用フラグ。読み書きの互換のため構造体に残しているだけで、参照する処理は無い |
| `<base>/<repo>/.worktrees.json::_config.branch_pattern（core.EntryConfig.BranchPattern）` | `wt worktree add` が作るブランチ名を制約する正規表現（`wt repo config set <repo> branch_pattern` で設定）。既定 `".*"`（全許可）。完全一致で照合する（`core.ValidateBranchName`） |
| `<base>/<repo>/.worktrees.json::_config.ai_policy（core.EntryConfig.AIPolicy）` | リポジトリの AI 作業方針ファイル（コンテナ相対、既定は `.wt/ai-policy.md`）。`wt policy` が解決する。リポジトリには置かない |
| `<設定ディレクトリ>/wt/settings.toml` | `[dev_ports]`（既定 9000-9999）/ `[idle_reaper]` / `[port_reaper]`（既定 1440分=1日）/ `[health_reaper]`（既定 interval=2分 / cooldown=10分 / max_retries=3）/ `[process_stats]` / `[proxy]`（`enabled` 既定 true / `port` 既定 8088） |
| `<キャッシュディレクトリ>/wt/daemon.lock` | `wt daemon` の二重起動を防ぐ flock。中身は daemon の PID と proxy の待ち受け先の JSON（`wt daemon status` が読む） |
| `<キャッシュディレクトリ>/wt/listeners.json` | dev ポート帯の LISTEN ソケットを最後に調べた結果（取得時刻・帯・port / pid / proc）。`wt repo ls` が 5 分以内なら使い、`lsof` を呼ばない（#205）。`ports.Status` が調べるたびと `wt daemon` が 1 分ごとに書き、`devserver.Serve` / `Down` が消す |
| `<キャッシュディレクトリ>/wt/run/<worktree_key>/running.json` | serve 済みサービスの記録（name / pid / port / cmd） |
| `<キャッシュディレクトリ>/wt/run/<worktree_key>/<svc>.log` | サービスごとの stdout+stderr |

## AI 作業方針（`wt policy`）

方針本文はコンテナに置き、リポジトリには置かない。`wt policy show` は未設定なら無出力で exit 0、設定済みでファイルが無ければ exit 1（設定ミスを隠さない）。`wt policy init` はテンプレートを作り `_config.ai_policy` を設定する（既存ファイルは上書きしない）。パスはコンテナ内に制限し、絶対パスと `..` を拒否する（`internal/wt/policy`）。`CLAUDE.md` / `AGENTS.md` は利用ユーザのグローバル方針であり、wt は生成も読み書きもしない。

## dev サービスのライフサイクル

1. **定義**: worktree の `.wt/dev.toml` → repo 既定（`.worktrees.json` の `_config.dev_services`）の優先順で実効化（`devserver.EffectiveConfig`）
2. **ポート割当**: worktree ごとに band から `BlockSize=5` の連続ブロックを確保し `port_base` を `.worktrees.json` に永続化。service i は `base+i` を使う
3. **起動**: `devserver.Serve` が各サービスを `Setpgid` 付きで起動（leader PID = PGID）。systemd が使える環境では `systemd-run --user --scope --slice=wt-dev.slice` でラップし、`wt-dev-<worktree>-<svc>-<suffix>.scope` として **wt の常駐プロセスの cgroup 外**に分離する（#122。unit の stop/restart が dev サービスと headless chrome を巻き添えにしない）。`systemd-run` 不在・`WT_NO_SYSTEMD_RUN=1` の環境では従来どおり `sh -c` 直接 spawn にフォールバック。`PORT` と全サービス分の `WT_PORT_<NAME>` を環境変数で共有。600ms の起動グレース後に生存確認し、`running.json` へ記録。稼働 scope は `systemctl --user list-units 'wt-dev-*'` で列挙できる。**`IsRunning` は `running.json` の PID プロセスグループが空でも `wt-dev-<worktree>-*.scope` が active なら true を返す**（python worker 等が setsid で detach するとプロセス group プローブでは死んで見えるため、systemd の scope 有無を fallback にする。#131）
4. **停止**: `devserver.Down` がプロセスグループごと SIGTERM（カレント worktree の記録済みサービスのみが対象。wt daemon・他 worktree には影響しない。scope は全メンバー終了で自動消滅）
5. **AutoStart 自動 serve**: `wt daemon` 起動時に `auto_start=true` かつ未稼働の worktree を `autostart.ServeAutoStart` が serve。既に稼働中の worktree は再起動しない
6. **idle reaper**: `auto_start=true` かつ稼働中で、dev ポート帯に ESTABLISHED 接続が TTL（既定30分）以上無い worktree を自動 down（手動 serve は対象外、#92/#93）。ただし dev 設定に `headless=true` のサービス（ポート listen しない worker/scheduler）を **1つでも含む worktree は除外**する（#129。接続数ベース活動判定で常駐バックグラウンド処理を誤停止しないため）
7. **幽霊ポート reaper**: 削除済み worktree の残骸（port_base だけ残る registry エントリ）を定期 prune（既定 1 日 1 回、`autostart.PortReaper`）。起動時と定期スケジュールで実行
8. **HealthReaper（クラッシュ検知＆自動復帰、#137）**: `devserver.Recorded` が非空（serve 済みで一度も明示的に `Down` されていない）にもかかわらず `devserver.IsRunning == false`（全サービス死亡）な worktree を「クラッシュ」とみなし、`devserver.Serve` で再起動する。`Reaper`（idle 判定で `Down` のみ）とは責務が逆（起動を試みる側）のため別 struct として実装。cooldown ウィンドウ内（既定10分）で試行回数が上限（既定3回）に達すると以降そのウィンドウでは再起動を試みず `slog.Warn` のみ出す（壊れた dev 設定が systemd scope を無限に生成し続ける新たな障害増幅器にならないためのガード）。復帰に成功した worktree は試行カウンタをリセットする。degraded（部分死）は対象外、全滅のみ対象

## 内蔵 proxy（#125）

`wt daemon` は built-in reverse proxy を **既定で同時起動**する（`cmd/daemon.go::startProxy`）。`<label>.<repo>.wt.localhost:<port>` 形式で各 worktree の domain-exposed dev サービスに名前でアクセスできる。

- 設定は `settings.toml` の `[proxy]`（`enabled`, `port`, `bind`）と、CLI フラグ（`--proxy-port` / `--no-proxy`）。CLI フラグが settings を上書きする
- `bind` の既定は `127.0.0.1`（#180）。`0.0.0.0` にすると LAN から一覧と dev サーバーに届く
- Host が `<label>.<repo>.wt.localhost` でない要求には、Host が IP アドレスか `localhost` のときだけ起動中の worktree 一覧を返し、それ以外は 404（DNS rebinding で一覧を読ませない。#180）
- 起動失敗（ポート衝突等）は warn を出し、daemon の他の処理（AutoStart・reaper）は続行する
- `wt daemon run --proxy-only` は proxy だけを動かす（lock を取らない）

## AutoStart（#103）

`.worktrees.json::auto_start` が true の worktree は、`wt daemon` 起動時に自動 serve され、idle reaper の対象になる。

- 切替は `wt worktree set <repo>/<worktree> autostart=on|off`（`internal/wt/tree/autostart.go::SetAutoStart`）
- 起動時に `autostart.ServeAutoStart` が `auto_start=true` の全 worktree を確認し、未稼働なら serve（既稼働は再起動しない）
- idle reaper は `auto_start=true` の worktree のみ対象。加えて `headless=true` サービスを含む worktree はスキップ（#129）

## wt worktree gc の3群フラグ（#127）

`wt worktree gc` のフラグは意味論で3群に整列されている（`cmd/actions.go` の `worktreeGcCmd` と `internal/wt/gc/gc.go` の `Options`）。結果は REPO / WORKTREE / BRANCH / RESULT / DETAIL の表で stdout に出る（`gc.RunResults`）。削除は走査したパスで行う（`tree.RemoveAt`。同じ名前のリポジトリが両ワークスペースにあっても取り違えない）:

- **Filter**（複数指定は AND）: `--done`（PR merged/closed または issue closed）/ `--older-than=STR`（30d / 24h）
- **Retention**: `--keep-branch`（ブランチを残す）
- **Safety**: `--dry-run`（列挙のみ）/ `-y, --yes`（確認省略）/ `--force`（下記の保護を外して削除）

既定では、どこにも残っていない作業がある候補を消さずにスキップし、理由を 1 行ずつ出す（`--dry-run` でも同じ。`internal/wt/gc/gc.go` の `keepReason`）:

- 未コミットの変更がある / 未追跡のファイルがある
- PR が merged 以外（closed・PR なし・`--older-than` のみ）で、どのリモートにも無いコミットがある（`git rev-list HEAD --not --remotes`）
- `git status` などの判定に失敗した

PR が merged で、HEAD がその PR の head と一致する（または祖先にあたる）ものはリモートに無いコミットを見ない。squash マージ後にリモートのブランチを消すと、コミットは main にもリモートにも残らないため。マージ後にコミットを足した worktree は例外にしない。`git status` は `--untracked-files=normal` を付け、`status.showUntrackedFiles=no` の設定でも未追跡を数える。

`--done` 指定時は、worktree に紐づかないローカルブランチ（`git for-each-ref refs/heads`。main/master といずれかの worktree でチェックアウト中のものは除く）も同じ `closedCandidate` 判定で候補にし、別見出し「worktree の無いブランチ」で表示して `git branch -D` で削除する（`internal/wt/gc/branches.go`）。ブランチ走査の対象リポジトリは worktree 候補（`tree.RmEntries`）とは独立に `listRepos` で列挙するため、main だけが残ったリポジトリや削除可能な worktree が 0 件の状態でも走査される（`--done` 無しで 0 件なら従来どおり「削除可能なworktreeがありません」エラー）。ブランチ先端に merged PR の head にもリモートにも無いコミットがあればスキップし、`--force` で強行する。`--older-than` 単独・`--keep-branch` 指定時は対象にしない。

`--merged` / `--closed` / `--include-dirty` の分離は撤廃され、`--done` と `--force` に統合された。tmux セッションの管理（`--no-tmux` / `--keep-tmux` / `killTmuxSession`）は撤去されており、`wt worktree rm` / `wt worktree gc` は tmux を一切参照しない。

## どこを触れば何が変わるか

| 変えたいこと | 触る場所 |
|---|---|
| dev サービスの起動・記録の仕組み | `internal/wt/devserver/run.go` |
| ポート帯・割当ロジック | `internal/wt/ports/` + `settings.toml [dev_ports]` |
| AutoStart 自動 serve / idle 停止の方針 | `internal/wt/autostart/` + `[idle_reaper]` |
| 幽霊ポート自動 prune | `internal/wt/autostart/port_reaper.go` + `[port_reaper]` |
| クラッシュ検知・自動復帰・リトライ上限 | `internal/wt/autostart/health_reaper.go` + `[health_reaper]` |
| devserver Down/Serve 呼び出しの監査ログ（trigger 種別） | `internal/wt/autostart/reaper.go` / `internal/wt/autostart/autostart.go` / `internal/wt/autostart/health_reaper.go` / `cmd/actions.go`（`slog.Info("devserver action", ...)`） |
| worktree の AutoStart フラグ（起動時自動 serve）| `.worktrees.json::auto_start` + `internal/wt/tree/autostart.go` + `cmd/actions.go::worktreeSetCmd` |
| 常駐時に起動する処理 | `cmd/daemon.go::runDaemon` |
| 見る系の出力形式（全コマンド共通） | `internal/cli/out/` |
| 終了コード・エラーの形 | `internal/cli/clierr/` |
| 見る系のコマンド定義 | `cmd/views.go` |
| 変えるコマンドの定義・確認の出し方 | `cmd/actions.go`（`confirm`）、`cmd/repo.go`、`cmd/repo_config.go` |
| 不整合の検査と修復 | `internal/wt/doctor/` + `cmd/doctor.go` |
| `wt repo ls` がキャッシュを使う期限・daemon の更新間隔 | `cmd/views.go::repoLsCacheMaxAge` / `cmd/daemon.go::listenCacheInterval`。キャッシュの中身は `internal/wt/listencache/` |
| `worktree ls` の列・状態の判定 | `internal/wt/status/status.go`（`stateOf`）。列順は `TSVHeader` で、スクリプトが依存してよい |

## 検証ゲート

- `make ci` = golangci-lint + go mod diff-check + `go test -cover ./...`
