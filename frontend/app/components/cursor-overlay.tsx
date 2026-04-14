import { useEffect, useRef, useState } from "react";
import { useReactFlow } from "@xyflow/react";
import type { CursorInfo } from "@/lib/use-canvas-sync";
import { getUserColor } from "@/lib/user-colors";

const IDLE_MS = 3000;

interface CursorOverlayProps {
  cursors: Map<string, CursorInfo>;
}

export function CursorOverlay({ cursors }: CursorOverlayProps) {
  if (cursors.size === 0) return null;

  return (
    <>
      {Array.from(cursors.values()).map((cursor) => (
        <SmoothCursor key={cursor.user_id} cursor={cursor} />
      ))}
    </>
  );
}

function SmoothCursor({ cursor }: { cursor: CursorInfo }) {
  const { flowToScreenPosition } = useReactFlow();
  const ref = useRef<HTMLDivElement>(null);
  const currentPos = useRef({ x: 0, y: 0 });
  const targetPos = useRef({ x: 0, y: 0 });
  const initialized = useRef(false);
  const rafId = useRef(0);
  const idleTimeout = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [isIdle, setIsIdle] = useState(false);
  const flowToScreen = useRef(flowToScreenPosition);
  flowToScreen.current = flowToScreenPosition;

  const color = getUserColor(cursor.user_id);

  // flowToScreenPosition returns viewport-relative coordinates, but the overlay
  // is positioned absolutely inside the ReactFlow container. Subtract the
  // container's top-left so cursors align with the canvas.
  function toContainerPos(flowPos: { x: number; y: number }) {
    const screen = flowToScreen.current(flowPos);
    const container = ref.current?.closest(".react-flow") as HTMLElement | null;
    if (!container) return screen;
    const rect = container.getBoundingClientRect();
    return { x: screen.x - rect.left, y: screen.y - rect.top };
  }

  useEffect(() => {
    targetPos.current = { x: cursor.x, y: cursor.y };

    if (!initialized.current) {
      currentPos.current = toContainerPos({ x: cursor.x, y: cursor.y });
      initialized.current = true;
    }

    setIsIdle(false);
    if (idleTimeout.current) clearTimeout(idleTimeout.current);
    idleTimeout.current = setTimeout(() => setIsIdle(true), IDLE_MS);

    return () => {
      if (idleTimeout.current) clearTimeout(idleTimeout.current);
    };
  }, [cursor.x, cursor.y]);

  useEffect(() => {
    function animate() {
      const cur = currentPos.current;
      const tgt = targetPos.current;

      // Recompute screen position each frame so panning/zooming stays correct.
      const screenTarget = toContainerPos({ x: tgt.x, y: tgt.y });

      const lerpFactor = 0.15;
      cur.x += (screenTarget.x - cur.x) * lerpFactor;
      cur.y += (screenTarget.y - cur.y) * lerpFactor;

      if (ref.current) {
        ref.current.style.transform = `translate(${cur.x}px, ${cur.y}px)`;
      }

      rafId.current = requestAnimationFrame(animate);
    }

    rafId.current = requestAnimationFrame(animate);
    return () => cancelAnimationFrame(rafId.current);
  }, []);

  return (
    <div
      ref={ref}
      className="pointer-events-none absolute top-0 left-0 z-[9999]"
      style={{ opacity: isIdle ? 0 : 1, transition: "opacity 200ms ease-out" }}
    >
      <svg
        width="16"
        height="20"
        viewBox="0 0 16 20"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
      >
        <path
          d="M0.928711 0.578125L15.0713 9.32812L7.14258 10.5781L3.35645 17.7656L0.928711 0.578125Z"
          fill={color}
          stroke="white"
          strokeWidth="1"
        />
      </svg>
      <span
        className="ml-3 -mt-1 inline-block whitespace-nowrap rounded px-1.5 py-0.5 text-xs text-white"
        style={{ backgroundColor: color }}
      >
        {cursor.user_name}
      </span>
    </div>
  );
}
