import { useEffect, useState } from "react";
import { Handle, Position, type NodeProps } from "@xyflow/react";
import { Container } from "lucide-react";
import { cn } from "@/lib/utils";

export interface ServiceNodeData extends Record<string, unknown> {
  label: string;
  image?: string;
  serviceId?: string;
  status?: string;
  iconUrl?: string;
}

type StatusKey =
  | "created"
  | "deploying"
  | "running"
  | "degraded"
  | "failed"
  | "stopped";

const STATUS: Record<
  StatusKey,
  {
    label: string;
    bar: string;
    dot: string;
    text: string;
    dotGlow?: string;
  }
> = {
  running: {
    label: "Running",
    bar: "bg-emerald-500",
    dot: "bg-emerald-500",
    text: "text-emerald-600 dark:text-emerald-400",
    dotGlow: "0 0 14px 0 rgb(16 185 129 / 0.55)",
  },
  deploying: {
    label: "Deploying",
    bar: "bg-amber-400",
    dot: "bg-amber-400",
    text: "text-amber-600 dark:text-amber-400",
    dotGlow: "0 0 14px 0 rgb(251 191 36 / 0.6)",
  },
  degraded: {
    label: "Degraded",
    bar: "bg-amber-500",
    dot: "bg-amber-500",
    text: "text-amber-700 dark:text-amber-400",
    dotGlow: "0 0 10px 0 rgb(245 158 11 / 0.4)",
  },
  failed: {
    label: "Failed",
    bar: "bg-red-500",
    dot: "bg-red-500",
    text: "text-red-600 dark:text-red-400",
    dotGlow: "0 0 14px 0 rgb(239 68 68 / 0.55)",
  },
  stopped: {
    label: "Stopped",
    bar: "bg-muted-foreground/30",
    dot: "bg-muted-foreground/60",
    text: "text-muted-foreground",
  },
  created: {
    label: "Created",
    bar: "bg-muted-foreground/25",
    dot: "bg-muted-foreground/50",
    text: "text-muted-foreground",
  },
};

export function ServiceNode({ data, selected }: NodeProps) {
  const { label, image, status, iconUrl } = data as ServiceNodeData;
  const key: StatusKey =
    status && status in STATUS ? (status as StatusKey) : "created";
  const meta = STATUS[key];
  const isDeploying = key === "deploying";

  const [iconBroken, setIconBroken] = useState(false);
  useEffect(() => {
    setIconBroken(false);
  }, [iconUrl]);
  const showCustomIcon = Boolean(iconUrl) && !iconBroken;

  return (
    <div
      className={cn(
        "group relative w-80 overflow-hidden rounded-xl border bg-card text-card-foreground",
        "shadow-md transition-[transform,border-color,box-shadow] duration-200 ease-out",
        "hover:-translate-y-px hover:shadow-lg",
        selected
          ? "border-primary/70 ring-[3px] ring-primary/15"
          : "border-border/80 hover:border-foreground/25",
      )}
    >
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 opacity-40 dark:opacity-25"
        style={{
          backgroundImage:
            "radial-gradient(circle at 1px 1px, currentColor 0.6px, transparent 0)",
          backgroundSize: "14px 14px",
          color: "var(--muted-foreground)",
          WebkitMaskImage:
            "radial-gradient(ellipse 85% 100% at 100% 0%, black 10%, transparent 75%)",
          maskImage:
            "radial-gradient(ellipse 85% 100% at 100% 0%, black 10%, transparent 75%)",
        }}
      />

      <div
        aria-hidden
        className={cn(
          "absolute inset-y-0 left-0 w-[3px] overflow-hidden transition-colors",
          meta.bar,
        )}
      >
        {isDeploying ? (
          <span
            className="absolute inset-x-0 h-1/3 bg-gradient-to-b from-transparent via-white/80 to-transparent"
            style={{
              animation: "service-node-scan 1.6s ease-in-out infinite",
            }}
          />
        ) : null}
      </div>

      <Handle
        type="target"
        position={Position.Left}
        className="!h-0 !w-0 !min-h-0 !min-w-0 !border-0 !bg-transparent !opacity-0"
      />

      <div className="relative px-5 pt-4 pb-4">
        <div className="flex items-center justify-between">
          <span className="font-mono text-[10px] font-medium uppercase tracking-[0.22em] text-muted-foreground/70">
            Service
          </span>
          {showCustomIcon ? (
            <img
              src={iconUrl}
              alt=""
              className="size-7 object-contain"
              onError={() => setIconBroken(true)}
              draggable={false}
            />
          ) : (
            <div className="grid size-9 place-items-center rounded-md border border-border/80 bg-background/60 shadow-[inset_0_1px_0_rgb(255_255_255/0.04)]">
              <Container
                className="size-5 text-muted-foreground"
                strokeWidth={1.75}
              />
            </div>
          )}
        </div>

        <h3 className="mt-3 truncate text-xl font-semibold leading-tight tracking-tight">
          {label}
        </h3>

        <code
          className={cn(
            "mt-4 block truncate font-mono text-xs leading-[1.35]",
            image
              ? "text-muted-foreground/70"
              : "italic text-muted-foreground/45",
          )}
        >
          {image ?? "— awaiting deployment"}
        </code>
      </div>

      <div className="relative flex items-center gap-3 border-t border-dashed border-border/70 px-5 py-3">
        <span className="relative flex size-2 items-center justify-center">
          <span
            className={cn("block size-2 rounded-full", meta.dot)}
            style={meta.dotGlow ? { boxShadow: meta.dotGlow } : undefined}
          />
          {isDeploying ? (
            <span
              className={cn(
                "absolute size-3 rounded-full opacity-50",
                meta.dot,
              )}
              style={{
                animation: "ping 1.6s cubic-bezier(0, 0, 0.2, 1) infinite",
              }}
            />
          ) : null}
        </span>
        <span
          className={cn(
            "font-mono text-[10px] font-semibold uppercase tracking-[0.24em]",
            meta.text,
          )}
        >
          {meta.label}
        </span>
      </div>

      <Handle
        type="source"
        position={Position.Right}
        className="!h-0 !w-0 !min-h-0 !min-w-0 !border-0 !bg-transparent !opacity-0"
      />
    </div>
  );
}
