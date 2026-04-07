import { useSystemStatus } from "../../lib/queries";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import { cn } from "@/lib/utils";

export default function SettingsSystem() {
  const { data, isLoading } = useSystemStatus();

  if (isLoading || !data) {
    return (
      <p className="py-8 text-center text-sm text-muted-foreground">
        Loading…
      </p>
    );
  }

  const host = data.host;
  const dk = data.docker;

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Host</CardTitle>
          <CardDescription>
            {host.hostname || "—"} · up {formatDuration(host.uptime)}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Bar
            label="CPU"
            pct={host.cpu.usage_pct}
            right={
              <span className="font-mono text-xs text-muted-foreground">
                {host.cpu.cores} cores · load {host.cpu.load1.toFixed(2)} /{" "}
                {host.cpu.load5.toFixed(2)} / {host.cpu.load15.toFixed(2)}
              </span>
            }
          />
          <Bar
            label="Memory"
            pct={host.memory.usage_pct}
            right={
              <span className="font-mono text-xs text-muted-foreground">
                {formatBytes(host.memory.used_bytes)} /{" "}
                {formatBytes(host.memory.total_bytes)}
              </span>
            }
          />
          {host.swap.total_bytes > 0 && (
            <Bar
              label="Swap"
              pct={host.swap.usage_pct}
              right={
                <span className="font-mono text-xs text-muted-foreground">
                  {formatBytes(host.swap.used_bytes)} /{" "}
                  {formatBytes(host.swap.total_bytes)}
                </span>
              }
            />
          )}
          {host.disks.map((d) => (
            <Bar
              key={d.mountpoint}
              label={d.mountpoint}
              labelMono
              pct={d.usage_pct}
              right={
                <span className="font-mono text-xs text-muted-foreground">
                  {formatBytes(d.used_bytes)} / {formatBytes(d.total_bytes)}
                </span>
              }
            />
          ))}
        </CardContent>
      </Card>

      <div className="grid gap-6 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Docker</CardTitle>
            <CardDescription>
              {dk.reachable ? "Connected" : "Unreachable"}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {!dk.reachable ? (
              <p className="text-sm text-muted-foreground">
                Docker daemon is not responding.
              </p>
            ) : (
              <dl className="grid grid-cols-[max-content_1fr] gap-x-6 gap-y-2 text-sm">
                <Row k="Containers">
                  {dk.containers_running} running / {dk.containers} total
                </Row>
                <Row k="Stopped">{dk.containers_stopped}</Row>
                <Row k="Images">
                  {dk.images} · {formatBytes(dk.images_size_bytes)}
                </Row>
                <Row k="Volumes">
                  {dk.volumes} · {formatBytes(dk.volumes_size_bytes)}
                </Row>
                <Row k="Build cache">{formatBytes(dk.build_cache_bytes)}</Row>
              </dl>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>DeployKit</CardTitle>
            <CardDescription>Managed objects.</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              Project and service counts coming soon.
            </p>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function Bar({
  label,
  pct,
  right,
  labelMono,
}: {
  label: string;
  pct: number;
  right?: React.ReactNode;
  labelMono?: boolean;
}) {
  const clamped = Math.max(0, Math.min(100, pct || 0));
  const color =
    clamped >= 90
      ? "bg-red-500"
      : clamped >= 70
        ? "bg-yellow-500"
        : "bg-emerald-500";

  return (
    <div>
      <div className="mb-1 flex items-baseline justify-between gap-3">
        <span
          className={cn(
            "text-sm font-medium",
            labelMono && "font-mono text-xs",
          )}
        >
          {label}
        </span>
        <div className="flex items-baseline gap-3">
          {right}
          <span className="w-12 text-right font-mono text-sm tabular-nums">
            {clamped.toFixed(0)}%
          </span>
        </div>
      </div>
      <div className="h-2 w-full overflow-hidden rounded-full bg-muted">
        <div
          className={cn("h-full transition-all duration-500", color)}
          style={{ width: `${clamped}%` }}
        />
      </div>
    </div>
  );
}

function Row({ k, children }: { k: string; children: React.ReactNode }) {
  return (
    <div className="contents">
      <dt className="text-muted-foreground">{k}</dt>
      <dd className="font-mono">{children}</dd>
    </div>
  );
}

function formatBytes(n: number): string {
  if (!n || n < 1024) return `${n || 0} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 10 ? 0 : 1)} ${units[i]}`;
}

function formatDuration(sec: number): string {
  if (!sec || sec < 0) return "—";
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}
