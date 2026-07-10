import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { type WorktreeProcessStats, formatBytes, formatDuration } from "@/api/stats";

interface ProcessStatsOverlayProps {
  stats: WorktreeProcessStats;
  warnBytes: number;
  dangerBytes: number;
  onClose: () => void;
}

export function ProcessStatsOverlay({
  stats,
  warnBytes,
  dangerBytes,
  onClose,
}: ProcessStatsOverlayProps) {
  const badgeColors = {
    ok: "bg-muted text-muted-foreground",
    warn: "bg-amber-100 text-amber-700",
    danger: "bg-red-100 text-red-700",
  };

  return (
    <div
      className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4"
      onClick={onClose}
    >
      <Card
        className="w-[48rem] max-w-[92vw] max-h-[90vh] flex flex-col"
        onClick={(e) => e.stopPropagation()}
      >
        <CardHeader className="shrink-0 border-b pb-4">
          <div className="flex items-center justify-between">
            <CardTitle className="flex items-center gap-2">
              <span className="font-mono text-sm">
                {stats.repo}/{stats.wt_name}
              </span>
              <span
                className={`text-xs px-2 py-0.5 rounded-full ${
                  badgeColors[stats.level]
                }`}
              >
                {formatBytes(stats.total_rss_bytes)}
              </span>
            </CardTitle>
            <Button variant="ghost" size="sm" onClick={onClose} aria-label="閉じる">
              <X className="h-4 w-4" />
            </Button>
          </div>
        </CardHeader>
        <CardContent className="overflow-y-auto pt-4">
          <Table>
            <TableHeader className="sticky top-0 z-10 bg-background">
              <TableRow>
                <TableHead>サービス</TableHead>
                <TableHead>PID</TableHead>
                <TableHead>ポート</TableHead>
                <TableHead>状態</TableHead>
                <TableHead>プロセス数</TableHead>
                <TableHead>メモリ</TableHead>
                <TableHead>稼働時間</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {stats.services.map((s) => (
                <TableRow key={`${s.pid}-${s.name}`}>
                  <TableCell className="font-medium">{s.name}</TableCell>
                  <TableCell>{s.pid}</TableCell>
                  <TableCell>{s.port}</TableCell>
                  <TableCell>
                    {s.alive ? (
                      <span className="text-green-600">稼働</span>
                    ) : (
                      <span className="text-muted-foreground">停止</span>
                    )}
                  </TableCell>
                  <TableCell>{s.procs}</TableCell>
                  <TableCell>{formatBytes(s.rss_bytes)}</TableCell>
                  <TableCell>{s.alive ? formatDuration(s.uptime_sec) : "—"}</TableCell>
                </TableRow>
              ))}
              {stats.services.length === 0 && (
                <TableRow>
                  <TableCell colSpan={7} className="text-center text-muted-foreground">
                    サービスがありません
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
          <div className="mt-4 text-xs text-muted-foreground flex justify-between">
            <span>
              warn={formatBytes(warnBytes)} / danger={formatBytes(dangerBytes)}（settings.toml [process_stats] で変更可）
            </span>
            <Button variant="outline" size="sm" onClick={onClose}>
              閉じる
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
