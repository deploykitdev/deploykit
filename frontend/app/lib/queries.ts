import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "./api";

export interface Project {
  id: string;
  name: string;
  slug: string;
  created_at: string;
}

export interface User {
  id: string;
  email: string;
  name: string;
  role: string;
  created_at: string;
  updated_at: string;
}

export interface PortMapping {
  container_port: number;
  host_port?: number;
  protocol?: string;
}

export interface ResourceLimits {
  cpu_shares?: number;
  memory_mb?: number;
}

export interface Deployment {
  id: string;
  service_id: string;
  image: string;
  env_vars: Record<string, string> | null;
  ports: PortMapping[] | null;
  resources?: ResourceLimits | null;
  replicas: number;
  created_at: string;
}

export interface Service {
  id: string;
  project_id: string;
  name: string;
  status: string;
  icon_url: string | null;
  active_deployment_id: string | null;
  created_at: string;
  updated_at: string;
  active_deployment?: Deployment | null;
}

export interface ReleaseInfo {
  version: string;
  url: string;
  notes?: string;
  published_at: string;
  fetched_at: string;
}

export interface SystemAbout {
  deploykit: {
    version: string;
    go_version: string;
    started_at: string;
  };
  docker: {
    reachable: boolean;
    error?: string;
    server_version?: string;
    api_version?: string;
    os?: string;
    kernel_version?: string;
    architecture?: string;
    storage_driver?: string;
    logging_driver?: string;
    cgroup_driver?: string;
    docker_root_dir?: string;
    warnings?: string[];
  };
  database: {
    path: string;
    size_bytes: number;
  };
  latest_release?: ReleaseInfo;
  update_available: boolean;
}

export type UpgradeState =
  | "idle"
  | "queued"
  | "running"
  | "done"
  | "failed";

export interface UpgradeStatus {
  state: UpgradeState;
  target_version?: string;
  started_at?: string;
  finished_at?: string;
  error?: string;
  log_tail?: string;
}

export interface SystemSettings {
  auto_update: boolean;
}

export interface SystemStatus {
  host: {
    hostname: string;
    uptime: number;
    cpu: {
      cores: number;
      usage_pct: number;
      load1: number;
      load5: number;
      load15: number;
    };
    memory: {
      total_bytes: number;
      used_bytes: number;
      usage_pct: number;
    };
    swap: {
      total_bytes: number;
      used_bytes: number;
      usage_pct: number;
    };
    disks: Array<{
      mountpoint: string;
      total_bytes: number;
      used_bytes: number;
      usage_pct: number;
    }>;
  };
  docker: {
    reachable: boolean;
    containers: number;
    containers_running: number;
    containers_stopped: number;
    images: number;
    images_size_bytes: number;
    volumes: number;
    volumes_size_bytes: number;
    build_cache_bytes: number;
  };
}

export type EnvVarScope = "project" | "group" | "service";

export interface EnvVar {
  id: string;
  scope: EnvVarScope;
  scope_id: string;
  key: string;
  value: string;
  created_at: string;
  updated_at: string;
}

// Pending change op codes — match the backend's deploykit.PendingChangeOp.
export type PendingChangeOp =
  | "project.update"
  | "service.create"
  | "service.update"
  | "service.delete"
  | "env_var.create"
  | "env_var.update"
  | "env_var.delete";

export type PendingChangeTarget = "project" | "service" | "env_var";

export interface PendingChange {
  id: string;
  project_id: string;
  seq: number;
  op: PendingChangeOp;
  target_type: PendingChangeTarget;
  target_id?: string | null;
  target_temp_id?: string | null;
  parent_temp_id?: string | null;
  payload: unknown;
  user_id?: string | null;
  created_at: string;
}

export interface ApplyResult {
  applied_count: number;
  temp_id_to_service_id: Record<string, string>;
  redeployed_service_ids: string[];
  created_deployments: Deployment[];
}

export interface PresetEnvVar {
  key: string;
  value: string;
  // Present on List responses, stripped on Get responses (server materializes).
  generate?: string;
}

export interface Preset {
  id: string;
  name: string;
  image: string;
  icon_url: string;
  ports?: PortMapping[];
  env_vars: PresetEnvVar[];
}

