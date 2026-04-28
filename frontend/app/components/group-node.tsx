import { useEffect, useRef, useState } from "react";
import { NodeResizer, type NodeProps } from "@xyflow/react";
import { cn } from "@/lib/utils";

export const GROUP_DEFAULT_WIDTH = 520;
export const GROUP_DEFAULT_HEIGHT = 360;
export const GROUP_MIN_WIDTH = 200;
export const GROUP_MIN_HEIGHT = 140;

export interface GroupNodeData extends Record<string, unknown> {
  label: string;
  // Injected by ProjectFlow via the dynamic nodeTypes closure so the node can
  // persist its own edits without reaching into the WS ref directly.
  commitGroup?: (id: string, label: string) => void;
  resizeGroup?: (id: string, width: number, height: number) => void;
}

export function GroupNode({ id, data, selected, width, height }: NodeProps) {
  const { label, commitGroup, resizeGroup } = data as GroupNodeData;

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(label);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!editing) setDraft(label);
  }, [label, editing]);

  useEffect(() => {
    if (editing) inputRef.current?.focus();
  }, [editing]);

  const enterEdit = () => setEditing(true);

  const commitText = () => {
    setEditing(false);
    if (draft !== label) commitGroup?.(id, draft);
  };

  const cancelEdit = () => {
    setDraft(label);
    setEditing(false);
  };

  const w = width ?? GROUP_DEFAULT_WIDTH;
  const h = height ?? GROUP_DEFAULT_HEIGHT;

  return (
    <div
      onDoubleClick={enterEdit}
      className={cn(
        "relative rounded-lg border-2 border-dashed bg-foreground/[0.025] backdrop-blur-[1px] transition-[border-color,box-shadow,background-color] duration-200 ease-out",
        selected
          ? "border-primary/70 ring-[3px] ring-primary/15 bg-foreground/[0.04]"
          : "border-foreground/20",
      )}
      style={{ width: w, height: h }}
    >
      <NodeResizer
        isVisible={selected}
        minWidth={GROUP_MIN_WIDTH}
        minHeight={GROUP_MIN_HEIGHT}
        lineClassName="!border-foreground/40"
        handleClassName="!bg-background !border-foreground/60"
        onResizeEnd={(_evt, params) => {
          resizeGroup?.(id, Math.round(params.width), Math.round(params.height));
        }}
      />
      <div className="absolute -top-3 left-3 flex items-center gap-1.5 rounded bg-background px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground ring-1 ring-foreground/10">
        {editing ? (
          <input
            ref={inputRef}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={commitText}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                commitText();
              } else if (e.key === "Escape") {
                e.preventDefault();
                cancelEdit();
              }
            }}
            onPointerDown={(e) => e.stopPropagation()}
            className="nodrag nowheel min-w-24 bg-transparent text-[11px] font-semibold uppercase tracking-wide text-foreground outline-none"
            placeholder="Group"
          />
        ) : (
          <span>{label || "Group"}</span>
        )}
      </div>
    </div>
  );
}
