import type { ReactNode } from "react";
import { Link } from "react-router";
import { useAuth } from "../lib/auth";
import { Button } from "@/components/ui/button";

export function DashboardLayout({ children, fluid }: { children: ReactNode; fluid?: boolean }) {
  const { user, logout } = useAuth();

  return (
    <div className={`flex flex-col bg-background dark:bg-card${fluid ? " h-screen" : " min-h-screen"}`}>
      <header>
        <div className="mx-auto flex items-center justify-between px-6 py-4">
          <h1 className="font-heading text-xl font-semibold tracking-tight">
            <Link to="/projects" className="hover:text-foreground/80">
              DeployKit
            </Link>
          </h1>
          <nav className="flex items-center gap-4">
            <span className="text-sm text-muted-foreground">
              {user?.name} ({user?.email})
            </span>
            <Button variant="outline" size="sm" onClick={logout}>
              Logout
            </Button>
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
