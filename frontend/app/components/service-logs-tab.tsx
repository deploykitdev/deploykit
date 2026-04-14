import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { ArrowDownIcon, PauseIcon, PlayIcon, Trash2Icon } from "lucide-react";
import { cn } from "@/lib/utils";
import { useLogStream, type LogItem, type LogStreamStatus } from "@/lib/use-log-stream";

interface ServiceLogsTabProps {
  projectId: string;
  serviceId: string;
  active: boolean;
}

const CONTAINER_COLORS = [
  "text-sky-400",
  "text-emerald-400",
  "text-violet-400",
  "text-amber-400",
  "text-pink-400",
  "text-teal-400",
];

function colorFor(containerId: string): string {
  let h = 0;
  for (let i = 0; i < containerId.length; i++) {
    h = (h * 31 + containerId.charCodeAt(i)) >>> 0;
  }
  return CONTAINER_COLORS[h % CONTAINER_COLORS.length];
}

export function ServiceLogsTab({ projectId, serviceId, active }: ServiceLogsTabProps) {
  const [paused, setPaused] = useState(false);
  const enabled = active && !paused;

  const { items, status, error, clear, reconnect } = useLogStream(
    projectId,
    serviceId,
    enabled,
  );

  const scrollRef = useRef<HTMLDivElement | null>(null);
  const [autoScroll, setAutoScroll] = useState(true);

  useLayoutEffect(() => {
    if (!autoScroll) return;
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [items, autoScroll]);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const onScroll = () => {
      const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
      setAutoScroll(atBottom);
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, []);

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between gap-3 border-b px-5 py-2.5">
        <div className="flex items-center gap-2">
          <StatusBadge status={status} paused={paused} />
          <span className="text-xs text-muted-foreground">
            {items.length} {items.length === 1 ? "line" : "lines"}
          </span>
        </div>
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={() => setPaused((p) => !p)}
            aria-label={paused ? "Resume" : "Pause"}
            className="flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            {paused ? <PlayIcon className="size-3.5" /> : <PauseIcon className="size-3.5" />}
          </button>
          <button
            type="button"
            onClick={clear}
            aria-label="Clear"
            className="flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <Trash2Icon className="size-3.5" />
          </button>
        </div>
      </div>

      <div className="relative flex-1 overflow-hidden bg-neutral-950">
        <div
          ref={scrollRef}
          className="h-full overflow-y-auto px-4 py-3 font-mono text-xs leading-relaxed"
        >
          {items.length === 0 ? (
            <EmptyState status={status} error={error} onReconnect={reconnect} />
          ) : (
            items.map((item) => <LogRow key={item.id} item={item} />)
          )}
        </div>

        {!autoScroll && items.length > 0 ? (
          <button
            type="button"
            onClick={() => {
              setAutoScroll(true);
              const el = scrollRef.current;
              if (el) el.scrollTop = el.scrollHeight;
            }}
            className="absolute right-4 bottom-4 flex items-center gap-1.5 rounded-full bg-foreground px-3 py-1.5 text-xs font-medium text-background shadow-lg transition-opacity hover:opacity-90"
          >
            <ArrowDownIcon className="size-3" />
            Jump to latest
          </button>
        ) : null}
      </div>
    </div>
  );
}

function LogRow({ item }: { item: LogItem }) {
  const color = colorFor(item.containerId);
  if ("kind" in item) {
    return (
      <div className="flex gap-2 py-0.5 text-muted-foreground italic">
        <span className={cn("shrink-0", color)}>{item.containerId}</span>
        <span>— container exited</span>
      </div>
    );
  }
  return (
    <div className="flex gap-2 py-0.5">
      <span className={cn("shrink-0", color)}>{item.containerId}</span>
      <span
        className={cn(
          "whitespace-pre-wrap break-all",
          item.stream === "stderr" ? "text-red-400" : "text-neutral-100",
        )}
      >
        {item.line}
      </span>
    </div>
  );
}

function StatusBadge({ status, paused }: { status: LogStreamStatus; paused: boolean }) {
  if (paused) {
    return <Badge color="bg-muted-foreground/50" label="Paused" />;
  }
  switch (status) {
    case "connecting":
      return <Badge color="bg-amber-500" label="Connecting" pulse />;
    case "streaming":
      return <Badge color="bg-emerald-500" label="Live" pulse />;
    case "ended":
      return <Badge color="bg-muted-foreground/50" label="Ended" />;
    case "error":
      return <Badge color="bg-red-500" label="Error" />;
    default:
      return <Badge color="bg-muted-foreground/50" label="Idle" />;
  }
}

function Badge({ color, label, pulse }: { color: string; label: string; pulse?: boolean }) {
  return (
    <span className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
      <span className={cn("inline-block size-2 rounded-full", color, pulse && "animate-pulse")} />
      {label}
    </span>
  );
}

function EmptyState({
  status,
  error,
  onReconnect,
}: {
  status: LogStreamStatus;
  error: string | null;
  onReconnect: () => void;
}) {
  if (status === "error") {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-neutral-400">
        <p className="text-sm">Log stream error</p>
        {error ? <p className="max-w-xs text-xs text-neutral-500">{error}</p> : null}
        <button
          type="button"
          onClick={onReconnect}
          className="mt-2 rounded-md bg-neutral-800 px-3 py-1.5 text-xs font-medium text-neutral-100 hover:bg-neutral-700"
        >
          Reconnect
        </button>
      </div>
    );
  }
  return (
    <div className="flex h-full items-center justify-center text-neutral-500">
      <p className="text-xs">
        {status === "connecting" ? "Connecting…" : "No log output yet."}
      </p>
    </div>
  );
}
