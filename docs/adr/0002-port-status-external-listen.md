# ADR 0002: 外部起動 dev サーバも「稼働中」扱いにし、wt 管理外を UI で区別する

- Status: Accepted
- Date: 2026-07-16
- Related: #116

## 背景

`GET /api/ports` の `running` は `devserver.RunStatus`（wt が `wt serve` で書いた `running.json` の生存 PID 数 > 0）だけで判定していた。そのため **外部から手動で起動した dev サーバが LISTEN していても Web UI では「停止中」と表示**されていた。CLI (`internal/wt/ports/list.go::liveCell`) は既に `Listening || Running` の OR で判定しており、**CLI と Web UI の稼働判定式がズレていた**。

## 決定

`portItem.Running` を「LISTEN あり **または** wt 記録あり」の OR に緩め、CLI と揃える。加えて `unmanaged` を新設し「LISTEN があるが wt の起動記録は無い」状態を UI で識別できるようにする。

具体的な導出（`internal/handler/ports.go::ListPorts`）:

| フィールド | 定義 | 意味 |
|---|---|---|
| `running` | `alive > 0 \|\| anyListening` | 稼働中扱い（LISTEN でも真） |
| `unmanaged` | `alive == 0 && anyListening` | LISTEN だが wt 記録無し（外部起動）。wt からは `down` 不可 |
| `degraded` | `alive > 0 && alive < total \|\| anyUnhealthy` | wt 記録の一部が死んでいる縮退状態。外部 LISTEN のみで真にならないよう `alive > 0` でガード |

UI（`WorktreeCard.tsx` / `TreesPage.tsx` / `WorktreeDetailPanel.tsx`）は `port.running` で「稼働/停止」を出し、`port.unmanaged` のとき「wt管理外」バッジを並置。詳細パネルの起動/停止トグルは `unmanaged` のとき `disabled`（`wt` は PID を握っていないので停止させる術がない）。

## 検討して捨てた案

- **`Running` のセマンティクスを変えず、UI 側で `running || listening` の OR を取る**: CLI と表現式を統一する意味では等価だが、`Degraded` の判定は `alive` を知らないと正しく引けないため結局 API 側にロジックが必要。APIで一度導出しておくのが素直。
- **per-port の `unmanaged` を返す**: 情報量は増えるが、item 単位で「wt管理下 / wt管理外」が混在するケースは稀（`wt serve` で起こしたなら全 service が記録される）。frontend が要求されるまでは item 単位で十分。

## 壊すと危ない前提 / 変えてよい前提

- **壊してはいけない**: `Degraded` の `alive > 0` ガード。ここを外すと外部 LISTEN のみの状態が「縮退稼働」として警告表示される。ユーザーは wt 側の問題と誤解する。
- **変えてよい**: バッジのラベル文言（「wt管理外」）。認知しやすさで調整可能。
- **変えてよい**: トグルの disable の代わりに「wt では停止できない旨のダイアログ」を出す等、UI 表現の変更。API の `unmanaged` フラグ意味は不変。
