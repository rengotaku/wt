# wt — worktree manager

`wt` は git worktree をコンテナ構造で管理する CLI ツールです。

## 主要コマンド

コマンド構成を「名詞 → 動詞」に移行中です（#188）。見る系は次のコマンドを使います。

```
wt repo ls                          リポジトリごとに 1 行の要約（worktree 数・状態ごとの件数・AutoStart 数）
wt repo show <repo>                 1 リポジトリの設定
wt worktree ls [<repo>] [--all]     worktree の一覧（省略時は今いるリポジトリ）
wt worktree show [<repo>/<wt>]      1 worktree の詳細（省略時は今いる worktree）
wt daemon status                    wt daemon が動いているか・PID・proxy の待ち受け先
```

変える系:

```
wt worktree add <repo> --issue <URL> | --branch <B>   worktree を作る
wt worktree rm <repo>/<wt> [--dry-run] [--yes]        worktree を消す
wt worktree gc --done | --older-than 30d [--yes]      条件に合う worktree をまとめて消す
wt worktree set <repo>/<wt> autostart=on|off          worktree 単位の設定
wt worktree update [<repo>/<wt>]                      ブランチの最新を取り込む
wt service up [<repo>/<wt>] / down [...]              dev サービスの起動・停止
wt service logs [<repo>/<wt>] [<service>] [-n 100]    dev サービスのログ
wt daemon run [--proxy-only]                          常駐（--proxy-only で proxy だけ）

wt repo add <url> [<container_base>]                   clone してコンテナにする
wt repo init <src-dir> [<target-dir>]                  既存の git リポジトリをコンテナの形に移す
wt repo rm <repo> [--dry-run] [--yes]                  リポジトリのコンテナを丸ごと消す
wt repo sync [<repo>]                                  main / master を pull --ff-only（表で結果を出す）
wt repo config set <repo> branch_pattern <正規表現>     ブランチ名の制約
wt repo config add|rm <repo> symlink <パス>            worktree 作成時の symlink 候補
wt repo config add <repo> service <名前> --cmd <cmd> [--domain] [--headless]
wt repo config rm <repo> symlink|service <値> | --all  dev サービス・symlink 候補を消す

wt doctor [<repo>] [--fix]                             不整合を見つける（--fix で直す）
```

変える系は進捗を stderr に、結果のリソースを `worktree show`（リポジトリなら `repo show`）と同じ形で stdout に出します。`worktree gc` は消した・残した worktree とブランチを REPO / WORKTREE / BRANCH / RESULT / DETAIL の表で出します（RESULT は deleted / skipped / candidate / failed）。消す系は端末なら確認を出し、`--yes` で省きます。端末でなく `--yes` も無いと終了コード 4 で止まります。
旧コマンド（`tree add` / `rm` / `gc` / `ls` / `autostart` / `fix`、`serve`、`down`、`proxy`、引数なしの `daemon`、`symlink`、`dev`、`ports alloc` / `prune`、`repo rescue-main`）は今も動き、使うと stderr に新しい名前を出します。

`wt doctor` が見るのは、main / master のフォルダに別のブランチが checkout されている（`main_branch`）、ディレクトリが無いのにポートを持つ登録（`stale_port`）、main 以外でポートが無い worktree（`unallocated_port`）、dev サービスの設定の誤り（`service_config`）、読めない `.worktrees.json`（`metadata`）です。直せないものが残ると終了コード 4 です。

出力は既定がタブ区切り（1 行 1 件、空は `-`）、`--json` で JSON、`--rich` で人向けの表です。
終了コード: 0 成功 / 1 予期しないエラー / 2 使い方の誤り / 3 対象が無い / 4 前提を満たさない / 5 外部コマンドの失敗。
`--json` のとき、エラーは `{"error":{"code":"not_found","message":"..."}}` の形で stderr に出ます。

以下は移行前からのコマンドです。

