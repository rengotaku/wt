import { Outlet, NavLink } from "react-router-dom";
import { cn } from "@/lib/utils";

const navLinkClass = ({ isActive }: { isActive: boolean }) =>
  cn(
    // base はタップ目標を確保するため py を増やし、md(デスクトップ)で現状の py-2 に戻す。
    // 横スクロールナビ内で潰れないよう shrink-0。
    "shrink-0 rounded-md px-3 py-2.5 text-sm font-medium transition-colors md:py-2",
    isActive
      ? "bg-primary-foreground/15 text-primary-foreground"
      : "text-primary-foreground/80 hover:bg-primary-foreground/10 hover:text-primary-foreground"
  );

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
        </div>
      </header>
      <main className="mx-auto w-full min-w-0 max-w-5xl flex-1 px-0 py-8 sm:px-4">
        <Outlet />
      </main>
    </div>
  );
}
