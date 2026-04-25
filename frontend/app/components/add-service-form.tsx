import { useEffect, useRef, useState } from "react";
import type { NodeProps } from "@xyflow/react";
import { GripVerticalIcon, Trash2Icon } from "lucide-react";
import type { DraftEnvVar, ServiceDraftPrefill } from "@/lib/use-canvas-sync";
import { Input } from "./ui/input";
import { Button } from "./ui/button";

export interface AddServiceFormData extends Record<string, unknown> {
  draftId: string;
  isSubmitting: boolean;
  errorMessage: string | null;
  prefill?: ServiceDraftPrefill;
  onSubmit: (
    draftId: string,
    values: {
      name: string;
      image: string;
      iconUrl?: string;
      envVars?: DraftEnvVar[];
    },
  ) => void;
  onCancel: (draftId: string) => void;
}

export function AddServiceForm({ data }: NodeProps) {
  const { draftId, isSubmitting, errorMessage, prefill, onSubmit, onCancel } =
    data as AddServiceFormData;
  const [name, setName] = useState(prefill?.name ?? "");
  const [image, setImage] = useState(prefill?.image ?? "");
  const [envVars, setEnvVars] = useState<DraftEnvVar[]>(
    () => prefill?.envVars?.map((ev) => ({ ...ev })) ?? [],
  );
  const nameInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    nameInputRef.current?.focus();
    if (prefill?.name) {
      // Pre-filled name → select it so the user can rename quickly.
      nameInputRef.current?.select();
    }
  }, [prefill?.name]);

  function updateEnvVar(index: number, patch: Partial<DraftEnvVar>) {
    setEnvVars((prev) =>
      prev.map((ev, i) => (i === index ? { ...ev, ...patch } : ev)),
    );
  }

  function removeEnvVar(index: number) {
    setEnvVars((prev) => prev.filter((_, i) => i !== index));
  }

  function addEnvVar() {
    setEnvVars((prev) => [...prev, { key: "", value: "" }]);
  }

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
          const trimmedEnvVars = envVars
            .map((ev) => ({ key: ev.key.trim(), value: ev.value }))
            .filter((ev) => ev.key !== "");
          onSubmit(draftId, {
            name: name.trim(),
            image: image.trim(),
            iconUrl: prefill?.iconUrl,
            envVars: trimmedEnvVars.length > 0 ? trimmedEnvVars : undefined,
          });
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

        {(envVars.length > 0 || prefill) && (
          <div className="flex flex-col gap-1.5">
            <div className="flex items-center justify-between">
              <span className="font-mono text-[9px] font-medium uppercase tracking-[0.22em] text-muted-foreground/60">
                Environment Variables
              </span>
              <button
                type="button"
                className="font-mono text-[9px] uppercase tracking-[0.22em] text-muted-foreground/70 hover:text-foreground"
                onClick={addEnvVar}
                disabled={isSubmitting}
              >
                + Add
              </button>
            </div>
            {envVars.length > 0 && (
              <div className="flex max-h-48 flex-col gap-1.5 overflow-y-auto">
                {envVars.map((ev, i) => (
                  <div key={i} className="flex items-center gap-1.5">
                    <Input
                      value={ev.key}
                      onChange={(e) =>
                        updateEnvVar(i, { key: e.target.value })
                      }
                      placeholder="KEY"
                      disabled={isSubmitting}
                      autoComplete="off"
                      className="h-7 flex-1 font-mono text-xs"
                    />
                    <Input
                      value={ev.value}
                      onChange={(e) =>
                        updateEnvVar(i, { value: e.target.value })
                      }
                      placeholder="value"
                      disabled={isSubmitting}
                      autoComplete="off"
                      className="h-7 flex-1 font-mono text-xs"
                    />
                    <button
                      type="button"
                      onClick={() => removeEnvVar(i)}
                      disabled={isSubmitting}
                      className="text-muted-foreground/60 hover:text-destructive"
                      title="Remove"
                    >
                      <Trash2Icon className="size-3.5" />
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

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
