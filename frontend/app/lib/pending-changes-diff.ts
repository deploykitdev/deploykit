import type { PendingChange, Service } from "./queries";

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
}

type ParsedPayload = Record<string, unknown>;

export function parsePayload(raw: unknown): ParsedPayload {
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

// Per-service override distilled from the pending change log. `iconUrlSet`
// being present (even with null) means an update was staged that explicitly
// touched the icon — null means clear, string means set; undefined means no
// staged icon change, so applied state should show.
export interface ServiceOverride {
  name?: string;
  iconUrlSet?: string | null;
  pendingDelete?: boolean;
}

// collectServiceOverrides walks the log once and produces per-service
// rename / icon / pending-delete overrides, keyed by applied service ID.
// Latest-write-wins for competing updates.
export function collectServiceOverrides(
  changes: PendingChange[] | undefined,
): Map<string, ServiceOverride> {
  const out = new Map<string, ServiceOverride>();
  if (!changes) return out;
  for (const c of changes) {
    if (c.op === "service.update" && c.target_id) {
      const payload = parsePayload(c.payload);
      const existing = out.get(c.target_id) ?? {};
      if (typeof payload.name === "string") existing.name = payload.name;
      if ("icon_url" in payload) {
        const raw = payload.icon_url;
        existing.iconUrlSet =
          typeof raw === "string" && raw !== "" ? raw : null;
      }
      out.set(c.target_id, existing);
    } else if (c.op === "service.delete" && c.target_id) {
      const existing = out.get(c.target_id) ?? {};
      existing.pendingDelete = true;
      out.set(c.target_id, existing);
    }
  }
  return out;
}

// collectServiceOverride is the single-service variant used by side panels
// that only care about one target.
export function collectServiceOverride(
  changes: PendingChange[] | undefined,
  serviceId: string,
): ServiceOverride | undefined {
  if (!changes) return undefined;
  let out: ServiceOverride | undefined;
  for (const c of changes) {
    if (c.target_id !== serviceId) continue;
    if (c.op === "service.update") {
      const payload = parsePayload(c.payload);
      if (!out) out = {};
      if (typeof payload.name === "string") out.name = payload.name;
      if ("icon_url" in payload) {
        const raw = payload.icon_url;
        out.iconUrlSet = typeof raw === "string" && raw !== "" ? raw : null;
      }
    } else if (c.op === "service.delete") {
      if (!out) out = {};
      out.pendingDelete = true;
    }
  }
  return out;
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
        const key = typeof payload.key === "string" ? payload.key : "";
        const scope = payload.scope;
        const scopeID = typeof payload.scope_id === "string" ? payload.scope_id : "";
        const newValue = typeof payload.value === "string" ? payload.value : "";
        const oldValue =
          typeof payload.old_value === "string" ? payload.old_value : null;
        if (!key || !scopeID) break;
        if (scope === "project") {
          const g = getGroup(
            "project",
            project ? `Project "${project.name}"` : "Project",
            "project",
          );
          mergeEnvVarEdit(g, key, oldValue, newValue);
        } else if (scope === "service") {
          const svc = services.find((s) => s.id === scopeID);
          const label = svc ? `Service "${svc.name}"` : "Service";
          const g = getGroup(`service:${scopeID}`, label, "service");
          mergeEnvVarEdit(g, key, oldValue, newValue);
        }
        break;
      }

      case "env_var.delete": {
        const key = typeof payload.key === "string" ? payload.key : "";
        const scope = payload.scope;
        const scopeID = typeof payload.scope_id === "string" ? payload.scope_id : "";
        const oldValue =
          typeof payload.old_value === "string" ? payload.old_value : null;
        if (!key || !scopeID) break;
        if (scope === "project") {
          const g = getGroup(
            "project",
            project ? `Project "${project.name}"` : "Project",
            "project",
          );
          mergeEnvVarRemove(g, key, oldValue);
        } else if (scope === "service") {
          const svc = services.find((s) => s.id === scopeID);
          const label = svc ? `Service "${svc.name}"` : "Service";
          const g = getGroup(`service:${scopeID}`, label, "service");
          mergeEnvVarRemove(g, key, oldValue);
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
  before: string | null,
  after: string,
) {
  const existing = g.envVars.find((e) => e.key === key);
  if (!existing) {
    g.envVars.push({ key, op: "edit", before, after });
    return;
  }
  existing.after = after;
  if (existing.op === "remove") {
    existing.op = "edit";
  }
}

function mergeEnvVarRemove(
  g: CompactedChange,
  key: string,
  before: string | null,
) {
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
  // Preserve the before value if the previous entry didn't have one.
  if (existing.before == null && before != null) {
    existing.before = before;
  }
}
