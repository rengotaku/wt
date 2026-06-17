import { X, FileCog, ScrollText } from "lucide-react";
import type { TreeItem, PortItem, IssueDetail, MergedPRInfo } from "@/api";
import { Button } from "@/components/ui/button";

export interface WorktreeDetail {
  tree: TreeItem;
  port?: PortItem;
  issueURL: string | null;
  issueDetail?: IssueDetail;
  pr?: MergedPRInfo;
  repoURL?: string;
}

interface WorktreeDetailPanelProps {
  detail: WorktreeDetail | null;
  onClose: () => void;
  onServe: () => void;
  onDown: () => void;
  onEditConfig: () => void;
  onShowLogs: () => void;
  portBusy: boolean;
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[7rem_1fr] gap-2 py-1.5 border-b last:border-b-0">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="text-sm break-words">{children}</div>
    </div>
  );
}

/**
 * Half-overlay (right-side) detail panel for a single worktree, opened by
 * clicking a row. Shows the full, untruncated details that the compact table
 * abbreviates.
 */
export function WorktreeDetailPanel({
  detail,
  onClose,
  onServe,
  onDown,
  onEditConfig,
  onShowLogs,
  portBusy,
}: WorktreeDetailPanelProps) {
  if (detail === null) return null;
  const { tree: t, port, issueURL, issueDetail, pr, repoURL } = detail;
  const livePorts = (port?.ports ?? []).filter((p) => p.listening);

  return (
    <div className="fixed inset-0 z-50 flex">
      <div className="flex-1 bg-black/40" onClick={onClose} />
      <aside className="w-full max-w-xl bg-background border-l border-border h-full flex flex-col shadow-xl">
        <header className="flex items-center justify-between p-4 border-b">
          <div className="min-w-0">
            <h2 className="text-lg font-semibold truncate">{t.wt_name}</h2>
            <p className="text-xs font-mono text-muted-foreground truncate">{t.repo}</p>
          </div>
          <Button variant="ghost" size="sm" onClick={onClose} aria-label="閉じる">
            <X className="h-4 w-4" />
          </Button>
        </header>

        <div className="flex flex-wrap items-center gap-3 border-b p-4">
          <div className="flex items-center gap-2">
            <button
              type="button"
              role="switch"
              aria-checked={!!port?.running}
              aria-label="サーバーの起動/停止"
              disabled={portBusy || !port?.has_dev_config}
              onClick={() => (port?.running ? onDown() : onServe())}
              className={`relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors disabled:opacity-50 ${
                port?.running ? "bg-green-600" : "bg-muted-foreground/30"
              }`}
            >
              <span
                className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform ${
                  port?.running ? "translate-x-6" : "translate-x-1"
                }`}
              />
            </button>
            <span className="text-sm">{port?.running ? "稼働中" : "停止中"}</span>
          </div>
          <Button variant="outline" size="sm" onClick={onEditConfig}>
            <FileCog className="h-3 w-3 mr-1" />
            dev.toml
          </Button>
          <Button variant="outline" size="sm" onClick={onShowLogs}>
            <ScrollText className="h-3 w-3 mr-1" />
            ログ
          </Button>
        </div>

        <div className="flex-1 overflow-y-auto p-4">
          <Row label="Repo">{t.repo}</Row>
          <Row label="フォルダ名">
            <span className="font-mono">{t.wt_name}</span>
          </Row>
          <Row label="Branch">
            <span className="font-mono">{t.branch || "—"}</span>
          </Row>
          <Row label="パス">
            <span className="font-mono text-xs break-all">{t.path}</span>
          </Row>
          <Row label="Issue">
            {issueURL ? (
              <a
                href={issueURL}
                target="_blank"
                rel="noopener noreferrer"
                className="text-blue-600 hover:underline"
              >
                {t.issue}
              </a>
            ) : (
              "—"
            )}
            {issueDetail && (
              <span
                className={
                  issueDetail.state === "OPEN"
                    ? "ml-2 text-green-600"
                    : "ml-2 text-muted-foreground"
                }
              >
                {issueDetail.state === "OPEN" ? "open" : "closed"}
              </span>
            )}
          </Row>
          <Row label="親 issue">
            {issueDetail?.parent_number ? (
              <a
                href={issueDetail.parent_url || "#"}
                target="_blank"
                rel="noopener noreferrer"
                className="text-blue-600 hover:underline"
              >
                #{issueDetail.parent_number}
              </a>
            ) : (
              "—"
            )}
          </Row>
          <Row label="PR">
            {pr ? (
              <a
                href={repoURL ? `${repoURL}/pull/${pr.number}` : "#"}
                target="_blank"
                rel="noopener noreferrer"
                className="text-blue-600 hover:underline"
              >
                #{pr.number} ({pr.state})
              </a>
            ) : (
              "—"
            )}
          </Row>
          <Row label="ポート帯">{port?.port_range || "未割当"}</Row>
          <Row label="稼働">
            {port?.running ? (
              livePorts.length > 0 ? (
                <div className="flex flex-wrap gap-2">
                  {livePorts.map((p) => (
                    <a
                      key={p.port}
                      href={`http://localhost:${p.port}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="font-mono text-xs text-blue-600 hover:underline"
                    >
                      {p.service ? `${p.service}:${p.port}` : `:${p.port}`}
                    </a>
                  ))}
                </div>
              ) : (
                "起動中"
              )
            ) : (
              "停止中"
            )}
          </Row>
          {port?.running && port.domain && (
            <Row label="ドメイン">
              <a
                href={`http://${port.domain}:8088`}
                target="_blank"
                rel="noopener noreferrer"
                className="font-mono text-xs text-blue-600 hover:underline"
              >
                {port.domain}:8088
              </a>
              <span className="ml-2 text-xs text-muted-foreground">
                （要 proxy 起動）
              </span>
            </Row>
          )}
          <Row label="変更ファイル">
            {t.diff_count > 0 ? (
              <span className="text-amber-600">{t.diff_count} 件</span>
            ) : (
              "なし"
            )}
          </Row>
          <Row label="tmux">{t.has_tmux ? "あり" : "なし"}</Row>
          <Row label="作成日">{t.created_at || "—"}</Row>
        </div>
      </aside>
    </div>
  );
}
