---
adr: 0005
title: コマンドを「名詞 → 動詞」に組み直し、既定の出力を機械が読める形にそろえる
status: accepted
superseded_by: null
date: 2026-11-26
issues: [188, 189, 190, 191, 192, 193]
tags: [cli, output, exit-code, doctor, migration]
description: tree / serve / status などの動詞中心のコマンドを repo / worktree / service / daemon / policy の名詞の下に組み直し、出力・終了コード・確認の出し方をすべてのコマンドでそろえた
---

# ADR 0005: コマンドを「名詞 → 動詞」に組み直し、既定の出力を機械が読める形にそろえる

## 背景

wt のコマンドは、機能を足すたびに動詞を増やしてきた（`tree add` / `serve` / `down` / `status` / `ports` / `symlink` / `dev` / `proxy` など）。状態を見るコマンドが `status` 1 つしかなく、リポジトリ単位・worktree 単位で絞れなかった。出力の形もコマンドごとに違い、絵文字付きの進捗と結果が同じ stdout に混ざっていた（`tree add` は `CD:` 行で結果を返していた）。

wt を操作するのは主に AI で、スクリプト（dotfiles の myroutine・hook・zsh 関数）も出力を読んでいる。

## 決定

- 名詞を `repo` / `worktree` / `service` / `daemon` / `policy` の 5 つにし、動詞をその下に置く。データの階層（リポジトリ → worktree → dev サービス）に合わせる。リポジトリ・worktree・ポートをまたぐ修復だけは名詞を持たない `doctor` に置く
- コマンドの構成と振る舞い（サブコマンドの切り方、引数、確認の出し方）は通常の CLI の作法で決める。AI に合わせるのは既定の出力の中身だけ
- 出力の決まり（`internal/cli/out`）: 既定はタブ区切りで 1 行 1 件、空は `-`、色・罫線なし。`--json` で JSON、`--rich` で人向けの表（色は端末のときだけ）
- 状態を変えるコマンドは、進捗と git の出力を stderr に、結果のリソースを `worktree show` / `repo show` と同じ形で stdout に出す。まとめて処理するもの（`worktree gc` / `repo sync` / `doctor`）は結果の表を出す
- 終了コード（`internal/cli/clierr`）: 0 成功 / 1 予期しない / 2 使い方 / 3 対象が無い / 4 前提を満たさない / 5 外部コマンド。`--json` のときはエラーも JSON で stderr に出す
- 消すコマンドは端末なら確認を出し、`--yes` で省く。端末でなく `--yes` も無ければ終了コード 4 で止まる。`--dry-run` で対象だけ出す
- 削除は、確認やスキャンの時点で解決したパスに対して行う（`tree.RemoveAt` / `repo.RmAt`）。名前で引き直すと、同じ名前のリポジトリが両ワークスペースにあるときに別のものを消しうる
- 移行は 5 段で行った。新コマンドを足し、旧コマンドを別名にして stderr で案内し、呼び出し元（dotfiles）を書き換えてから旧コマンドを消した

## 捨てた案

- **動詞中心のまま `status` にオプションを足す**: 絞り込みの軸（リポジトリ・worktree・サービス）が増えるたびにフラグが増え、どのフラグが組み合わせられるかが読めなくなる
- **AI 専用のコマンド（`wt schema` など）やエラーの `retryable` を足す**: AI のためにコマンドの構成を変えない、と決めたので入れない。必要な情報は既定の出力と終了コードで足りる
- **旧コマンドを別名として残し続ける**: 名前が 2 通りあると、文書とスクリプトが古い名前を使い続ける。呼び出し元を書き換え終えた時点で消す

## 変えてよい前提 / 壊すと危ない前提

- **変えてよい**: `--rich` の見た目、stderr に出す進捗の文言、コマンドの追加
- **壊すと危ない**: 既定の出力の列とキーの並び（足すのは末尾に）、`worktree gc` / `repo sync` / `doctor` の表の列と RESULT・STATUS の語、`worktree show --json` の `services[].domain`、終了コードの意味。dotfiles の myroutine と hook がこれらを読んでいる
