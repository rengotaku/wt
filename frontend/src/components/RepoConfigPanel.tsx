import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Trash2, X } from "lucide-react";
import { reposApi, type RepoConfig, type DevService } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription } from "@/components/ui/alert";

interface RepoConfigPanelProps {
  repoName: string | null;
  onClose: () => void;
}

export function RepoConfigPanel({ repoName, onClose }: RepoConfigPanelProps) {
  if (repoName === null) return null;
  return (
    <div className="fixed inset-0 z-50 flex">
      <div className="flex-1 bg-black/40" onClick={onClose} />
      <PanelBody key={repoName} repoName={repoName} onClose={onClose} />
    </div>
  );
}

function PanelBody({ repoName, onClose }: { repoName: string; onClose: () => void }) {
  const queryClient = useQueryClient();

  const { data, isLoading, error } = useQuery({
    queryKey: ["repos", repoName, "config"],
    queryFn: () => reposApi.getConfig(repoName),
    staleTime: 0,
  });

  const [candidates, setCandidates] = useState<string[] | null>(null);
  const [devServices, setDevServices] = useState<DevService[] | null>(null);
  const [saveError, setSaveError] = useState("");
  const [saved, setSaved] = useState(false);

  // 初回データ受信時のみ初期化（以降はユーザー編集値を保持）
  const effective = candidates ?? data?.symlink_candidates ?? [];
  const effectiveDev = devServices ?? data?.dev_services ?? [];

  const updateMutation = useMutation({
    mutationFn: (cfg: RepoConfig) => reposApi.updateConfig(repoName, cfg),
    onSuccess: (next) => {
      setCandidates(next.symlink_candidates ?? []);
      setDevServices(next.dev_services ?? []);
      setSaveError("");
      setSaved(true);
      queryClient.invalidateQueries({ queryKey: ["repos", repoName, "config"] });
      queryClient.invalidateQueries({ queryKey: ["ports"] });
    },
    onError: (e: Error) => {
      setSaveError(e.message);
      setSaved(false);
    },
  });

  const update = (next: string[]) => {
    setCandidates(next);
    setSaved(false);
  };
  const handleAdd = () => update([...effective, ""]);
  const handleChange = (idx: number, value: string) =>
    update(effective.map((c, i) => (i === idx ? value : c)));
  const handleRemove = (idx: number) => update(effective.filter((_, i) => i !== idx));

  const updateDev = (next: DevService[]) => {
    setDevServices(next);
    setSaved(false);
  };
  const handleDevAdd = () =>
    updateDev([...effectiveDev, { name: "", cmd: "", domain: false, headless: false }]);
  const handleDevRemove = (idx: number) => updateDev(effectiveDev.filter((_, i) => i !== idx));
  const handleDevField = (idx: number, patch: Partial<DevService>) =>
    updateDev(effectiveDev.map((s, i) => (i === idx ? { ...s, ...patch } : s)));

  const handleSave = () =>
    updateMutation.mutate({ symlink_candidates: effective, dev_services: effectiveDev });

  return (
    <aside className="w-full max-w-md bg-background border-l border-border h-full flex flex-col shadow-xl">
      <header className="flex items-center justify-between p-4 border-b">
        <div>
          <h2 className="text-lg font-semibold">設定</h2>
          <p className="text-xs font-mono text-muted-foreground">{repoName}</p>
        </div>
        <Button variant="ghost" size="sm" onClick={onClose} aria-label="閉じる">
          <X className="h-4 w-4" />
        </Button>
      </header>

      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        <section className="space-y-2">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium">symlink_candidates</h3>
            <Button variant="outline" size="sm" onClick={handleAdd} disabled={isLoading}>
              <Plus className="h-3 w-3 mr-1" />
              追加
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            新しい worktree を作成したとき、main
            側からシンボリックリンクで持ち込む候補パス（コンテナ相対）。
          </p>

          {isLoading && <p className="text-sm text-muted-foreground">読み込み中...</p>}
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{(error as Error).message}</AlertDescription>
            </Alert>
          )}

          {!isLoading && !error && (
            <div className="space-y-2">
              {effective.length === 0 ? (
                <p className="text-sm text-muted-foreground">候補は未登録です。</p>
              ) : (
                effective.map((value, idx) => (
                  <div key={idx} className="flex gap-2">
                    <Input
                      value={value}
                      onChange={(e) => handleChange(idx, e.target.value)}
                      placeholder="例: .env または config/local"
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
                ))
              )}
            </div>
          )}
        </section>

        <section className="space-y-2 border-t pt-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium">dev サービス（リポジトリ既定）</h3>
            <Button variant="outline" size="sm" onClick={handleDevAdd} disabled={isLoading}>
              <Plus className="h-3 w-3 mr-1" />
              追加
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            この repo の全 worktree で使われる dev.toml の既定。メタデータに保存され
            repo にはコミットされません。worktree 個別に上書きも可能です。cmd 内の{" "}
            <code>${"{port}"}</code> は割当ポートに、<code>$WT_PORT_&lt;NAME&gt;</code> は
            兄弟サービスのポートに展開されます。
          </p>
          {effectiveDev.length === 0 ? (
            <p className="text-sm text-muted-foreground">既定は未設定です。</p>
          ) : (
            <div className="space-y-3">
              {effectiveDev.map((svc, idx) => (
                <div key={idx} className="space-y-1.5 rounded-md border p-2">
                  <div className="flex gap-2">
                    <Input
                      value={svc.name}
                      onChange={(e) => handleDevField(idx, { name: e.target.value })}
                      placeholder="name (例: api)"
                      className="flex-1 font-mono text-xs"
                    />
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleDevRemove(idx)}
                      aria-label="削除"
                    >
                      <Trash2 className="h-3 w-3" />
                    </Button>
                  </div>
                  <Input
                    value={svc.cmd}
                    onChange={(e) => handleDevField(idx, { cmd: e.target.value })}
                    placeholder="cmd (例: npm run dev -- --port ${port})"
                    className="font-mono text-xs"
                  />
                  <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
                    <input
                      type="checkbox"
                      checked={svc.domain}
                      onChange={(e) => handleDevField(idx, { domain: e.target.checked })}
                    />
                    ドメイン公開（wt proxy）
                  </label>
                  <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
                    <input
                      type="checkbox"
                      checked={svc.headless ?? false}
                      onChange={(e) => handleDevField(idx, { headless: e.target.checked })}
                    />
                    ポートを張らない（worker/scheduler）
                  </label>
                </div>
              ))}
            </div>
          )}
        </section>

        {saveError && (
          <Alert variant="destructive">
            <AlertDescription>{saveError}</AlertDescription>
          </Alert>
        )}
        {saved && <p className="text-xs text-green-600">保存しました。</p>}
      </div>

      <footer className="p-4 border-t flex justify-end gap-2">
        <Button variant="outline" onClick={onClose}>
          閉じる
        </Button>
        <Button onClick={handleSave} disabled={updateMutation.isPending || isLoading}>
          {updateMutation.isPending ? "保存中..." : "保存"}
        </Button>
      </footer>
    </aside>
  );
}
