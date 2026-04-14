import { useEffect, useRef, useState } from "react";
import type { NodeProps } from "@xyflow/react";
import { GripVerticalIcon } from "lucide-react";
import { Input } from "./ui/input";
import { Button } from "./ui/button";

export interface AddServiceFormData extends Record<string, unknown> {
  draftId: string;
  isSubmitting: boolean;
  errorMessage: string | null;
  onSubmit: (draftId: string, values: { name: string; image: string }) => void;
  onCancel: (draftId: string) => void;
}

export function AddServiceForm({ data }: NodeProps) {
  const { draftId, isSubmitting, errorMessage, onSubmit, onCancel } =
    data as AddServiceFormData;
  const [name, setName] = useState("");
  const [image, setImage] = useState("");
  const nameInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    nameInputRef.current?.focus();
  }, []);

  return (
    <div
      className="w-64 overflow-hidden rounded-md border bg-popover text-popover-foreground shadow-lg ring-1 ring-foreground/10 nopan"
      onContextMenu={(e) => {
        e.preventDefault();
        e.stopPropagation();
      }}
    >
      <div
        className="service-draft-drag-handle flex h-6 cursor-grab items-center justify-center border-b bg-muted/50 text-muted-foreground active:cursor-grabbing"
        title="Drag to move"
      >
        <GripVerticalIcon className="size-3 rotate-90" />
      </div>
      <form
        style={{ padding: "0.75rem" }}
        onSubmit={(e) => {
          e.preventDefault();
          if (!name.trim() || !image.trim()) return;
          onSubmit(draftId, { name: name.trim(), image: image.trim() });
        }}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.preventDefault();
            e.stopPropagation();
            onCancel(draftId);
          }
        }}
        className="flex flex-col gap-2"
      >
        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium" htmlFor={`name-${draftId}`}>
            Name
          </label>
          <Input
            id={`name-${draftId}`}
            ref={nameInputRef}
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="web"
            disabled={isSubmitting}
            autoComplete="off"
          />
        </div>
        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium" htmlFor={`image-${draftId}`}>
            Image
          </label>
          <Input
            id={`image-${draftId}`}
            value={image}
            onChange={(e) => setImage(e.target.value)}
            placeholder="nginx:latest"
            disabled={isSubmitting}
            autoComplete="off"
          />
        </div>
        {errorMessage ? (
          <p className="text-xs text-destructive">{errorMessage}</p>
        ) : null}
        <div className="flex justify-end gap-2 pt-1">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => onCancel(draftId)}
            disabled={isSubmitting}
          >
            Cancel
          </Button>
          <Button
            type="submit"
            size="sm"
            disabled={isSubmitting || !name.trim() || !image.trim()}
          >
            {isSubmitting ? "Creating…" : "Create"}
          </Button>
        </div>
      </form>
    </div>
  );
}
