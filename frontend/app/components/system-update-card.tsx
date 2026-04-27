import { useEffect, useState } from "react";
import {
  AlertCircle,
  ArrowUpCircle,
  CheckCircle2,
  Loader2,
  RefreshCw,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  type SystemAbout,
  type SystemSettings,
  useRefreshLatestRelease,
  useRequestUpgrade,
  useSystemSettings,
  useUpdateSystemSettings,
  useUpgradeStatus,
} from "@/lib/queries";
import { toast } from "sonner";

type Props = {
  about: SystemAbout;
  // onUpgradeFinished is invoked when the upgrade goroutine reports a
  // terminal state — the parent can refetch /system/about to pick up the
  // new running version.
  onUpgradeFinished?: () => void;
};

export function SystemUpdateCard({ about, onUpgradeFinished }: Props) {
  const [confirmOpen, setConfirmOpen] = useState(false);

  // Poll the upgrade status only when the user has at least kicked one
  // off (or when the runner reports it's still busy). 1.5s is brisk
  // enough for log streaming without hammering the disk.
  const [active, setActive] = useState(false);
  const upgradeStatus = useUpgradeStatus({
    enabled: true,
    refetchMs: active ? 1500 : undefined,
  });
  const status = upgradeStatus.data;

  useEffect(() => {
    if (!status) return;
    const busy = status.state === "queued" || status.state === "running";
    setActive(busy);
    if (status.state === "done" || status.state === "failed") {
      onUpgradeFinished?.();
    }
  }, [status, onUpgradeFinished]);

  const refresh = useRefreshLatestRelease();
  const requestUpgrade = useRequestUpgrade();

  const settings = useSystemSettings();
  const updateSettings = useUpdateSystemSettings();

  const latest = about.latest_release;
  const updateAvailable = about.update_available;

  const onUpgrade = async () => {
    if (!latest) return;
    try {
      await requestUpgrade.mutateAsync(latest.version);
      setActive(true);
      setConfirmOpen(false);
      toast.success(`Upgrade to ${latest.version} queued.`);
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Failed to start upgrade.",
      );
    }
  };

  const onToggleAutoUpdate = async (next: boolean) => {
    try {
      await updateSettings.mutateAsync({ auto_update: next });
    } catch (err) {
      toast.error(
        err instanceof Error
          ? err.message
          : "Failed to save auto-update preference.",
      );
    }
  };

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>Updates</CardTitle>
          <CardDescription>
            {updateAvailable && latest ? (
              <span className="text-foreground">
                {latest.version} available — you're on{" "}
                <span className="font-mono">{about.deploykit.version}</span>
              </span>
            ) : latest ? (
              <span>Up to date ({latest.version}).</span>
            ) : (
              <span>Checking upstream releases…</span>
            )}
          </CardDescription>
          <CardAction>
            <Button
              size="sm"
              variant="outline"
              onClick={() => refresh.mutate()}
              disabled={refresh.isPending}
            >
              <RefreshCw
                className={
                  "mr-1.5 size-3.5" + (refresh.isPending ? " animate-spin" : "")
                }
              />
              Check now
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent className="space-y-4">
          {updateAvailable && latest ? (
            <div className="flex items-start justify-between gap-4 rounded-md border border-blue-500/30 bg-blue-500/5 p-3">
              <div className="flex items-start gap-2 text-sm">
                <ArrowUpCircle className="mt-0.5 size-4 shrink-0 text-blue-500" />
                <div>
                  <p className="font-medium text-foreground">
                    {latest.version} is available
                  </p>
                  {latest.url && (
                    <a
                      href={latest.url}
                      target="_blank"
                      rel="noreferrer"
                      className="text-xs text-muted-foreground underline underline-offset-3 hover:text-foreground"
                    >
                      Release notes
                    </a>
                  )}
                </div>
              </div>
              <Button
                size="sm"
                onClick={() => setConfirmOpen(true)}
                disabled={active}
              >
                {active ? (
                  <>
                    <Loader2 className="mr-1.5 size-3.5 animate-spin" />
                    Upgrading
                  </>
                ) : (
                  "Update now"
                )}
              </Button>
            </div>
          ) : null}

          <UpgradeProgress status={status} />

          <AutoUpdateRow
            settings={settings.data}
            isLoading={settings.isLoading}
            isSaving={updateSettings.isPending}
            onToggle={onToggleAutoUpdate}
          />
        </CardContent>
      </Card>

      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Upgrade DeployKit?</DialogTitle>
            <DialogDescription>
              {latest && (
                <>
                  This will install{" "}
                  <span className="font-mono">{latest.version}</span> and
                  restart the API. You may briefly lose connection — the page
                  will reconnect once the new version is up.
                </>
              )}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose
              render={(props) => (
                <Button {...props} variant="outline">
                  Cancel
                </Button>
              )}
            />
            <Button onClick={onUpgrade} disabled={requestUpgrade.isPending}>
              {requestUpgrade.isPending ? (
                <>
                  <Loader2 className="mr-1.5 size-3.5 animate-spin" />
                  Starting
                </>
              ) : (
                "Upgrade"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function UpgradeProgress({
  status,
}: {
  status: ReturnType<typeof useUpgradeStatus>["data"];
}) {
  if (!status || status.state === "idle") return null;

  return (
    <div className="rounded-md border border-border bg-muted/30 p-3 text-sm">
      <div className="flex items-center gap-2">
        {status.state === "running" || status.state === "queued" ? (
          <Loader2 className="size-4 animate-spin text-blue-500" />
        ) : status.state === "done" ? (
          <CheckCircle2 className="size-4 text-emerald-500" />
        ) : (
          <AlertCircle className="size-4 text-destructive" />
        )}
        <span className="font-medium capitalize">{status.state}</span>
        {status.target_version && (
          <span className="text-muted-foreground">
            → <span className="font-mono">{status.target_version}</span>
          </span>
        )}
      </div>
      {status.error && (
        <p className="mt-2 break-all text-destructive">{status.error}</p>
      )}
      {status.log_tail && (
        <pre className="mt-2 max-h-40 overflow-auto rounded bg-background p-2 font-mono text-xs leading-snug">
          {status.log_tail}
        </pre>
      )}
    </div>
  );
}

function AutoUpdateRow({
  settings,
  isLoading,
  isSaving,
  onToggle,
}: {
  settings: SystemSettings | undefined;
  isLoading: boolean;
  isSaving: boolean;
  onToggle: (next: boolean) => void;
}) {
  const enabled = settings?.auto_update ?? false;
  return (
    <div className="flex items-center justify-between rounded-md border border-border p-3">
      <div>
        <p className="text-sm font-medium">Automatic updates</p>
        <p className="text-xs text-muted-foreground">
          Daily check for new releases. When enabled, deploykit will install
          the latest version automatically — skipped if a deployment is in
          progress.
        </p>
      </div>
      <Button
        size="sm"
        variant={enabled ? "default" : "outline"}
        onClick={() => onToggle(!enabled)}
        disabled={isLoading || isSaving}
        aria-pressed={enabled}
      >
        {enabled ? "On" : "Off"}
      </Button>
    </div>
  );
}
