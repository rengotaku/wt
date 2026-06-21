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
