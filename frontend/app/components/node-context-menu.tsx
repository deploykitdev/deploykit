import { useEffect, useRef } from "react";
import { Trash2Icon } from "lucide-react";

interface NodeContextMenuProps {
  position: { x: number; y: number } | null;
  onClose: () => void;
  onDelete: () => void;
}

export function NodeContextMenu({
  position,
  onClose,
  onDelete,
}: NodeContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!position) return;

    function handleClickOutside(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onClose();
      }
    }

    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [position, onClose]);

  if (!position) return null;

  return (
    <div
      ref={menuRef}
      className="fixed z-50 min-w-40 rounded-md bg-popover p-1 text-popover-foreground shadow-md ring-1 ring-foreground/10 animate-in fade-in-0 zoom-in-95"
      style={{ top: position.y, left: position.x }}
    >
      <button
        className="flex w-full cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-destructive outline-hidden select-none hover:bg-destructive/10"
        onClick={() => {
          onDelete();
          onClose();
        }}
      >
        <Trash2Icon className="size-4" />
        Delete Node
      </button>
    </div>
  );
}
