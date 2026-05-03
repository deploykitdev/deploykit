import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  ConnectionMode,
  Panel,
  useReactFlow,
  type Node,
  type NodeChange,
  type XYPosition,
} from "@xyflow/react";
import { toast } from "sonner";
import { useCanvasSync } from "@/lib/use-canvas-sync";
import { CanvasLoader } from "./canvas-loader";
import { usePendingChanges, useProjectServices, type Service } from "@/lib/queries";
import {
  collectServiceOverrides,
  collectTakenServiceNames,
} from "@/lib/pending-changes-diff";
import { CursorOverlay } from "./cursor-overlay";
import { AvatarStack } from "./avatar-stack";
import { CanvasContextMenu } from "./canvas-context-menu";
import { AddDatabaseDialog } from "./add-database-dialog";
import { NodeContextMenu } from "./node-context-menu";
import { CanvasControls } from "./canvas-controls";
import { AddServiceForm } from "./add-service-form";
import { DraftingServiceGhost } from "./drafting-service-ghost";
import { ServiceNode, type ServiceNodeData } from "./service-node";
import { NoteNode, NOTE_DEFAULT_COLOR } from "./note-node";
import { GroupNode } from "./group-node";
import { ServiceDetailPanel } from "./service-detail-panel";
import { GroupDetailPanel } from "./group-detail-panel";
import { PendingChangesPanel } from "./pending-changes-panel";
import { PendingServicePanel } from "./pending-service-panel";
import { FloatingEdge } from "./floating-edge";

const nodeTypes = {
  service: ServiceNode,
  "service-draft": AddServiceForm,
  "service-drafting": DraftingServiceGhost,
  note: NoteNode,
  group: GroupNode,
};

const edgeTypes = {
  floating: FloatingEdge,
};