```
wt tree add <repo> <issue>  worktree を作成
wt tree ls                  worktree 一覧
wt status [--json|--rich]   worktree の稼働・ポート・AutoStart・proxy URL と wt daemon の状態（既定は AI 向けのタブ区切り）
wt tree rm                  worktree を削除
wt tree gc                  マージ済み worktree を一括削除
wt tree autostart <repo> <worktree> on|off  wt daemon 起動時に serve する対象を切り替え

wt serve                    .wt/dev.toml のサーバーを割当ポートで起動
wt down                     wt serve で起動したサーバーを停止
wt daemon                   内蔵 proxy・AutoStart・各 reaper を常駐実行（HTTP API は持たない）
wt proxy                    リバースプロキシだけを単独で起動

wt policy show              リポジトリの方針を表示（未設定なら無出力）
wt policy path              方針ファイルの絶対パス
wt policy init              方針ファイルのテンプレートを作成
```

## dev サービス設定（`wt repo config`）

worktree の dev サーバ（`wt service up` / `wt daemon` の AutoStart で使う）を CLI から設定できます。
`wt repo config add|rm <repo> service ...` は、リポジトリの
**repo 既定 dev サービス**（`<container>/.worktrees.json` の `_config.dev_services`）を編集します。
ここで定義したサービスは、専用の上書きを持たない全 worktree に継承されます
（メタデータ管理のためリポジトリにはコミットされません）。

```bash
# 例: API と Vite フロントを定義
wt repo config add myrepo service api --cmd 'go run . web -p ${port}'
wt repo config add myrepo service web --cmd 'npm run dev -- --port ${port} --strictPort' --domain
wt repo show myrepo    # service.<名前>.cmd / domain / headless の行で確かめる
wt doctor myrepo       # 設定の誤りは service_config として出る
```

### スキーマ

- 各 service は宣言順に割当ブロックの `base+i` ポートを受け取り、`cmd` 内の `${port}` に置換されます。
- 全 service のポートは `WT_PORT_<NAME>`（例: `WT_PORT_API`）環境変数で相互参照できます。
- `--domain` を付けた service は `wt proxy` 経由の名前アクセス対象になります。

### 設定レイヤと優先順位

dev 設定は次の優先順で実効化されます（旧 `wt dev show` が実効ソースと警告を表示）:

1. **worktree 上書き**（その worktree 専用。`.worktrees.json` の `dev_services`。CLI からは編集しない）
2. **repo 既定**（`_config.dev_services` ＝ `wt repo config` が編集する層）
3. **コミット済み `.wt/dev.toml`**（リポジトリにコミットされたファイル）

上位レイヤがあると下位は使われません（例: worktree 上書きがあると repo 既定は
その worktree には反映されません）。`wt dev show` はこの状況を警告します。

## リポジトリごとの AI 作業方針（`wt policy`）

リポジトリごとの AI 作業方針を、リポジトリの外（コンテナ）に置きます。

- 保存先: `_config.ai_policy`（`.worktrees.json`、既定は `.wt/ai-policy.md`）
- wt は保存と解決だけを担当します。`CLAUDE.md` / `AGENTS.md` は生成も読み書きもしません
- 読み込みは利用ユーザのグローバル方針が `wt policy show` を呼びます

```bash
wt policy init          # .wt/ai-policy.md のテンプレートを作成
wt policy show          # 本文を表示（未設定なら無出力・exit 0）
wt policy show --repo myrepo
wt policy path          # 絶対パス（未設定なら exit 1）
```

`_config.ai_policy` はコンテナからの相対パスです。`../` などコンテナの外を指す指定は拒否します。

## 常駐（`wt daemon`）

`wt daemon` は次をまとめて常駐実行します。LaunchAgent などから起動する想定です。

- 内蔵 proxy（`<label>.<repo>.wt.localhost` → 各 worktree の dev サーバー。`[proxy]`）
- AutoStart の worktree を起動時に `serve`（`wt tree autostart` で設定）
- idle / port / health の各 reaper（`[idle_reaper]` `[port_reaper]` `[health_reaper]`）

