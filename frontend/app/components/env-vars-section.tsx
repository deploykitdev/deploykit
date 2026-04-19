import { useMemo, useState } from "react";
import { EyeIcon, EyeOffIcon, MoreVerticalIcon, PlusIcon } from "lucide-react";
import { toast } from "sonner";
import {
  type EnvVar,
  type PendingChange,
  type EnvVarScope,
} from "@/lib/queries";
import { parsePayload } from "@/lib/pending-changes-diff";
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
import { cn } from "@/lib/utils";
import { EnvVarFormDialog } from "./env-var-form-dialog";
import { EnvVarDeleteDialog } from "./env-var-delete-dialog";

interface EnvVarsSectionProps {
  envVars: EnvVar[] | undefined;
  isLoading: boolean;
  isError: boolean;
  // Mutations stage pending changes; the resolved value is ignored.
  onCreate: (data: { key: string; value: string }) => Promise<unknown>;
  onUpdate: (args: { id: string; value: string }) => Promise<unknown>;
  onDelete: (id: string) => Promise<void>;
  // Pending change log scoped to the target, plus callback to cancel a
  // single staged change by ID. When omitted the section hides pending-state
  // UI entirely — useful for callers that haven't wired it up yet.
  pendingChanges?: PendingChange[];
  onCancelPending?: (changeId: string) => Promise<void>;
  // Scope of the env vars shown — used to filter pending changes to just
  // this target.
  scope: EnvVarScope;
  scopeId: string;
  // Copy shown in the delete confirmation, phrased for the scope.
  helperText: string;
  title: string;
  description?: string;
}

type RowState = "unchanged" | "add" | "edit" | "remove";

interface EffectiveRow {
  key: string;
  // What the user will see at deploy time.
  value: string;
  // The applied env var row, if one exists.
  appliedId: string | null;
  // Pending change rows affecting this key, in log order. Undo cancels all
  // of them together — staging a delete + create for the same key is a
  // single logical edit, and partial undo would leave surprising state.
  pendingChangeIds: string[];
  state: RowState;
  // Value before the staged edit / delete. Used for "before → after" hints.
  previousValue: string | null;
}

export function EnvVarsSection({
  envVars,
  isLoading,
  isError,
  onCreate,
  onUpdate,
  onDelete,
  pendingChanges,
  onCancelPending,
  scope,
  scopeId,
  helperText,
  title,
  description,
}: EnvVarsSectionProps) {
  const [addOpen, setAddOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<EffectiveRow | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<EffectiveRow | null>(null);
  // Keyed by the row key so the button shows a spinner on the row being
  // undone (and not globally, which looks stuck with multiple pending rows).
  const [cancelling, setCancelling] = useState<string | null>(null);
  const [revealed, setRevealed] = useState<Set<string>>(new Set());

  function toggleReveal(key: string) {
    setRevealed((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }

  const rows = useMemo(
    () => buildEffectiveRows(envVars, pendingChanges, scope, scopeId),
    [envVars, pendingChanges, scope, scopeId],
  );

  async function handleCancel(rowKey: string, changeIds: string[]) {
    if (!onCancelPending || changeIds.length === 0) return;
    setCancelling(rowKey);
    try {
      for (const id of changeIds) {
        await onCancelPending(id);
      }
      toast.success("Change discarded");
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Failed to discard change.",
      );
    } finally {
      setCancelling(null);
    }
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
      ) : rows.length === 0 ? (
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
              {rows.map((row) => {
                const displayValue =
                  row.state === "remove"
                    ? row.previousValue ?? ""
                    : row.value;
                const isRevealed = revealed.has(row.key);
                const isPending = row.state !== "unchanged";
                return (
                  <TableRow
                    key={row.key}
                    className={cn(
                      row.state === "remove" &&
                        "opacity-60 [&_td]:line-through decoration-muted-foreground",
                    )}
                  >
                    <TableCell className="font-mono text-xs">
                      <div className="flex items-center gap-2">
                        <span>{row.key}</span>
                        {isPending ? <PendingBadge state={row.state} /> : null}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <code className="flex-1 truncate font-mono text-xs">
                          {isRevealed
                            ? displayValue || (
                                <em className="not-italic text-muted-foreground">
                                  empty
                                </em>
                              )
                            : maskValue(displayValue)}
                        </code>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon-xs"
                          aria-label={isRevealed ? "Hide value" : "Show value"}
                          onClick={() => toggleReveal(row.key)}
                        >
                          {isRevealed ? (
                            <EyeOffIcon className="size-3.5" />
                          ) : (
                            <EyeIcon className="size-3.5" />
                          )}
                        </Button>
                      </div>
                    </TableCell>
                    <TableCell>
                      <RowActions
                        row={row}
                        onEdit={() => setEditTarget(row)}
                        onDelete={() => setDeleteTarget(row)}
                        onCancel={
                          row.pendingChangeIds.length > 0
                            ? () =>
                                handleCancel(row.key, row.pendingChangeIds)
                            : undefined
                        }
                        cancelling={cancelling === row.key}
                      />
                    </TableCell>
                  </TableRow>
                );
              })}
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
            toast.success(`Staged ${data.key}`);
          },
        }}
      />
      {editTarget && editTarget.appliedId && (
        <EnvVarFormDialog
          open={true}
          onOpenChange={(open) => !open && setEditTarget(null)}
          action={{
            mode: "edit",
            envVar: asEnvVar(editTarget, scope, scopeId),
            submit: async (data) => {
              await onUpdate(data);
              toast.success(`Staged change to ${editTarget.key}`);
            },
          }}
        />
      )}
      <EnvVarDeleteDialog
        envVar={deleteTarget ? asEnvVar(deleteTarget, scope, scopeId) : null}
        onClose={() => setDeleteTarget(null)}
        onDelete={async (id) => {
          await onDelete(id);
          toast.success(`Staged deletion of ${deleteTarget?.key ?? "variable"}`);
        }}
        helperText={helperText}
      />
    </div>
  );
}

