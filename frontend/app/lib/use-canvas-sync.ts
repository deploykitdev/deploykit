import { useEffect, useRef, useCallback, useState } from "react";
import {
  type Node,
  type Edge,
  type NodeChange,
  type EdgeChange,
  type Connection,
  applyNodeChanges,
  applyEdgeChanges,
  addEdge,
} from "@xyflow/react";
import { useQueryClient } from "@tanstack/react-query";
import { CanvasWebSocket, type ConnectionStatus } from "./canvas-ws";
import {
  queryKeys,
  type Service,
  type Deployment,
  type PendingChange,
} from "./queries";
import { randomUUID } from "./utils";

export interface CursorInfo {
  user_id: string;
  user_name: string;
  x: number;
  y: number;
}

export interface ConnectedUser {
  user_id: string;
  user_name: string;
}

interface CanvasNode {
  id: string;
  project_id: string;
  type: string;
  label: string;
  position_x: number;
  position_y: number;
  width?: number;
  height?: number;
  service_id?: string;
  parent_id?: string;
  data: string;
}

interface CanvasEdge {
  id: string;
  project_id: string;
  source_id: string;
  target_id: string;
  label?: string;
  data: string;
}

interface NodePosition {
  id: string;
  position_x: number;
  position_y: number;
}

export interface ServiceDraft {
  draft_id: string;
  user_id: string;
  user_name: string;
  x: number;
  y: number;
}

export interface DraftEnvVar {
  key: string;
  value: string;
}

export interface ServiceDraftPrefill {
  name?: string;
  image?: string;
  iconUrl?: string;
  envVars?: DraftEnvVar[];
}

export interface LocalServiceDraft {
  draftId: string;
  x: number;
  y: number;
  isSubmitting: boolean;
  errorMessage: string | null;
  prefill?: ServiceDraftPrefill;
}

function toFlowNode(dbNode: CanvasNode): Node {
  let extraData = {};
  try {
    extraData = JSON.parse(dbNode.data);
  } catch {
    // ignore
  }

  let flowType: string;
  switch (dbNode.type) {
    case "group":
      flowType = "group";
      break;
    case "service":
      flowType = "service";
      break;
    case "note":
      flowType = "note";
      break;
    default:
      flowType = "default";
  }

  // Strip React Flow's default `.react-flow__node-group` styling (a solid
  // border + faint fill) so the group's own dashed border can stand alone.
  const groupStyleOverride =
    dbNode.type === "group"
      ? { border: "none", background: "transparent", padding: 0 }
      : {};

  const sizeStyle =
    dbNode.width && dbNode.height
      ? { width: dbNode.width, height: dbNode.height }
      : {};

  const style = { ...groupStyleOverride, ...sizeStyle };

  return {
    id: dbNode.id,
    type: flowType,
    position: { x: dbNode.position_x, y: dbNode.position_y },
    data: { label: dbNode.label, serviceId: dbNode.service_id, ...extraData },
    ...(Object.keys(style).length > 0 ? { style } : {}),
    // React Flow grouping: when parent_id is set, position is relative to the
    // parent. We intentionally don't set extent:'parent' so users can drag a
    // child out of the group to detach it (handled in onNodeDragStop).
    ...(dbNode.parent_id ? { parentId: dbNode.parent_id } : {}),
  };
}

// Pluck out the callback fields ProjectFlow injects into node data so a
// server-side echo doesn't blow them away on the next render.
function injectedCallbacks(data: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  if (typeof data.commitNote === "function") out.commitNote = data.commitNote;
  if (typeof data.commitGroup === "function") out.commitGroup = data.commitGroup;
  if (typeof data.openPanel === "function") out.openPanel = data.openPanel;
  if (typeof data.deleteGroup === "function") out.deleteGroup = data.deleteGroup;
  if (typeof data.resizeGroup === "function") out.resizeGroup = data.resizeGroup;
  return out;
}

// stripInjected returns data without the locally-injected callback fields,
// so a comparison against a server payload doesn't see them.
function stripInjected(data: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(data)) {
    if (typeof v === "function") continue;
    out[k] = v;
  }
  return out;
}