export const queryKeys = {
  projects: ["projects"] as const,
  project: (id: string) => ["projects", id] as const,
  projectServices: (projectId: string) =>
    ["projects", projectId, "services"] as const,
  service: (projectId: string, serviceId: string) =>
    ["projects", projectId, "services", serviceId] as const,
  deployments: (projectId: string, serviceId: string) =>
    ["projects", projectId, "services", serviceId, "deployments"] as const,
  projectEnvVars: (projectId: string) =>
    ["projects", projectId, "env-vars"] as const,
  serviceEnvVars: (projectId: string, serviceId: string) =>
    ["projects", projectId, "services", serviceId, "env-vars"] as const,
  groupEnvVars: (projectId: string, groupId: string) =>
    ["projects", projectId, "groups", groupId, "env-vars"] as const,
  pendingChanges: (projectId: string) =>
    ["projects", projectId, "pending-changes"] as const,
  users: ["users"] as const,
  systemAbout: ["system", "about"] as const,
  systemStatus: ["system", "status"] as const,
  systemRelease: ["system", "release"] as const,
  systemUpgrade: ["system", "upgrade"] as const,
  systemSettings: ["system", "settings"] as const,
  databasePresets: ["presets", "databases"] as const,
};

export function useDatabasePresets() {
  return useQuery({
    queryKey: queryKeys.databasePresets,
    queryFn: () => api<Preset[]>("/presets/databases"),
    // Catalog is static for the lifetime of the backend process.
    staleTime: Infinity,
  });
}

// fetchDatabasePreset is imperative on purpose: each call re-runs server-side
// generators, so we want a fresh request when the user picks a preset card,
// not a cached response.
export function fetchDatabasePreset(id: string) {
  return api<Preset>(`/presets/databases/${id}`);
}

export function useProjects() {
  return useQuery({
    queryKey: queryKeys.projects,
    queryFn: () =>
      api<{ data: Project[] }>("/projects").then((r) => r.data ?? []),
  });
}

export function useProject(id: string) {
  return useQuery({
    queryKey: queryKeys.project(id),
    queryFn: () => api<Project>(`/projects/${id}`),
  });
}

export function useProjectServices(projectId: string) {
  return useQuery({
    queryKey: queryKeys.projectServices(projectId),
    queryFn: () =>
      api<{ data: Service[]; total_count: number }>(
        `/projects/${projectId}/services?limit=100`,
      ).then((r) => r.data ?? []),
  });
}

export function useService(projectId: string, serviceId: string | null) {
  return useQuery({
    queryKey: serviceId
      ? queryKeys.service(projectId, serviceId)
      : ["projects", projectId, "services", "none"],
    queryFn: () =>
      api<Service>(`/projects/${projectId}/services/${serviceId}`),
    enabled: !!serviceId,
  });
}

export interface ServiceUpdate {
  name?: string;
  // Empty string clears the stored icon (backend stores NULL).
  icon_url?: string;
}

export function useUpdateService(projectId: string, serviceId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (update: ServiceUpdate) =>
      api<PendingChange>(`/projects/${projectId}/services/${serviceId}`, {
        method: "PATCH",
        body: JSON.stringify(update),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pendingChanges(projectId) });
    },
  });
}

export function useDeployments(projectId: string, serviceId: string | null) {
  return useQuery({
    queryKey: serviceId
      ? queryKeys.deployments(projectId, serviceId)
      : ["projects", projectId, "services", "none", "deployments"],
    queryFn: () =>
      api<{ data: Deployment[]; total_count: number }>(
        `/projects/${projectId}/services/${serviceId}/deployments`,
      ).then((r) => r.data ?? []),
    enabled: !!serviceId,
  });
}

export function useCreateProject() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) =>
      api<Project>("/projects", {
        method: "POST",
        body: JSON.stringify({ name }),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.projects });
    },
  });
}

export function useUpdateProject(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (update: { name?: string }) =>
      api<PendingChange>(`/projects/${id}`, {
        method: "PATCH",
        body: JSON.stringify(update),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pendingChanges(id) });
    },
  });
}

export function useUsers() {
  return useQuery({
    queryKey: queryKeys.users,
    queryFn: () =>
      api<{ data: User[]; total_count: number }>("/users").then(
        (r) => r.data ?? [],
      ),
  });
}

