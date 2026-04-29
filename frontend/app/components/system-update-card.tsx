import { useEffect, useLayoutEffect, useRef, useState } from "react";
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
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  type SystemAbout,
  type SystemSettings,
  type UpgradeStatus,
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

type DialogView = "closed" | "confirm" | "progress";

export function SystemUpdateCard({ about, onUpgradeFinished }: Props) {
  const [dialogView, setDialogView] = useState<DialogView>("closed");

  const refresh = useRefreshLatestRelease();
  const requestUpgrade = useRequestUpgrade();
  const settings = useSystemSettings();
  const updateSettings = useUpdateSystemSettings();

  const upgradeStatus = useUpgradeStatus({
    enabled: true,
    refetchMs: 1500,
  });
  const status = upgradeStatus.data;

  // keep showing running on transient query errors — install.sh restarts the
  // API mid-upgrade, so polls will fail for a few seconds; we don't want to
  // flash failed during that window.
  const busy =
    requestUpgrade.isPending ||
    status?.state === "queued" ||
    status?.state === "running";

  useEffect(() => {
    if (!status) return;
    if (status.state === "done" || status.state === "failed") {
      onUpgradeFinished?.();
    }
  }, [status, onUpgradeFinished]);

  const latest = about.latest_release;
  const updateAvailable = about.update_available;

  const onUpgrade = async () => {
    if (!latest) return;
    try {
      await requestUpgrade.mutateAsync(latest.version);
      setDialogView("progress");
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

  const onPrimaryClick = () => {
    setDialogView(busy ? "progress" : "confirm");
  };

  const onDialogOpenChange = (open: boolean) => {
    if (!open) setDialogView("closed");
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
          {(updateAvailable && latest) || busy ? (
            <div className="flex items-start justify-between gap-4 rounded-md border border-blue-500/30 bg-blue-500/5 p-3">
              <div className="flex items-start gap-2 text-sm">
                <ArrowUpCircle className="mt-0.5 size-4 shrink-0 text-blue-500" />
                <div>
                  <p className="font-medium text-foreground">
                    {busy
                      ? `Upgrading${
                          status?.target_version
                            ? ` to ${status.target_version}`
                            : "…"
                        }`
                      : `${latest!.version} is available`}
                  </p>
                  {!busy && latest?.url && (
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
              <Button size="sm" onClick={onPrimaryClick}>
                {busy ? (
                  <>
                    <Loader2 className="mr-1.5 size-3.5 animate-spin" />
                    View upgrade…
                  </>
                ) : (
                  "Update now"
                )}
              </Button>
            </div>
          ) : null}

          <UpgradeProgressBlock status={status} />

          <AutoUpdateRow
            settings={settings.data}
            isLoading={settings.isLoading}
            isSaving={updateSettings.isPending}
            onToggle={onToggleAutoUpdate}
          />
        </CardContent>
      </Card>

      <Dialog
        open={dialogView !== "closed"}
        onOpenChange={onDialogOpenChange}
      >
        <DialogContent>
          {dialogView === "confirm" ? (
            <ConfirmView
              version={latest?.version}
              onCancel={() => setDialogView("closed")}
              onConfirm={onUpgrade}
              starting={requestUpgrade.isPending}
            />
          ) : (
            <ProgressView
              status={status}
              starting={requestUpgrade.isPending}
              onClose={() => setDialogView("closed")}
            />
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}

function ConfirmView({
  version,
  onCancel,
  onConfirm,
  starting,
}: {
  version: string | undefined;
  onCancel: () => void;
  onConfirm: () => void;
  starting: boolean;
}) {
  return (
    <>
      <DialogHeader>
        <DialogTitle>Upgrade DeployKit?</DialogTitle>
        <DialogDescription>
          {version && (
            <>
              This will install{" "}
              <span className="font-mono">{version}</span> and restart the API.
              You may briefly lose connection — the page will reconnect once
              the new version is up.
            </>
          )}
        </DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <Button variant="outline" onClick={onCancel} disabled={starting}>
          Cancel
        </Button>
        <Button onClick={onConfirm} disabled={starting}>
          {starting ? (
            <>
              <Loader2 className="mr-1.5 size-3.5 animate-spin" />
              Starting
            </>
          ) : (
            "Upgrade"
          )}
        </Button>
      </DialogFooter>
    </>
  );
}

function ProgressView({
  status,
  starting,
  onClose,
}: {
  status: UpgradeStatus | undefined;
  starting: boolean;
  onClose: () => void;
}) {
  const stage = deriveStage(status, starting);

  let title = "Starting upgrade…";
  let description: React.ReactNode = null;
  let icon = (
    <Loader2 className="size-5 animate-spin text-blue-500" />
  );

  if (stage === "running") {
    title = status?.target_version
      ? `Installing ${status.target_version}…`
      : "Installing…";
    description = (
      <>
        The API will briefly restart. This dialog will keep streaming logs and
        report the result when it's done.
      </>
    );
  } else if (stage === "done") {
    title = status?.target_version
      ? `Upgrade complete — now running ${status.target_version}`
      : "Upgrade complete";
    description = "The new version is live.";
    icon = <CheckCircle2 className="size-5 text-emerald-500" />;
  } else if (stage === "failed") {
    title = "Upgrade failed";
    description = status?.error ?? "The installer reported an error.";
    icon = <AlertCircle className="size-5 text-destructive" />;
  } else if (stage === "starting") {
    description = "Queuing the install request…";
  }

  return (
    <>
      <DialogHeader>
        <div className="flex items-start gap-3">
          {icon}
          <div className="flex-1 min-w-0">
            <DialogTitle>{title}</DialogTitle>
            {description && (
              <DialogDescription className="mt-1 break-words">
                {description}
              </DialogDescription>
            )}
          </div>
        </div>
      </DialogHeader>

      <LogStream logTail={status?.log_tail} />

      <DialogFooter>
        {stage === "running" || stage === "starting" ? (
          <Button variant="outline" onClick={onClose}>
            Hide
          </Button>
        ) : (
          <Button onClick={onClose}>Close</Button>
        )}
      </DialogFooter>
    </>
  );
}

type Stage = "starting" | "running" | "done" | "failed";

function deriveStage(
  status: UpgradeStatus | undefined,
  starting: boolean,
): Stage {
  if (status?.state === "done") return "done";
  if (status?.state === "failed") return "failed";
  if (status?.state === "queued" || status?.state === "running") {
    return "running";
  }
  if (starting) return "starting";
  return "starting";
}

function LogStream({ logTail }: { logTail: string | undefined }) {
  const ref = useRef<HTMLPreElement>(null);
  const stickyRef = useRef(true);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el || !stickyRef.current) return;
    el.scrollTop = el.scrollHeight;
  }, [logTail]);

  const onScroll = () => {
    const el = ref.current;
    if (!el) return;
    const distanceFromBottom =
      el.scrollHeight - el.scrollTop - el.clientHeight;
    stickyRef.current = distanceFromBottom < 16;
  };

  return (
    <pre
      ref={ref}
      onScroll={onScroll}
      className="max-h-64 min-h-32 overflow-auto rounded bg-muted/50 p-3 font-mono text-xs leading-snug whitespace-pre-wrap break-words"
    >
      {logTail?.trim() ? logTail : "Waiting for installer output…"}
    </pre>
  );
}

function UpgradeProgressBlock({
  status,
}: {
  status: UpgradeStatus | undefined;
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
