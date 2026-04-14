import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Panel,
  useReactFlow,
  type Node,
  type NodeChange,
} from "@xyflow/react";
import { toast } from "sonner";
import { useCanvasSync } from "@/lib/use-canvas-sync";
import { CursorOverlay } from "./cursor-overlay";
import { AvatarStack } from "./avatar-stack";
import { CanvasContextMenu } from "./canvas-context-menu";
import { NodeContextMenu } from "./node-context-menu";
import { CanvasControls } from "./canvas-controls";
import { AddServiceForm } from "./add-service-form";
import { DraftingServiceGhost } from "./drafting-service-ghost";
import { ServiceNode } from "./service-node";

const nodeTypes = {
  service: ServiceNode,
  "service-draft": AddServiceForm,
  "service-drafting": DraftingServiceGhost,
};

const DRAFT_NODE_WIDTH = 256;
const DRAFT_NODE_HEIGHT = 220;
const GHOST_NODE_HEIGHT = 60;

interface ProjectFlowProps {
  projectId: string;
}

export function ProjectFlow({ projectId }: ProjectFlowProps) {
  return (
    <ReactFlowProvider>
      <ProjectFlowInner projectId={projectId} />
    </ReactFlowProvider>
  );
}

function ProjectFlowInner({ projectId }: ProjectFlowProps) {
  const {
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
  } = useCanvasSync(projectId);

  const handleNodesChange = useCallback(
    (changes: NodeChange[]) => {
      const passthrough: NodeChange[] = [];
      for (const change of changes) {
        if (
          change.type === "position" &&
          "id" in change &&
          typeof change.id === "string" &&
          change.id.startsWith("draft-") &&
          change.position
        ) {
          moveLocalDraft(
            change.id.slice("draft-".length),
            change.position.x,
            change.position.y,
          );
          continue;
        }
        // Drop other change types for virtual draft/ghost nodes so
        // applyNodeChanges doesn't thrash the source nodes state.
        if (
          "id" in change &&
          typeof change.id === "string" &&
          (change.id.startsWith("draft-") || change.id.startsWith("ghost-"))
        ) {
          continue;
        }
        passthrough.push(change);
      }
      if (passthrough.length > 0) onNodesChange(passthrough);
    },
    [onNodesChange, moveLocalDraft],
  );

  const { screenToFlowPosition } = useReactFlow();

  const [menu, setMenu] = useState<
    | { screenX: number; screenY: number; flowX: number; flowY: number }
    | null
  >(null);
  const [nodeMenu, setNodeMenu] = useState<
    { screenX: number; screenY: number; nodeId: string } | null
  >(null);

  const onPaneContextMenu = useCallback(
    (event: MouseEvent | React.MouseEvent) => {
      event.preventDefault();
      const flow = screenToFlowPosition({ x: event.clientX, y: event.clientY });
      setMenu({
        screenX: event.clientX,
        screenY: event.clientY,
        flowX: flow.x,
        flowY: flow.y,
      });
    },
    [screenToFlowPosition],
  );

  const closeMenu = useCallback(() => {
    setMenu(null);
    setNodeMenu(null);
  }, []);

  const closeNodeMenu = useCallback(() => setNodeMenu(null), []);

  const onNodeContextMenu = useCallback(
    (event: React.MouseEvent, node: Node) => {
      if (node.type === "service-draft" || node.type === "service-drafting") {
        return;
      }
      event.preventDefault();
      setMenu(null);
      setNodeMenu({
        screenX: event.clientX,
        screenY: event.clientY,
        nodeId: node.id,
      });
    },
    [],
  );

  const handleDeleteNode = useCallback(() => {
    if (!nodeMenu) return;
    deleteNode(nodeMenu.nodeId);
  }, [nodeMenu, deleteNode]);

  const handleAddService = useCallback(() => {
    if (!menu) return;
    // Offset so the form's top-left is roughly at the click, not centered on it.
    openServiceDraft(menu.flowX, menu.flowY);
  }, [menu, openServiceDraft]);

  const composedNodes = useMemo<Node[]>(() => {
    if (remoteDrafts.size === 0 && localDrafts.size === 0) return nodes;
    const extras: Node[] = [];
    for (const [, draft] of remoteDrafts) {
      extras.push({
        id: `ghost-${draft.draft_id}`,
        type: "service-drafting",
        position: { x: draft.x, y: draft.y },
        data: { userName: draft.user_name },
        draggable: false,
        selectable: false,
        initialWidth: DRAFT_NODE_WIDTH,
        initialHeight: GHOST_NODE_HEIGHT,
        style: { width: DRAFT_NODE_WIDTH, height: GHOST_NODE_HEIGHT },
      });
    }
    for (const [, draft] of localDrafts) {
      extras.push({
        id: `draft-${draft.draftId}`,
        type: "service-draft",
        position: { x: draft.x, y: draft.y },
        data: {
          draftId: draft.draftId,
          isSubmitting: draft.isSubmitting,
          errorMessage: draft.errorMessage,
          onSubmit: submitServiceDraft,
          onCancel: cancelServiceDraft,
        },
        selectable: false,
        dragHandle: ".service-draft-drag-handle",
        initialWidth: DRAFT_NODE_WIDTH,
        initialHeight: DRAFT_NODE_HEIGHT,
        style: { width: DRAFT_NODE_WIDTH, height: DRAFT_NODE_HEIGHT },
      });
    }
    return [...nodes, ...extras];
  }, [nodes, remoteDrafts, localDrafts, submitServiceDraft, cancelServiceDraft]);

  // Surface disconnect + auto-reconnect attempts so users aren't left staring
  // at a canvas that has silently stopped syncing. Only fire the disconnect
  // toast after we've actually been connected at least once — otherwise the
  // initial "disconnected" state shows a spurious toast on mount.
  const disconnectedToastRef = useRef<string | number | null>(null);
  const hasConnectedRef = useRef(false);
  useEffect(() => {
    if (connectionStatus === "connected") {
      hasConnectedRef.current = true;
      if (disconnectedToastRef.current != null) {
        toast.dismiss(disconnectedToastRef.current);
        disconnectedToastRef.current = null;
        toast.success("Canvas reconnected");
      }
      return;
    }

    if (!hasConnectedRef.current) return;

    if (connectionStatus === "disconnected" && disconnectedToastRef.current == null) {
      disconnectedToastRef.current = toast.error("Canvas disconnected", {
        description: "Changes will not sync until you reconnect.",
        duration: Infinity,
        action: {
          label: "Reconnect",
          onClick: () => reconnect(),
        },
      });
    }
  }, [connectionStatus, reconnect]);

  return (
    <div className="w-full flex-1 min-h-0 relative overflow-hidden">
      <ReactFlow
        nodes={composedNodes}
        edges={edges}
        nodeTypes={nodeTypes}
        onNodesChange={handleNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onPaneContextMenu={onPaneContextMenu}
        onNodeContextMenu={onNodeContextMenu}
        onPaneClick={closeMenu}
        fitView
        maxZoom={1}
      >
        <Background />
        <Panel position="top-right">
          <AvatarStack users={connectedUsers} />
        </Panel>
        <CursorTracker onCursorMove={sendCursorMove} />
        <CursorOverlay cursors={cursors} />
        <CanvasControls />
      </ReactFlow>
      <CanvasContextMenu
        position={menu ? { x: menu.screenX, y: menu.screenY } : null}
        onClose={closeMenu}
        onAddService={handleAddService}
      />
      <NodeContextMenu
        position={
          nodeMenu ? { x: nodeMenu.screenX, y: nodeMenu.screenY } : null
        }
        onClose={closeNodeMenu}
        onDelete={handleDeleteNode}
      />
    </div>
  );
}

function CursorTracker({
  onCursorMove,
}: {
  onCursorMove: (x: number, y: number) => void;
}) {
  const { screenToFlowPosition } = useReactFlow();
  const callbackRef = useRef(onCursorMove);
  const positionRef = useRef(screenToFlowPosition);
  callbackRef.current = onCursorMove;
  positionRef.current = screenToFlowPosition;

  const markerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const marker = markerRef.current;
    if (!marker) return;

    const container = marker.closest(".react-flow") as HTMLElement | null;
    if (!container) return;

    // Listen on window so drag pointer-capture doesn't suppress updates, but
    // gate on whether the pointer is over the canvas container.
    const handler = (e: PointerEvent) => {
      const rect = container.getBoundingClientRect();
      if (
        e.clientX < rect.left ||
        e.clientX > rect.right ||
        e.clientY < rect.top ||
        e.clientY > rect.bottom
      ) {
        return;
      }
      const pos = positionRef.current({ x: e.clientX, y: e.clientY });
      callbackRef.current(pos.x, pos.y);
    };

    window.addEventListener("pointermove", handler, { passive: true });
    return () => window.removeEventListener("pointermove", handler);
  }, []);

  return <div ref={markerRef} style={{ display: "none" }} />;
}
