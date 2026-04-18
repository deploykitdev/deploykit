import { useState } from "react";
import { EyeIcon, EyeOffIcon, MoreVerticalIcon, PlusIcon } from "lucide-react";
import { toast } from "sonner";
import type { EnvVar } from "@/lib/queries";
import { Button } from "./ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "./ui/dropdown-menu";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "./ui/table";
import { EnvVarFormDialog } from "./env-var-form-dialog";
import { EnvVarDeleteDialog } from "./env-var-delete-dialog";

interface EnvVarsSectionProps {
  envVars: EnvVar[] | undefined;
  isLoading: boolean;
  isError: boolean;
  onCreate: (data: { key: string; value: string }) => Promise<EnvVar>;
  onUpdate: (args: { id: string; value: string }) => Promise<EnvVar>;
  onDelete: (id: string) => Promise<void>;
  // redeployMessage is shown in the delete confirmation, phrased for the scope
  // (e.g. "All services in this project will be redeployed.").
  redeployMessage: string;
  title: string;
  description?: string;
}

export function EnvVarsSection({
  envVars,
  isLoading,
  isError,
  onCreate,
  onUpdate,
  onDelete,
  redeployMessage,
  title,
  description,
}: EnvVarsSectionProps) {
  const [addOpen, setAddOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<EnvVar | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<EnvVar | null>(null);
  const [revealed, setRevealed] = useState<Set<string>>(new Set());

  function toggleReveal(id: string) {
    setRevealed((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-base font-medium">{title}</h2>
          {description && (
            <p className="text-sm text-muted-foreground">{description}</p>
          )}
        </div>
        <Button size="sm" onClick={() => setAddOpen(true)}>
          <PlusIcon className="size-3.5" />
          Add variable
        </Button>
      </div>

      {isLoading ? (
        <p className="text-sm text-muted-foreground">Loading variables…</p>
      ) : isError ? (
        <p className="text-sm text-destructive">Failed to load variables.</p>
      ) : !envVars || envVars.length === 0 ? (
        <EmptyState onAdd={() => setAddOpen(true)} />
      ) : (
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[40%]">Key</TableHead>
                <TableHead>Value</TableHead>
                <TableHead className="w-10" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {envVars.map((ev) => (
                <TableRow key={ev.id}>
                  <TableCell className="font-mono text-xs">{ev.key}</TableCell>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <code className="flex-1 truncate font-mono text-xs">
                        {revealed.has(ev.id)
                          ? ev.value || <em className="not-italic text-muted-foreground">empty</em>
                          : maskValue(ev.value)}
                      </code>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-xs"
                        aria-label={revealed.has(ev.id) ? "Hide value" : "Show value"}
                        onClick={() => toggleReveal(ev.id)}
                      >
                        {revealed.has(ev.id) ? (
                          <EyeOffIcon className="size-3.5" />
                        ) : (
                          <EyeIcon className="size-3.5" />
                        )}
                      </Button>
                    </div>
                  </TableCell>
                  <TableCell>
                    <DropdownMenu>
                      <DropdownMenuTrigger
                        render={
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon-xs"
                            aria-label="Variable actions"
                          />
                        }
                      >
                        <MoreVerticalIcon className="size-3.5" />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem onClick={() => setEditTarget(ev)}>
                          Edit
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          variant="destructive"
                          onClick={() => setDeleteTarget(ev)}
                        >
                          Delete
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <EnvVarFormDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        action={{
          mode: "create",
          submit: async (data) => {
            await onCreate(data);
            toast.success(`Added ${data.key}`);
          },
        }}
      />
      {editTarget && (
        <EnvVarFormDialog
          open={true}
          onOpenChange={(open) => !open && setEditTarget(null)}
          action={{
            mode: "edit",
            envVar: editTarget,
            submit: async (data) => {
              await onUpdate(data);
              toast.success(`Updated ${editTarget.key}`);
            },
          }}
        />
      )}
      <EnvVarDeleteDialog
        envVar={deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onDelete={async (id) => {
          await onDelete(id);
          toast.success(`Deleted ${deleteTarget?.key ?? "variable"}`);
        }}
        redeployMessage={redeployMessage}
      />
    </div>
  );
}

function EmptyState({ onAdd }: { onAdd: () => void }) {
  return (
    <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed py-10 text-center">
      <p className="text-sm font-medium">No variables yet</p>
      <p className="max-w-[280px] text-xs text-muted-foreground">
        Environment variables are injected into every container in scope at
        deploy time.
      </p>
      <Button size="sm" variant="outline" onClick={onAdd}>
        <PlusIcon className="size-3.5" />
        Add variable
      </Button>
    </div>
  );
}

function maskValue(value: string): string {
  if (value.length === 0) return "—";
  return "•".repeat(Math.min(value.length, 16));
}
