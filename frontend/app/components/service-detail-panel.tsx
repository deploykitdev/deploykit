import { useEffect, useMemo, useState } from "react";
import { Panel } from "@xyflow/react";
import { XIcon } from "lucide-react";
import { Tabs, TabList, Tab, TabPanel } from "./ui/tabs";
import { useDeployments, usePendingChanges, useService } from "@/lib/queries";
import { collectServiceOverride } from "@/lib/pending-changes-diff";
import { ServiceDeploymentsTab } from "./service-deployments-tab";
import { ServiceVariablesTab } from "./service-variables-tab";
import { ServiceMetricsTab } from "./service-metrics-tab";
import { ServiceSettingsTab } from "./service-settings-tab";
import { ServiceLogsTab } from "./service-logs-tab";
import { ServiceIconEditor } from "./service-icon-editor";

interface ServiceDetailPanelProps {
  projectId: string;
  serviceId: string;
  onClose: () => void;
}

export function ServiceDetailPanel({
  projectId,
  serviceId,
  onClose,
}: ServiceDetailPanelProps) {
  const serviceQuery = useService(projectId, serviceId);
  const deploymentsQuery = useDeployments(projectId, serviceId);
  const { data: pendingChanges } = usePendingChanges(projectId);
  const [tab, setTab] = useState("deployments");

  // Merge staged service.update entries into a single "target" override so
  // the header reflects pending renames / icon changes before deploy.
  const override = useMemo(
    () => collectServiceOverride(pendingChanges, serviceId),
    [pendingChanges, serviceId],
  );
  const pendingDelete = override?.pendingDelete ?? false;

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onClose]);

  const service = serviceQuery.data;
  const name = override?.name ?? service?.name ?? "Service";
  const effectiveIconUrl =
    override?.iconUrlSet !== undefined
      ? override.iconUrlSet
      : service?.icon_url ?? null;
  const activeDeploymentId = service?.active_deployment_id ?? null;
  const activeImage = service?.active_deployment?.image ?? null;

  return (
    <Panel
      position="top-right"
      className="!top-4 !right-4 !bottom-4 !m-0 flex w-[520px] max-w-[90vw] flex-col overflow-hidden rounded-xl border bg-background/95 shadow-xl backdrop-blur pointer-events-auto"
    >
      <div className="flex items-center justify-between gap-3 border-b px-5 py-4">
        <div className="flex items-center gap-2.5 min-w-0">
          <ServiceIconEditor
            projectId={projectId}
            serviceId={serviceId}
            iconUrl={effectiveIconUrl}
          />
          <h2
            className={
              pendingDelete
                ? "truncate text-lg font-semibold line-through decoration-destructive/70"
                : "truncate text-lg font-semibold"
            }
          >
            {name}
          </h2>
        </div>
        <button
          type="button"
          onClick={onClose}
          aria-label="Close"
          className="flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          <XIcon className="size-4" />
        </button>
      </div>

      <Tabs
        value={tab}
        onValueChange={(v) => setTab(String(v))}
        className="flex min-h-0 flex-1 flex-col gap-0"
      >
        <TabList className="px-5">
          <Tab value="deployments">Deployments</Tab>
          <Tab value="logs">Logs</Tab>
          <Tab value="variables">Variables</Tab>
          <Tab value="metrics">Metrics</Tab>
          <Tab value="settings">Settings</Tab>
        </TabList>

        {activeImage ? (
          <div className="flex items-center gap-3 border-b px-5 py-2.5 text-xs text-muted-foreground">
            <code className="truncate rounded bg-muted px-1.5 py-0.5 font-mono">
              {activeImage}
            </code>
          </div>
        ) : null}

        <div className="min-h-0 flex-1 overflow-hidden">
          <TabPanel value="deployments" className="h-full overflow-y-auto p-5">
            <ServiceDeploymentsTab
              deployments={deploymentsQuery.data}
              isLoading={deploymentsQuery.isLoading}
              error={deploymentsQuery.error}
              activeDeploymentId={activeDeploymentId}
            />
          </TabPanel>
          <TabPanel value="logs" className="h-full">
            <ServiceLogsTab
              projectId={projectId}
              serviceId={serviceId}
              active={tab === "logs"}
              status={service?.status}
            />
          </TabPanel>
          <TabPanel value="variables" className="h-full overflow-y-auto p-5">
            <ServiceVariablesTab projectId={projectId} serviceId={serviceId} />
          </TabPanel>
          <TabPanel value="metrics" className="h-full overflow-y-auto p-5">
            <ServiceMetricsTab />
          </TabPanel>
          <TabPanel value="settings" className="h-full overflow-y-auto p-5">
            <ServiceSettingsTab
              projectId={projectId}
              serviceId={serviceId}
            />
          </TabPanel>
        </div>
      </Tabs>
    </Panel>
  );
}
