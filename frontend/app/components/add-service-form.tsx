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
      className="nopan relative w-80 overflow-hidden rounded-xl border border-primary/40 bg-card text-card-foreground shadow-lg ring-[3px] ring-primary/10"
      onContextMenu={(e) => {
        e.preventDefault();
        e.stopPropagation();
      }}
    >
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 opacity-40 dark:opacity-25"
        style={{
          backgroundImage:
            "radial-gradient(circle at 1px 1px, currentColor 0.6px, transparent 0)",
          backgroundSize: "14px 14px",
          color: "var(--muted-foreground)",
          WebkitMaskImage:
            "radial-gradient(ellipse 85% 100% at 100% 0%, black 10%, transparent 75%)",
          maskImage:
            "radial-gradient(ellipse 85% 100% at 100% 0%, black 10%, transparent 75%)",
        }}
      />

      <div
        aria-hidden
        className="absolute inset-y-0 left-0 w-[3px] bg-primary"
      />

      <div
        className="service-draft-drag-handle relative flex cursor-grab items-center gap-2 border-b px-5 py-2.5 active:cursor-grabbing"
        title="Drag to move"
      >
        <GripVerticalIcon className="-ml-1.5 size-3.5 text-muted-foreground/50" />
        <span className="font-mono text-[10px] font-medium uppercase tracking-[0.22em] text-muted-foreground/70">
          New Service
        </span>
      </div>

      <form
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
        className="relative flex flex-col gap-3.5 px-5 pt-4 pb-4"
      >
        <div className="flex flex-col gap-1.5">
          <label
            className="font-mono text-[9px] font-medium uppercase tracking-[0.22em] text-muted-foreground/60"
            htmlFor={`name-${draftId}`}
          >
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

        <div className="flex flex-col gap-1.5">
          <label
            className="font-mono text-[9px] font-medium uppercase tracking-[0.22em] text-muted-foreground/60"
            htmlFor={`image-${draftId}`}
          >
            Image
          </label>
          <Input
            id={`image-${draftId}`}
            value={image}
            onChange={(e) => setImage(e.target.value)}
            placeholder="nginx:latest"
            disabled={isSubmitting}
            autoComplete="off"
            className="font-mono"
          />
        </div>

        {errorMessage ? (
          <p className="text-xs text-destructive">{errorMessage}</p>
        ) : null}

        <div className="mt-0.5 flex items-center justify-end gap-2">
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