// Recursive value equality for the kinds of values a server payload can
// carry: primitives, arrays, and plain objects. Reference-equal objects
// short-circuit. Used by the node:upserted skip-replace path so future
// data fields with nested shapes (lists, configs) don't force unnecessary
// re-renders just because the server sent a fresh-but-equal instance.
function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (a == null || b == null) return false;
  if (typeof a !== "object" || typeof b !== "object") return false;
  if (Array.isArray(a)) {
    if (!Array.isArray(b) || a.length !== b.length) return false;
    for (let i = 0; i < a.length; i++) {
      if (!deepEqual(a[i], b[i])) return false;
    }
    return true;
  }
  if (Array.isArray(b)) return false;
  const ao = a as Record<string, unknown>;
  const bo = b as Record<string, unknown>;
  const aKeys = Object.keys(ao);
  const bKeys = Object.keys(bo);
  if (aKeys.length !== bKeys.length) return false;
  for (const k of aKeys) {
    if (!deepEqual(ao[k], bo[k])) return false;
  }
  return true;
}

// React Flow requires a parent node to appear in the nodes array before any
// of its children — otherwise the child's parentId reference can't resolve at
// the moment React Flow walks the array. The backend already sorts this way
// in getNodes(), but local mutations (upserts, reparents) need to re-establish
// the invariant.
function ensureParentsFirst(nodes: Node[]): Node[] {
  if (!nodes.some((n) => n.parentId)) return nodes;
  const parents = nodes.filter((n) => !n.parentId);
  const children = nodes.filter((n) => n.parentId);
  return [...parents, ...children];
}

function toFlowEdge(dbEdge: CanvasEdge): Edge {
  let managed: string | undefined;
  if (dbEdge.data) {
    try {
      managed = (JSON.parse(dbEdge.data) as { managed?: string }).managed;
    } catch {
      // ignore — treat as user edge
    }
  }
  const isManaged = managed === "env-ref";

  return {
    id: dbEdge.id,
    source: dbEdge.source_id,
    target: dbEdge.target_id,
    type: "floating",
    ...(dbEdge.label ? { label: dbEdge.label } : {}),
    ...(isManaged
      ? {
          deletable: false,
          className: "managed-edge",
          style: { stroke: "var(--color-primary)", strokeDasharray: "4 4" },
          labelStyle: { fontSize: 10, color: "var(--color-primary)" },
          data: { managed },
        }
      : {}),
  };
}

