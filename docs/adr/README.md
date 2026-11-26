# ADR 索引

<!-- generate-adr-index.zsh による自動生成。手で編集しない -->

| ADR | タイトル | status | date | 要旨 |
|---|---|---|---|---|
| [0004](0004-remove-web-ui.md) | wt web（Web UI と HTTP API）を廃止し、常駐処理を wt daemon に移す | accepted | 2026-11-26 | 認証の無い HTTP API が外部サイトから呼べたため Web UI ごと廃止し、proxy・AutoStart・reaper は API の無い wt daemon で常駐させる |

## frontmatter 未整備（索引対象外）

- 0001-wt-web-operability-improvements.md
- 0002-port-status-external-listen.md
- 0003-health-reaper-crash-recovery.md
