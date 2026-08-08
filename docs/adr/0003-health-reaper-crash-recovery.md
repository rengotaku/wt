# ADR 0003: dev サービスのクラッシュ検知と自動復帰（HealthReaper）

- Status: Accepted
- Date: 2026-08-08
- Related: #137, #138, #139

## 背景

`wt tree add` 実行後、既存 worktree の dev サービス（api・web）が停止→再起動され、web(vite) が再起動直後に `Terminated` となったまま復帰せず、約2時間45分ユーザーが気づくまで 502 を返し続けたインシデントが発生した（#137）。

コード調査の結果、`wt tree add` から他 worktree の dev サービスを直接停止・再起動するコードパスは見つからず、SIGTERM の直接原因（L2）は特定できなかった。root-cause-layers で対応範囲を判定し、L2 は #138 に切り出し、**L3（監査ログ不足）と L4（クラッシュからの自動復帰機構が無い）に対応する**ことにした。

## 決定

### 1. `devserver.Down`/`Serve` の全呼び出し箇所に trigger 付き監査ログを追加する（L3）

`manual-api` / `pull-restart` / `idle-reaper` / `autostart` / `cli-serve` / `cli-down` / `crash-recovery` の7種類の trigger を `slog.Info("devserver action", "worktree", ..., "action", ..., "trigger", ...)` で記録する。次回同種のインシデントで `journalctl` から原因を即座に特定できるようにする。

### 2. 既存 `Reaper` を拡張せず、新規 `HealthReaper` を追加する（L4）

`Reaper`（idle 判定で `Down` のみ）と `HealthReaper`（クラッシュ検知で `Serve` のみ）はトリガー条件が正反対（無活動 vs 異常停止）であり、1つの struct に混ぜると idle 判定ロジックが読みにくくなるため分離した。

### 3. クラッシュ判定は `Recorded()` 非空 かつ `IsRunning()` 偽 の組み合わせで行う

`devserver.Down()` は成功時に必ず `running.json` を削除する。つまり「記録はあるのに稼働していない」状態は、明示的な `Down` を経由していない＝クラッシュを意味する。全滅（`IsRunning`==false）のみを対象とし、部分死（degraded）はスコープ外（#137 で明示）。

### 4. cooldown + MaxRetries で無限リトライを防ぐ

HealthReaper 自身が「無限に systemd scope を作り続ける新しい増幅器」にならないよう、worktree ごとに直近 `Cooldown`（既定10分）以内 `MaxRetries`（既定3回）までしか再試行しない。上限到達時は `slog.Warn` で giving-up を明示し、以後は放置する。

### 5. codex レビューで判明した2つの false PASS を修正した（実装後レビューで発覚）

- `Serve()` 失敗時に `running.json` が再作成されないため、素朴な実装では次 Tick で「明示停止された」と誤判定し、MaxRetries に達する前に永久にスキップされていた → `r.retries` に追跡中のエントリがあれば `Recorded()` が空でも `recover()` を継続するよう修正
- `IsRunning()`==true を観測した瞬間に無条件で retry state を消していたため、cooldown 内でクラッシュ→復帰を繰り返す worktree は毎回「初回試行」扱いになり MaxRetries が実質無効化されていた → cooldown window が経過していない限り retry state を保持するよう修正

## 検討して捨てた案

- **`Reaper` を拡張して同じ Tick 内でクラッシュ判定も行う**: idle 判定（無活動で `Down`）とクラッシュ判定（異常停止で `Serve`）はトリガー条件も対応するアクションも正反対で、1つの関数に混ぜると条件分岐が読みにくくなる。別コンポーネントの方が単体テストも書きやすい。
- **`AutoStart` フラグで HealthReaper の対象を絞る**: 手動 `wt serve` で起動した worktree がクラッシュした場合も復帰対象にすべき（本 issue の主目的が「殺したら殺したまま」の解消）という判断から、`Recorded()` の有無のみで判定し `AutoStart` では絞らないことにした。
- **明示 Down との完全同期（TOCTOU 解消）**: `devserver.Down()` の全呼び出し元に `HealthReaper.Forget()` 等の参照を配線すれば TOCTOU 競合を根絶できるが、現状の handler/autostart 層は reaper 系コンポーネントへの直接参照を持たない設計（`.worktrees.json` と `running.json` 経由の疎結合）になっており、この設計を崩す規模の変更になる。発生確率が低く（マイクロ秒〜Cooldown ウィンドウ内の narrow な競合）、`trigger=crash-recovery` ログで可視化され、ユーザーが `wt dev down` を再実行すれば自己修復できるため #139 に defer した。

## 壊すと危ない前提 / 変えてよい前提

- **壊してはいけない**: `devserver.Down()` は成功時に必ず `running.json` を削除するという契約。HealthReaper のクラッシュ判定（`Recorded()` 非空 かつ `IsRunning()` 偽 ＝ クラッシュ）はこの契約に依存している。`Down()` が状態をクリアしなくなると、明示停止した worktree まで自動復帰されてしまう。
- **壊してはいけない**: `r.retries` map の更新は `Tick()` 内で `Recorded()`/`IsRunning()` の判定より先に「追跡中かどうか」を見る順序。順序を入れ替えると #137 で修正した2つの false PASS が再発する。
- **変えてよい**: `Cooldown`/`MaxRetries`/`Interval` の既定値（`internal/wt/settings/settings.go`）。運用実績を見て調整可能。
- **変えてよい**: 部分死（degraded）ケースへの対応拡張。現状スコープ外だが、`devserver.RunStatus` の `alive/total` を使えば同じ枠組みで拡張できる。
