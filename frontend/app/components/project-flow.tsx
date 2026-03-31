import { useCallback, useEffect, useRef } from "react";
import { ReactFlow, Background, Controls, useReactFlow } from "@xyflow/react";
import { useCanvasSync } from "@/lib/use-canvas-sync";
import { CursorOverlay } from "./cursor-overlay";

interface ProjectFlowProps {
  projectId: string;
}

export function ProjectFlow({ projectId }: ProjectFlowProps) {
  const {
    nodes,
    edges,
    cursors,
    onNodesChange,
    onEdgesChange,
    onConnect,
    sendCursorMove,
  } = useCanvasSync(projectId);

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
