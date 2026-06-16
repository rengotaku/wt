import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { X, RefreshCw } from "lucide-react";
import { portsApi } from "@/api";
import { Button } from "@/components/ui/button";

export interface LogTarget {
  repo: string;
  wt: string;
}

interface LogPanelProps {
  target: LogTarget | null;
  onClose: () => void;
}

/**
 * Side panel showing each dev service's captured stdout+stderr. Polls while open
 * so a running server's output streams in. Logs persist after stop, so a crashed
 * serve can still be inspected.
 */
export function LogPanel({ target, onClose }: LogPanelProps) {
  if (target === null) return null;
  return (
    <div className="fixed inset-0 z-50 flex">
      <div className="flex-1 bg-black/40" onClick={onClose} />
      <PanelBody key={`${target.repo}/${target.wt}`} target={target} onClose={onClose} />
    </div>
  );
}

function PanelBody({ target, onClose }: { target: LogTarget; onClose: () => void }) {
  const { repo, wt } = target;
  const [autoRefresh, setAutoRefresh] = useState(true);

  const { data, isLoading, error, refetch, isFetching } = useQuery({
    queryKey: ["ports", repo, wt, "logs"],
    queryFn: () => portsApi.logs(repo, wt),
    refetchInterval: autoRefresh ? 2000 : false,
    staleTime: 0,
  });

  const logs = data?.logs ?? [];

  return (
    <aside className="w-full max-w-2xl bg-background border-l border-border h-full flex flex-col shadow-xl">
      <header className="flex items-center justify-between p-4 border-b">
        <div>
          <h2 className="text-lg font-semibold">サーバーログ</h2>
          <p className="text-xs font-mono text-muted-foreground">
            {repo}/{wt}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <label className="flex items-center gap-1 text-xs text-muted-foreground cursor-pointer">
            <input
              type="checkbox"
              checked={autoRefresh}
              onChange={(e) => setAutoRefresh(e.target.checked)}
            />
            自動更新
          </label>
          <Button variant="outline" size="sm" onClick={() => refetch()} disabled={isFetching}>
            <RefreshCw className={`h-3 w-3 ${isFetching ? "animate-spin" : ""}`} />
          </Button>
          <Button variant="ghost" size="sm" onClick={onClose} aria-label="閉じる">
            <X className="h-4 w-4" />
          </Button>
        </div>
      </header>

      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {isLoading && <p className="text-sm text-muted-foreground">読み込み中...</p>}
        {error && (
          <p className="text-sm text-red-600">{(error as Error).message}</p>
        )}
        {!isLoading && !error && logs.length === 0 && (
          <p className="text-sm text-muted-foreground">
            ログがありません。「起動」するとサーバーの標準出力がここに出ます。
          </p>
        )}
        {logs.map((svc) => (
          <section key={svc.name} className="space-y-1">
            <h3 className="text-sm font-medium font-mono">{svc.name}</h3>
            <pre className="text-xs bg-muted rounded-md p-2 overflow-x-auto whitespace-pre-wrap break-words max-h-96 overflow-y-auto">
              {svc.content || "（出力なし）"}
            </pre>
          </section>
        ))}
      </div>
    </aside>
  );
}
