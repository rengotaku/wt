import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { portsApi, type ListenerRow } from "@/api";
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
            <Table>
              <TableHeader>
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