function RowActions({
  row,
  onEdit,
  onDelete,
  onCancel,
  cancelling,
}: {
  row: EffectiveRow;
  onEdit: () => void;
  onDelete: () => void;
  onCancel?: () => void;
  cancelling: boolean;
}) {
  // Pending rows only surface an "Undo" action — editing a staged change on
  // top of another staged change is outside v1's per-entry scope.
  if (row.state !== "unchanged") {
    if (!onCancel) return null;
    return (
      <Button
        type="button"
        variant="ghost"
        size="xs"
        disabled={cancelling}
        onClick={onCancel}
      >
        {cancelling ? "Undoing…" : "Undo"}
      </Button>
    );
  }
  return (
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
        <DropdownMenuItem onClick={onEdit}>Edit</DropdownMenuItem>
        <DropdownMenuItem variant="destructive" onClick={onDelete}>
          Delete
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function PendingBadge({ state }: { state: RowState }) {
  const tone = {
    add: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",
    edit: "bg-amber-500/10 text-amber-700 dark:text-amber-400",
    remove: "bg-destructive/10 text-destructive",
    unchanged: "",
  }[state];
  const label = {
    add: "pending add",
    edit: "pending edit",
    remove: "pending delete",
    unchanged: "",
  }[state];
  return (
    <span className={cn("rounded px-1.5 py-0.5 text-[10px] font-medium", tone)}>
      {label}
    </span>
  );
}

// buildEffectiveRows merges applied env vars with pending changes targeting
// the same (scope, scopeId), producing what the list should render. A single
// row can have multiple staged changes (e.g. delete then re-create → "edit"
// from the user's POV) — pendingChangeIds carries them all so Undo can
// cancel them as a unit.
function buildEffectiveRows(
  applied: EnvVar[] | undefined,
  changes: PendingChange[] | undefined,
  scope: EnvVarScope,
  scopeId: string,
): EffectiveRow[] {
  const rows = new Map<string, EffectiveRow>();

  for (const ev of applied ?? []) {
    if (ev.scope !== scope || ev.scope_id !== scopeId) continue;
    rows.set(ev.key, {
      key: ev.key,
      value: ev.value,
      appliedId: ev.id,
      pendingChangeIds: [],
      state: "unchanged",
      previousValue: null,
    });
  }

  for (const c of changes ?? []) {
    const payload = parsePayload(c.payload);
    if (c.op === "env_var.create") {
      if (
        c.target_id !== scopeId ||
        payload.scope !== scope ||
        typeof payload.key !== "string"
      ) {
        continue;
      }
      const key = payload.key;
      const newValue =
        typeof payload.value === "string" ? payload.value : "";
      const existing = rows.get(key);
      if (existing && existing.state === "remove") {
        // Delete-then-create on the same key collapses to a logical edit:
        // net effect is "applied value becomes newValue", and Undo should
        // cancel both staged changes or we'd leave the user with an
        // orphaned delete after a single undo.
        existing.state = "edit";
        existing.value = newValue;
        existing.pendingChangeIds.push(c.id);
      } else {
        rows.set(key, {
          key,
          value: newValue,
          appliedId: existing?.appliedId ?? null,
          pendingChangeIds: [c.id],
          state: "add",
          previousValue: null,
        });
      }
    } else if (c.op === "env_var.update") {
      if (payload.scope !== scope || payload.scope_id !== scopeId) continue;
      const key = typeof payload.key === "string" ? payload.key : "";
      const existing = rows.get(key);
      if (!existing) continue;
      existing.pendingChangeIds.push(c.id);
      if (existing.state === "unchanged") {
        existing.previousValue = existing.value;
      }
      existing.state = existing.state === "add" ? "add" : "edit";
      existing.value = typeof payload.value === "string" ? payload.value : "";
    } else if (c.op === "env_var.delete") {
      if (payload.scope !== scope || payload.scope_id !== scopeId) continue;
      const key = typeof payload.key === "string" ? payload.key : "";
      const existing = rows.get(key);
      if (!existing) continue;
      existing.pendingChangeIds.push(c.id);
      if (existing.previousValue == null) {
        existing.previousValue = existing.value;
      }
      existing.state = "remove";
    }
  }

  const arr = Array.from(rows.values());
  arr.sort((a, b) => {
    const pendingA = a.state !== "unchanged" ? 1 : 0;
    const pendingB = b.state !== "unchanged" ? 1 : 0;
    if (pendingA !== pendingB) return pendingA - pendingB;
    return a.key.localeCompare(b.key);
  });
  return arr;
}

// asEnvVar converts an effective row back to the EnvVar shape the dialogs
// expect. Synthesizes a placeholder id for pending-only rows (which the
// dialogs don't act on anyway — those rows use the Undo button).
function asEnvVar(row: EffectiveRow, scope: EnvVarScope, scopeId: string): EnvVar {
  return {
    id: row.appliedId ?? "",
    scope,
    scope_id: scopeId,
    key: row.key,
    value: row.value,
    created_at: "",
    updated_at: "",
  };
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