export function useCanvasSync(projectId: string) {
  const queryClient = useQueryClient();
  const [nodes, setNodes] = useState<Node[]>([]);
  const [edges, setEdges] = useState<Edge[]>([]);
  const [cursors, setCursors] = useState<Map<string, CursorInfo>>(new Map());
  const [connectedUsers, setConnectedUsers] = useState<ConnectedUser[]>([]);
  const [connectionStatus, setConnectionStatus] =
    useState<ConnectionStatus>("disconnected");
  const prevStatusRef = useRef<ConnectionStatus>("disconnected");
  const [remoteDrafts, setRemoteDrafts] = useState<Map<string, ServiceDraft>>(
    new Map(),
  );
  const [localDrafts, setLocalDrafts] = useState<Map<string, LocalServiceDraft>>(
    new Map(),
  );
  const wsRef = useRef<CanvasWebSocket | null>(null);
  const dragThrottleLastRef = useRef<number>(0);
  const dragThrottleTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const dragLatestRef = useRef<NodePosition[] | null>(null);
  const cursorThrottleRef = useRef<number>(0);
  const remoteTargetsRef = useRef<Map<string, { x: number; y: number }>>(
    new Map(),
  );
  const lerpRafRef = useRef<number>(0);

  useEffect(() => {
    const ws = new CanvasWebSocket(projectId);
    wsRef.current = ws;

    ws.on<{ nodes: CanvasNode[]; edges: CanvasEdge[] }>(
      "canvas:state",
      (payload) => {
        setNodes(ensureParentsFirst(payload.nodes.map(toFlowNode)));
        setEdges(payload.edges.map(toFlowEdge));
      },
    );

    ws.on<CanvasNode>("node:upserted", (node) => {
      const flowNode = toFlowNode(node);
      setNodes((prev) => {
        const idx = prev.findIndex((n) => n.id === flowNode.id);
        if (idx >= 0) {
          const prevNode = prev[idx];
          // If our optimistic state already matches the server's view, skip
          // replacing the node object entirely. Re-creating identical objects
          // makes React Flow re-render and produces a brief visible jump on
          // drag-stop after a reparent or auto-fit.
          const prevStyle = (prevNode.style ?? {}) as { width?: number; height?: number };
          const newStyle = (flowNode.style ?? {}) as { width?: number; height?: number };
          const samePosition =
            Math.abs(prevNode.position.x - flowNode.position.x) < 0.5 &&
            Math.abs(prevNode.position.y - flowNode.position.y) < 0.5;
          const sameSize =
            (prevStyle.width ?? 0) === (newStyle.width ?? 0) &&
            (prevStyle.height ?? 0) === (newStyle.height ?? 0);
          const sameParent = (prevNode.parentId ?? null) === (flowNode.parentId ?? null);
          // Diff every server-controlled data field shallowly. Injected
          // callbacks (commitNote etc.) live on prev only and are filtered
          // out of the comparison so they don't cause spurious mismatches.
          const prevData = stripInjected((prevNode.data ?? {}) as Record<string, unknown>);
          const newData = (flowNode.data ?? {}) as Record<string, unknown>;
          const sameType = prevNode.type === flowNode.type;
          const sameData = deepEqual(prevData, newData);
          if (samePosition && sameSize && sameParent && sameType && sameData) {
            return prev;
          }
          const next = [...prev];
          // Preserve React Flow's transient interaction flags AND any caller-
          // injected callbacks on data (commitNote, commitGroup, etc.).
          next[idx] = {
            ...flowNode,
            data: { ...newData, ...injectedCallbacks(prevData) },
            selected: prevNode.selected,
            dragging: prevNode.dragging,
          };
          return ensureParentsFirst(next);
        }
        return ensureParentsFirst([...prev, flowNode]);
      });
    });

    ws.on<{ id: string }>("node:deleted", ({ id }) => {
      // If a group is deleted, orphan its children locally — backend uses
      // ON DELETE SET NULL on parent_id so children survive on the canvas at
      // their last absolute position. Convert relative coords back to absolute
      // before clearing parentId so they don't snap to (0,0).
      setNodes((prev) => {
        const removed = prev.find((n) => n.id === id);
        const removedAbsX = removed?.position.x ?? 0;
        const removedAbsY = removed?.position.y ?? 0;
        return prev
          .filter((n) => n.id !== id)
          .map((n) => {
            if (n.parentId !== id) return n;
            const { parentId: _p, extent: _e, ...rest } = n;
            return {
              ...rest,
              position: {
                x: n.position.x + removedAbsX,
                y: n.position.y + removedAbsY,
              },
            };
          });
      });
      setEdges((prev) =>
        prev.filter((e) => e.source !== id && e.target !== id),
      );
    });

    ws.on<{ positions: NodePosition[]; user_id: string }>(
      "node:moved",
      ({ positions }) => {
        for (const p of positions) {
          remoteTargetsRef.current.set(p.id, {
            x: p.position_x,
            y: p.position_y,
          });
        }
        if (!lerpRafRef.current) {
          const tick = () => {
            const targets = remoteTargetsRef.current;
            if (targets.size === 0) {
              lerpRafRef.current = 0;
              return;
            }
            const LERP = 0.25;
            const SNAP = 0.5;
            setNodes((prev) =>
              prev.map((n) => {
                const t = targets.get(n.id);
                if (!t) return n;
                const dx = t.x - n.position.x;
                const dy = t.y - n.position.y;
                if (Math.abs(dx) < SNAP && Math.abs(dy) < SNAP) {
                  targets.delete(n.id);
                  return { ...n, position: { x: t.x, y: t.y } };
                }
                return {
                  ...n,
                  position: {
                    x: n.position.x + dx * LERP,
                    y: n.position.y + dy * LERP,
                  },
                };
              }),
            );
            lerpRafRef.current = requestAnimationFrame(tick);
          };
          lerpRafRef.current = requestAnimationFrame(tick);
        }
      },
    );

    ws.on<CanvasEdge>("edge:upserted", (edge) => {
      const flowEdge = toFlowEdge(edge);
      setEdges((prev) => {
        const idx = prev.findIndex((e) => e.id === flowEdge.id);
        if (idx >= 0) {
          const next = [...prev];
          next[idx] = flowEdge;
          return next;
        }
        return [...prev, flowEdge];
      });
    });

    ws.on<{ id: string }>("edge:deleted", ({ id }) => {
      setEdges((prev) => prev.filter((e) => e.id !== id));
    });

    ws.on<CursorInfo>("cursor:updated", (cursor) => {
      setCursors((prev) => new Map(prev).set(cursor.user_id, cursor));
    });

    ws.on<{ users: ConnectedUser[] }>("users:list", ({ users }) => {
      setConnectedUsers(users);
    });

    ws.on<ConnectedUser>("user:joined", (user) => {
      setConnectedUsers((prev) => {
        if (prev.some((u) => u.user_id === user.user_id)) return prev;
        return [...prev, user];
      });
    });

    ws.on<{ user_id: string }>("user:left", ({ user_id }) => {
      setCursors((prev) => {
        const next = new Map(prev);
        next.delete(user_id);
        return next;
      });
      setConnectedUsers((prev) => prev.filter((u) => u.user_id !== user_id));
    });

    ws.on<{ drafts: ServiceDraft[] }>("service:drafts", ({ drafts }) => {
      setRemoteDrafts(new Map(drafts.map((d) => [d.draft_id, d])));
    });

    ws.on<ServiceDraft>("service:drafting", (draft) => {
      setRemoteDrafts((prev) => new Map(prev).set(draft.draft_id, draft));
    });

    ws.on<{ draft_id: string }>("service:draft-cancelled", ({ draft_id }) => {
      setRemoteDrafts((prev) => {
        const next = new Map(prev);
        next.delete(draft_id);
        return next;
      });
    });

    // service:create no longer commits a service — it stages a pending
    // change and upserts a placeholder canvas node. The draft form closes
    // on success; actual service creation happens on deploy.
    ws.on<{ draft_id: string }>(
      "service:created",
      ({ draft_id }) => {
        setLocalDrafts((prev) => {
          if (!prev.has(draft_id)) return prev;
          const next = new Map(prev);
          next.delete(draft_id);
          return next;
        });
      },
    );

    ws.on<{ draft_id: string; message: string }>(
      "service:create-error",
      ({ draft_id, message }) => {
        setLocalDrafts((prev) => {
          const existing = prev.get(draft_id);
          if (!existing) return prev;
          const next = new Map(prev);
          next.set(draft_id, {
            ...existing,
            isSubmitting: false,
            errorMessage: message,
          });
          return next;
        });
      },
    );

    ws.on<{ service: Service }>("service:upserted", ({ service }) => {
      queryClient.setQueryData<Service>(
        queryKeys.service(projectId, service.id),
        service,
      );
      queryClient.setQueryData<Service[]>(
        queryKeys.projectServices(projectId),
        (prev) => {
          if (!prev) return prev;
          const idx = prev.findIndex((s) => s.id === service.id);
          if (idx >= 0) {
            const next = [...prev];
            next[idx] = service;
            return next;
          }
          return [...prev, service];
        },
      );
    });

    ws.on<{ service_id: string; old_status: string; new_status: string }>(
      "service:status-changed",
      ({ service_id, new_status }) => {
        queryClient.setQueryData<Service>(
          queryKeys.service(projectId, service_id),
          (prev) => (prev ? { ...prev, status: new_status } : prev),
        );
        queryClient.setQueryData<Service[]>(
          queryKeys.projectServices(projectId),
          (prev) =>
            prev
              ? prev.map((s) =>
                  s.id === service_id ? { ...s, status: new_status } : s,
                )
              : prev,
        );
      },
    );

    ws.on<{ service_id: string }>("service:deleted", ({ service_id }) => {
      queryClient.removeQueries({
        queryKey: queryKeys.service(projectId, service_id),
      });
      queryClient.setQueryData<Service[]>(
        queryKeys.projectServices(projectId),
        (prev) => (prev ? prev.filter((s) => s.id !== service_id) : prev),
      );
    });

    // --- Pending change sync: keep the query cache in lock-step with the
    // server's log so every client (including other users' sessions) reflects
    // staged edits in real time. ---

    ws.on<{ changes: PendingChange[] }>("pending-changes:state", ({ changes }) => {
      queryClient.setQueryData<PendingChange[]>(
        queryKeys.pendingChanges(projectId),
        changes,
      );
    });

    ws.on<PendingChange>("pending-change:added", (pc) => {
      queryClient.setQueryData<PendingChange[]>(
        queryKeys.pendingChanges(projectId),
        (prev) => {
          if (!prev) return [pc];
          if (prev.some((p) => p.id === pc.id)) return prev;
          const next = [...prev, pc];
          next.sort((a, b) => a.seq - b.seq);
          return next;
        },
      );
    });

    ws.on<{ ids: string[] }>("pending-changes:removed", ({ ids }) => {
      if (!ids || ids.length === 0) return;
      const drop = new Set(ids);
      queryClient.setQueryData<PendingChange[]>(
        queryKeys.pendingChanges(projectId),
        (prev) => (prev ? prev.filter((p) => !drop.has(p.id)) : prev),
      );
    });

    ws.on("pending-changes:cleared", () => {
      queryClient.setQueryData<PendingChange[]>(
        queryKeys.pendingChanges(projectId),
        [],
      );
    });

    // Deploy landed: the server has already pushed a fresh canvas:state.
    // Invalidate applied-state queries so the UI rereads services, env vars,
    // deployments, project name, etc. Also empty the pending change list.
    ws.on("pending-changes:applied", () => {
      queryClient.setQueryData<PendingChange[]>(
        queryKeys.pendingChanges(projectId),
        [],
      );
      queryClient.invalidateQueries({
        predicate: (q) => {
          const key = q.queryKey;
          return (
            Array.isArray(key) && key[0] === "projects" && key[1] === projectId
          );
        },
      });
    });

    ws.on<{ deployment: Deployment }>(
      "deployment:created",
      ({ deployment }) => {
        // CreateDeployment transitions the service to "deploying" in the DB,
        // so mirror that here to avoid a gap until the reconciler catches up.
        const patch = (s: Service): Service => ({
          ...s,
          status: "deploying",
          active_deployment_id: deployment.id,
          active_deployment: deployment,
        });
        queryClient.setQueryData<Service>(
          queryKeys.service(projectId, deployment.service_id),
          (prev) => (prev ? patch(prev) : prev),
        );
        queryClient.setQueryData<Service[]>(
          queryKeys.projectServices(projectId),
          (prev) =>
            prev
              ? prev.map((s) => (s.id === deployment.service_id ? patch(s) : s))
              : prev,
        );
        queryClient.invalidateQueries({
          queryKey: queryKeys.deployments(projectId, deployment.service_id),
        });
      },
    );

    const unsubscribeStatus = ws.onStatusChange((status) => {
      // On reconnect (reconnecting → connected) resync service queries in case
      // push updates arrived while disconnected.
      if (status === "connected" && prevStatusRef.current === "reconnecting") {
        queryClient.invalidateQueries({
          queryKey: ["projects", projectId, "services"],
        });
      }
      prevStatusRef.current = status;
      setConnectionStatus(status);
    });

    ws.connect();

    return () => {
      unsubscribeStatus();
      ws.disconnect();
      wsRef.current = null;
      if (lerpRafRef.current) {
        cancelAnimationFrame(lerpRafRef.current);
        lerpRafRef.current = 0;
      }
      remoteTargetsRef.current.clear();
    };
  }, [projectId, queryClient]);

  const reconnect = useCallback(() => {
    wsRef.current?.reconnect();
  }, []);

  const onNodesChange = useCallback((changes: NodeChange[]) => {
    setNodes((nds) => applyNodeChanges(changes, nds));

    // Debounce position updates during drag.
    const positionChanges = changes.filter(
      (c): c is NodeChange & { type: "position"; position: { x: number; y: number } } =>
        c.type === "position" && "position" in c && c.position != null,
    );
    if (positionChanges.length > 0) {
      const positions = positionChanges.map((c) => ({
        id: c.id,
        position_x: c.position.x,
        position_y: c.position.y,
      }));
      dragLatestRef.current = positions;

      const THROTTLE_MS = 50;
      const now = Date.now();
      const elapsed = now - dragThrottleLastRef.current;

      const flush = () => {
        if (!dragLatestRef.current) return;
        wsRef.current?.send("node:move", { positions: dragLatestRef.current });
        dragThrottleLastRef.current = Date.now();
        dragLatestRef.current = null;
      };

      if (elapsed >= THROTTLE_MS) {
        flush();
      } else if (!dragThrottleTimerRef.current) {
        dragThrottleTimerRef.current = setTimeout(() => {
          dragThrottleTimerRef.current = undefined;
          flush();
        }, THROTTLE_MS - elapsed);
      }
    }
  }, []);

  const onEdgesChange = useCallback((changes: EdgeChange[]) => {
    // Strip remove-changes targeting managed edges. React Flow already honours
    // `deletable: false` on the edge itself, but selection+delete via keyboard
    // can bypass that, and we don't want to broadcast a delete the server will
    // reject.
    setEdges((eds) => {
      const managedIDs = new Set(
        eds
          .filter((e) => (e.data as { managed?: string } | undefined)?.managed)
          .map((e) => e.id),
      );
      const safeChanges = changes.filter(
        (c) => !(c.type === "remove" && managedIDs.has(c.id)),
      );
      for (const change of safeChanges) {
        if (change.type === "remove") {
          wsRef.current?.send("edge:delete", { id: change.id });
        }
      }
      return applyEdgeChanges(safeChanges, eds);
    });
  }, []);

  const onConnect = useCallback((connection: Connection) => {
    // Optimistically add the edge locally.
    const edgeId = `e-${connection.source}-${connection.target}`;
    setEdges((eds) => addEdge({ ...connection, id: edgeId }, eds));

    wsRef.current?.send("edge:upsert", {
      id: edgeId,
      source_id: connection.source,
      target_id: connection.target,
      data: "{}",
    });
  }, []);

  const openServiceDraft = useCallback(
    (x: number, y: number, prefill?: ServiceDraftPrefill) => {
      const draftId = randomUUID();
      setLocalDrafts((prev) => {
        const next = new Map(prev);
        next.set(draftId, {
          draftId,
          x,
          y,
          isSubmitting: false,
          errorMessage: null,
          prefill,
        });
        return next;
      });
      wsRef.current?.send("service:draft-start", { draft_id: draftId, x, y });
    },
    [],
  );

  const cancelServiceDraft = useCallback((draftId: string) => {
    setLocalDrafts((prev) => {
      if (!prev.has(draftId)) return prev;
      const next = new Map(prev);
      next.delete(draftId);
      wsRef.current?.send("service:draft-cancel", { draft_id: draftId });
      return next;
    });
  }, []);

  const deleteNode = useCallback((nodeId: string) => {
    wsRef.current?.send("node:delete", { id: nodeId });
  }, []);

  const addNote = useCallback(
    (x: number, y: number, backgroundColor: string) => {
      const id = randomUUID();
      wsRef.current?.send("node:upsert", {
        id,
        type: "note",
        label: "",
        position_x: x,
        position_y: y,
        width: 220,
        height: 220,
        data: JSON.stringify({ backgroundColor }),
      });
    },
    [],
  );

  const addGroup = useCallback((x: number, y: number) => {
    const id = randomUUID();
    wsRef.current?.send("node:upsert", {
      id,
      type: "group",
      label: "",
      position_x: x,
      position_y: y,
      width: 520,
      height: 360,
      data: "{}",
    });
    return id;
  }, []);

  // Keep a ref to the latest nodes so commitNote can look up the current
  // position without forcing the callback (and the dynamic nodeTypes map that
  // closes over it) to re-create on every node state update.
  const nodesRef = useRef<Node[]>(nodes);
  useEffect(() => {
    nodesRef.current = nodes;
  }, [nodes]);

  const commitNote = useCallback(
    (id: string, label: string, backgroundColor: string) => {
      const node = nodesRef.current.find((n) => n.id === id);
      if (!node) return;
      wsRef.current?.send("node:upsert", {
        id,
        type: "note",
        label,
        position_x: node.position.x,
        position_y: node.position.y,
        width: 220,
        height: 220,
        data: JSON.stringify({ backgroundColor }),
      });
    },
    [],
  );

  const commitGroup = useCallback((id: string, label: string) => {
    const node = nodesRef.current.find((n) => n.id === id);
    if (!node) return;
    const style = (node.style ?? {}) as { width?: number; height?: number };
    wsRef.current?.send("node:upsert", {
      id,
      type: "group",
      label,
      position_x: node.position.x,
      position_y: node.position.y,
      width: style.width ?? 320,
      height: style.height ?? 240,
      data: "{}",
    });
  }, []);

  // reparentNode persists a node's parent change AND optimistically updates
  // local state so callers running on the same tick (e.g. autoFitGroup) see
  // the new parent immediately rather than waiting for the WS echo. Pass null
  // to detach. The caller picks the right relative position — when a node is
  // dropped into a group, its position should be in the group's coordinate
  // space; when detached, in absolute canvas coords.
  const reparentNode = useCallback(
    (
      nodeId: string,
      parentId: string | null,
      relativePosition: { x: number; y: number },
    ) => {
      const node = nodesRef.current.find((n) => n.id === nodeId);
      if (!node) return;
      const style = (node.style ?? {}) as { width?: number; height?: number };
      const data = (node.data ?? {}) as Record<string, unknown>;
      const persistData = stripInjected(data);
      // Strip injected callbacks before serializing back to the server.
      // Only the four DB-recognized types can be reparented; anything else is
      // a programming error in the caller.
      let dbType: "service" | "note" | "group";
      if (node.type === "service" || node.type === "note" || node.type === "group") {
        dbType = node.type;
      } else {
        console.warn("reparentNode: unsupported node type", node.type, "for node", nodeId);
        return;
      }
      wsRef.current?.send("node:upsert", {
        id: nodeId,
        type: dbType,
        label: (data.label as string | undefined) ?? "",
        position_x: relativePosition.x,
        position_y: relativePosition.y,
        ...(style.width != null && style.height != null
          ? { width: style.width, height: style.height }
          : {}),
        ...(data.serviceId ? { service_id: data.serviceId as string } : {}),
        parent_id: parentId,
        data: JSON.stringify(persistData),
      });

      // Optimistic local update so autoFit reading nodesRef on the next tick
      // sees the new parent assignment.
      setNodes((prev) => {
        const next = prev.map((n) => {
          if (n.id !== nodeId) return n;
          const cleaned = { ...n, position: relativePosition };
          if (parentId) {
            cleaned.parentId = parentId;
          } else {
            delete (cleaned as { parentId?: string }).parentId;
          }
          return cleaned;
        });
        return ensureParentsFirst(next);
      });
    },
    [],
  );

  // resizeGroup persists a manual resize from the NodeResizer UI. Keeps the
  // group's existing position/label and updates only width/height.
  const resizeGroup = useCallback((id: string, width: number, height: number) => {
    const node = nodesRef.current.find((n) => n.id === id);
    if (!node) return;
    wsRef.current?.send("node:upsert", {
      id,
      type: "group",
      label: ((node.data ?? {}) as { label?: string }).label ?? "",
      position_x: node.position.x,
      position_y: node.position.y,
      width,
      height,
      data: "{}",
    });
  }, []);

  const moveLocalDraft = useCallback(
    (draftId: string, x: number, y: number) => {
      setLocalDrafts((prev) => {
        const existing = prev.get(draftId);
        if (!existing) return prev;
        const next = new Map(prev);
        next.set(draftId, { ...existing, x, y });
        return next;
      });
      wsRef.current?.send("service:draft-start", { draft_id: draftId, x, y });
    },
    [],
  );

  // Snapshot keyed by draft ID so submitServiceDraft can read position without
  // touching setLocalDrafts — updater functions run twice under StrictMode and
  // side effects inside them would double-fire the WS send.
  const localDraftsRef = useRef<Map<string, LocalServiceDraft>>(new Map());
  useEffect(() => {
    localDraftsRef.current = localDrafts;
  }, [localDrafts]);

  const submitServiceDraft = useCallback(
    (
      draftId: string,
      values: {
        name: string;
        image: string;
        iconUrl?: string;
        envVars?: DraftEnvVar[];
      },
    ) => {
      const existing = localDraftsRef.current.get(draftId);
      if (!existing) return;
      wsRef.current?.send("service:create", {
        draft_id: draftId,
        name: values.name,
        image: values.image,
        icon_url: values.iconUrl,
        env_vars: values.envVars,
        x: existing.x,
        y: existing.y,
      });
      setLocalDrafts((prev) => {
        const curr = prev.get(draftId);
        if (!curr) return prev;
        const next = new Map(prev);
        next.set(draftId, { ...curr, isSubmitting: true, errorMessage: null });
        return next;
      });
    },
    [],
  );

  const sendCursorMove = useCallback((x: number, y: number) => {
    const now = Date.now();
    if (now - cursorThrottleRef.current < 100) return; // 10Hz throttle
    cursorThrottleRef.current = now;
    wsRef.current?.send("cursor:move", { x, y });
  }, []);

  return {
    nodes,
    edges,
    cursors,
    connectedUsers,
    connectionStatus,
    remoteDrafts,
    localDrafts,
    reconnect,
    onNodesChange,
    onEdgesChange,
    onConnect,
    sendCursorMove,
    openServiceDraft,
    cancelServiceDraft,
    submitServiceDraft,
    moveLocalDraft,
    deleteNode,
    addNote,
    commitNote,
    addGroup,
    commitGroup,
    resizeGroup,
    reparentNode,
  };
}
