import { FormEvent, useCallback, useEffect, useState } from "react";
import { Link } from "react-router";
import { RequireAuth } from "../lib/auth";
import { api, ApiError } from "../lib/api";
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
import { Plus } from "lucide-react";

interface Project {
  id: string;
  name: string;
  slug: string;
  created_at: string;
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
  const [projects, setProjects] = useState<Project[]>([]);
  const [dialogOpen, setDialogOpen] = useState(false);

  const fetchProjects = useCallback(() => {
    api<{ data: Project[] }>("/projects").then((res) =>
      setProjects(res.data ?? []),
    );
  }, []);

  useEffect(() => {
    fetchProjects();
  }, [fetchProjects]);

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
              fetchProjects();
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
        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {projects.map((p) => (
            <Link key={p.id} to={`/projects/${p.id}`} className="group/link">
              <Card className="gap-0 rounded-lg p-0 transition-colors duration-200 hover:ring-primary/50">
                <div className="px-5 pt-4 pb-3">
                  <p className="text-base font-semibold">{p.name}</p>
                </div>
                <div className="mx-4 rounded-lg bg-muted/50 dark:bg-muted/30">
                  <div
                    className="h-32 rounded-lg opacity-[0.03] dark:opacity-[0.06]"
                    style={{
                      backgroundImage:
                        "radial-gradient(circle, currentColor 1px, transparent 1px)",
                      backgroundSize: "16px 16px",
                    }}
                  />
                </div>
                <div className="flex items-center gap-2 px-5 py-4 text-xs text-muted-foreground">
                  <span className="relative flex size-2">
                    <span className="absolute inline-flex size-full animate-ping rounded-full bg-emerald-400 opacity-75" />
                    <span className="relative inline-flex size-2 rounded-full bg-emerald-500" />
                  </span>
                  <span className="font-mono">{p.slug}</span>
                  <span>&middot;</span>
                  <span>{timeAgo(p.created_at)}</span>
                </div>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </DashboardLayout>
  );
}

function CreateProjectDialog({ onCreated }: { onCreated: () => void }) {
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setSubmitting(true);

    try {
      await api<Project>("/projects", {
        method: "POST",
        body: JSON.stringify({ name }),
      });
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
    } finally {
      setSubmitting(false);
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
          <Button type="submit" disabled={submitting}>
            {submitting ? "Creating..." : "Create Project"}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  );
}
