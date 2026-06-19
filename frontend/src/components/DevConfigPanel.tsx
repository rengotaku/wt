import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Trash2, X } from "lucide-react";
import { portsApi, type DevService } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription } from "@/components/ui/alert";

export interface DevConfigTarget {
  repo: string;
  wt: string;
}

interface DevConfigPanelProps {
  target: DevConfigTarget | null;
  onClose: () => void;
}

/**
 * Side panel to create/edit a worktree's .wt/dev.toml from the Web UI. Each
 * service gets a port from the worktree's block (declaration order = base+i);
 * use ${port} in the command and tick "ドメイン公開" to expose it via wt proxy.
 */
export function DevConfigPanel({ target, onClose }: DevConfigPanelProps) {
  if (target === null) return null;
  // この panel は常に WorktreeDetailPanel(z-50) の上から開かれる。詳細パネルを
  // 完全に隠さず左端を少し残してその上に重ねるため、z を一段上げ、背面は暗幕を
  // 敷かず透明のクリック捕捉のみにする（詳細パネルの縁がそのまま透けて見える）。
  return (
    <div className="fixed inset-0 z-[60] flex">
      <div className="flex-1" onClick={onClose} />
      <PanelBody key={`${target.repo}/${target.wt}`} target={target} onClose={onClose} />
    </div>
  );
}

function PanelBody({ target, onClose }: { target: DevConfigTarget; onClose: () => void }) {
  const queryClient = useQueryClient();
  const { repo, wt } = target;

  const { data, isLoading, error } = useQuery({
    queryKey: ["ports", repo, wt, "devconfig"],
    queryFn: () => portsApi.getDevConfig(repo, wt),
    staleTime: 0,
  });

  const [services, setServices] = useState<DevService[] | null>(null);
  const [saveError, setSaveError] = useState("");
  const [saved, setSaved] = useState(false);

  // Initialise from server once; afterwards keep the user's edits.
  const effective = services ?? data?.services ?? [];

  const saveMutation = useMutation({
    mutationFn: (svcs: DevService[]) => portsApi.putDevConfig(repo, wt, svcs),
    onSuccess: (next) => {
      setServices(next.services);
      setSaveError("");
      setSaved(true);
      queryClient.invalidateQueries({ queryKey: ["ports"] });
    },
    onError: (e: Error) => {
      setSaveError(e.message);
      setSaved(false);
    },
  });

  const update = (next: DevService[]) => {
    setServices(next);
    setSaved(false);
  };
  const handleAdd = () => update([...effective, { name: "", cmd: "", domain: false }]);
  const handleRemove = (idx: number) => update(effective.filter((_, i) => i !== idx));
  const handleField = (idx: number, patch: Partial<DevService>) =>
    update(effective.map((s, i) => (i === idx ? { ...s, ...patch } : s)));
  const handleSave = () => saveMutation.mutate(effective);

  return (
    <aside className="w-full max-w-[34rem] bg-background border-l border-border h-full flex flex-col shadow-2xl">
      <header className="flex items-center justify-between p-4 border-b">
        <div>
          <h2 className="text-lg font-semibold">dev.toml 編集</h2>
          <p className="text-xs font-mono text-muted-foreground">
            {repo}/{wt}
          </p>
        </div>
        <Button variant="ghost" size="sm" onClick={onClose} aria-label="閉じる">
          <X className="h-4 w-4" />
        </Button>
      </header>

      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-medium">services</h3>
          <Button variant="outline" size="sm" onClick={handleAdd} disabled={isLoading}>
            <Plus className="h-3 w-3 mr-1" />
            追加
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          宣言順に割当ブロックの base+i が割り当てられ、cmd 内の <code>${"{port}"}</code>{" "}
          に置換されます。「ドメイン公開」で wt proxy 経由の名前アクセス対象になります。
          保存するとこの worktree 専用の上書きとしてメタデータに保存されます（repo に
          コミットされません）。
        </p>
        {data && (
          <p className="text-xs">
            現在の参照元:{" "}
            <span className="font-medium">
              {data.source === "worktree"
                ? "この worktree 専用の上書き"
                : data.source === "repo"
                  ? "リポジトリ既定を継承中"
                  : data.source === "file"
                    ? "コミット済み .wt/dev.toml"
                    : "未設定"}
            </span>
          </p>
        )}

        {isLoading && <p className="text-sm text-muted-foreground">読み込み中...</p>}
        {error && (
          <Alert variant="destructive">
            <AlertDescription>{(error as Error).message}</AlertDescription>
          </Alert>
        )}

        {!isLoading && !error && (
          <div className="space-y-3">
            {effective.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                service は未定義です。「追加」で作成してください。
              </p>
            ) : (
              effective.map((svc, idx) => (
                <div key={idx} className="space-y-1.5 rounded-md border p-2">
                  <div className="flex gap-2">
                    <Input
                      value={svc.name}
                      onChange={(e) => handleField(idx, { name: e.target.value })}
                      placeholder="name (例: api)"
                      className="flex-1 font-mono text-xs"
                    />
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleRemove(idx)}
                      aria-label="削除"
                    >
                      <Trash2 className="h-3 w-3" />
                    </Button>
                  </div>
                  <Input
                    value={svc.cmd}
                    onChange={(e) => handleField(idx, { cmd: e.target.value })}
                    placeholder="cmd (例: npm run dev -- --port ${port})"
                    className="font-mono text-xs"
                  />
                  <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
                    <input
                      type="checkbox"
                      checked={svc.domain}
                      onChange={(e) => handleField(idx, { domain: e.target.checked })}
                    />
                    ドメイン公開（wt proxy）
                  </label>
                </div>
              ))
            )}
          </div>
        )}

        {saveError && (
          <Alert variant="destructive">
            <AlertDescription>{saveError}</AlertDescription>
          </Alert>
        )}
        {saved && <p className="text-xs text-green-600">保存しました。</p>}
      </div>

      <footer className="p-4 border-t flex justify-end gap-2">
        {data?.source === "worktree" && (
          <Button
            variant="outline"
            className="mr-auto"
            onClick={() => saveMutation.mutate([])}
            disabled={saveMutation.isPending}
            title="この worktree の上書きを消してリポジトリ既定に戻す"
          >
            既定に戻す
          </Button>
        )}
        <Button variant="outline" onClick={onClose}>
          閉じる
        </Button>
        <Button onClick={handleSave} disabled={saveMutation.isPending || isLoading}>
          {saveMutation.isPending ? "保存中..." : "保存"}
        </Button>
      </footer>
    </aside>
  );
}
