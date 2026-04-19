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
import { Link } from "react-router";
import { SettingsIcon } from "lucide-react";
import { useCanvasSync } from "@/lib/use-canvas-sync";
import { useProjectServices, type Service } from "@/lib/queries";
import { CursorOverlay } from "./cursor-overlay";
import { AvatarStack } from "./avatar-stack";
import { CanvasContextMenu } from "./canvas-context-menu";
import { NodeContextMenu } from "./node-context-menu";
import { CanvasControls } from "./canvas-controls";
import { AddServiceForm } from "./add-service-form";
import { DraftingServiceGhost } from "./drafting-service-ghost";
import { ServiceNode, type ServiceNodeData } from "./service-node";
import { ServiceDetailPanel } from "./service-detail-panel";

const nodeTypes = {
  service: ServiceNode,
  "service-draft": AddServiceForm,
  "service-drafting": DraftingServiceGhost,
};

const DRAFT_NODE_WIDTH = 320;
const DRAFT_NODE_HEIGHT = 272;
const GHOST_NODE_HEIGHT = DRAFT_NODE_HEIGHT;

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

  const { data: servicesData } = useProjectServices(projectId);
  const servicesById = useMemo(() => {
    const map = new Map<string, Service>();
    for (const s of servicesData ?? []) map.set(s.id, s);
    return map;
  }, [servicesData]);

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

  const { screenToFlowPosition, getNodes, getViewport, setCenter } =
    useReactFlow();

  const [menu, setMenu] = useState<
    | { screenX: number; screenY: number; flowX: number; flowY: number }
    | null
  >(null);
  const [nodeMenu, setNodeMenu] = useState<
    { screenX: number; screenY: number; nodeId: string } | null
  >(null);
  const [selectedServiceId, setSelectedServiceId] = useState<string | null>(
    null,
  );

  // Close the detail panel if the selected service is no longer on the canvas
  // (e.g. it was deleted via context menu or by another user).
  useEffect(() => {
    if (!selectedServiceId) return;
    const stillPresent = nodes.some(
      (n) =>
        n.type === "service" &&
        (n.data as ServiceNodeData | undefined)?.serviceId ===
          selectedServiceId,
    );
    if (!stillPresent) setSelectedServiceId(null);
  }, [nodes, selectedServiceId]);

  const onNodeClick = useCallback(
    (_event: React.MouseEvent, node: Node) => {
      if (node.type !== "service") return;
      const data = node.data as ServiceNodeData | undefined;
      if (data?.serviceId) setSelectedServiceId(data.serviceId);
    },
    [],
  );

  // Recenter the selected node into the canvas area left of the detail panel.
  // PANEL_RESERVED matches the panel's effective width (w-[520px] + gutters).
  // Only re-run when the selected id changes so ongoing node drags don't retrigger.
  useEffect(() => {
    if (!selectedServiceId) return;
    const PANEL_RESERVED = 540;
    const target = getNodes().find(
      (n) =>
        n.type === "service" &&
        (n.data as ServiceNodeData | undefined)?.serviceId ===
          selectedServiceId,
    );
    if (!target) return;
    const width = target.measured?.width ?? 256;
    const height = target.measured?.height ?? 80;
    const centerX = target.position.x + width / 2;
    const centerY = target.position.y + height / 2;
    const { zoom } = getViewport();
    setCenter(centerX + PANEL_RESERVED / 2 / zoom, centerY, {
      zoom,
      duration: 400,
    });
  }, [selectedServiceId, getNodes, getViewport, setCenter]);

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

  const decoratedNodes = useMemo<Node[]>(() => {
    if (servicesById.size === 0) return nodes;
    return nodes.map((n) => {
      if (n.type !== "service") return n;
      const data = n.data as ServiceNodeData | undefined;
      const serviceId = data?.serviceId;
      if (!serviceId) return n;
      const svc = servicesById.get(serviceId);
      if (!svc) return n;
      const nextStatus = svc.status;
      const nextImage = svc.active_deployment?.image;
      const nextIconUrl = svc.icon_url ?? undefined;
      const nextLabel = svc.name;
      if (
        data?.status === nextStatus &&
        data?.image === nextImage &&
        data?.iconUrl === nextIconUrl &&
        data?.label === nextLabel
      ) {
        return n;
      }
      return {
        ...n,
        data: {
          ...data,
          label: nextLabel,
          status: nextStatus,
          image: nextImage,
          iconUrl: nextIconUrl,
        },
      };
    });
  }, [nodes, servicesById]);

  const composedNodes = useMemo<Node[]>(() => {
    if (remoteDrafts.size === 0 && localDrafts.size === 0) return decoratedNodes;
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
    return [...decoratedNodes, ...extras];
  }, [decoratedNodes, remoteDrafts, localDrafts, submitServiceDraft, cancelServiceDraft]);

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
        onNodeClick={onNodeClick}
        onPaneClick={closeMenu}
        fitView
        maxZoom={1}
        panOnScroll
        zoomOnScroll={false}
      >
        <Background />
        <Panel position="top-left">
          <Link
            to={`/projects/${projectId}/settings`}
            title="Project settings"
            className="flex size-8 items-center justify-center rounded-md border bg-background/80 text-muted-foreground backdrop-blur transition-colors hover:bg-background hover:text-foreground"
          >
            <SettingsIcon className="size-4" />
          </Link>
        </Panel>
        {selectedServiceId ? null : (
          <Panel position="top-right">
            <AvatarStack users={connectedUsers} />
          </Panel>
        )}
        {selectedServiceId ? (
          <ServiceDetailPanel
            projectId={projectId}
            serviceId={selectedServiceId}
            onClose={() => setSelectedServiceId(null)}
          />
        ) : null}
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
