import type { NodeProps } from "@xyflow/react";

export interface DraftingServiceGhostData extends Record<string, unknown> {
  userName: string;
}

export function DraftingServiceGhost({ data }: NodeProps) {
  const { userName } = data as DraftingServiceGhostData;
  return (
    <div
      className="rounded-md border-2 border-dashed border-muted-foreground/40 bg-background/50 px-3 py-2 text-sm text-muted-foreground"
      style={{ width: 256 }}
    >
      <p className="italic">{userName} is adding a service…</p>
    </div>
  );
}
