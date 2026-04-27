import {
  BaseEdge,
  EdgeLabelRenderer,
  getSmoothStepPath,
  Position,
  useInternalNode,
  type EdgeProps,
  type InternalNode,
  type Node,
} from "@xyflow/react";

// FloatingEdge anchors its endpoints to the closest point on each node's
// bounding box, so the connection slides around the perimeter as nodes move
// rather than always anchoring to fixed left/right handles.
//
// Side selection is slope-based (compare |dy| * hw vs |dx| * hh) rather than
// rounding the boundary point and matching it back to a ±1px window of an
// edge — that older approach flipped between sides under subpixel measurement
// jitter, causing the path to flicker when nothing was actually moving.

interface BoundaryPoint {
  x: number;
  y: number;
  pos: Position;
}

function projectOntoBox(
  cx: number,
  cy: number,
  w: number,
  h: number,
  ox: number,
  oy: number,
): BoundaryPoint {
  if (w <= 0 || h <= 0) return { x: cx, y: cy, pos: Position.Right };

  const dx = ox - cx;
  const dy = oy - cy;
  const hw = w / 2;
  const hh = h / 2;

  if (dx === 0 && dy === 0) {
    return { x: cx + hw, y: cy, pos: Position.Right };
  }

  // Compare |dy/dx| against the corner slope hh/hw without division — this
  // stays stable under subpixel jitter, unlike rounding the boundary point.
  if (Math.abs(dy) * hw > Math.abs(dx) * hh) {
    const t = hh / Math.abs(dy);
    return {
      x: cx + dx * t,
      y: dy > 0 ? cy + hh : cy - hh,
      pos: dy > 0 ? Position.Bottom : Position.Top,
    };
  }

  const t = hw / Math.abs(dx);
  return {
    x: dx > 0 ? cx + hw : cx - hw,
    y: cy + dy * t,
    pos: dx > 0 ? Position.Right : Position.Left,
  };
}

function nodeCenter(node: InternalNode<Node>) {
  const pos = node.internals.positionAbsolute;
  const w = node.measured.width ?? 0;
  const h = node.measured.height ?? 0;
  return { cx: pos.x + w / 2, cy: pos.y + h / 2, w, h };
}

function getEdgeParams(
  source: InternalNode<Node>,
  target: InternalNode<Node>,
) {
  const s = nodeCenter(source);
  const t = nodeCenter(target);
  const sp = projectOntoBox(s.cx, s.cy, s.w, s.h, t.cx, t.cy);
  const tp = projectOntoBox(t.cx, t.cy, t.w, t.h, s.cx, s.cy);
  return {
    sx: sp.x,
    sy: sp.y,
    tx: tp.x,
    ty: tp.y,
    sourcePos: sp.pos,
    targetPos: tp.pos,
  };
}

export function FloatingEdge({
  id,
  source,
  target,
  markerEnd,
  style,
  label,
  labelStyle,
}: EdgeProps) {
  const sourceNode = useInternalNode(source);
  const targetNode = useInternalNode(target);

  if (!sourceNode || !targetNode) return null;

  const { sx, sy, tx, ty, sourcePos, targetPos } = getEdgeParams(
    sourceNode,
    targetNode,
  );

  const [edgePath, labelX, labelY] = getSmoothStepPath({
    sourceX: sx,
    sourceY: sy,
    sourcePosition: sourcePos,
    targetPosition: targetPos,
    targetX: tx,
    targetY: ty,
    borderRadius: 8,
  });

  return (
    <>
      <BaseEdge id={id} path={edgePath} markerEnd={markerEnd} style={style} />
      {label ? (
        <EdgeLabelRenderer>
          <div
            style={{
              position: "absolute",
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
              pointerEvents: "all",
              ...labelStyle,
            }}
            className="rounded bg-background/90 px-1.5 py-0.5 font-mono"
          >
            {label}
          </div>
        </EdgeLabelRenderer>
      ) : null}
    </>
  );
}
