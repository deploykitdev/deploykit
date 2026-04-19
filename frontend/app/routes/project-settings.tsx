import { FormEvent, useEffect, useState } from "react";
import { Link, useParams } from "react-router";
import { ChevronLeftIcon } from "lucide-react";
import { toast } from "sonner";
import { RequireAuth } from "../lib/auth";
import { ApiError } from "../lib/api";
import {
  useCreateProjectEnvVar,
  useDeleteProjectEnvVar,
  useProject,
  useProjectEnvVars,
  useUpdateProject,
  useUpdateProjectEnvVar,
  type Project,
} from "../lib/queries";
import { DashboardLayout } from "@/components/dashboard-layout";
import { EnvVarsSection } from "@/components/env-vars-section";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, FieldLabel, FieldError } from "@/components/ui/field";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

type SettingsTab = "general" | "variables";

function ProjectSettings() {
  const { projectId } = useParams();
  const id = projectId!;
  const { data: project, isLoading, error } = useProject(id);
  const [tab, setTab] = useState<SettingsTab>("general");

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

  const navItem = (key: SettingsTab, label: string) => (
    <button
      type="button"
      onClick={() => setTab(key)}
      className={
        "rounded-md px-3 py-1.5 text-left font-medium transition-colors " +
        (tab === key
          ? "bg-muted text-foreground"
          : "text-muted-foreground hover:bg-muted/50 hover:text-foreground")
      }
    >
      {label}
    </button>
  );

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
          {navItem("general", "General")}
          {navItem("variables", "Variables")}
        </nav>
        <section>
          {tab === "general" && <GeneralSection project={project} />}
          {tab === "variables" && (
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
          )}
        </section>
      </div>
    </DashboardLayout>
  );
}

function GeneralSection({ project }: { project: Project }) {
  const [name, setName] = useState(project.name);
  const [error, setError] = useState("");
  const updateProject = useUpdateProject(project.id);

  useEffect(() => {
    setName(project.name);
  }, [project.name]);

  const trimmed = name.trim();
  const dirty = trimmed !== project.name;
  const canSubmit = dirty && trimmed !== "" && !updateProject.isPending;

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    if (!canSubmit) return;

    try {
      await updateProject.mutateAsync({ name: trimmed });
      toast.success("Project renamed");
    } catch (err) {
      if (err instanceof ApiError) {
        try {
          const body = JSON.parse(err.message);
          if (body.errors?.name) {
            setError(body.errors.name[0]);
          } else {
            setError(body.message || "Failed to rename project.");
          }
        } catch {
          setError("Failed to rename project.");
        }
      } else {
        setError("Failed to rename project.");
      }
    }
  }

  return (
    <form onSubmit={handleSubmit}>
      <Card>
        <CardHeader>
          <CardTitle>Project name</CardTitle>
          <CardDescription>
            A human-friendly name for this project. The URL slug stays the same.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Field data-invalid={error ? true : undefined}>
            <FieldLabel htmlFor="project-name">Name</FieldLabel>
            <Input
              id="project-name"
              value={name}
              onChange={(e) => {
                setName(e.target.value);
                if (error) setError("");
              }}
            />
            <FieldError>{error || undefined}</FieldError>
          </Field>
          <p className="mt-3 text-xs text-muted-foreground">
            Slug: <span className="font-mono">{project.slug}</span>
          </p>
        </CardContent>
        <CardFooter>
          <Button type="submit" disabled={!canSubmit}>
            {updateProject.isPending ? "Saving..." : "Save"}
          </Button>
        </CardFooter>
      </Card>
    </form>
  );
}

export default function ProjectSettingsRoute() {
  return (
    <RequireAuth>
      <ProjectSettings />
    </RequireAuth>
  );
}
