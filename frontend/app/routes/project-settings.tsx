import { Link, useParams } from "react-router";
import { ChevronLeftIcon } from "lucide-react";
import { RequireAuth } from "../lib/auth";
import {
  useCreateProjectEnvVar,
  useDeleteProjectEnvVar,
  useProject,
  useProjectEnvVars,
  useUpdateProjectEnvVar,
} from "../lib/queries";
import { DashboardLayout } from "@/components/dashboard-layout";
import { EnvVarsSection } from "@/components/env-vars-section";

function ProjectSettings() {
  const { projectId } = useParams();
  const id = projectId!;
  const { data: project, isLoading, error } = useProject(id);

  const envVarsQuery = useProjectEnvVars(id);
  const create = useCreateProjectEnvVar(id);
  const update = useUpdateProjectEnvVar(id);
  const del = useDeleteProjectEnvVar(id);

  if (isLoading) {
    return (
      <DashboardLayout>
        <p>Loading…</p>
      </DashboardLayout>
    );
  }

  if (error || !project) {
    return (
      <DashboardLayout>
        <p>Project not found.</p>
      </DashboardLayout>
    );
  }

  return (
    <DashboardLayout>
      <div className="mt-4 mb-6 flex flex-col gap-1">
        <Link
          to={`/projects/${id}`}
          className="flex w-fit items-center gap-1 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <ChevronLeftIcon className="size-3.5" />
          Back to project
        </Link>
        <h1 className="text-2xl font-semibold">{project.name} — Settings</h1>
      </div>

      <div className="grid grid-cols-[12rem_1fr] gap-8">
        <nav className="flex flex-col gap-0.5 text-sm">
          <span className="rounded-md bg-muted px-3 py-1.5 font-medium">
            Variables
          </span>
        </nav>
        <section>
          <EnvVarsSection
            envVars={envVarsQuery.data}
            isLoading={envVarsQuery.isLoading}
            isError={envVarsQuery.isError}
            onCreate={(data) => create.mutateAsync(data)}
            onUpdate={(args) => update.mutateAsync(args)}
            onDelete={(envVarId) =>
              del.mutateAsync(envVarId).then(() => undefined)
            }
            title="Project variables"
            description="Shared by every service in this project. Services override keys by setting them locally."
            redeployMessage="All services in this project will be redeployed."
          />
        </section>
      </div>
    </DashboardLayout>
  );
}

export default function ProjectSettingsRoute() {
  return (
    <RequireAuth>
      <ProjectSettings />
    </RequireAuth>
  );
}