export function useCreateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: {
      name: string;
      email: string;
      password: string;
      role: string;
    }) =>
      api<User>("/users", {
        method: "POST",
        body: JSON.stringify(data),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.users });
    },
  });
}

export function useUpdateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      ...data
    }: {
      id: string;
      name?: string;
      email?: string;
      role?: string;
    }) =>
      api<User>(`/users/${id}`, {
        method: "PATCH",
        body: JSON.stringify(data),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.users });
    },
  });
}

export function useUpdateProfile() {
  return useMutation({
    mutationFn: (data: {
      name?: string;
      email?: string;
      new_password?: string;
      current_password: string;
    }) =>
      api<User>("/auth/profile", {
        method: "PATCH",
        body: JSON.stringify(data),
      }),
  });
}

export function useSystemAbout() {
  return useQuery({
    queryKey: queryKeys.systemAbout,
    queryFn: () => api<SystemAbout>("/system/about"),
  });
}

export function useSystemStatus() {
  return useQuery({
    queryKey: queryKeys.systemStatus,
    queryFn: () => api<SystemStatus>("/system/status"),
    refetchInterval: 3000,
    refetchIntervalInBackground: false,
  });
}

export function useRefreshLatestRelease() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      api<ReleaseInfo>("/system/release?refresh=1"),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.systemAbout });
      qc.invalidateQueries({ queryKey: queryKeys.systemRelease });
    },
  });
}

export function useUpgradeStatus(opts: { enabled?: boolean; refetchMs?: number } = {}) {
  const { enabled = true, refetchMs } = opts;
  return useQuery({
    queryKey: queryKeys.systemUpgrade,
    queryFn: () => api<UpgradeStatus>("/system/upgrade"),
    enabled,
    refetchInterval: refetchMs,
  });
}

export function useRequestUpgrade() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (version: string) =>
      api<UpgradeStatus>("/system/upgrade", {
        method: "POST",
        body: JSON.stringify({ version }),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.systemUpgrade });
    },
  });
}

export function useSystemSettings() {
  return useQuery({
    queryKey: queryKeys.systemSettings,
    queryFn: () => api<SystemSettings>("/system/settings"),
  });
}

export function useUpdateSystemSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (update: Partial<SystemSettings>) =>
      api<SystemSettings>("/system/settings", {
        method: "PATCH",
        body: JSON.stringify(update),
      }),
    onSuccess: (data) => {
      qc.setQueryData(queryKeys.systemSettings, data);
    },
  });
}

