import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { treesApi, type GcRequest } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Alert, AlertDescription } from "@/components/ui/alert";

export function GcPage() {
  const [opts, setOpts] = useState<GcRequest>({
    merged: false,
    closed: false,
    include_dirty: false,
    older_than: "",
    no_tmux: false,
    dry_run: true,
    yes: false,
  });
  const [output, setOutput] = useState("");
  const [error, setError] = useState("");

  const gcMutation = useMutation({
    mutationFn: treesApi.gc,
    onSuccess: (res) => {
      setOutput(res.output);
      setError("");
    },
    onError: (e: Error) => setError(e.message),
  });

  const run = (execute: boolean) => {
    gcMutation.mutate({ ...opts, dry_run: !execute, yes: execute });
  };

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>GC オプション</CardTitle>
          <p className="text-sm text-muted-foreground">
            不要になった worktree
            を一括削除する機能です。オプションで絞り込み条件を指定し、
            まず「プレビュー」で削除対象を確認してから「GC 実行」で削除します。
            <br />
            <span className="text-xs">
              ※ main / master worktree と、未コミットの変更がある worktree は対象外です。
            </span>
          </p>
        </CardHeader>
        <CardContent className="space-y-4">
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={opts.merged ?? false}
              onChange={(e) => setOpts({ ...opts, merged: e.target.checked })}
            />
            マージ済み PR のみ対象
          </label>
          <p className="text-xs text-muted-foreground ml-5">
            マージ済みブランチを持つ worktree のみを GC 対象にします。
          </p>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={opts.closed ?? false}
              onChange={(e) => setOpts({ ...opts, closed: e.target.checked })}
            />
            closed な issue/PR のみ対象
          </label>
          <p className="text-xs text-muted-foreground ml-5">
            対応する issue / PR が close（マージ含む）された worktree を対象にします。
            放置された没ブランチの掃除に。
          </p>
          <label className="flex items-center gap-2 text-sm ml-5">
            <input
              type="checkbox"
              checked={opts.include_dirty ?? false}
              disabled={!opts.closed}
              onChange={(e) => setOpts({ ...opts, include_dirty: e.target.checked })}
            />
            未コミット変更ありも含める（closed 時）
          </label>
          <p className="text-xs text-muted-foreground ml-5">
            通常は変更ありの worktree は対象外ですが、closed なものは変更があっても削除します。
          </p>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={opts.no_tmux ?? false}
              onChange={(e) => setOpts({ ...opts, no_tmux: e.target.checked })}
            />
            tmux セッションなしのみ対象
          </label>
          <p className="text-xs text-muted-foreground ml-5">
            アクティブな tmux セッションが存在しない worktree のみを対象にします。
          </p>
          <div className="flex items-center gap-2">
            <label className="text-sm whitespace-nowrap">最終コミット</label>
            <Input
              className="w-32"
              placeholder="30d / 24h"
              value={opts.older_than ?? ""}
              onChange={(e) => setOpts({ ...opts, older_than: e.target.value })}
            />
            <span className="text-sm text-muted-foreground">以上前</span>
          </div>
          <p className="text-xs text-muted-foreground">
            指定期間より古い最終コミットを持つ worktree を対象にします（例: 30d =
            30日前、24h = 24時間前）。
          </p>

          <div className="flex gap-2">
            <Button
              variant="outline"
              onClick={() => run(false)}
              disabled={gcMutation.isPending}
            >
              プレビュー (dry-run)
            </Button>
            <Button
              variant="destructive"
              onClick={() => run(true)}
              disabled={gcMutation.isPending}
            >
              {gcMutation.isPending ? "実行中..." : "GC 実行"}
            </Button>
          </div>

          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>

      {output && (
        <Card>
          <CardHeader>
            <CardTitle>出力</CardTitle>
          </CardHeader>
          <CardContent>
            <pre className="text-xs font-mono whitespace-pre-wrap bg-muted p-3 rounded">
              {output || "(出力なし)"}
            </pre>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
