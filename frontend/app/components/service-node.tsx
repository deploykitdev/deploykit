import { Handle, Position, type NodeProps } from "@xyflow/react";
import { BoxIcon } from "lucide-react";

export interface ServiceNodeData extends Record<string, unknown> {
  label: string;
  image?: string;
}

export function ServiceNode({ data, selected }: NodeProps) {
  const { label, image } = data as ServiceNodeData;

  return (
    <div
      className={
        "w-64 overflow-hidden rounded-lg border bg-card text-card-foreground shadow-md transition-shadow " +
        (selected
          ? "ring-2 ring-primary ring-offset-2 ring-offset-background"
          : "hover:shadow-lg")
      }
    >
      <Handle
        type="target"
        position={Position.Left}
        className="!h-0 !w-0 !min-h-0 !min-w-0 !border-0 !bg-transparent !opacity-0"
      />
      <div className="flex items-center gap-2 border-b bg-muted/40 px-3 py-2">
        <BoxIcon className="size-4 text-muted-foreground" />
        <span className="truncate text-sm font-medium">{label}</span>
      </div>
      <div className="px-3 py-2">
        {image ? (
          <div className="flex items-center gap-1.5">
            <span className="text-xs text-muted-foreground">Image</span>
            <code className="truncate rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
              {image}
            </code>
          </div>
        ) : (
          <span className="text-xs italic text-muted-foreground">
            No deployment yet
          </span>
        )}
      </div>
      <Handle
        type="source"
        position={Position.Right}
        className="!h-0 !w-0 !min-h-0 !min-w-0 !border-0 !bg-transparent !opacity-0"
      />
    </div>
  );
}