export function useDeleteUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api(`/users/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.users });
    },
  });
}

// --- Env vars ---
//
// All env var mutations now stage a pending change on the backend. The
// applied env_vars list endpoints still reflect committed values; the
// UI layers pending entries on top for display.

export function useProjectEnvVars(projectId: string) {
  return useQuery({
    queryKey: queryKeys.projectEnvVars(projectId),
    queryFn: () =>
      api<{ data: EnvVar[] }>(`/projects/${projectId}/env-vars`).then(
        (r) => r.data ?? [],
      ),
  });
}

export function useCreateProjectEnvVar(projectId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: { key: string; value: string }) =>
      api<PendingChange>(`/projects/${projectId}/env-vars`, {
        method: "POST",
        body: JSON.stringify(data),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pendingChanges(projectId) });
    },
  });
}

export function useUpdateProjectEnvVar(projectId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, value }: { id: string; value: string }) =>
      api<PendingChange>(`/projects/${projectId}/env-vars/${id}`, {
        method: "PATCH",
        body: JSON.stringify({ value }),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pendingChanges(projectId) });
    },
  });
}

export function useDeleteProjectEnvVar(projectId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api<PendingChange>(`/projects/${projectId}/env-vars/${id}`, {
        method: "DELETE",
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pendingChanges(projectId) });
    },
  });
}

export function useServiceEnvVars(projectId: string, serviceId: string) {
  return useQuery({
    queryKey: queryKeys.serviceEnvVars(projectId, serviceId),
    queryFn: () =>
      api<{ data: EnvVar[] }>(
        `/projects/${projectId}/services/${serviceId}/env-vars`,
      ).then((r) => r.data ?? []),
    // Pending-added services don't have env vars in the applied table yet.
    enabled: !serviceId.startsWith("pending:"),
  });
}

export function useCreateServiceEnvVar(projectId: string, serviceId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: { key: string; value: string }) =>
      api<PendingChange>(
        `/projects/${projectId}/services/${serviceId}/env-vars`,
        {
          method: "POST",
          body: JSON.stringify(data),
        },
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pendingChanges(projectId) });
    },
  });
}

export function useUpdateServiceEnvVar(projectId: string, serviceId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, value }: { id: string; value: string }) =>
      api<PendingChange>(
        `/projects/${projectId}/services/${serviceId}/env-vars/${id}`,
        {
          method: "PATCH",
          body: JSON.stringify({ value }),
        },
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pendingChanges(projectId) });
    },
  });
}

export function useDeleteServiceEnvVar(projectId: string, serviceId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api<PendingChange>(
        `/projects/${projectId}/services/${serviceId}/env-vars/${id}`,
        {
          method: "DELETE",
        },
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pendingChanges(projectId) });
    },
  });
}

export function useGroupEnvVars(projectId: string, groupId: string | null) {
  return useQuery({
    queryKey: queryKeys.groupEnvVars(projectId, groupId ?? ""),
    queryFn: () =>
      api<{ data: EnvVar[] }>(
        `/projects/${projectId}/groups/${groupId}/env-vars`,
      ).then((r) => r.data ?? []),
    enabled: !!groupId,
  });
}

export function useCreateGroupEnvVar(projectId: string, groupId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: { key: string; value: string }) =>
      api<PendingChange>(
        `/projects/${projectId}/groups/${groupId}/env-vars`,
        {
          method: "POST",
          body: JSON.stringify(data),
        },
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pendingChanges(projectId) });
    },
  });
}

export function useUpdateGroupEnvVar(projectId: string, groupId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, value }: { id: string; value: string }) =>
      api<PendingChange>(
        `/projects/${projectId}/groups/${groupId}/env-vars/${id}`,
        {
          method: "PATCH",
          body: JSON.stringify({ value }),
        },
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pendingChanges(projectId) });
    },
  });
}

export function useDeleteGroupEnvVar(projectId: string, groupId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api<PendingChange>(
        `/projects/${projectId}/groups/${groupId}/env-vars/${id}`,
        {
          method: "DELETE",
        },
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pendingChanges(projectId) });
    },
  });
}

// --- Pending changes ---

export function usePendingChanges(projectId: string) {
  return useQuery({
    queryKey: queryKeys.pendingChanges(projectId),
    queryFn: () =>
      api<{ data: PendingChange[] }>(
        `/projects/${projectId}/pending-changes`,
      ).then((r) => r.data ?? []),
  });
}

export function useAddPendingServiceEnvVar(projectId: string, tempId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: { key: string; value: string }) =>
      api<PendingChange>(
        `/projects/${projectId}/pending-services/${tempId}/env-vars`,
        {
          method: "POST",
          body: JSON.stringify(data),
        },
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pendingChanges(projectId) });
    },
  });
}

// useRemovePendingChange backs out a single pending change entry. Used for
// env var edits on pending-added services; the bottom panel still owns the
// all-or-nothing Discard for everything else.
export function useRemovePendingChange(projectId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (changeId: string) =>
      api(`/projects/${projectId}/pending-changes/${changeId}`, {
        method: "DELETE",
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pendingChanges(projectId) });
    },
  });
}

export function useDiscardPendingChanges(projectId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      api(`/projects/${projectId}/pending-changes`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pendingChanges(projectId) });
    },
  });
}

// useDeployProject applies every pending change atomically and triggers
// reconciliation. On success it invalidates all project-scoped data so the
// UI reloads applied state.
export function useDeployProject(projectId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      api<ApplyResult>(`/projects/${projectId}/deploy`, { method: "POST" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.pendingChanges(projectId) });
      qc.invalidateQueries({ queryKey: queryKeys.project(projectId) });
      qc.invalidateQueries({ queryKey: queryKeys.projectServices(projectId) });
      qc.invalidateQueries({ queryKey: queryKeys.projectEnvVars(projectId) });
      qc.invalidateQueries({
        predicate: (q) => {
          const key = q.queryKey;
          return (
            Array.isArray(key) && key[0] === "projects" && key[1] === projectId
          );
        },
      });
    },
  });
}
