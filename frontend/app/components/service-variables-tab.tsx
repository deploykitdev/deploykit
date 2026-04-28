import { Link } from "react-router";
import {
  useCreateServiceEnvVar,
  useDeleteServiceEnvVar,
  useGroupEnvVars,
  usePendingChanges,
  useProjectEnvVars,
  useRemovePendingChange,
  useServiceEnvVars,
  useUpdateServiceEnvVar,
} from "@/lib/queries";
import { EnvVarsSection } from "./env-vars-section";

interface ServiceVariablesTabProps {
  projectId: string;
  serviceId: string;
  // Canvas group id this service is parented to, if any. Drives the
  // "inherited from group" hint.
  groupId?: string | null;
}

export function ServiceVariablesTab({
  projectId,
  serviceId,
  groupId,
}: ServiceVariablesTabProps) {
  const envVarsQuery = useServiceEnvVars(projectId, serviceId);
  const projectVarsQuery = useProjectEnvVars(projectId);
  const groupVarsQuery = useGroupEnvVars(projectId, groupId ?? null);
  const pendingChangesQuery = usePendingChanges(projectId);
  const create = useCreateServiceEnvVar(projectId, serviceId);
  const update = useUpdateServiceEnvVar(projectId, serviceId);
  const del = useDeleteServiceEnvVar(projectId, serviceId);
  const removePending = useRemovePendingChange(projectId);

  const inheritedCount = projectVarsQuery.data?.length ?? 0;
  const groupInheritedCount = groupVarsQuery.data?.length ?? 0;

  return (
    <div className="flex flex-col gap-4">
      {inheritedCount > 0 && (
        <div className="flex items-center justify-between gap-3 rounded-md border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
          <span>
            {inheritedCount} project variable{inheritedCount === 1 ? "" : "s"}{" "}
            also applies to this service.
          </span>
          <Link
            to={`/projects/${projectId}/settings`}
            className="font-medium text-foreground underline-offset-4 hover:underline"
          >
            Manage →
          </Link>
        </div>
      )}
      {groupInheritedCount > 0 && (
        <div className="flex items-center justify-between gap-3 rounded-md border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
          <span>
            {groupInheritedCount} group variable
            {groupInheritedCount === 1 ? "" : "s"} also apply to this service.
          </span>
          <span className="font-medium text-foreground">
            Click the group on the canvas to manage.
          </span>
        </div>
      )}
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
        scope="service"
        scopeId={serviceId}
        title="Service variables"
        description="Override or add variables just for this service."
        helperText="The deletion will be staged; deploy the project to apply it."
      />
    </div>
  );
}

// ComingSoon is still used by service-metrics-tab and service-settings-tab.
export function ComingSoon({
  icon,
  title,
  description,
}: {
  icon: React.ReactNode;
  title: string;
  description: string;
}) {
  return (
    <div className="flex flex-col items-center gap-2 py-16 text-center">
      <div className="flex size-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
        {icon}
      </div>
      <p className="text-sm font-medium">{title} — coming soon</p>
      <p className="max-w-[280px] text-xs text-muted-foreground">
        {description}
      </p>
    </div>
  );
}
