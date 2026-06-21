import { Copy, Check, Pin } from "lucide-react";
import type { TreeItem, PortItem } from "@/api";
import { Button } from "@/components/ui/button";

interface WorktreeCardProps {
  tree: TreeItem;
  port?: PortItem;
  pinned: boolean;
  selected: boolean;
  copied: boolean;
  isNew: boolean;
  onToggleSelect: () => void;
  onTogglePin: () => void;
  onCopy: () => void;
  onRepoClick: () => void;
  onOpenDetail: () => void;
  registerRef: (el: HTMLElement | null) => void;
}

// 一覧テーブルのポート列と同じ表示ロジック（カード版）。
function PortBadge({ port }: { port?: PortItem }) {
  if (!port?.has_dev_config) {
    return <span className="text-muted-foreground">—</span>;
  }
  if (port.running && port.degraded) {
    return (
      <span
        className="inline-flex items-center gap-1 text-amber-600"
        title="起動したサービスの一部が停止しています"
      >
        <span className="h-2 w-2 rounded-full bg-amber-500" />⚠ 一部停止
      </span>
    );
  }
  return (
    <span
      className={
        port.running
          ? "inline-flex items-center gap-1 text-green-700"
          : "inline-flex items-center gap-1 text-muted-foreground"
      }
      title={port.port_range ?? "未割当"}
    >
      <span
        className={`h-2 w-2 rounded-full ${port.running ? "bg-green-600" : "bg-muted-foreground/40"}`}
      />
      {port.running ? "稼働" : "停止"}
    </span>
  );
}

/**
 * スマートフォン向けの worktree 1 件カード。`md` 未満でテーブルの代わりに縦積みで
 * 表示する（テーブルはデスクトップ専用）。aria-label・ハンドラはテーブル行と揃えて
 * あるため、操作（選択/ピン/コピー/詳細/ポート状態）はカード上でタップ完結する。
 */
export function WorktreeCard({
  tree: t,
  port,
  pinned,
  selected,
  copied,
  isNew,
  onToggleSelect,
  onTogglePin,
  onCopy,
  onRepoClick,
  onOpenDetail,
  registerRef,
}: WorktreeCardProps) {
  return (
    <div
      ref={registerRef}
      onClick={(e) => {
        // インタラクティブ要素のタップでは詳細を開かない（テーブル行と同じ挙動）。
        if ((e.target as HTMLElement).closest("a,button,input,select,label")) return;
        onOpenDetail();
      }}
      className={[
        "rounded-lg border bg-card p-3 text-sm shadow-sm active:bg-muted/50",
        t.is_main ? "opacity-70" : "",
        isNew ? "row-highlight" : "",
      ]
        .filter(Boolean)
        .join(" ")}
    >
      <div className="flex items-start gap-2">
        <label
          className="flex min-h-10 items-center"
          onClick={(e) => e.stopPropagation()}
        >
          <input
            type="checkbox"
            className="size-5"
            checked={selected}
            onChange={onToggleSelect}
            aria-label={`${t.repo}/${t.wt_name} を選択`}
          />
        </label>
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onTogglePin();
          }}
          title={pinned ? "ピンを解除" : "ピン留め"}
          aria-label={
            pinned
              ? `${t.repo}/${t.wt_name} のピンを解除`
              : `${t.repo}/${t.wt_name} をピン留め`
          }
          className={`-m-1 flex min-h-10 min-w-10 items-center justify-center p-1 ${
            pinned ? "text-amber-500" : "text-muted-foreground/50"
          }`}
        >
          <Pin className={`h-4 w-4 ${pinned ? "fill-current" : ""}`} />
        </button>
        <div className="min-w-0 flex-1">
          <button
            type="button"
            className="block max-w-full truncate text-left text-xs text-blue-600 hover:underline"
            title={t.repo}
            aria-label={`${t.repo} で絞り込み`}
            onClick={(e) => {
              e.stopPropagation();
              onRepoClick();
            }}
          >
            {t.repo}
          </button>
          <div className="truncate font-medium">{t.wt_name}</div>
          <div className="truncate font-mono text-xs text-muted-foreground">
            {t.branch || "—"}
          </div>
        </div>
        <Button
          variant="ghost"
          size="sm"
          className="h-10 w-10 shrink-0 p-0"
          onClick={(e) => {
            e.stopPropagation();
            onCopy();
          }}
          title={copied ? "コピーしました" : `${t.wt_name}\n${t.path}`}
          aria-label={`${t.wt_name} のパスをコピー`}
        >
          {copied ? (
            <Check className="h-4 w-4 text-green-600" />
          ) : (
            <Copy className="h-4 w-4" />
          )}
        </Button>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 border-t pt-2 text-xs text-muted-foreground">
        <span className="inline-flex items-center gap-1">
          ポート: <PortBadge port={port} />
        </span>
        <span>
          変更:{" "}
          {t.diff_count > 0 ? (
            <span className="font-medium text-amber-600">{t.diff_count}</span>
          ) : (
            "0"
          )}
        </span>
        <span>tmux: {t.has_tmux ? <span className="text-green-600">✓</span> : "—"}</span>
        <span>{t.created_at || "—"}</span>
      </div>
    </div>
  );
}
