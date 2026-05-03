import { useState } from "react";
import {
  AlertCircleIcon,
  ChevronDownIcon,
  GitBranchIcon,
  Loader2Icon,
  MoreVerticalIcon,
} from "lucide-react";
import type { Deployment, DeploymentStatus } from "@/lib/queries";

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

  // The reconciler-promoted "active" deployment (status=healthy, pointed at by
  // services.active_deployment_id) is the one currently serving. It may not be
  // the most recent — a pending/starting/failed deployment is shown above it
  // as in-flight, but doesn't replace it until promotion.
  const active = activeDeploymentId
    ? list.find((d) => d.id === activeDeploymentId) ?? null
    : null;

  const inFlight = list.filter(
    (d) =>
      d.id !== active?.id &&
      (d.status === "pending" ||
        d.status === "starting" ||
        d.status === "failed"),
  );
  const history = list.filter(
    (d) => d.id !== active?.id && !inFlight.includes(d),
  );

  return (
    <div className="flex flex-col gap-4">
      {inFlight.map((d) => (
        <DeploymentRow key={d.id} deployment={d} active={false} />
      ))}

      {active ? <DeploymentRow deployment={active} active={true} /> : null}

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
                <DeploymentRow key={d.id} deployment={d} active={false} />
              ))}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

function statusPillClasses(status: DeploymentStatus, active: boolean): string {
  if (active) return "bg-emerald-500/20 text-emerald-300";
  switch (status) {
    case "starting":
    case "pending":
      return "bg-blue-500/20 text-blue-300";
    case "failed":
      return "bg-red-500/20 text-red-300";
    case "cancelled":
      return "bg-muted text-muted-foreground line-through";
    case "superseded":
    case "healthy":
    default:
      return "bg-muted text-muted-foreground";
  }
}

function statusLabel(status: DeploymentStatus, active: boolean): string {
  if (active) return "Active";
  switch (status) {
    case "pending":
      return "Pending";
    case "starting":
      return "Starting";
    case "healthy":
      return "Healthy";
    case "failed":
      return "Failed";
    case "superseded":
      return "Superseded";
    case "cancelled":
      return "Cancelled";
    default:
      return status;
  }
}

function DeploymentRow({
  deployment,
  active,
}: {
  deployment: Deployment;
  active: boolean;
}) {
  const isFailed = deployment.status === "failed";
  const isInFlight =
    deployment.status === "pending" || deployment.status === "starting";

  return (
    <div
      className={
        "rounded-lg border p-3 " +
        (active
          ? "border-emerald-500/40 bg-emerald-500/5"
          : isFailed
            ? "border-red-500/40 bg-red-500/5"
            : "bg-card hover:bg-muted/40")
      }
    >
      <div className="flex items-center gap-3">
        <span
          className={
            "rounded px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider " +
            statusPillClasses(deployment.status, active)
          }
        >
          {statusLabel(deployment.status, active)}
        </span>
        {isInFlight ? (
          <Loader2Icon className="size-4 shrink-0 animate-spin text-blue-400" />
        ) : isFailed ? (
          <AlertCircleIcon className="size-4 shrink-0 text-red-400" />
        ) : (
          <GitBranchIcon className="size-4 shrink-0 text-muted-foreground" />
        )}
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium">{deployment.image}</div>
          <div className="text-xs text-muted-foreground">
            {timeAgo(deployment.created_at)}
            {deployment.attempt_count > 1 && isFailed
              ? ` · ${deployment.attempt_count} attempts`
              : null}
          </div>
        </div>
        {active ? (
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
      {isFailed ? (
        <div className="mt-2 space-y-1.5 rounded bg-red-500/10 px-2 py-1.5 text-xs text-red-300">
          {deployment.failure_reason ? (
            <div className="font-medium">{deployment.failure_reason}</div>
          ) : null}
          {typeof deployment.exit_code === "number" ? (
            <div className="text-red-300/80">
              Exit code: <span className="font-mono">{deployment.exit_code}</span>
            </div>
          ) : null}
          {deployment.log_tail ? (
            <pre className="max-h-48 overflow-auto whitespace-pre-wrap rounded bg-black/40 p-2 font-mono text-[11px] leading-snug text-red-100">
              {deployment.log_tail}
            </pre>
          ) : (
            <div className="text-red-300/60 italic">
              (no logs — container never started)
            </div>
          )}
        </div>
      ) : null}
    </div>
  );
}
