import { useEffect, useRef, useState } from "react";
import { NodeToolbar, Position, useReactFlow, type NodeProps } from "@xyflow/react";
import { cn } from "@/lib/utils";

export const NOTE_COLORS = [
  "#FFE69A", // yellow
  "#FFB3BA", // pink
  "#BAE1FF", // blue
  "#BAFFC9", // green
  "#E0BBE4", // purple
  "#D9D9D9", // gray
] as const;

export const NOTE_DEFAULT_COLOR = NOTE_COLORS[0];
export const NOTE_SIZE = 220;

export interface NoteNodeData extends Record<string, unknown> {
  label: string;
  backgroundColor?: string;
  // Injected by ProjectFlow via the dynamic nodeTypes closure so the node can
  // persist its own edits without reaching into the WS ref directly.
  commitNote?: (id: string, label: string, backgroundColor: string) => void;
}

export function NoteNode({ id, data, selected }: NodeProps) {
  const { label, backgroundColor, commitNote } = data as NoteNodeData;
  const color = backgroundColor ?? NOTE_DEFAULT_COLOR;

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(label);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const { getNode } = useReactFlow();

  // Keep local draft in sync when remote edits land while we're not editing.
  useEffect(() => {
    if (!editing) setDraft(label);
  }, [label, editing]);

  useEffect(() => {
    if (editing) textareaRef.current?.focus();
  }, [editing]);

  const enterEdit = () => setEditing(true);

  const commitText = () => {
    setEditing(false);
    if (draft !== label) commitNote?.(id, draft, color);
  };

  const cancelEdit = () => {
    setDraft(label);
    setEditing(false);
  };

  const pickColor = (next: string) => {
    if (next === color) return;
    // Use the latest label from the source-of-truth node, not the in-flight
    // textarea draft, so a stray color click doesn't also commit unsaved text.
    const current = getNode(id);
    const currentLabel = (current?.data as NoteNodeData | undefined)?.label ?? label;
    commitNote?.(id, currentLabel, next);
  };

  return (
    <>
      <NodeToolbar isVisible={selected} position={Position.Top} offset={10}>
        <div
          className="flex items-center gap-1.5 rounded-full border border-border/70 bg-popover px-2 py-1.5 shadow-md"
          onPointerDown={(e) => e.stopPropagation()}
          onMouseDown={(e) => e.stopPropagation()}
        >
          {NOTE_COLORS.map((c) => (
            <button
              key={c}
              type="button"
              onPointerDown={(e) => e.stopPropagation()}
              onMouseDown={(e) => e.stopPropagation()}
              onClick={(e) => {
                e.stopPropagation();
                pickColor(c);
              }}
              className={cn(
                "size-4 rounded-full ring-1 ring-black/10 transition-transform hover:scale-110",
                c === color && "ring-2 ring-offset-1 ring-offset-popover ring-foreground/70",
              )}
              style={{ backgroundColor: c }}
              aria-label={`Set note color ${c}`}
            />
          ))}
        </div>
      </NodeToolbar>

      <div
        onDoubleClick={enterEdit}
        className={cn(
          "relative flex select-none rounded-sm shadow-md transition-[box-shadow,transform] duration-150",
          "shadow-[0_8px_24px_-8px_rgb(0_0_0/0.35),0_2px_6px_-2px_rgb(0_0_0/0.2)]",
          selected && "ring-2 ring-foreground/40",
        )}
        style={{
          width: NOTE_SIZE,
          height: NOTE_SIZE,
          backgroundColor: color,
          transform: "rotate(-1deg)",
          color: "#1f2937",
        }}
      >
        {editing ? (
          <textarea
            ref={textareaRef}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={commitText}
            onKeyDown={(e) => {
              if (e.key === "Escape") {
                e.preventDefault();
                cancelEdit();
              }
            }}
            onPointerDown={(e) => e.stopPropagation()}
            className="nodrag nowheel size-full resize-none bg-transparent p-3.5 text-[15px] leading-snug font-medium outline-none placeholder:text-black/30"
            placeholder="Type a note…"
          />
        ) : (
          <div
            className={cn(
              "size-full overflow-hidden whitespace-pre-wrap break-words p-3.5 text-[15px] leading-snug font-medium",
              !label && "italic opacity-50",
            )}
          >
            {label || "Double-click to edit"}
          </div>
        )}
      </div>
    </>
  );
}
