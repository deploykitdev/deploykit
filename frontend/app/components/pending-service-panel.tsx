import { FormEvent, useEffect, useMemo, useState } from "react";
import { Panel } from "@xyflow/react";
import { ContainerIcon, PlusIcon, TrashIcon, XIcon } from "lucide-react";
import { toast } from "sonner";
import { ApiError } from "@/lib/api";
import {
  useAddPendingServiceEnvVar,
  usePendingChanges,
  useRemovePendingChange,
  type PendingChange,
} from "@/lib/queries";
import { parsePayload } from "@/lib/pending-changes-diff";
import { Button } from "./ui/button";
import { Input } from "./ui/input";

interface PendingServicePanelProps {
  projectId: string;
  // The canvas node ID of the pending-added service. Doubles as the
  // service.create entry's target_temp_id.
  nodeId: string;
  onClose: () => void;
}

export function PendingServicePanel({
  projectId,
  nodeId,
  onClose,
}: PendingServicePanelProps) {
  const { data: pendingChanges } = usePendingChanges(projectId);
  const addEnvVar = useAddPendingServiceEnvVar(projectId, nodeId);
  const removeChange = useRemovePendingChange(projectId);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onClose]);

  const summary = useMemo(
    () => extractPendingService(pendingChanges, nodeId),
    [pendingChanges, nodeId],
  );

  if (!summary) return <AutoClose onClose={onClose} />;

  return (
    <Panel
      position="top-right"
      className="!top-4 !right-4 !bottom-4 !m-0 flex w-[520px] max-w-[90vw] flex-col overflow-hidden rounded-xl border bg-background/95 shadow-xl backdrop-blur pointer-events-auto"
    >
      <div className="flex items-center justify-between gap-3 border-b px-5 py-4">
        <div className="flex items-center gap-2.5 min-w-0">
          <div className="grid size-7 shrink-0 place-items-center rounded-md border border-border/80 bg-background/60">
            <ContainerIcon className="size-4 text-muted-foreground" />
          </div>
          <div className="min-w-0">
            <h2 className="truncate text-lg font-semibold">{summary.name}</h2>
            <p className="text-xs font-medium uppercase tracking-widest text-amber-600 dark:text-amber-400">
              Pending deploy
            </p>
          </div>
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

      <div className="min-h-0 flex-1 overflow-y-auto p-5 space-y-5">
        <section>
          <h3 className="text-sm font-medium mb-1.5">Image</h3>
          <code className="block truncate rounded bg-muted px-2 py-1.5 font-mono text-xs">
            {summary.image}
          </code>
        </section>

        <EnvVarsEditor
          entries={summary.envVars}
          staticEntries={summary.inlineEnvVars}
          onAdd={(data) =>
            addEnvVar
              .mutateAsync(data)
              .then(() => {
                toast.success(`Staged ${data.key}`);
              })
              .catch((err) => {
                const message = extractMessage(err, "Failed to stage env var.");
                toast.error(message);
                throw err;
              })
          }
          onRemove={(changeId, key) =>
            removeChange
              .mutateAsync(changeId)
              .then(() => {
                toast.success(`Removed ${key}`);
              })
              .catch((err) => {
                toast.error(
                  extractMessage(err, "Failed to remove env var."),
                );
              })
          }
        />

        <div className="rounded-md border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
          This service hasn't been deployed yet. Click Deploy in the bottom
          panel to create it and start the container. To remove it, right-click
          the node and choose Delete.
        </div>
      </div>
    </Panel>
  );
}

interface EnvVarEntry {
  key: string;
  value: string;
  // Non-null when the entry was staged as its own env_var.create pending
  // change (so the user can back it out individually). Null when the entry
  // came bundled into the service.create payload and must be discarded with
  // the whole service.
  changeId: string | null;
}

interface PendingServiceSummary {
  name: string;
  image: string;
  envVars: EnvVarEntry[];
  // Subset that was baked into the service.create payload — shown but not
  // individually removable.
  inlineEnvVars: Set<string>;
}

