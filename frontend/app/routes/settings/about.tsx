import { useSystemAbout } from "../../lib/queries";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardAction,
} from "@/components/ui/card";
import { RefreshCw, AlertCircle } from "lucide-react";
import { SystemUpdateCard } from "@/components/system-update-card";

export default function SettingsAbout() {
  const { data, isLoading, refetch, isFetching } = useSystemAbout();

  if (isLoading || !data) {
    return (
      <p className="py-8 text-center text-sm text-muted-foreground">
        Loading…
      </p>
    );
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>DeployKit</CardTitle>
          <CardDescription>This running instance.</CardDescription>
          <CardAction>
            <Button
              size="sm"
              variant="outline"
              onClick={() => refetch()}
              disabled={isFetching}
            >
              <RefreshCw
                className={
                  "mr-1.5 size-3.5" + (isFetching ? " animate-spin" : "")
                }
              />
              Refresh
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent>
          <InfoGrid
            rows={[
              ["Version", data.deploykit.version],
              ["Go", data.deploykit.go_version],
              ["Started", formatDateTime(data.deploykit.started_at)],
              ["Uptime", formatUptimeSince(data.deploykit.started_at)],
            ]}
          />
        </CardContent>
      </Card>

      <SystemUpdateCard about={data} onUpgradeFinished={() => refetch()} />

      <Card>
        <CardHeader>
          <CardTitle>Docker daemon</CardTitle>
          <CardDescription>
            {data.docker.reachable ? "Connected" : "Unreachable"}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {!data.docker.reachable ? (
            <div className="flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
              <AlertCircle className="mt-0.5 size-4 shrink-0" />
              <div>
                <p className="font-medium">Docker is unreachable</p>
                {data.docker.error && (
                  <p className="mt-1 break-all font-mono text-xs opacity-80">
                    {data.docker.error}
                  </p>
                )}
              </div>
            </div>
          ) : (
            <>
              <InfoGrid
                rows={[
                  ["Server version", data.docker.server_version ?? "—"],
                  ["API version", data.docker.api_version ?? "—"],
                  ["OS", data.docker.os ?? "—"],
                  ["Kernel", data.docker.kernel_version ?? "—"],
                  ["Architecture", data.docker.architecture ?? "—"],
                  ["Storage driver", data.docker.storage_driver ?? "—"],
                  ["Logging driver", data.docker.logging_driver ?? "—"],
                  ["Cgroup driver", data.docker.cgroup_driver ?? "—"],
                  ["Root dir", data.docker.docker_root_dir ?? "—"],
                ]}
              />
              {data.docker.warnings && data.docker.warnings.length > 0 && (
                <div className="mt-4 rounded-md border border-yellow-500/30 bg-yellow-500/10 p-3 text-sm">
                  <p className="mb-1 font-medium text-yellow-700 dark:text-yellow-400">
                    Daemon warnings
                  </p>
                  <ul className="list-disc pl-4 text-yellow-700/90 dark:text-yellow-400/90">
                    {data.docker.warnings.map((w) => (
                      <li key={w}>{w}</li>
                    ))}
                  </ul>
                </div>
              )}
            </>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Database</CardTitle>
          <CardDescription>SQLite file backing this instance.</CardDescription>
        </CardHeader>
        <CardContent>
          <InfoGrid
            rows={[
              ["Path", data.database.path],
              ["Size", formatBytes(data.database.size_bytes)],
            ]}
          />
        </CardContent>
      </Card>
    </div>
  );
}

function InfoGrid({ rows }: { rows: Array<[string, string]> }) {
  return (
    <dl className="grid grid-cols-[max-content_1fr] gap-x-6 gap-y-2 text-sm">
      {rows.map(([k, v]) => (
        <div key={k} className="contents">
          <dt className="text-muted-foreground">{k}</dt>
          <dd className="break-all font-mono">{v}</dd>
        </div>
      ))}
    </dl>
  );
}

function formatDateTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

function formatUptimeSince(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime();
  if (Number.isNaN(ms) || ms < 0) return "—";
  const sec = Math.floor(ms / 1000);
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 10 ? 0 : 1)} ${units[i]}`;
}
