# ADR 0001: wt Web UI 運用性改善（ピン/自動起動分離・repo 非表示・幽霊ポート prune・一覧刷新）

- Status: Accepted
- Date: 2026-07-10
- Epic: #102（sub: #103 / #104 / #105 / #106）

## 背景

wt Web UI の worktree 一覧まわりで運用ノイズが溜まっていた。ピンが「先頭固定」と「起動時 auto-serve」を暗黙に兼ねていて分離できない、使い終わった repo を一覧から隠せない、削除済み worktree の幽霊ポートが残る、一覧テーブルの列・見出しが過剰、といった課題を一括で解消する。

## 決定

### 1. ピンと自動起動を独立フラグに分離（#103）

- `.worktrees.json` の worktree エントリに `pinned` と `auto_start` を**独立フラグ**として持つ。`pinned` は一覧先頭固定のみ、`auto_start` は `wt web` 起動時の auto-serve のみを担う。
- 自動起動のトグルは詳細パネル（`WorktreeDetailPanel`）に置き、`PUT /api/trees/{repo}/{wt}/autostart` で永続化する。
- **既存 pin の auto_start は引き継がない**（全 OFF スタート、詳細画面で個別 ON）。暗黙結合を断つのが目的なので、移行で結合を再現しない。

### 2. 非表示は repo 単位、`_config` に永続（#104）

- 非表示の粒度は **repo 単位**（worktree 単位は入れない）。repos ページでトグルし、hidden repo 配下の worktree を `ListTrees` から除外する。
- hidden 状態は `.worktrees.json` の `_config`（`core.EntryConfig.Hidden`）に置く。global `settings.toml` ではなく repo コンテナローカルの `_config` にするのは、hidden が **per-repo かつ非コミット（git 管理外）のローカル表示設定**だから。表示制御のみで worktree 実体・エントリ・稼働 dev サーバには触れない。

### 3. 幽霊ポートは定期 prune（#105）

- 削除済み worktree の残骸（port_base だけ残る registry エントリ）を定期 prune。設定は `settings.PortReaper`（既定 interval=1日）。掃除はバックグラウンドで自動化し、手動 `wt ports prune` も残す。

### 4. 一覧テーブルはヘッダ固定＋列を絞る（#106）

- テーブル thead を sticky にし行リストのみ縦スクロール（`Table` に optional `wrapperClassName` を追加し後方互換を保つ）。ページ見出しを削除、ツールバーを1行化、列トグルを「表示列 ▾」ドロップダウンに集約。
- tmux 列・親 issue 列は**フロント表示のみ撤去**。`has_tmux`/issueDetail のバックエンド配線は他で使う可能性を考慮して残す。

## 変えてよい前提 / 壊すと危ない前提

- **変えてよい**: 列の既定表示・ドロップダウンの中身・sticky の max-height 値・PortReaper の interval 既定値。
- **壊すと危ない**:
  - `pinned` と `auto_start` の**独立性**（片方のトグルで他方を変えない）。両方向のテストがある（`SetTreePin_DoesNotTouchAutoStart` / `SetTreeAutoStart_DoesNotTouchPinned`）。
  - hidden を `_config`（非コミット）に置く前提。コミット対象の設定に移すと git に個人の表示設定が漏れる。
  - `Table` の `wrapperClassName` は **optional**。必須化すると他画面（Ports 等）の既存呼び出しが壊れる。
  - 列削除はフロント表示のみ。`has_tmux`/issueDetail のバックエンド取得を消すと、将来の再表示や他利用が壊れる。

## 検討して捨てた案

- hidden を global `settings.toml` の `hidden_repos: []` で持つ案 → per-repo 情報を global にフラット化すると repo 追加/削除との整合が面倒。`_config` に置く方が repo ライフサイクルと一致するため却下。
- ピン移行時に既存 pin へ auto_start を引き継ぐ案 → 暗黙結合の再現になり目的に反するため却下。