const defaultEdgeOptions = { type: "floating" };

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
    isInitialStateLoaded,
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
    setLocalDraftMeasured,
    deleteNode,
    addNote,
    commitNote,
    addGroup,
    commitGroup,
    resizeGroup,
    reparentNode,
  } = useCanvasSync(projectId);

  const { data: servicesData } = useProjectServices(projectId);
  const { data: pendingChanges } = usePendingChanges(projectId);
  const servicesById = useMemo(() => {
    const map = new Map<string, Service>();
    for (const s of servicesData ?? []) map.set(s.id, s);
    return map;
  }, [servicesData]);

  // Collapse the log into per-service overrides (latest-write-wins) so the
  // canvas shows staged renames / icon changes before deploy. Also tracks
  // pending-delete flags for visual decoration.
  const pendingOverridesByServiceId = useMemo(
    () => collectServiceOverrides(pendingChanges),
    [pendingChanges],
  );

  // Lowercased name set the create-service form blocks against. Mirrors the
  // backend's stage-time check so submit is disabled before the WS round-trip.
  const takenServiceNames = useMemo(
    () => collectTakenServiceNames(servicesData, pendingChanges),
    [servicesData, pendingChanges],
  );

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
        // The prefill draft has auto height, so RF needs the ResizeObserver
        // measurement to mark it dimensioned and flip visibility from hidden.
        // Drafts aren't in the source nodes state, so route the dimensions
        // back into localDrafts and let composedNodes inject `measured` on
        // the next render.
        if (
          change.type === "dimensions" &&
          "id" in change &&
          typeof change.id === "string" &&
          change.id.startsWith("draft-") &&
          change.dimensions
        ) {
          setLocalDraftMeasured(
            change.id.slice("draft-".length),
            change.dimensions,
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
    [onNodesChange, moveLocalDraft, setLocalDraftMeasured],
  );

  const { screenToFlowPosition, getNodes, getViewport, setCenter } =
    useReactFlow();

  const [menu, setMenu] = useState<
    | { screenX: number; screenY: number; flowX: number; flowY: number }
    | null
  >(null);
  const [databaseDialog, setDatabaseDialog] = useState<
    { flowX: number; flowY: number } | null
  >(null);
  const [nodeMenu, setNodeMenu] = useState<
    { screenX: number; screenY: number; nodeId: string } | null
  >(null);
  const [selectedServiceId, setSelectedServiceId] = useState<string | null>(
    null,
  );
  // Pending-added services have no backing service row yet; track them by
  // canvas node ID (= the pending service.create target_temp_id).
  const [selectedPendingNodeId, setSelectedPendingNodeId] = useState<
    string | null
  >(null);
  const [selectedGroupId, setSelectedGroupId] = useState<string | null>(null);

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

  useEffect(() => {
    if (!selectedPendingNodeId) return;
    const stillPresent = nodes.some(
      (n) => n.type === "service" && n.id === selectedPendingNodeId,
    );
    if (!stillPresent) setSelectedPendingNodeId(null);
  }, [nodes, selectedPendingNodeId]);

  useEffect(() => {
    if (!selectedGroupId) return;
    const stillPresent = nodes.some(
      (n) => n.type === "group" && n.id === selectedGroupId,
    );
    if (!stillPresent) setSelectedGroupId(null);
  }, [nodes, selectedGroupId]);

  const onNodeClick = useCallback(
    (_event: React.MouseEvent, node: Node) => {
      if (node.type === "group") {
        setSelectedServiceId(null);
        setSelectedPendingNodeId(null);
        setSelectedGroupId(node.id);
        return;
      }
      if (node.type !== "service") return;
      const data = node.data as ServiceNodeData | undefined;
      // Pending-deleted nodes are non-interactive — the service is on its way
      // out and editing it makes no sense. The bottom panel is the place to
      // cancel the deletion by discarding.
      if (data?.pendingDelete) return;
      setSelectedGroupId(null);
      if (data?.serviceId) {
        setSelectedPendingNodeId(null);
        setSelectedServiceId(data.serviceId);
      } else {
        setSelectedServiceId(null);
        setSelectedPendingNodeId(node.id);
      }
    },
    [],
  );

  // Recenter the selected node into the canvas area left of the detail panel.
  // PANEL_RESERVED matches the panel's effective width (w-[520px] + gutters).
  // Only re-run when the selected id changes so ongoing node drags don't retrigger.
  useEffect(() => {
    if (!selectedServiceId && !selectedPendingNodeId && !selectedGroupId) return;
    const PANEL_RESERVED = 540;
    const all = getNodes();
    const byId = new Map(all.map((n) => [n.id, n]));
    const target = (() => {
      if (selectedGroupId) return byId.get(selectedGroupId);
      return all.find((n) => {
        if (n.type !== "service") return false;
        if (selectedPendingNodeId) return n.id === selectedPendingNodeId;
        return (
          (n.data as ServiceNodeData | undefined)?.serviceId === selectedServiceId
        );
      });
    })();
    if (!target) return;
    // Children of a group store their position relative to the group, so walk
    // the parent chain to get absolute canvas coords.
    const absoluteOf = (n: Node): { x: number; y: number } => {
      if (!n.parentId) return n.position;
      const parent = byId.get(n.parentId);
      if (!parent) return n.position;
      const p = absoluteOf(parent);
      return { x: p.x + n.position.x, y: p.y + n.position.y };
    };
    const abs = absoluteOf(target);
    const width =
      target.measured?.width ??
      (target.style as { width?: number } | undefined)?.width ??
      256;
    const height =
      target.measured?.height ??
      (target.style as { height?: number } | undefined)?.height ??
      80;
    const centerX = abs.x + width / 2;
    const centerY = abs.y + height / 2;
    const { zoom } = getViewport();
    setCenter(centerX + PANEL_RESERVED / 2 / zoom, centerY, {
      zoom,
      duration: 400,
    });
  }, [
    selectedServiceId,
    selectedPendingNodeId,
    selectedGroupId,
    getNodes,
    getViewport,
    setCenter,
  ]);

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

  // Reparent a child when its drag ends over (or no longer over) a group node.
  // React Flow stores child positions relative to the parent, so we work in
  // absolute coordinates for intersection tests, then convert back.
  const handleNodeDragStop = useCallback(
    (_event: React.MouseEvent | MouseEvent | TouchEvent, draggedNode: Node) => {
      if (draggedNode.type === "group") {
        // Dragging a group moves its children with it (relative coords stay
        // the same), so no reparent. Auto-fit isn't needed either.
        return;
      }
      if (draggedNode.type === "service-draft" || draggedNode.type === "service-drafting") {
        return;
      }

      const all = getNodes();
      const draggedById = new Map(all.map((n) => [n.id, n]));

      // Compute absolute position of the dragged node.
      const absoluteOf = (n: Node): XYPosition => {
        if (!n.parentId) return n.position;
        const parent = draggedById.get(n.parentId);
        if (!parent) return n.position;
        const parentAbs = absoluteOf(parent);
        return { x: parentAbs.x + n.position.x, y: parentAbs.y + n.position.y };
      };

      const draggedW =
        draggedNode.measured?.width ??
        (draggedNode.style as { width?: number } | undefined)?.width ??
        0;
      const draggedH =
        draggedNode.measured?.height ??
        (draggedNode.style as { height?: number } | undefined)?.height ??
        0;
      const draggedAbs = absoluteOf(draggedNode);
      const cx = draggedAbs.x + draggedW / 2;
      const cy = draggedAbs.y + draggedH / 2;

      // Find a group whose bounds contain the dragged node's center. With
      // flat (non-nested) groups there's at most one match; if multiple
      // groups overlap the result is the *last* in iteration order, which
      // matches the order returned by getNodes() (creation time, ASC).
      let target: Node | null = null;
      for (const n of all) {
        if (n.type !== "group") continue;
        if (n.id === draggedNode.id) continue;
        const gAbs = absoluteOf(n);
        const gw = n.measured?.width ?? (n.style as { width?: number } | undefined)?.width ?? 0;
        const gh = n.measured?.height ?? (n.style as { height?: number } | undefined)?.height ?? 0;
        if (cx >= gAbs.x && cx <= gAbs.x + gw && cy >= gAbs.y && cy <= gAbs.y + gh) {
          target = n;
        }
      }

      const currentParentId = draggedNode.parentId ?? null;
      const newParentId = target?.id ?? null;
      if (currentParentId === newParentId) {
        // No parent change — nothing to do. Group is resized manually.
        return;
      }

      if (newParentId && target) {
        // Joining a (new) group — convert absolute to relative to target.
        const tAbs = absoluteOf(target);
        reparentNode(draggedNode.id, newParentId, {
          x: draggedAbs.x - tAbs.x,
          y: draggedAbs.y - tAbs.y,
        });
      } else {
        // Detaching — keep absolute position so the node visually stays put.
        reparentNode(draggedNode.id, null, draggedAbs);
      }
    },
    [getNodes, reparentNode],
  );

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

  const handleAddDatabase = useCallback(() => {
    if (!menu) return;
    setDatabaseDialog({ flowX: menu.flowX, flowY: menu.flowY });
  }, [menu]);

  const handleAddNote = useCallback(() => {
    if (!menu) return;
    // Center the 220×220 note on the click point.
    addNote(menu.flowX - 110, menu.flowY - 110, NOTE_DEFAULT_COLOR);
  }, [menu, addNote]);

  const handleAddGroup = useCallback(() => {
    if (!menu) return;
    // Center the 520×360 group on the click point.
    addGroup(menu.flowX - 260, menu.flowY - 180);
  }, [menu, addGroup]);

  const decoratedNodes = useMemo<Node[]>(() => {
    return nodes.map((n) => {
      if (n.type === "note") {
        // Inject the commit callback into note data so the component can
        // persist text/color edits without prop drilling.
        const data = (n.data ?? {}) as Record<string, unknown>;
        if (data.commitNote === commitNote) return n;
        return { ...n, data: { ...data, commitNote } };
      }
      if (n.type === "group") {
        const data = (n.data ?? {}) as Record<string, unknown>;
        if (
          data.commitGroup === commitGroup &&
          data.resizeGroup === resizeGroup
        ) {
          return n;
        }
        return { ...n, data: { ...data, commitGroup, resizeGroup } };
      }
      if (n.type !== "service") return n;
      const data = n.data as ServiceNodeData | undefined;
      const serviceId = data?.serviceId;
      if (!serviceId) return n;
      const svc = servicesById.get(serviceId);
      if (!svc) return n;
      const override = pendingOverridesByServiceId.get(serviceId);
      const nextStatus = svc.status;
      const nextImage = svc.active_deployment?.image;
      const nextIconUrl =
        override?.iconUrlSet !== undefined
          ? override.iconUrlSet ?? undefined
          : svc.icon_url ?? undefined;
      const nextLabel = override?.name ?? svc.name;
      const nextPendingDelete = override?.pendingDelete ?? false;
      if (
        data?.status === nextStatus &&
        data?.image === nextImage &&
        data?.iconUrl === nextIconUrl &&
        data?.label === nextLabel &&
        (data?.pendingDelete ?? false) === nextPendingDelete
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
          pendingDelete: nextPendingDelete,
        },
      };
    });
  }, [nodes, servicesById, pendingOverridesByServiceId, commitNote, commitGroup, resizeGroup]);

  // Close the detail panel if the selected service gets marked for deletion.
  // Otherwise the user could edit a service they're about to drop.
  useEffect(() => {
    if (
      selectedServiceId &&
      pendingOverridesByServiceId.get(selectedServiceId)?.pendingDelete
    ) {
      setSelectedServiceId(null);
    }
  }, [selectedServiceId, pendingOverridesByServiceId]);

  // Inputs the side panels and the pending-changes panel need. Memoized so
  // they don't get fresh references on every render and cascade re-renders
  // into the children.
  const selectedServiceParentId = useMemo<string | null>(() => {
    if (!selectedServiceId) return null;
    const node = nodes.find(
      (n) =>
        n.type === "service" &&
        (n.data as ServiceNodeData | undefined)?.serviceId === selectedServiceId,
    );
    return node?.parentId ?? null;
  }, [nodes, selectedServiceId]);

  const selectedGroupLabel = useMemo<string>(() => {
    if (!selectedGroupId) return "";
    const node = nodes.find((n) => n.id === selectedGroupId);
    return ((node?.data ?? {}) as { label?: string }).label ?? "";
  }, [nodes, selectedGroupId]);

  const groupNodesForPanel = useMemo(
    () =>
      nodes
        .filter((n) => n.type === "group")
        .map((n) => ({
          id: n.id,
          label: ((n.data ?? {}) as { label?: string }).label ?? "",
        })),
    [nodes],
  );

  const serviceParentsForPanel = useMemo<Map<string, string | null>>(() => {
    const out = new Map<string, string | null>();
    for (const n of nodes) {
      if (n.type !== "service") continue;
      const svcId = (n.data as ServiceNodeData | undefined)?.serviceId;
      if (!svcId) continue;
      out.set(svcId, n.parentId ?? null);
    }
    return out;
  }, [nodes]);

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
      const hasPrefill = !!draft.prefill;
      // Carry the latest ResizeObserver measurement back onto the node so RF
      // marks it dimensioned. Without this, the prefill variant (which has
      // no fixed height) keeps `visibility: hidden` once a position update
      // re-creates the user node and resets internal measurement state.
      const measured =
        draft.measuredWidth !== undefined && draft.measuredHeight !== undefined
          ? { width: draft.measuredWidth, height: draft.measuredHeight }
          : undefined;
      extras.push({
        id: `draft-${draft.draftId}`,
        type: "service-draft",
        position: { x: draft.x, y: draft.y },
        data: {
          draftId: draft.draftId,
          isSubmitting: draft.isSubmitting,
          errorMessage: draft.errorMessage,
          prefill: draft.prefill,
          takenNames: takenServiceNames,
          onSubmit: submitServiceDraft,
          onCancel: cancelServiceDraft,
        },
        selectable: false,
        dragHandle: ".service-draft-drag-handle",
        initialWidth: DRAFT_NODE_WIDTH,
        ...(measured ? { measured } : {}),
        // Height is auto when a prefill (with env vars) is present; React
        // Flow measures the rendered node so the form can grow.
        ...(hasPrefill
          ? { style: { width: DRAFT_NODE_WIDTH } }
          : {
              initialHeight: DRAFT_NODE_HEIGHT,
              style: { width: DRAFT_NODE_WIDTH, height: DRAFT_NODE_HEIGHT },
            }),
      });
    }
    return [...decoratedNodes, ...extras];
  }, [decoratedNodes, remoteDrafts, localDrafts, takenServiceNames, submitServiceDraft, cancelServiceDraft]);

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
        edgeTypes={edgeTypes}
        defaultEdgeOptions={defaultEdgeOptions}
        connectionMode={ConnectionMode.Loose}
        onNodesChange={handleNodesChange}
        onNodeDragStop={handleNodeDragStop}
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
        {selectedServiceId || selectedPendingNodeId ? null : (
          <Panel position="top-right">
            <AvatarStack users={connectedUsers} />
          </Panel>
        )}
        {selectedServiceId ? (
          <ServiceDetailPanel
            projectId={projectId}
            serviceId={selectedServiceId}
            groupId={selectedServiceParentId}
            onClose={() => setSelectedServiceId(null)}
          />
        ) : null}
        {selectedGroupId ? (
          <GroupDetailPanel
            projectId={projectId}
            groupId={selectedGroupId}
            groupLabel={selectedGroupLabel}
            onClose={() => setSelectedGroupId(null)}
          />
        ) : null}
        {selectedPendingNodeId ? (
          <PendingServicePanel
            projectId={projectId}
            nodeId={selectedPendingNodeId}
            onClose={() => setSelectedPendingNodeId(null)}
          />
        ) : null}
        <CursorTracker onCursorMove={sendCursorMove} />
        <CursorOverlay cursors={cursors} />
        <CanvasControls projectId={projectId} />
      </ReactFlow>
      <CanvasContextMenu
        position={menu ? { x: menu.screenX, y: menu.screenY } : null}
        onClose={closeMenu}
        onAddService={handleAddService}
        onAddDatabase={handleAddDatabase}
        onAddNote={handleAddNote}
        onAddGroup={handleAddGroup}
      />
      <AddDatabaseDialog
        open={databaseDialog !== null}
        onOpenChange={(open) => {
          if (!open) setDatabaseDialog(null);
        }}
        onPick={(preset) => {
          if (!databaseDialog) return;
          openServiceDraft(databaseDialog.flowX, databaseDialog.flowY, {
            name: preset.id,
            image: preset.image,
            iconUrl: preset.icon_url,
            envVars: preset.env_vars.map((ev) => ({
              key: ev.key,
              value: ev.value,
            })),
          });
          setDatabaseDialog(null);
        }}
      />
      <NodeContextMenu
        position={
          nodeMenu ? { x: nodeMenu.screenX, y: nodeMenu.screenY } : null
        }
        onClose={closeNodeMenu}
        onDelete={handleDeleteNode}
      />
      <PendingChangesPanel
        projectId={projectId}
        groupNodes={groupNodesForPanel}
        serviceParents={serviceParentsForPanel}
      />
      {!isInitialStateLoaded && <CanvasLoader />}
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
