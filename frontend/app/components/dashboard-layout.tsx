import type { ReactNode } from "react";
import { Link } from "react-router";
import { useIsAdmin } from "../lib/auth";
import { AppLogo } from "./app-logo";
import { UserMenu } from "./user-menu";
import { Settings } from "lucide-react";

export function DashboardLayout({ children, fluid }: { children: ReactNode; fluid?: boolean }) {
  const isAdmin = useIsAdmin();

  return (
    <div className={`flex flex-col bg-background dark:bg-card${fluid ? " h-screen" : " min-h-screen"}`}>
      <header>
        <div className="mx-auto flex items-center justify-between px-6 py-4">
          <Link to="/projects" className="hover:text-foreground/80">
            <AppLogo className="h-7 w-auto" />
          </Link>
          <nav className="flex items-center gap-4">
            {isAdmin && (
              <Link
                to="/settings"
                className="text-muted-foreground transition-colors hover:text-foreground"
                title="Settings"
              >
                <Settings className="size-4" />
              </Link>
            )}
            <UserMenu />
          </nav>
        </div>
      </header>
      <div className={`border m-1.5 mt-0 rounded-xl bg-slate-50 dark:bg-background flex-1${fluid ? " flex flex-col overflow-hidden" : ""}`}>
        {fluid ? children : (
          <main className="mx-auto max-w-5xl px-6 py-8">
            {children}
          </main>
        )}
      </div>
    </div>
  );
}
