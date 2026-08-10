import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { ChevronDown, ChevronRight, AlertTriangle } from "lucide-react";
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

// 幽霊ポート（削除済み worktree の残骸）。ReposPage の「リポジトリを追加」カードと
// 同じクリック開閉パターン。検出時は自動で開き、0件なら閉じたままにする
// （ユーザーが一度でも手で開閉したら、その状態を優先する）。
function StalePortsCard() {
  const [manualOpen, setManualOpen] = useState<boolean | null>(null);
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

  // 掃除成功直後は stale が空になり自動判定だけでは閉じてしまうため、
  // 完了メッセージが見える間（isSuccess）は開いたままにする。
  const open = manualOpen ?? (stale.length > 0 || prune.isSuccess);

  return (
    <Card>
      <CardHeader className="p-0">
        <button
          type="button"
          className="flex w-full flex-col space-y-1.5 p-6 text-left cursor-pointer select-none"
          onClick={() => setManualOpen(!open)}
          aria-expanded={open}
          aria-controls="stale-ports-content"
        >
          <CardTitle className="flex items-center gap-2">
            {open ? (
              <ChevronDown className="h-4 w-4" />
            ) : (
              <ChevronRight className="h-4 w-4" />
            )}
            幽霊ポート（削除済み worktree の残骸）
            {stale.length > 0 && (
              <span className="text-amber-600 font-medium">{stale.length}件</span>
            )}
          </CardTitle>
        </button>
      </CardHeader>
      {open && (
        <CardContent id="stale-ports-content" className="space-y-3">
          <p className="text-sm text-muted-foreground">
            <code>wt tree rm</code> を経由せず消された worktree が{" "}
            <code>.worktrees.json</code> に <code>port_base</code>{" "}
            だけ残した残骸です。ポート帯を死蔵し、割当枯渇の原因になります。掃除すると
            該当ブロックが回収されます（登録の削除のみ・ファイルは触りません）。
          </p>
          {prune.isError && (
            <p className="text-sm text-red-600">
              {(prune.error as Error).message}
            </p>
          )}
          {prune.isSuccess && (
            <p className="text-sm text-green-700">
              {prune.data.count} 件を掃除し、{prune.data.count} ブロックを回収しました。
            </p>
          )}
          {isFetching && stale.length === 0 ? (
            <p className="text-sm text-muted-foreground">確認中...</p>
          ) : stale.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              幽霊エントリはありません。
            </p>
          ) : (
            <>
              <div className="flex justify-end">
                <Button
                  variant="destructive"
                  size="sm"
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
                  {prune.isPending
                    ? "掃除中..."
                    : `掃除して回収（${stale.length}件）`}
                </Button>
              </div>
              <Table wrapperClassName="max-h-64">
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
            </>
          )}
        </CardContent>
      )}
    </Card>
  );
}

// 稼働中ポート一覧カード。
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
        {unknownCount > 0 && (
          <label className="mt-1 flex items-center gap-2 text-sm text-muted-foreground">
            <input
              type="checkbox"
              checked={showUnknown}
              onChange={(e) => setShowUnknown(e.target.checked)}
            />
            プロセス不明の行も表示する
            {showUnknown
              ? `（不明プロセス${unknownCount}件を表示中）`
              : `（${unknownCount}件を隠しています）`}
          </label>
        )}
      </CardHeader>
      <CardContent>
        {listeners.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            LISTEN 中のポートがありません（ss が無い環境かも）
          </p>
        ) : visible.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            プロセス不明の行のみです。上のチェックを入れると表示されます。
          </p>
        ) : (
          <Table wrapperClassName="max-h-[28rem]">
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
      </CardContent>
    </Card>
  );
}

// backend の parseOlderThan（internal/wt/gc/gc.go）と同じ形式・正数判定を
// フロントでも行う。"0d" / "0h" のようにフォーマットは正しいが期間が0の値を
// 「フィルタ指定あり」と誤判定しないようにする（backend は olderSecs<=0 を
// フィルタ未指定として拒否するため、判定基準をここで一致させる）。
function isPositiveOlderThan(value: string): boolean {
  const m = /^(\d+)([dh])$/.exec(value.trim());
  if (!m) return false;
  return Number(m[1]) > 0;
}

// GC カード。危険度はボタンの destructive variant とアイコンだけで示し、
// カード枠・背景ブロックは他ページと同じニュートラルな見た目に揃える。
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

  // フィルタが1つも無いと「削除対象」の絞り込みが一切効かず、main/master
  // 以外の全 worktree が対象になる（backend は fail-safe でエラーを返すが、
  // フロントでも早期に気づかせて実行自体をブロックする）。
  const hasFilter = Boolean(opts.done) || isPositiveOlderThan(opts.older_than ?? "");

  return (
    <Card>
      <CardHeader>
        <CardTitle>GC オプション</CardTitle>
        <p className="text-sm text-muted-foreground">
          不要になった worktree を一括削除する機能です。オプションで絞り込み条件を指定し、
          まず「プレビュー」で削除対象を確認してから「GC 実行」で削除します。
          main / master worktree は対象外です。
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        <section className="space-y-2">
          <h3 className="text-sm font-medium text-muted-foreground">削除対象</h3>
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

        <section className="space-y-2">
          <h3 className="text-sm font-medium text-muted-foreground">危険なオプション</h3>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={opts.force ?? false}
              onChange={(e) => setOpts({ ...opts, force: e.target.checked })}
            />
            <AlertTriangle className="h-3.5 w-3.5 shrink-0 text-amber-500" />
            dirty な worktree も対象に含める（--force 相当）
          </label>
          <p className="text-xs text-muted-foreground ml-5">
            未コミットの変更がある worktree も削除対象にします。
          </p>
        </section>

        {!hasFilter && (
          <p className="flex items-center gap-1.5 text-xs text-amber-600">
            <AlertTriangle className="h-3.5 w-3.5 shrink-0" />
            「削除対象」のフィルタを1つ以上指定してください。未指定のままでは
            main/master 以外の全 worktree が対象になるため実行できません。
          </p>
        )}

        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
      </CardContent>
      <CardFooter className="flex-col gap-2 py-4 sm:flex-row sm:justify-end">
        <Button
          variant="outline"
          className="w-full sm:w-auto"
          onClick={() => run(false)}
          disabled={gcMutation.isPending || !hasFilter}
        >
          プレビュー (dry-run)
        </Button>
        <Button
          variant="destructive"
          className="w-full sm:w-auto"
          onClick={() => run(true)}
          disabled={gcMutation.isPending || !hasFilter}
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
    <div className="space-y-6">
      <StalePortsCard />
      <PortsCard />
      <GcCard />
    </div>
  );
}
