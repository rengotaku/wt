import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { settingsApi, type Settings } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Alert, AlertDescription } from "@/components/ui/alert";

export function SettingsPage() {
  const queryClient = useQueryClient();
  const { data, isLoading } = useQuery<Settings>({
    queryKey: ["settings"],
    queryFn: settingsApi.get,
  });

  // 編集中は overlay（null = 未編集でサーバ値を表示）。effect で setState しない。
  const [startEdit, setStartEdit] = useState<string | null>(null);
  const [endEdit, setEndEdit] = useState<string | null>(null);
  const [error, setError] = useState("");

  const start = startEdit ?? (data ? String(data.dev_ports.start) : "");
  const end = endEdit ?? (data ? String(data.dev_ports.end) : "");

  const mutation = useMutation({
    mutationFn: settingsApi.update,
    onSuccess: (res) => {
      setError("");
      setStartEdit(null);
      setEndEdit(null);
      queryClient.setQueryData(["settings"], res);
      queryClient.invalidateQueries({ queryKey: ["ports"] });
      toast.success("設定を保存しました", {
        description: `dev ポート帯: ${res.dev_ports.start}-${res.dev_ports.end}`,
      });
    },
    onError: (e: Error) => setError(e.message),
  });

  const save = () => {
    const s = Number(start);
    const e = Number(end);
    if (!Number.isInteger(s) || !Number.isInteger(e)) {
      setError("開始・終了は整数で入力してください");
      return;
    }
    mutation.mutate({ dev_ports: { start: s, end: e } });
  };

  const blockSize = data?.dev_ports.block_size ?? 5;
  const span = Number(end) - Number(start) + 1;
  const blocks = Number.isFinite(span) && span > 0 ? Math.floor(span / blockSize) : 0;

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>開発ポート帯</CardTitle>
          <p className="text-sm text-muted-foreground">
            worktree に割り当てる開発ポートの範囲です（既定 9000-9999）。
            <br />
            設定は <code className="font-mono text-xs">~/.config/wt/settings.toml</code> に保存され、
            以後の worktree 作成時の採番に使われます。
          </p>
        </CardHeader>
        <CardContent className="space-y-4">
          {isLoading ? (
            <p className="text-sm text-muted-foreground">読み込み中...</p>
          ) : (
            <>
              <div className="flex flex-wrap items-end gap-3">
                <div>
                  <label className="block text-xs text-muted-foreground mb-1">開始ポート</label>
                  <Input
                    className="w-32"
                    inputMode="numeric"
                    value={start}
                    onChange={(e) => setStartEdit(e.target.value)}
                  />
                </div>
                <span className="pb-2">–</span>
                <div>
                  <label className="block text-xs text-muted-foreground mb-1">終了ポート</label>
                  <Input
                    className="w-32"
                    inputMode="numeric"
                    value={end}
                    onChange={(e) => setEndEdit(e.target.value)}
                  />
                </div>
                <Button onClick={save} disabled={mutation.isPending}>
                  {mutation.isPending ? "保存中..." : "保存"}
                </Button>
              </div>

              <p className="text-xs text-muted-foreground">
                1 worktree = {blockSize} ポート/ブロック → この帯で最大 {blocks} worktree
                を割り当て可能。
              </p>

              {error && (
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
