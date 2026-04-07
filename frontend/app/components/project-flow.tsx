import { useEffect, useRef } from "react";
import { ReactFlow, Background, Controls, Panel, useReactFlow } from "@xyflow/react";
import { toast } from "sonner";
import { useCanvasSync } from "@/lib/use-canvas-sync";
import { CursorOverlay } from "./cursor-overlay";
import { AvatarStack } from "./avatar-stack";

interface ProjectFlowProps {
  projectId: string;
}

export function ProjectFlow({ projectId }: ProjectFlowProps) {
  const {
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
  } = useCanvasSync(projectId);

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
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        fitView
        maxZoom={1}
      >
        <Background />
        <Controls />
        <Panel position="top-right">
          <AvatarStack users={connectedUsers} />
        </Panel>
        <CursorTracker onCursorMove={sendCursorMove} />
        <CursorOverlay cursors={cursors} />
      </ReactFlow>
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

    const container = marker.closest(".react-flow");
    if (!container) return;

    const handler = (event: Event) => {
      const e = event as MouseEvent;
      const pos = positionRef.current({ x: e.clientX, y: e.clientY });
      callbackRef.current(pos.x, pos.y);
    };

    container.addEventListener("mousemove", handler, { passive: true });
    return () => container.removeEventListener("mousemove", handler);
  }, []);

  return <div ref={markerRef} style={{ display: "none" }} />;
}
