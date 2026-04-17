import type { NodeProps } from "@xyflow/react";

export interface DraftingServiceGhostData extends Record<string, unknown> {
  userName: string;
}

export function DraftingServiceGhost({ data }: NodeProps) {
  const { userName } = data as DraftingServiceGhostData;
  return (
    <div className="flex h-full w-80 flex-col items-center justify-center gap-2 rounded-xl border-2 border-dashed border-muted-foreground/40 bg-background/50 px-5 py-4">
      <span className="size-1.5 shrink-0 animate-pulse rounded-full bg-muted-foreground/60" />
      <p className="truncate text-sm italic text-muted-foreground">
        {userName} is adding a service…
      </p>
    </div>
  );
}