function EnvVarsEditor({
  entries,
  staticEntries,
  onAdd,
  onRemove,
}: {
  entries: EnvVarEntry[];
  staticEntries: Set<string>;
  onAdd: (data: { key: string; value: string }) => Promise<unknown>;
  onRemove: (changeId: string, key: string) => Promise<void>;
}) {
  const [adding, setAdding] = useState(false);
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [pendingRemoval, setPendingRemoval] = useState<string | null>(null);

  async function handleAdd(e: FormEvent) {
    e.preventDefault();
    const trimmed = key.trim();
    if (!trimmed) return;
    setSubmitting(true);
    try {
      await onAdd({ key: trimmed, value });
      setKey("");
      setValue("");
      setAdding(false);
    } catch {
      // toast already surfaced
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <section>
      <div className="flex items-center justify-between mb-1.5">
        <h3 className="text-sm font-medium">
          Environment variables
          {entries.length > 0 ? (
            <span className="ml-1.5 text-xs text-muted-foreground">
              ({entries.length})
            </span>
          ) : null}
        </h3>
        {!adding ? (
          <Button size="xs" variant="outline" onClick={() => setAdding(true)}>
            <PlusIcon className="size-3" />
            Add
          </Button>
        ) : null}
      </div>

      {entries.length === 0 && !adding ? (
        <p className="text-xs text-muted-foreground">
          None staged yet. Add variables now so they're injected on the first
          deploy.
        </p>
      ) : null}

      {entries.length > 0 ? (
        <ul className="divide-y rounded-md border">
          {entries.map((ev) => {
            const isStatic = staticEntries.has(ev.key);
            const isRemoving = pendingRemoval === ev.changeId;
            return (
              <li
                key={ev.key}
                className="flex items-center gap-2 px-3 py-2 text-xs font-mono"
              >
                <span className="text-foreground shrink-0">{ev.key}</span>
                <span className="text-muted-foreground">=</span>
                <span className="flex-1 truncate text-muted-foreground">
                  {ev.value || "\u00a0"}
                </span>
                {ev.changeId && !isStatic ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    aria-label={`Remove ${ev.key}`}
                    disabled={isRemoving}
                    onClick={() => {
                      if (!ev.changeId) return;
                      setPendingRemoval(ev.changeId);
                      onRemove(ev.changeId, ev.key).finally(() =>
                        setPendingRemoval(null),
                      );
                    }}
                  >
                    <TrashIcon className="size-3" />
                  </Button>
                ) : null}
              </li>
            );
          })}
        </ul>
      ) : null}

      {adding ? (
        <form
          onSubmit={handleAdd}
          className="mt-3 grid gap-2 rounded-md border border-dashed px-3 py-2.5"
        >
          <div className="grid grid-cols-2 gap-2">
            <Input
              autoFocus
              placeholder="KEY"
              value={key}
              onChange={(e) => setKey(e.target.value)}
              disabled={submitting}
              className="font-mono text-xs"
            />
            <Input
              placeholder="value"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              disabled={submitting}
              className="font-mono text-xs"
            />
          </div>
          <div className="flex items-center justify-end gap-2">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => {
                setAdding(false);
                setKey("");
                setValue("");
              }}
              disabled={submitting}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              size="sm"
              disabled={submitting || key.trim() === ""}
            >
              {submitting ? "Staging…" : "Stage"}
            </Button>
          </div>
        </form>
      ) : null}
    </section>
  );
}

function extractPendingService(
  changes: PendingChange[] | undefined,
  tempId: string,
): PendingServiceSummary | null {
  if (!changes) return null;
  const create = changes.find(
    (c) => c.op === "service.create" && c.target_temp_id === tempId,
  );
  if (!create) return null;

  const payload = parsePayload(create.payload);
  const name = typeof payload.name === "string" ? payload.name : "(unnamed)";
  const image = typeof payload.image === "string" ? payload.image : "";

  const merged = new Map<string, EnvVarEntry>();
  const inlineKeys = new Set<string>();

  // Inline env vars baked into the service.create payload aren't individually
  // removable — changeId stays null.
  const inline = Array.isArray(payload.env_vars)
    ? (payload.env_vars as Array<{ key?: string; value?: string }>)
    : [];
  for (const ev of inline) {
    if (typeof ev.key !== "string" || ev.key === "") continue;
    merged.set(ev.key, {
      key: ev.key,
      value: ev.value ?? "",
      changeId: null,
    });
    inlineKeys.add(ev.key);
  }

  // Env vars staged later under this temp id — these can be removed.
  for (const c of changes) {
    if (c.op !== "env_var.create" || c.parent_temp_id !== tempId) continue;
    const p = parsePayload(c.payload);
    const key = typeof p.key === "string" ? p.key : null;
    if (!key) continue;
    merged.set(key, {
      key,
      value: typeof p.value === "string" ? p.value : "",
      changeId: c.id,
    });
  }

  return {
    name,
    image,
    envVars: Array.from(merged.values()),
    inlineEnvVars: inlineKeys,
  };
}

function extractMessage(err: unknown, fallback: string): string {
  if (err instanceof ApiError) {
    try {
      const body = JSON.parse(err.message);
      if (typeof body.message === "string") return body.message;
    } catch {
      // fall through
    }
  }
  if (err instanceof Error) return err.message || fallback;
  return fallback;
}

function AutoClose({ onClose }: { onClose: () => void }) {
  useEffect(() => {
    onClose();
  }, [onClose]);
  return null;
}