設定はすべて `settings.toml` です。場所は `$WT_CONFIG_DIR/wt/settings.toml`、未指定ならユーザー設定ディレクトリ配下
（macOS: `~/Library/Application Support/wt/settings.toml`、Linux: `~/.config/wt/settings.toml`）。
proxy の既定 bind は `127.0.0.1` です。LAN の別端末から開くときだけ `[proxy]` に `bind = "0.0.0.0"` を書きます（起動中の worktree 一覧と、loopback だけで待ち受ける dev サーバーまで LAN に出ます。信頼できるネットワークでだけ使ってください）。

同時に動かせる `wt daemon` は 1 つだけです（2 つ目は lock でエラー終了）。Web UI と HTTP API は廃止しました（#178）。

旧 `wt web` を常駐させていた環境は、LaunchAgent / systemd unit の起動引数を `wt daemon` に替えて再読み込みします
（`-p` / `--port` は無くなりました）。旧プロセスが残っていると proxy のポートが空かず、`wt daemon` はエラー終了します。

## アイドル停止 (idle reaper)

`wt daemon` は、AutoStart で自動 serve された dev サービスが一定時間アイドル状態（ブラウザ等から dev ポートへの通信がない状態）になった場合、自動で `down` しメモリを解放します。

- 既定の挙動は `enabled=true`、TTL は 30分、監視間隔は 2分です（`settings.toml` の `[idle_reaper]` セクションで設定可能）。
- **手動で serve した worktree は対象外**です（AutoStart の worktree のみが自動 down します）。
- **注意**: vite の HMR websocket のようにブラウザタブが開きっぱなしでコネクションが維持されている場合や、兄弟サービス間での通信（ポート間の ESTABLISHED 接続）がある場合は「活動中」と見なされ、停止しません。

## git-crypt 対応

### 概要

`wt tree add` は **git-crypt を使っているリポジトリを自動検出**し、worktree 作成後に `git-crypt unlock` を試みます。

- main worktree が unlock 済みでも、新規 worktree では smudge filter が鍵を解決できず checkout が失敗するケースがあります (git-crypt #211 系の挙動)。
- `wt` は worktree 作成直後に自動で unlock することで、ユーザーが毎回手動 unlock しなくても済むようにします。

### 検出ロジック

以下のいずれかが true のとき、git-crypt 対象 repo と判定します:

1. `.gitattributes` に `filter=git-crypt` が含まれる
2. `git config filter.git-crypt.smudge` が設定されている

### 鍵探索の優先順位

1. **コンテナ registry** — `.worktrees.json` の `_config.git_crypt_key` で登録された鍵
   (`wt repo add --git-crypt-key <path>` で設定)
2. **repo ローカル git config** — `git config wt.gitCryptKey`
3. **ホームディレクトリのデフォルト** — `~/.git-crypt-key`

### smudge エラーからのリカバリ

`git worktree add` が smudge filter エラーで失敗した場合:

1. `--no-checkout` で worktree のみ作成
2. 上記の優先順位で鍵を探索
3. 鍵が見つかれば `git-crypt unlock <keyfile>` を実行
4. `git reset --hard HEAD` でファイルを展開

鍵が見つからない場合は **warning を出して続行**します (worktree は作成済み)。

### ログメッセージ

| 状況 | メッセージ |
|------|----------|
| unlock 開始 | `🔐 git-crypt 対象 repo を検出。~/.git-crypt-key で unlock 中...` |
| unlock 成功 | `✅ git-crypt unlock 成功` |
| 鍵が見つからない | `⚠️  git-crypt 対象だが鍵が見つかりません。手動で unlock してください: git-crypt unlock <key>` |

### wt repo add --git-crypt-key

clone 時に鍵パスを registry に登録します:

```bash
wt repo add https://github.com/owner/repo --git-crypt-key ~/.git-crypt-key
```

既存リポジトリへの設定は `git config wt.gitCryptKey <path>` で行います。

## コンテナ構造

```
~/Workspace/
  <repo>/                     ← コンテナ
    .worktrees.json            ← worktree メタデータ
    main/                      ← main worktree
    <repo>--feat-issue-N-XXXX/ ← feature worktree
```
