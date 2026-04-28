import { useMemo, useState } from "react";
import { toast } from "sonner";
import {
  ChevronDownIcon,
  ChevronUpIcon,
  MinusIcon,
  PencilIcon,
  PlusIcon,
  TrashIcon,
} from "lucide-react";
import { Button } from "./ui/button";
import {
  useDeployProject,
  useDiscardPendingChanges,
  usePendingChanges,
  useProject,
  useProjectServices,
} from "@/lib/queries";
import {
  compactChanges,
  type CompactedChange,
  type EnvVarDiffEntry,
} from "@/lib/pending-changes-diff";

interface PendingChangesPanelProps {
  projectId: string;
  // Canvas group nodes (id + label) so group-scoped env var changes can be
  // labeled "Group <name>" in the compacted view.
  groupNodes?: { id: string; label: string }[];
  // Map of service id → current canvas parent group id (null = top-level).
  // Used to render the "after" side of a staged reparent.
  serviceParents?: Map<string, string | null>;
}

export function PendingChangesPanel({ projectId, groupNodes, serviceParents }: PendingChangesPanelProps) {
  const { data: changes } = usePendingChanges(projectId);
  const { data: project } = useProject(projectId);
  const { data: services } = useProjectServices(projectId);
  const discard = useDiscardPendingChanges(projectId);
  const deploy = useDeployProject(projectId);
  const [expanded, setExpanded] = useState(false);

  const groups = useMemo(() => {
    if (!changes || changes.length === 0) return [];
    return compactChanges({
      changes,
      project: project ? { id: project.id, name: project.name } : null,
      services: services ?? [],
      groups: groupNodes,
      serviceParents,
    });
  }, [changes, project, services, groupNodes, serviceParents]);

  if (!changes || changes.length === 0) return null;

  const count = changes.length;
  const deploying = deploy.isPending;
  const discarding = discard.isPending;

  const handleDeploy = () => {
    deploy.mutate(undefined, {
      onSuccess: (res) => {
        toast.success(
          `Deployed ${res.applied_count} ${res.applied_count === 1 ? "change" : "changes"}`,
        );
      },
      onError: (err) => {
        toast.error("Deploy failed", {
          description: err instanceof Error ? err.message : "Unknown error",
        });
      },
    });
  };

  const handleDiscard = () => {
    if (!window.confirm(`Discard all ${count} pending changes?`)) return;
    discard.mutate(undefined, {
      onError: (err) => {
        toast.error("Failed to discard changes", {
          description: err instanceof Error ? err.message : "Unknown error",
        });
      },
    });
  };

  return (
    <div className="pointer-events-auto absolute bottom-4 left-1/2 z-10 w-[min(640px,calc(100vw-2rem))] -translate-x-1/2 rounded-xl border bg-background/95 shadow-2xl backdrop-blur">
      <div className="flex items-center gap-3 px-4 py-3">
        <button
          type="button"
          onClick={() => setExpanded((e) => !e)}
          className="flex items-center gap-2 text-sm font-medium hover:text-foreground"
          aria-label={expanded ? "Collapse details" : "Show details"}
        >
          {expanded ? (
            <ChevronDownIcon className="size-4 text-muted-foreground" />
          ) : (
            <ChevronUpIcon className="size-4 text-muted-foreground" />
          )}
          <span>
            {count} pending {count === 1 ? "change" : "changes"}
          </span>
        </button>
        <div className="ml-auto flex items-center gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={handleDiscard}
            disabled={discarding || deploying}
          >
            Discard all
          </Button>
          <Button size="sm" onClick={handleDeploy} disabled={deploying || discarding}>
            {deploying ? "Deploying…" : "Deploy"}
          </Button>
        </div>
      </div>

      {expanded && groups.length > 0 ? (
        <div className="max-h-[50vh] overflow-y-auto border-t">
          <ul className="divide-y">
            {groups.map((g) => (
              <li key={g.key} className="px-4 py-3">
                <CompactedGroupView group={g} />
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}

function CompactedGroupView({ group }: { group: CompactedChange }) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-2 text-sm font-medium">
        <span>{group.targetLabel}</span>
        {group.pendingCreate ? (
          <span className="rounded bg-emerald-500/10 px-1.5 py-0.5 text-xs text-emerald-600 dark:text-emerald-400">
            creating
          </span>
        ) : null}
        {group.pendingDelete ? (
          <span className="rounded bg-destructive/10 px-1.5 py-0.5 text-xs text-destructive">
            deleting
          </span>
        ) : null}
      </div>
      {group.fields.map((f) => (
        <div
          key={f.label}
          className="flex items-center gap-2 pl-2 text-xs text-muted-foreground"
        >
          <PencilIcon className="size-3 shrink-0" />
          <span className="shrink-0">{f.label}:</span>
          {f.before != null ? (
            <code className="rounded bg-muted px-1 py-0.5 text-foreground line-through decoration-muted-foreground/50">
              {f.before}
            </code>
          ) : null}
          <span className="text-muted-foreground">→</span>
          {f.after != null ? (
            <code className="rounded bg-muted px-1 py-0.5 text-foreground">
              {f.after}
            </code>
          ) : (
            <span className="text-muted-foreground italic">cleared</span>
          )}
        </div>
      ))}
      {group.envVars.map((ev) => (
        <EnvVarDiffRow key={ev.key} entry={ev} />
      ))}
    </div>
  );
}

function EnvVarDiffRow({ entry }: { entry: EnvVarDiffEntry }) {
  const Icon =
    entry.op === "add" ? PlusIcon : entry.op === "remove" ? MinusIcon : PencilIcon;
  const tone =
    entry.op === "add"
      ? "text-emerald-600 dark:text-emerald-400"
      : entry.op === "remove"
        ? "text-destructive"
        : "text-amber-600 dark:text-amber-400";
  return (
    <div className="flex items-center gap-2 pl-2 text-xs text-muted-foreground">
      <Icon className={`size-3 shrink-0 ${tone}`} />
      <code className="shrink-0 rounded bg-muted px-1 py-0.5 font-mono text-foreground">
        {entry.key}
      </code>
      {entry.op !== "add" && entry.before != null ? (
        <code className="truncate rounded bg-muted px-1 py-0.5 text-foreground line-through decoration-muted-foreground/50">
          {entry.before}
        </code>
      ) : null}
      {entry.op !== "remove" && entry.after != null ? (
        <>
          {entry.op !== "add" ? <span>→</span> : null}
          <code className="truncate rounded bg-muted px-1 py-0.5 text-foreground">
            {entry.after}
          </code>
        </>
      ) : null}
      {entry.op === "remove" ? (
        <span className="text-muted-foreground italic flex items-center gap-1">
          <TrashIcon className="size-3" /> removed
        </span>
      ) : null}
    </div>
  );
}
