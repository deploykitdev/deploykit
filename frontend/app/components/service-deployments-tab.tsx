import { useState } from "react";
import { ChevronDownIcon, GitBranchIcon, MoreVerticalIcon } from "lucide-react";
import type { Deployment } from "@/lib/queries";

interface ServiceDeploymentsTabProps {
  deployments: Deployment[] | undefined;
  isLoading: boolean;
  error: unknown;
  activeDeploymentId: string | null;
}

function timeAgo(dateStr: string): string {
  const seconds = Math.floor(
    (Date.now() - new Date(dateStr).getTime()) / 1000,
  );
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  const months = Math.floor(days / 30);
  if (months < 12) return `${months}mo ago`;
  return `${Math.floor(months / 12)}y ago`;
}

export function ServiceDeploymentsTab({
  deployments,
  isLoading,
  error,
  activeDeploymentId,
}: ServiceDeploymentsTabProps) {
  const [historyOpen, setHistoryOpen] = useState(true);

  if (isLoading) {
    return (
      <p className="text-sm text-muted-foreground">Loading deployments…</p>
    );
  }

  if (error) {
    return (
      <p className="text-sm text-destructive">
        Failed to load deployments.
      </p>
    );
  }

  const list = deployments ?? [];

  if (list.length === 0) {
    return (
      <div className="flex flex-col items-center gap-1 py-12 text-center">
        <p className="text-sm font-medium">No deployments yet</p>
        <p className="text-xs text-muted-foreground">
          Deployments will appear here once this service is deployed.
        </p>
      </div>
    );
  }

  const active = activeDeploymentId
    ? list.find((d) => d.id === activeDeploymentId) ?? null
    : null;
  const history = list.filter((d) => d.id !== active?.id);

  return (
    <div className="flex flex-col gap-4">
      {active ? <DeploymentRow deployment={active} status="active" /> : null}

      {history.length > 0 ? (
        <div className="flex flex-col gap-2">
          <button
            type="button"
            onClick={() => setHistoryOpen((v) => !v)}
            className="flex items-center gap-1.5 text-xs font-medium uppercase tracking-wider text-muted-foreground hover:text-foreground"
          >
            <ChevronDownIcon
              className={
                "size-4 transition-transform " +
                (historyOpen ? "" : "-rotate-90")
              }
            />
            History
          </button>
          {historyOpen ? (
            <div className="flex flex-col gap-2">
              {history.map((d) => (
                <DeploymentRow key={d.id} deployment={d} status="inactive" />
              ))}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

function DeploymentRow({
  deployment,
  status,
}: {
  deployment: Deployment;
  status: "active" | "inactive";
}) {
  const isActive = status === "active";
  return (
    <div
      className={
        "rounded-lg border p-3 " +
        (isActive
          ? "border-emerald-500/40 bg-emerald-500/5"
          : "bg-card hover:bg-muted/40")
      }
    >
      <div className="flex items-center gap-3">
        <span
          className={
            "rounded px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider " +
            (isActive
              ? "bg-emerald-500/20 text-emerald-300"
              : "bg-muted text-muted-foreground")
          }
        >
          {isActive ? "Active" : "Inactive"}
        </span>
        <GitBranchIcon className="size-4 shrink-0 text-muted-foreground" />
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium">{deployment.image}</div>
          <div className="text-xs text-muted-foreground">
            {timeAgo(deployment.created_at)}
          </div>
        </div>
        {isActive ? (
          <button
            type="button"
            disabled
            className="rounded border px-2.5 py-1 text-xs font-medium text-muted-foreground opacity-60"
            title="Logs not yet available"
          >
            View logs
          </button>
        ) : null}
        <button
          type="button"
          aria-label="More actions"
          className="flex size-7 items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          <MoreVerticalIcon className="size-4" />
        </button>
      </div>
    </div>
  );
}
