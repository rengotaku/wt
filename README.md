# wt — worktree manager

`wt` は git worktree をコンテナ構造で管理する CLI ツールです。

## 主要コマンド

```
wt repo add <url>           リモートリポジトリを clone してコンテナ化
wt repo init <src-dir>      既存 git リポをコンテナ形式に scaffold
wt repo ls                  登録リポジトリ一覧
wt repo sync                全リポの main/master を pull --ff-only
wt repo rm <repo> --force   リポジトリ全体を削除

wt tree add <repo> <issue>  worktree を作成
wt tree ls                  worktree 一覧
wt tree rm                  worktree を削除
wt tree gc                  マージ済み worktree を一括削除

wt serve                    .wt/dev.toml のサーバーを割当ポートで起動
wt down                     wt serve で起動したサーバーを停止

wt dev add <name> --cmd <cmd> [--domain]  dev サービスを追加/更新（repo 既定）
wt dev rm <name>            dev サービスを削除
wt dev show [--json]        実効 dev 設定とソース・警告を表示
wt dev validate             repo 既定 dev 設定を検証
wt dev clear                repo 既定 dev サービスを全削除
```

## dev サービス設定（`wt dev`）

worktree の dev サーバ（`wt serve` / Web の「起動」で使う）を CLI から設定できます。
`wt serve` と同じく **cwd ベース**で、現在の worktree が属するリポジトリの
**repo 既定 dev サービス**（`<container>/.worktrees.json` の `_config.dev_services`）を編集します。
ここで定義したサービスは、専用の上書きを持たない全 worktree に継承されます
（メタデータ管理のためリポジトリにはコミットされません）。

```bash
# 例: API と Vite フロントを定義
wt dev add api --cmd 'go run . web -p ${port}'
wt dev add web --cmd 'npm run dev -- --port ${port} --strictPort' --domain
wt dev show          # 実効設定・ソース・警告（AI は --json で機械可読出力）
wt dev validate
```

### スキーマ

- 各 service は宣言順に割当ブロックの `base+i` ポートを受け取り、`cmd` 内の `${port}` に置換されます。
- 全 service のポートは `WT_PORT_<NAME>`（例: `WT_PORT_API`）環境変数で相互参照できます。
- `--domain` を付けた service は `wt proxy` 経由の名前アクセス対象になります。

### 設定レイヤと優先順位

dev 設定は次の優先順で実効化されます（`wt dev show` が実効ソースと警告を表示）:

1. **worktree 上書き**（その worktree 専用・Web UI で設定）
2. **repo 既定**（`_config.dev_services` ＝ `wt dev` が編集する層）
3. **コミット済み `.wt/dev.toml`**（リポジトリにコミットされたファイル）

上位レイヤがあると下位は使われません（例: worktree 上書きがあると `wt dev` の repo 既定は
その worktree には反映されません）。`wt dev show` はこの状況を警告します。

## プロセス・メモリ使用量の監視

Web UI の worktree 一覧は、dev サービスとして起動したプロセス群の合計メモリ使用量（RSS）を
「状態」列に表示します。dev サービスのメモリ肥大（例: air/node の長時間運用）に一覧上で気付き、
すぐ停止できるようにするための機能です。

- **状態の可視化**: worktree 合計 RSS がしきい値を超えると、状態列が `warn`（アンバー）/
  `danger`（赤・行背景も赤）で強調表示されます。
- **詳細オーバーレイ**: 状態列のメモリ表示をクリックすると、サービスごとの PID・ポート・
  稼働/停止・プロセス数・メモリ・稼働時間の一覧がオーバーレイで確認できます。

### 設定項目 (`settings.toml`)

`~/.config/wt/settings.toml` の `[process_stats]` セクションでしきい値（MB 単位）を変更できます。

```toml
[process_stats]
warn_mb = 2048    # warn（アンバー）になるしきい値。既定: 2048 (2GiB)
danger_mb = 4096  # danger（赤）になるしきい値。既定: 4096 (4GiB)
```

### 注意点

- **Linux 専用**: `/proc/<pid>/stat` を直接読み、サービスのプロセスグループ単位で集計します。
- **RSS は共有ページを重複カウント**するため、実メモリ消費よりやや大きめに出ます（目安として使ってください）。

## アイドル停止 (idle reaper)

`wt web` は、pin されて自動 serve された dev サービスが一定時間アイドル状態（ブラウザ等から dev ポートへの通信がない状態）になった場合、自動で `down` しメモリを解放します。

- 既定の挙動は `enabled=true`、TTL は 30分、監視間隔は 2分です（`~/.config/wt/settings.toml` の `[idle_reaper]` セクションで設定可能）。
- **手動で serve した worktree は対象外**です（pin された worktree のみが自動 down します）。
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
