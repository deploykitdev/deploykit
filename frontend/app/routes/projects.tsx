import { FormEvent, useState } from "react";
import { Link } from "react-router";
import { RequireAuth } from "../lib/auth";
import { ApiError } from "../lib/api";
import { useProjects, useCreateProject, type Project } from "../lib/queries";
import { DashboardLayout } from "@/components/dashboard-layout";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, FieldLabel, FieldError } from "@/components/ui/field";
import { Card } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Plus, ArrowUpRight } from "lucide-react";

function gradientFromSlug(slug: string): { from: string; to: string } {
  let hash = 0;
  for (let i = 0; i < slug.length; i++) {
    hash = (hash * 31 + slug.charCodeAt(i)) | 0;
  }
  const hue = Math.abs(hash) % 360;
  return {
    from: `hsl(${hue}, 70%, 58%)`,
    to: `hsl(${(hue + 45) % 360}, 70%, 46%)`,
  };
}

function timeAgo(dateStr: string): string {
  const seconds = Math.floor(
    (Date.now() - new Date(dateStr).getTime()) / 1000,
  );
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  const months = Math.floor(days / 30);
  if (months < 12) return `${months}mo ago`;
  return `${Math.floor(months / 12)}y ago`;
}

export default function Projects() {
  return (
    <RequireAuth>
      <ProjectsList />
    </RequireAuth>
  );
}

function ProjectsList() {
  const { data: projects = [] } = useProjects();
  const [dialogOpen, setDialogOpen] = useState(false);

  return (
    <DashboardLayout>
      <div className="mt-4 mb-12 flex items-center justify-between">
        <h2 className="text-2xl font-semibold">Projects</h2>
        <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
          <DialogTrigger render={<Button />}>
            <Plus className="mr-1.5 size-4" />
            New Project
          </DialogTrigger>
          <CreateProjectDialog
            onCreated={() => {
              setDialogOpen(false);
            }}
          />
        </Dialog>
      </div>
      {projects.length === 0 ? (
        <button
          type="button"
          onClick={() => setDialogOpen(true)}
          className="group flex w-full cursor-pointer items-center justify-center rounded-xl border-2 border-dashed border-border py-16 transition-colors hover:ring-primary/50 hover:bg-accent/50"
        >
          <div className="flex flex-col items-center gap-3 text-muted-foreground group-hover:text-foreground">
            <div className="flex size-12 items-center justify-center rounded-full border-2 border-dashed border-current transition-colors">
              <Plus className="size-5" />
            </div>
            <div className="text-center">
              <p className="text-sm font-medium">Create your first project</p>
              <p className="mt-1 text-xs text-muted-foreground">
                Deploy an app to get started
              </p>
            </div>
          </div>
        </button>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {projects.map((p) => {
            const { from, to } = gradientFromSlug(p.slug);
            return (
              <Link key={p.id} to={`/projects/${p.id}`} className="group">
                <Card className="relative gap-0 overflow-hidden p-0 transition-all duration-200 group-hover:-translate-y-0.5 group-hover:shadow-md group-hover:ring-primary/40">
                  <div
                    className="relative h-20"
                    style={{
                      background: `linear-gradient(135deg, ${from}, ${to})`,
                    }}
                  >
                    <div
                      className="absolute inset-0 opacity-15"
                      style={{
                        backgroundImage:
                          "radial-gradient(circle at 1px 1px, white 1px, transparent 0)",
                        backgroundSize: "14px 14px",
                      }}
                    />
                    <ArrowUpRight className="absolute right-3 top-3 size-4 text-white/70 transition-all duration-200 group-hover:-translate-y-0.5 group-hover:translate-x-0.5 group-hover:text-white" />
                  </div>
                  <div className="absolute left-4 top-[58px] flex size-11 items-center justify-center rounded-full bg-card text-base font-semibold ring-1 ring-border">
                    {p.name.charAt(0).toUpperCase()}
                  </div>
                  <div className="px-5 pt-8 pb-4">
                    <p className="truncate text-base font-semibold">{p.name}</p>
                    <div className="mt-1 flex items-center gap-1.5 text-xs text-muted-foreground">
                      <span className="truncate font-mono">{p.slug}</span>
                      <span aria-hidden>·</span>
                      <span className="shrink-0">{timeAgo(p.created_at)}</span>
                    </div>
                  </div>
                </Card>
              </Link>
            );
          })}
        </div>
      )}
    </DashboardLayout>
  );
}

function CreateProjectDialog({ onCreated }: { onCreated: () => void }) {
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const createProject = useCreateProject();

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");

    try {
      await createProject.mutateAsync(name);
      setName("");
      onCreated();
    } catch (err) {
      if (err instanceof ApiError) {
        try {
          const body = JSON.parse(err.message);
          if (body.errors?.name) {
            setError(body.errors.name[0]);
          } else {
            setError(body.message || "Failed to create project.");
          }
        } catch {
          setError("Failed to create project.");
        }
      } else {
        setError("Failed to create project.");
      }
    }
  }

  return (
    <DialogContent>
      <form onSubmit={handleSubmit}>
        <DialogHeader>
          <DialogTitle>New Project</DialogTitle>
          <DialogDescription>
            Give your project a name to get started.
          </DialogDescription>
        </DialogHeader>
        <Field data-invalid={error ? true : undefined} className="py-4">
          <FieldLabel htmlFor="project-name">Name</FieldLabel>
          <Input
            id="project-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="My Project"
            autoFocus
          />
          <FieldError>{error || undefined}</FieldError>
        </Field>
        <DialogFooter>
          <Button type="submit" disabled={createProject.isPending}>
            {createProject.isPending ? "Creating..." : "Create Project"}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  );
}
