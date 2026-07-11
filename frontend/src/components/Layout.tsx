import { Outlet, NavLink } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { cn } from "@/lib/utils";
import {
  portsApi,
  statsApi,
  formatBytes,
  type PortItem,
  type ProcessStatsResponse,
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
    <div className="ml-auto flex items-center gap-3 text-xs text-primary-foreground/70">
      <span title="inotify instances (使用/上限)">
        inotify {data.inotify_instances}/{data.inotify_max}
      </span>
      <span title="wt 管理プロセスの合計メモリ">
        mem {formatBytes(data.total_rss_bytes)}
      </span>
    </div>
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
            <NavLink to="/gc" className={navLinkClass}>
              GC
            </NavLink>
            <NavLink to="/ports" className={navLinkClass}>
              Ports
            </NavLink>
            <NavLink to="/settings" className={navLinkClass}>
              Settings
            </NavLink>
          </nav>
          <HeaderStats />
        </div>
      </header>
      <main className="mx-auto w-full min-w-0 max-w-5xl flex-1 px-0 py-8 sm:px-4">
        <Outlet />
      </main>
    </div>
  );
}
