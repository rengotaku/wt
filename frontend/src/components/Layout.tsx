import { Outlet, NavLink } from "react-router-dom";
import { cn } from "@/lib/utils";

const navLinkClass = ({ isActive }: { isActive: boolean }) =>
  cn(
    "rounded-md px-3 py-2 text-sm font-medium transition-colors",
    isActive
      ? "bg-primary-foreground/15 text-primary-foreground"
      : "text-primary-foreground/80 hover:bg-primary-foreground/10 hover:text-primary-foreground"
  );

export function Layout() {
  return (
    <div className="flex min-h-screen flex-col bg-background">
      <header className="bg-primary text-primary-foreground shadow">
        <div className="mx-auto flex h-14 max-w-5xl items-center px-4">
          <img src="/favicon.ico" alt="wt" className="mr-6 h-8 w-8" />
          <nav className="flex items-center gap-1">
            <NavLink to="/" end className={navLinkClass}>
              Worktrees
            </NavLink>
            <NavLink to="/repos" className={navLinkClass}>
              Repos
            </NavLink>
            <NavLink to="/gc" className={navLinkClass}>
              GC
            </NavLink>
          </nav>
        </div>
      </header>
      <main className="mx-auto w-full max-w-5xl flex-1 px-4 py-8">
        <Outlet />
      </main>
    </div>
  );
}
