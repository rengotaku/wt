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
```

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
