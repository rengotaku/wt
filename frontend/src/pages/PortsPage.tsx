import { useQuery } from "@tanstack/react-query";
import { portsApi, type PortItem, type PortState } from "@/api";
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

function LiveCell({ ports }: { ports: PortState[] }) {
  if (ports.length === 0) {
    return <span className="text-muted-foreground">—</span>;
  }
  const up = ports.filter((p) => p.listening);
  if (up.length === 0) {
    return <span className="text-muted-foreground text-xs">idle</span>;
  }
  return (
    <span className="flex flex-wrap gap-1">
      {up.map((p) => (
        <span
          key={p.port}
          className="rounded bg-green-100 px-1.5 py-0.5 text-xs font-mono text-green-700"
          title={p.pid ? `PID ${p.pid}` : undefined}
        >
          {p.port}
          {p.proc ? ` ${p.proc}` : ""}
          {p.pid ? `(${p.pid})` : ""}
        </span>
      ))}
    </span>
  );
}

export function PortsPage() {
  const {
    data: ports = [],
    isLoading,
    isError,
    error,
    refetch,
    isFetching,
  } = useQuery<PortItem[]>({
    queryKey: ["ports"],
    queryFn: portsApi.list,
  });

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>開発ポート (9000-9999)</CardTitle>
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
            各 worktree に割り当てられたポートブロック（5ポート/ブロック）と稼働状況。
            稼働中のポートは緑色で表示されます。
          </p>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <p className="text-sm text-muted-foreground">読み込み中...</p>
          ) : isError ? (
            <p className="text-sm text-destructive">
              取得に失敗しました: {(error as Error)?.message}
            </p>
          ) : ports.length === 0 ? (
            <p className="text-sm text-muted-foreground">worktree がありません</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Repo</TableHead>
                  <TableHead>Worktree</TableHead>
                  <TableHead>ポート範囲</TableHead>
                  <TableHead>稼働</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {ports.map((p) => (
                  <TableRow key={`${p.repo}/${p.wt_name}`}>
                    <TableCell>{p.repo}</TableCell>
                    <TableCell className="font-mono text-xs">{p.wt_name}</TableCell>
                    <TableCell className="font-mono text-xs">
                      {p.port_range ?? (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell>
                      <LiveCell ports={p.ports} />
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
