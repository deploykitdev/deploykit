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

function getNodeCenterIntersection(
  node: InternalNode<Node>,
  other: InternalNode<Node>,
) {
  const nodePos = node.internals.positionAbsolute;
  const otherPos = other.internals.positionAbsolute;

  const w = (node.measured.width ?? 0) / 2;
  const h = (node.measured.height ?? 0) / 2;

  const x2 = nodePos.x + w;
  const y2 = nodePos.y + h;
  const x1 = otherPos.x + (other.measured.width ?? 0) / 2;
  const y1 = otherPos.y + (other.measured.height ?? 0) / 2;

  const xx1 = (x1 - x2) / (2 * w) - (y1 - y2) / (2 * h);
  const yy1 = (x1 - x2) / (2 * w) + (y1 - y2) / (2 * h);
  const a = 1 / (Math.abs(xx1) + Math.abs(yy1) || 1);
  const xx3 = a * xx1;
  const yy3 = a * yy1;
  const x = w * (xx3 + yy3) + x2;
  const y = h * (-xx3 + yy3) + y2;

  return { x, y };
}

function getEdgePosition(
  node: InternalNode<Node>,
  point: { x: number; y: number },
): Position {
  const pos = node.internals.positionAbsolute;
  const nx = Math.round(pos.x);
  const ny = Math.round(pos.y);
  const px = Math.round(point.x);
  const py = Math.round(point.y);
  const w = node.measured.width ?? 0;
  const h = node.measured.height ?? 0;

  if (px <= nx + 1) return Position.Left;
  if (px >= nx + w - 1) return Position.Right;
  if (py <= ny + 1) return Position.Top;
  if (py >= ny + h - 1) return Position.Bottom;
  return Position.Top;
}

function getEdgeParams(
  source: InternalNode<Node>,
  target: InternalNode<Node>,
) {
  const sp = getNodeCenterIntersection(source, target);
  const tp = getNodeCenterIntersection(target, source);
  return {
    sx: sp.x,
    sy: sp.y,
    tx: tp.x,
    ty: tp.y,
    sourcePos: getEdgePosition(source, sp),
    targetPos: getEdgePosition(target, tp),
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
