import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { portsApi, type ListenerRow, type StaleItem } from "@/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

function StalePortsCard() {
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

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <CardTitle>幽霊ポート（削除済み worktree の残骸）</CardTitle>
          {stale.length > 0 && (
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
          )}
        </div>
        <p className="text-sm text-muted-foreground">
          <code>wt tree rm</code> を経由せず消された worktree が{" "}
          <code>.worktrees.json</code> に <code>port_base</code>{" "}
          だけ残した残骸です。ポート帯を死蔵し、割当枯渇の原因になります。掃除すると
          該当ブロックが回収されます（登録の削除のみ・ファイルは触りません）。
        </p>
      </CardHeader>
      <CardContent>
        {prune.isError && (
          <p className="mb-3 text-sm text-red-600">
            {(prune.error as Error).message}
          </p>
        )}
        {prune.isSuccess && (
          <p className="mb-3 text-sm text-green-700">
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
          <Table wrapperClassName="max-h-[calc(100vh-250px)]">
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
        )}
      </CardContent>
    </Card>
  );
}

export function PortsPage() {
  const {
    data: listeners = [],
    refetch,
    isFetching,
  } = useQuery<ListenerRow[]>({
    queryKey: ["port-listeners"],
    queryFn: portsApi.listeners,
  });

  return (
    <div className="space-y-6">
      <StalePortsCard />
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>稼働中ポート（マシン全体）</CardTitle>
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
            この PC で LISTEN 中の全 TCP ポート。<strong>wt</strong>（worktree に割当）か{" "}
            <strong>foreign</strong>（別プロジェクト等の占有）かを表示します。ポート衝突の原因特定に使えます。
            <br />
            各 worktree のポート割当・サーバー起動は{" "}
            <Link to="/" className="text-blue-600 hover:underline">
              Worktrees
            </Link>{" "}
            一覧で確認・操作できます。
          </p>
        </CardHeader>
        <CardContent>
          {listeners.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              LISTEN 中のポートがありません（ss が無い環境かも）
            </p>
          ) : (
            <Table wrapperClassName="max-h-[calc(100vh-250px)]">
              <TableHeader className="sticky top-0 z-10 bg-background shadow-[0_1px_2px_rgba(0,0,0,0.1)]">
                <TableRow>
                  <TableHead>ポート</TableHead>
                  <TableHead>プロセス</TableHead>
                  <TableHead>PID</TableHead>
                  <TableHead>区分</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {listeners.map((l) => (
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
    </div>
  );
}
