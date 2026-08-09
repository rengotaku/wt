import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { portsApi, treesApi, type ListenerRow, type StaleItem, type GcRequest } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

// 幽霊ポート（削除済み worktree の残骸）: 正常時（0件）はコンパクトな1行表示に留め、
// 検出時のみ amber の警告パネルへ展開する。常時カード1枚を占有させない。
function StalePortsSection() {
  const queryClient = useQueryClient();
  const { data: stale = [], isFetching } = useQuery<StaleItem[]>({
    queryKey: ["port-stale"],
    queryFn: portsApi.stale,
  });

  const prune = useMutation({
    mutationFn: portsApi.prune,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["port-stale"] });
      queryClient.invalidateQueries({ queryKey: ["ports"] });
      queryClient.invalidateQueries({ queryKey: ["port-listeners"] });
    },
  });

  if (isFetching && stale.length === 0 && !prune.isSuccess) {
    return (
      <p className="text-xs text-muted-foreground">幽霊ポートを確認中...</p>
    );
  }

  if (stale.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">
        {prune.isSuccess
          ? `幽霊ポートを${prune.data.count}件掃除しました。`
          : "幽霊ポート（削除済み worktree の残骸）はありません。"}
      </p>
    );
  }

  return (
    <div className="rounded-lg border border-amber-300/60 bg-amber-50/60 p-4 dark:border-amber-900 dark:bg-amber-950/20">
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="text-sm font-semibold text-amber-900 dark:text-amber-200">
            幽霊ポートが{stale.length}件見つかりました
          </p>
          <p className="mt-1 text-xs text-amber-800/80 dark:text-amber-200/70">
            <code>wt tree rm</code> を経由せず消された worktree が{" "}
            <code>.worktrees.json</code> に <code>port_base</code>{" "}
            だけ残した残骸です。ポート帯を死蔵し、割当枯渇の原因になります。
          </p>
        </div>
        <Button
          variant="destructive"
          size="sm"
          className="shrink-0"
          disabled={prune.isPending}
          onClick={() => {
            if (
              window.confirm(
                `${stale.length} 件の幽霊エントリを削除し、ポートブロックを回収します。よろしいですか？`,
              )
            ) {
              prune.mutate();
            }
          }}
        >
          {prune.isPending ? "掃除中..." : `掃除して回収（${stale.length}件）`}
        </Button>
      </div>
      {prune.isError && (
        <p className="mt-3 text-sm text-red-600">
          {(prune.error as Error).message}
        </p>
      )}
      <Table wrapperClassName="mt-3 max-h-64">
        <TableHeader className="sticky top-0 z-10 bg-background shadow-[0_1px_2px_rgba(0,0,0,0.1)]">
          <TableRow>
            <TableHead>リポジトリ</TableHead>
            <TableHead>worktree</TableHead>
            <TableHead>ポート</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {stale.map((s) => (
            <TableRow key={`${s.repo}/${s.wt_name}`}>
              <TableCell>{s.repo}</TableCell>
              <TableCell className="text-xs">{s.wt_name}</TableCell>
              <TableCell className="font-mono text-xs">
                {s.port_range || "—"}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

// Ports セクションの主カード。幽霊ポートの状態は上部に折り込み、
// 稼働中ポート一覧をメインコンテンツとして扱う。
function PortsCard() {
  const [showUnknown, setShowUnknown] = useState(false);
  const {
    data: listeners = [],
    refetch,
    isFetching,
  } = useQuery<ListenerRow[]>({
    queryKey: ["port-listeners"],
    queryFn: portsApi.listeners,
  });

  const unknownCount = listeners.filter((l) => !l.proc).length;
  const visible = showUnknown ? listeners : listeners.filter((l) => l.proc);

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <CardTitle>稼働中ポート</CardTitle>
          <Button
            variant="outline"
            size="sm"
            onClick={() => refetch()}
            disabled={isFetching}
          >
            {isFetching ? "更新中..." : "更新"}
          </Button>
        </div>
        <p className="text-sm text-muted-foreground">
          このマシンで LISTEN 中の全 TCP ポート。<strong>wt</strong>（worktree に割当）か{" "}
          <strong>foreign</strong>（別プロジェクト等の占有）かを表示します。ポート衝突の原因特定に使えます。
          各 worktree のポート割当・サーバー起動は{" "}
          <Link to="/" className="text-blue-600 hover:underline">
            Worktrees
          </Link>{" "}
          一覧で確認・操作できます。
        </p>
        <StalePortsSection />
      </CardHeader>
      <CardContent>
        {listeners.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            LISTEN 中のポートがありません（ss が無い環境かも）
          </p>
        ) : (
          <>
            <div className="flex flex-col gap-2 border-t pt-3 sm:flex-row sm:items-center sm:justify-between">
              <span className="text-xs text-muted-foreground">
                {visible.length}件を表示中
                {!showUnknown && unknownCount > 0 && `（不明プロセス${unknownCount}件を隠しています）`}
              </span>
              {unknownCount > 0 && (
                <label className="flex items-center gap-2 text-sm text-muted-foreground">
                  <input
                    type="checkbox"
                    checked={showUnknown}
                    onChange={(e) => setShowUnknown(e.target.checked)}
                  />
                  プロセス不明の行も表示する
                </label>
              )}
            </div>
            {visible.length === 0 ? (
              <p className="mt-3 text-sm text-muted-foreground">
                プロセス不明の行のみです。上のチェックを入れると表示されます。
              </p>
            ) : (
              <Table wrapperClassName="mt-3 max-h-[28rem]">
                <TableHeader className="sticky top-0 z-10 bg-background shadow-[0_1px_2px_rgba(0,0,0,0.1)]">
                  <TableRow>
                    <TableHead>ポート</TableHead>
                    <TableHead>プロセス</TableHead>
                    <TableHead>PID</TableHead>
                    <TableHead>区分</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {visible.map((l) => (
                    <TableRow key={l.port}>
                      <TableCell className="font-mono">{l.port}</TableCell>
                      <TableCell className="text-xs">{l.proc || "—"}</TableCell>
                      <TableCell className="font-mono text-xs">{l.pid || "—"}</TableCell>
                      <TableCell>
                        {l.managed ? (
                          <span className="rounded bg-green-100 px-1.5 py-0.5 text-xs text-green-700">
                            {`wt: ${l.owner ?? ""}`}
                          </span>
                        ) : (
                          <span className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">
                            foreign
                          </span>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}

// GC カード。削除対象（ニュートラル）と危険なオプション（amber）を視覚的に分離し、
// カード枠は destructive 系の薄い色でGC操作であることを示す。
function GcCard() {
  const [opts, setOpts] = useState<GcRequest>({
    done: false,
    older_than: "",
    force: false,
    dry_run: true,
    yes: false,
  });
  const [output, setOutput] = useState("");
  const [outputWasDryRun, setOutputWasDryRun] = useState(true);
  const [error, setError] = useState("");

  const gcMutation = useMutation({
    mutationFn: treesApi.gc,
    onSuccess: (res, variables) => {
      setOutput(res.output);
      setOutputWasDryRun(variables.dry_run ?? true);
      setError("");
    },
    onError: (e: Error) => setError(e.message),
  });

  const run = (execute: boolean) => {
    gcMutation.mutate({ ...opts, dry_run: !execute, yes: execute });
  };

  return (
    <Card className="border-destructive/30">
      <CardHeader>
        <CardTitle>GC オプション</CardTitle>
        <p className="text-sm text-muted-foreground">
          不要になった worktree を一括削除する機能です。オプションで絞り込み条件を指定し、
          まず「プレビュー」で削除対象を確認してから「GC 実行」で削除します。
          main / master worktree は対象外です。
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        <section className="space-y-2 rounded-lg border bg-muted/20 p-4">
          <h3 className="text-sm font-medium">削除対象</h3>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={opts.done ?? false}
              onChange={(e) => setOpts({ ...opts, done: e.target.checked })}
            />
            done な PR / issue の worktree を対象
          </label>
          <p className="text-xs text-muted-foreground ml-5">
            対応する PR が merged / closed、または issue が closed の worktree
            を対象にします。放置された没ブランチの掃除に。
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
        </section>

        <section className="space-y-2 rounded-lg border border-amber-300/60 bg-amber-50/60 p-4 dark:border-amber-900 dark:bg-amber-950/20">
          <h3 className="text-sm font-semibold text-amber-900 dark:text-amber-200">
            危険なオプション
          </h3>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={opts.force ?? false}
              onChange={(e) => setOpts({ ...opts, force: e.target.checked })}
            />
            dirty な worktree も対象に含める（--force 相当）
          </label>
          <p className="text-xs text-muted-foreground ml-5">
            未コミットの変更がある worktree も削除対象にします。
          </p>
        </section>

        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
      </CardContent>
      <CardFooter className="flex-col gap-2 rounded-b-xl border-t bg-muted/20 py-4 sm:flex-row sm:justify-end">
        <Button
          variant="outline"
          className="w-full sm:w-auto"
          onClick={() => run(false)}
          disabled={gcMutation.isPending}
        >
          プレビュー (dry-run)
        </Button>
        <Button
          variant="destructive"
          className="w-full sm:w-auto"
          onClick={() => run(true)}
          disabled={gcMutation.isPending}
        >
          {gcMutation.isPending ? "実行中..." : "GC 実行"}
        </Button>
      </CardFooter>
      {output && (
        <div className="border-t px-6 py-4">
          <h3 className="mb-2 text-sm font-medium">
            {outputWasDryRun ? "プレビュー結果" : "GC実行結果"}
          </h3>
          <pre className="max-h-64 overflow-auto rounded-md bg-muted p-3 font-mono text-xs whitespace-pre-wrap">
            {output}
          </pre>
        </div>
      )}
    </Card>
  );
}

export function MaintenancePage() {
  return (
    <div className="min-w-0 space-y-12">
      <section className="min-w-0 space-y-4">
        <header className="border-b pb-3">
          <h2 className="text-xl font-semibold tracking-tight">Ports</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            ポートの使用状況と不要な割り当てを確認します。
          </p>
        </header>
        <PortsCard />
      </section>
      <section className="min-w-0 space-y-4">
        <header className="border-b border-destructive/30 pb-3">
          <h2 className="text-xl font-semibold tracking-tight">GC</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            不要になった worktree を確認し、一括削除します。
          </p>
        </header>
        <GcCard />
      </section>
    </div>
  );
}
