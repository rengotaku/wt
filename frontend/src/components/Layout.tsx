import { Outlet, NavLink } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { cn } from "@/lib/utils";
import {
  portsApi,
  statsApi,
  buildInfoApi,
  formatBytes,
  type PortItem,
  type ProcessStatsResponse,
  type BuildInfoResponse,
} from "@/api";

const navLinkClass = ({ isActive }: { isActive: boolean }) =>
  cn(
    // base はタップ目標を確保するため py を増やし、md(デスクトップ)で現状の py-2 に戻す。
    // 横スクロールナビ内で潰れないよう shrink-0。
    "shrink-0 rounded-md px-3 py-2.5 text-sm font-medium transition-colors md:py-2",
    isActive
      ? "bg-primary-foreground/15 text-primary-foreground"
      : "text-primary-foreground/80 hover:bg-primary-foreground/10 hover:text-primary-foreground"
  );

// ヘッダ右上の inotify/メモリ表示。queryKey・queryFn・refetchInterval は
// TreesPage.tsx の同名 useQuery と完全に一致させ、TanStack Query のキャッシュを
// 共有することで二重fetchを避ける。
function HeaderStats() {
  const { data: portItems = [] } = useQuery<PortItem[]>({
    queryKey: ["ports"],
    queryFn: portsApi.list,
    refetchOnWindowFocus: false,
    refetchInterval: (query) =>
      (query.state.data as PortItem[] | undefined)?.some((p) => p.running)
        ? 3000
        : false,
  });

  const { data } = useQuery<ProcessStatsResponse>({
    queryKey: ["process-stats"],
    queryFn: statsApi.list,
    refetchOnWindowFocus: false,
    refetchInterval: () => (portItems.some((p) => p.running) ? 10000 : false),
  });

  if (!data) return null;

  return (
    <div className="flex items-center gap-3 text-xs text-primary-foreground/70">
      <span title="inotify instances (使用/上限)">
        inotify {data.inotify_instances}/{data.inotify_max}
      </span>
      <span title="wt 管理プロセスの合計メモリ">
        mem {formatBytes(data.total_rss_bytes)}
      </span>
    </div>
  );
}

// 短縮 SHA。空文字なら "?" にフォールバック。
function shortSha(sha: string): string {
  return sha ? sha.slice(0, 7) : "?";
}

function fmtUnix(sec: number): string {
  if (!sec) return "?";
  const d = new Date(sec * 1000);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// ソース repo が稼働中バイナリの起動時刻より新しい commit を持っているとき、
// ヘッダに黄色バッジを出す。判定不能（source repo path 未埋め込み / 動的ビルド）
// では出さない（fail-closed）。API とロジックの詳細は
// internal/handler/buildinfo.go を参照。
function StaleBinaryBadge() {
  const { data } = useQuery<BuildInfoResponse>({
    queryKey: ["build-info"],
    queryFn: buildInfoApi.get,
    refetchOnWindowFocus: true,
    refetchInterval: 60_000,
    retry: false,
  });

  if (!data || data.is_dev || !data.is_stale) return null;

  const title =
    `稼働中のバイナリより新しい commit がソース repo (${data.source_repo}) にあります。\n` +
    `binary start: ${fmtUnix(data.start_time)} (build ${shortSha(data.build_commit)} @ ${fmtUnix(data.build_commit_time)})\n` +
    `head [${data.head_branch || "?"}]: ${shortSha(data.head_commit)} @ ${fmtUnix(data.head_commit_time)}\n` +
    `再ビルド → 差し替え → 再起動が必要です（wt-web-reflect スキル参照）。`;

  return (
    <span
      title={title}
      className="rounded-md bg-amber-400/95 px-2 py-0.5 text-xs font-semibold text-amber-950 shadow-sm"
      data-testid="stale-binary-badge"
    >
      ⚠ バイナリ古い可能性
    </span>
  );
}

export function Layout() {
  return (
    <div className="flex min-h-screen flex-col bg-background">
      <header className="bg-primary text-primary-foreground shadow">
        <div className="mx-auto flex h-14 min-w-0 max-w-5xl items-center px-3 sm:px-4">
          <img src="/favicon.ico" alt="wt" className="mr-6 h-8 w-8 shrink-0" />
          {/* 狭幅ではナビを横スクロール可能にしてはみ出しを防ぐ。md でデスクトップの
              通常配置（overflow 制御なし）に戻すので >=768px は現状と等価。 */}
          <nav className="flex min-w-0 items-center gap-1 overflow-x-auto md:overflow-x-visible">
            <NavLink to="/" end className={navLinkClass}>
              Worktrees
            </NavLink>
            <NavLink to="/repos" className={navLinkClass}>
              Repos
            </NavLink>
            <NavLink to="/maintenance" className={navLinkClass}>
              Maintenance
            </NavLink>
            <NavLink to="/settings" className={navLinkClass}>
              Settings
            </NavLink>
          </nav>
          <div className="ml-auto flex items-center gap-3">
            <StaleBinaryBadge />
            <HeaderStats />
          </div>
        </div>
      </header>
      <main className="mx-auto w-full min-w-0 max-w-5xl flex-1 px-0 py-8 sm:px-4">
        <Outlet />
      </main>
    </div>
  );
}
