import { NavLink, Outlet } from "react-router";
import { RequireAdmin } from "../lib/auth";
import { DashboardLayout } from "@/components/dashboard-layout";
import { cn } from "@/lib/utils";

export default function SettingsLayout() {
  return (
    <RequireAdmin>
      <DashboardLayout>
        <h1 className="mt-4 mb-8 text-2xl font-semibold">Settings</h1>
        <nav className="flex gap-1 border-b border-border mb-6">
          <SettingsNavLink to="/settings" end>
            General
          </SettingsNavLink>
          <SettingsNavLink to="/settings/users">Users</SettingsNavLink>
        </nav>
        <Outlet />
      </DashboardLayout>
    </RequireAdmin>
  );
}

function SettingsNavLink({
  to,
  end,
  children,
}: {
  to: string;
  end?: boolean;
  children: React.ReactNode;
}) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) =>
        cn(
          "-mb-px border-b-2 px-3 pb-2.5 text-sm font-medium transition-colors",
          isActive
            ? "border-primary text-foreground"
            : "border-transparent text-muted-foreground hover:text-foreground",
        )
      }
    >
      {children}
    </NavLink>
  );
}
