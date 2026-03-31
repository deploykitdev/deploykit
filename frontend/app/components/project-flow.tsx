import { useCallback } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  useNodesState,
  useEdgesState,
  addEdge,
  type OnConnect,
  type Node,
  type Edge,
} from "@xyflow/react";

const initialNodes: Node[] = [
  { id: "1", position: { x: 100, y: 200 }, data: { label: "Source" } },
  { id: "2", position: { x: 400, y: 200 }, data: { label: "Build" } },
];

const initialEdges: Edge[] = [
  { id: "e1-2", source: "1", target: "2" },
];

export function ProjectFlow() {
  const [nodes, , onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

  const onConnect: OnConnect = useCallback(
    (params) => setEdges((eds) => addEdge(params, eds)),
    [setEdges],
  );

  return (
    <div className="w-full flex-1 min-h-0">
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
      </ReactFlow>
    </div>
  );
}
