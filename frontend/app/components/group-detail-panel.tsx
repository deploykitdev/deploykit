import { useEffect } from "react";
import { Panel } from "@xyflow/react";
import { GroupIcon, XIcon } from "lucide-react";
import {
  useCreateGroupEnvVar,
  useDeleteGroupEnvVar,
  useGroupEnvVars,
  usePendingChanges,
  useRemovePendingChange,
  useUpdateGroupEnvVar,
} from "@/lib/queries";
import { EnvVarsSection } from "./env-vars-section";

interface GroupDetailPanelProps {
  projectId: string;
  groupId: string;
  groupLabel: string;
  onClose: () => void;
}

export function GroupDetailPanel({
  projectId,
  groupId,
  groupLabel,
  onClose,
}: GroupDetailPanelProps) {
  const envVarsQuery = useGroupEnvVars(projectId, groupId);
  const pendingChangesQuery = usePendingChanges(projectId);
  const create = useCreateGroupEnvVar(projectId, groupId);
  const update = useUpdateGroupEnvVar(projectId, groupId);
  const del = useDeleteGroupEnvVar(projectId, groupId);
  const removePending = useRemovePendingChange(projectId);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onClose]);

  return (
    <Panel
      position="top-right"
      className="!top-4 !right-4 !bottom-4 !m-0 flex w-[520px] max-w-[90vw] flex-col overflow-hidden rounded-xl border bg-background/95 shadow-xl backdrop-blur pointer-events-auto"
    >
      <div className="flex items-center justify-between gap-3 border-b px-5 py-4">
        <div className="flex min-w-0 items-center gap-2.5">
          <div className="flex size-8 items-center justify-center rounded-md bg-muted text-muted-foreground">
            <GroupIcon className="size-4" />
          </div>
          <h2 className="truncate text-lg font-semibold">
            {groupLabel || "Group"}
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

      <div className="flex-1 overflow-y-auto px-5 py-4">
        <EnvVarsSection
          envVars={envVarsQuery.data}
          isLoading={envVarsQuery.isLoading}
          isError={envVarsQuery.isError}
          onCreate={(data) => create.mutateAsync(data)}
          onUpdate={(args) => update.mutateAsync(args)}
          onDelete={(id) => del.mutateAsync(id).then(() => undefined)}
          pendingChanges={pendingChangesQuery.data}
          onCancelPending={(id) =>
            removePending.mutateAsync(id).then(() => undefined)
          }
          scope="group"
          scopeId={groupId}
          title="Group variables"
          description="Variables shared by every service inside this group. Service-scoped variables override these."
          helperText="The deletion will be staged; deploy the project to apply it."
        />
      </div>
    </Panel>
  );
}
