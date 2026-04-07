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
import { CanvasWebSocket, type ConnectionStatus } from "./canvas-ws";

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

function toFlowNode(dbNode: CanvasNode): Node {
  let extraData = {};
  try {
    extraData = JSON.parse(dbNode.data);
  } catch {
    // ignore
  }

  return {
    id: dbNode.id,
    type: dbNode.type === "group" ? "group" : "default",
    position: { x: dbNode.position_x, y: dbNode.position_y },
    data: { label: dbNode.label, serviceId: dbNode.service_id, ...extraData },
    ...(dbNode.width && dbNode.height
      ? { style: { width: dbNode.width, height: dbNode.height } }
      : {}),
  };
}

function toFlowEdge(dbEdge: CanvasEdge): Edge {
  return {
    id: dbEdge.id,
    source: dbEdge.source_id,
    target: dbEdge.target_id,
    ...(dbEdge.label ? { label: dbEdge.label } : {}),
  };
}

export function useCanvasSync(projectId: string) {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [edges, setEdges] = useState<Edge[]>([]);
  const [cursors, setCursors] = useState<Map<string, CursorInfo>>(new Map());
  const [connectedUsers, setConnectedUsers] = useState<ConnectedUser[]>([]);
  const [connectionStatus, setConnectionStatus] =
    useState<ConnectionStatus>("disconnected");
  const wsRef = useRef<CanvasWebSocket | null>(null);
  const dragDebounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const cursorThrottleRef = useRef<number>(0);

  useEffect(() => {
    const ws = new CanvasWebSocket(projectId);
    wsRef.current = ws;

    ws.on<{ nodes: CanvasNode[]; edges: CanvasEdge[] }>(
      "canvas:state",
      (payload) => {
        setNodes(payload.nodes.map(toFlowNode));
        setEdges(payload.edges.map(toFlowEdge));
      },
    );

    ws.on<CanvasNode>("node:upserted", (node) => {
      const flowNode = toFlowNode(node);
      setNodes((prev) => {
        const idx = prev.findIndex((n) => n.id === flowNode.id);
        if (idx >= 0) {
          const next = [...prev];
          next[idx] = flowNode;
          return next;
        }
        return [...prev, flowNode];
      });
    });

    ws.on<{ id: string }>("node:deleted", ({ id }) => {
      setNodes((prev) => prev.filter((n) => n.id !== id));
      setEdges((prev) =>
        prev.filter((e) => e.source !== id && e.target !== id),
      );
    });

    ws.on<{ positions: NodePosition[]; user_id: string }>(
      "node:moved",
      ({ positions }) => {
        setNodes((prev) => {
          const posMap = new Map(
            positions.map((p) => [p.id, { x: p.position_x, y: p.position_y }]),
          );
          return prev.map((n) => {
            const pos = posMap.get(n.id);
            if (pos) {
              return { ...n, position: pos };
            }
            return n;
          });
        });
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

    const unsubscribeStatus = ws.onStatusChange(setConnectionStatus);

    ws.connect();

    return () => {
      unsubscribeStatus();
      ws.disconnect();
      wsRef.current = null;
    };
  }, [projectId]);

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
      if (dragDebounceRef.current) clearTimeout(dragDebounceRef.current);
      dragDebounceRef.current = setTimeout(() => {
        wsRef.current?.send("node:move", {
          positions: positionChanges.map((c) => ({
            id: c.id,
            position_x: c.position.x,
            position_y: c.position.y,
          })),
        });
      }, 50);
    }
  }, []);

  const onEdgesChange = useCallback((changes: EdgeChange[]) => {
    setEdges((eds) => applyEdgeChanges(changes, eds));

    for (const change of changes) {
      if (change.type === "remove") {
        wsRef.current?.send("edge:delete", { id: change.id });
      }
    }
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
    reconnect,
    onNodesChange,
    onEdgesChange,
    onConnect,
    sendCursorMove,
  };
}
