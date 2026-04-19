import type { EnvVar, PendingChange, Service } from "./queries";

// Compacted view of the pending change log. Multiple edits targeting the
// same resource are collapsed into a single line for display. The raw log
// stays as-is in the DB; this projection is UI-only.
export interface CompactedChange {
  // Stable key for React list rendering.
  key: string;
  targetLabel: string;
  targetKind: "project" | "service" | "env_var";
  // For services being created this apply — the draft name. Used to show a
  // creating-service block even before deploy.
  pendingCreate?: boolean;
  pendingDelete?: boolean;
  // Field-level changes: renames, icon updates.
  fields: { label: string; before: string | null; after: string | null }[];
  // Env var changes keyed under the containing target.
  envVars: EnvVarDiffEntry[];
}

export interface EnvVarDiffEntry {
  key: string;
  op: "add" | "edit" | "remove";
  before: string | null;
  after: string | null;
}

export interface CompactInput {
  changes: PendingChange[];
  project: { id: string; name: string } | null | undefined;
  services: Service[];
  projectEnvVars: EnvVar[];
  // Service-scoped env vars resolved for each known service. Keyed by service id.
  serviceEnvVars: Record<string, EnvVar[]>;
}

type ParsedPayload = Record<string, unknown>;

function parsePayload(raw: unknown): ParsedPayload {
  if (typeof raw === "string") {
    try {
      return JSON.parse(raw);
    } catch {
      return {};
    }
  }
  if (raw && typeof raw === "object") {
    return raw as ParsedPayload;
  }
  return {};
}

// findEnvVar locates an env var by ID across both project and service scopes.
function findEnvVar(id: string, input: CompactInput): EnvVar | undefined {
  for (const ev of input.projectEnvVars) {
    if (ev.id === id) return ev;
  }
  for (const list of Object.values(input.serviceEnvVars)) {
    for (const ev of list) {
      if (ev.id === id) return ev;
    }
  }
  return undefined;
}

