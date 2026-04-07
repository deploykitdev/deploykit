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

export const queryKeys = {
  projects: ["projects"] as const,
  project: (id: string) => ["projects", id] as const,
  users: ["users"] as const,
  systemAbout: ["system", "about"] as const,
  systemStatus: ["system", "status"] as const,
};

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
