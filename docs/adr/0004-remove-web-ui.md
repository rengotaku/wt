---
adr: 0004
title: wt web（Web UI と HTTP API）を廃止し、常駐処理を wt daemon に移す
status: accepted
superseded_by: null
date: 2026-11-26
issues: [178]
tags: [web, daemon, security, csrf, dns-rebinding, autostart]
description: 認証の無い HTTP API が外部サイトから呼べたため Web UI ごと廃止し、proxy・AutoStart・reaper は API の無い wt daemon で常駐させる
---

# ADR 0004: wt web（Web UI と HTTP API）を廃止し、常駐処理を wt daemon に移す

## 背景

#178 の点検で、`wt web` の JSON API（127.0.0.1:8090）が認証・Host 検査・Origin 検査・Content-Type 検査のいずれも持たないことが分かった。外部サイトから CSRF（`text/plain` の POST）や DNS rebinding で呼べ、`PUT devconfig` → `POST serve` や「任意リポジトリの clone → serve」で任意コマンド実行に至る経路があった。フロントエンドの依存にも既知の脆弱性が 23 件あった。

一方、wt の操作は AI エージェントが CLI で行っており、人が Web UI を操作する運用ではなくなっていた。

`wt web` は UI 以外に、内蔵 proxy・AutoStart の自動 serve・idle / port / health の各 reaper を同じプロセスで常駐させていた（LaunchAgent / systemd unit から起動）。

## 決定

- `wt web`・`internal/handler`・`internal/static`・`frontend/`・`internal/buildinfo`・`internal/wt/procstats` を削除する
- 常駐処理は新コマンド `wt daemon` に移す。中身は旧 `cmd/web.go` から HTTP サーバーを抜いたもの（内蔵 proxy・`ServeAutoStart`・3 種類の reaper）。SIGINT / SIGTERM で終了する
- `wt daemon` は lock ファイル（ユーザーキャッシュディレクトリの `wt/daemon.lock`）で 1 プロセスに限る。旧 `wt web` は API ポートの bind 失敗で 2 つ目が終了していたが、HTTP を持たない daemon にはその歯止めが無いため。proxy の bind に失敗したら daemon はエラー終了し、サービスマネージャーの再起動に任せる
- Web UI でしか操作できなかったもののうち、AutoStart の切替だけ `wt tree autostart <repo> <worktree> on|off` として CLI に移す
- それ以外（pin・repo の非表示・settings の編集・ログ閲覧・メモリ表示・マージ済み PR 表示・worktree 単位の dev 設定上書き）は CLI に移さない。settings は `settings.toml` を、ログはユーザーキャッシュディレクトリの `wt/run/<key>/<svc>.log` を直接読む。`settings.toml` の `[process_stats]` は読み捨てる（未知キーとして無視される）
- `.worktrees.json` の `pinned` / `_config.hidden` / worktree の `dev_services` は、既存ファイルを書き換えずに読み書きできるよう構造体に残す

## 捨てた案

- **Host / Origin / Content-Type を検査する middleware を足して Web を残す**: #178 の修正案。塞げるが、使っていない UI のために API・フロントエンド依存・ビルド工程を保守し続けることになる
- **全機能を CLI に移す**: 操作するのは AI で、ファイルを直接読めば足りる機能（ログ・settings）に CLI を足しても得るものが無い
- **`wt web` を `wt daemon` の別名として残す**: LaunchAgent の起動引数を変えずに済むが、廃止したはずの名前が残り続ける。起動引数を 1 回直せば済むので残さない

## 変えてよい前提 / 壊すと危ない前提

- **変えてよい**: `wt daemon` で起動する処理の顔ぶれ、`wt tree autostart` の引数形式
- **壊すと危ない**: wt は外部から呼べる待ち受けを持たない。待ち受けは内蔵 proxy だけで、proxy は dev サーバーへの中継しかしない。管理操作（worktree の作成・削除・コマンド実行）をネットワーク越しに受ける口を再び作るなら、認証と Host / Origin 検査を同時に入れる
