import { useEffect, useRef } from "react";
import { BoxesIcon, BoxIcon, DatabaseIcon, HardDriveIcon } from "lucide-react";

interface CanvasContextMenuProps {
  position: { x: number; y: number } | null;
  onClose: () => void;
  onAddService: () => void;
  onAddDatabase: () => void;
}

export function CanvasContextMenu({
  position,
  onClose,
  onAddService,
  onAddDatabase,
}: CanvasContextMenuProps) {
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
        className="flex w-full cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-hidden select-none hover:bg-accent hover:text-accent-foreground"
        onClick={() => {
          onAddService();
          onClose();
        }}
      >
        <BoxIcon className="size-4" />
        Add Service
      </button>
      <button
        className="flex w-full cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-hidden select-none hover:bg-accent hover:text-accent-foreground"
        onClick={() => {
          onAddDatabase();
          onClose();
        }}
      >
        <DatabaseIcon className="size-4" />
        Add Database
      </button>
      <button
        className="flex w-full cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-hidden select-none hover:bg-accent hover:text-accent-foreground"
        onClick={onClose}
      >
        <BoxesIcon className="size-4" />
        Add Template
      </button>
      <button
        className="flex w-full cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-hidden select-none hover:bg-accent hover:text-accent-foreground"
        onClick={onClose}
      >
        <HardDriveIcon className="size-4" />
        Add Volume
      </button>
    </div>
  );
}