// compactChanges produces a per-target summary of the log, collapsing
// redundant or cancelling entries.
export function compactChanges(input: CompactInput): CompactedChange[] {
  const { changes, project, services } = input;
  if (changes.length === 0) return [];

  // Group by (targetKind, resolvedKey). Use the real target id when we have
  // it, else the temp id or parent temp id.
  const groups = new Map<string, CompactedChange>();
  const getGroup = (
    key: string,
    targetLabel: string,
    targetKind: CompactedChange["targetKind"],
  ): CompactedChange => {
    const existing = groups.get(key);
    if (existing) return existing;
    const g: CompactedChange = {
      key,
      targetLabel,
      targetKind,
      fields: [],
      envVars: [],
    };
    groups.set(key, g);
    return g;
  };

  // Track temp ids of pending-created services so env vars under them map
  // to the pending-service group, not a missing real service.
  const pendingServiceNames = new Map<string, string>(); // temp_id -> name
  for (const c of changes) {
    if (c.op === "service.create" && c.target_temp_id) {
      const p = parsePayload(c.payload);
      const name = typeof p.name === "string" ? p.name : "(unnamed)";
      pendingServiceNames.set(c.target_temp_id, name);
    }
  }

  for (const c of changes) {
    const payload = parsePayload(c.payload);

    switch (c.op) {
      case "project.update": {
        const g = getGroup(
          "project",
          project ? `Project "${project.name}"` : "Project",
          "project",
        );
        const newName = typeof payload.name === "string" ? payload.name : null;
        if (newName != null && project && newName !== project.name) {
          // Merge: latest rename wins.
          const existing = g.fields.find((f) => f.label === "Name");
          if (existing) existing.after = newName;
          else g.fields.push({ label: "Name", before: project.name, after: newName });
        }
        break;
      }

      case "service.create": {
        if (!c.target_temp_id) break;
        const key = `service:temp:${c.target_temp_id}`;
        const name =
          typeof payload.name === "string" ? payload.name : "(unnamed)";
        const g = getGroup(key, `Service "${name}"`, "service");
        g.pendingCreate = true;
        // Carry inline env vars from the create payload so they show under
        // the same service block.
        const inlineEnvs = Array.isArray(payload.env_vars)
          ? (payload.env_vars as Array<{ key?: string; value?: string }>)
          : [];
        for (const ev of inlineEnvs) {
          if (!ev.key) continue;
          g.envVars.push({
            key: ev.key,
            op: "add",
            before: null,
            after: ev.value ?? "",
          });
        }
        break;
      }

      case "service.update": {
        if (!c.target_id) break;
        const svc = services.find((s) => s.id === c.target_id);
        const label = svc ? `Service "${svc.name}"` : "Service";
        const g = getGroup(`service:${c.target_id}`, label, "service");
        if (typeof payload.name === "string") {
          const before = svc?.name ?? null;
          const after = payload.name;
          if (before !== after) {
            const existing = g.fields.find((f) => f.label === "Name");
            if (existing) existing.after = after;
            else g.fields.push({ label: "Name", before, after });
          }
        }
        if ("icon_url" in payload) {
          const after =
            typeof payload.icon_url === "string" && payload.icon_url !== ""
              ? payload.icon_url
              : null;
          const before = svc?.icon_url ?? null;
          if (before !== after) {
            const existing = g.fields.find((f) => f.label === "Icon");
            if (existing) existing.after = after;
            else g.fields.push({ label: "Icon", before, after });
          }
        }
        break;
      }

      case "service.delete": {
        if (!c.target_id) break;
        const svc = services.find((s) => s.id === c.target_id);
        const label = svc ? `Service "${svc.name}"` : "Service";
        const g = getGroup(`service:${c.target_id}`, label, "service");
        g.pendingDelete = true;
        break;
      }

      case "env_var.create": {
        const key = typeof payload.key === "string" ? payload.key : "";
        const value = typeof payload.value === "string" ? payload.value : "";
        if (!key) break;

        // Env var target: parent_temp_id (pending service), target_id project or service id.
        if (c.parent_temp_id) {
          const name = pendingServiceNames.get(c.parent_temp_id) ?? "(unnamed)";
          const g = getGroup(
            `service:temp:${c.parent_temp_id}`,
            `Service "${name}"`,
            "service",
          );
          g.pendingCreate = true;
          mergeEnvVarAdd(g, key, value);
        } else if (payload.scope === "project" && c.target_id) {
          const g = getGroup(
            "project",
            project ? `Project "${project.name}"` : "Project",
            "project",
          );
          mergeEnvVarAdd(g, key, value);
        } else if (payload.scope === "service" && c.target_id) {
          const svc = services.find((s) => s.id === c.target_id);
          const label = svc ? `Service "${svc.name}"` : "Service";
          const g = getGroup(`service:${c.target_id}`, label, "service");
          mergeEnvVarAdd(g, key, value);
        }
        break;
      }

      case "env_var.update": {
        if (!c.target_id) break;
        const ev = findEnvVar(c.target_id, input);
        if (!ev) break;
        const newValue = typeof payload.value === "string" ? payload.value : "";
        if (ev.scope === "project") {
          const g = getGroup(
            "project",
            project ? `Project "${project.name}"` : "Project",
            "project",
          );
          mergeEnvVarEdit(g, ev.key, ev.value, newValue);
        } else {
          const svc = services.find((s) => s.id === ev.scope_id);
          const label = svc ? `Service "${svc.name}"` : "Service";
          const g = getGroup(`service:${ev.scope_id}`, label, "service");
          mergeEnvVarEdit(g, ev.key, ev.value, newValue);
        }
        break;
      }

      case "env_var.delete": {
        if (!c.target_id) break;
        const ev = findEnvVar(c.target_id, input);
        if (!ev) break;
        if (ev.scope === "project") {
          const g = getGroup(
            "project",
            project ? `Project "${project.name}"` : "Project",
            "project",
          );
          mergeEnvVarRemove(g, ev.key, ev.value);
        } else {
          const svc = services.find((s) => s.id === ev.scope_id);
          const label = svc ? `Service "${svc.name}"` : "Service";
          const g = getGroup(`service:${ev.scope_id}`, label, "service");
          mergeEnvVarRemove(g, ev.key, ev.value);
        }
        break;
      }
    }
  }

  // Filter out no-op groups: no fields, no env var changes, no create/delete marker.
  return Array.from(groups.values()).filter(
    (g) =>
      g.pendingCreate ||
      g.pendingDelete ||
      g.fields.length > 0 ||
      g.envVars.length > 0,
  );
}

function mergeEnvVarAdd(g: CompactedChange, key: string, value: string) {
  const existing = g.envVars.find((e) => e.key === key);
  if (!existing) {
    g.envVars.push({ key, op: "add", before: null, after: value });
    return;
  }
  if (existing.op === "remove") {
    // Remove then add → net edit from the old value to the new.
    existing.op = "edit";
    existing.after = value;
    return;
  }
  // add/edit → keep kind, update after value.
  existing.after = value;
}

function mergeEnvVarEdit(
  g: CompactedChange,
  key: string,
  before: string,
  after: string,
) {
  const existing = g.envVars.find((e) => e.key === key);
  if (!existing) {
    if (before === after) return;
    g.envVars.push({ key, op: "edit", before, after });
    return;
  }
  // add/edit path — update terminal value.
  existing.after = after;
  if (existing.op === "remove") {
    existing.op = "edit";
  }
}

function mergeEnvVarRemove(g: CompactedChange, key: string, before: string) {
  const existing = g.envVars.find((e) => e.key === key);
  if (!existing) {
    g.envVars.push({ key, op: "remove", before, after: null });
    return;
  }
  if (existing.op === "add") {
    // Add then remove on a pending-new key → net no-op; drop it.
    g.envVars = g.envVars.filter((e) => e.key !== key);
    return;
  }
  existing.op = "remove";
  existing.after = null;
}
