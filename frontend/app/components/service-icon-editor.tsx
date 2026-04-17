import { useEffect, useRef, useState } from "react";
import { BoxIcon, PencilIcon } from "lucide-react";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { cn } from "@/lib/utils";
import { useUpdateService } from "@/lib/queries";

interface ServiceIconEditorProps {
  projectId: string;
  serviceId: string;
  iconUrl: string | null;
}

export function ServiceIconEditor({
  projectId,
  serviceId,
  iconUrl,
}: ServiceIconEditorProps) {
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState(iconUrl ?? "");
  const [error, setError] = useState<string | null>(null);
  const [imgBroken, setImgBroken] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const updateService = useUpdateService(projectId, serviceId);

  useEffect(() => {
    setImgBroken(false);
  }, [iconUrl]);

  useEffect(() => {
    if (!open) return;
    setValue(iconUrl ?? "");
    setError(null);
    requestAnimationFrame(() => inputRef.current?.select());

    function handleClickOutside(e: MouseEvent) {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    document.addEventListener("mousedown", handleClickOutside);
    document.addEventListener("keydown", handleKey);
    return () => {
      document.removeEventListener("mousedown", handleClickOutside);
      document.removeEventListener("keydown", handleKey);
    };
  }, [open, iconUrl]);

  const showCustom = Boolean(iconUrl) && !imgBroken;

  async function submit(nextValue: string) {
    setError(null);
    try {
      await updateService.mutateAsync({ icon_url: nextValue });
      setOpen(false);
    } catch (err) {
      const message =
        err instanceof Error ? err.message : "Could not save icon.";
      setError(message);
    }
  }

  return (
    <div ref={rootRef} className="relative shrink-0">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-label="Change service icon"
        aria-expanded={open}
        className={cn(
          "group/icon relative grid size-8 place-items-center overflow-hidden rounded-md transition-colors",
          showCustom
            ? "hover:ring-2 hover:ring-foreground/15"
            : "border border-border/80 bg-background/60 shadow-[inset_0_1px_0_rgb(255_255_255/0.04)] hover:border-foreground/30 hover:bg-muted",
          open &&
            (showCustom
              ? "ring-2 ring-primary/40"
              : "border-primary/60 ring-[2px] ring-primary/15"),
        )}
      >
        {showCustom ? (
          <img
            src={iconUrl ?? undefined}
            alt=""
            className="size-6 object-contain"
            onError={() => setImgBroken(true)}
            draggable={false}
          />
        ) : (
          <BoxIcon
            className="size-4 text-muted-foreground"
            strokeWidth={1.75}
          />
        )}
        <span className="pointer-events-none absolute inset-0 grid place-items-center bg-background/70 opacity-0 transition-opacity group-hover/icon:opacity-100">
          <PencilIcon className="size-3.5 text-foreground" strokeWidth={2} />
        </span>
      </button>

      {open ? (
        <div
          role="dialog"
          aria-label="Service icon URL"
          className="absolute left-0 top-full z-50 mt-2 w-72 rounded-lg border bg-popover p-3 text-popover-foreground shadow-lg ring-1 ring-foreground/5 animate-in fade-in-0 zoom-in-95"
        >
          <div className="mb-2 font-mono text-[10px] font-medium uppercase tracking-[0.22em] text-muted-foreground/70">
            Icon URL
          </div>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              submit(value.trim());
            }}
            className="flex flex-col gap-2"
          >
            <Input
              ref={inputRef}
              type="url"
              placeholder="https://example.com/icon.svg"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              autoFocus
              disabled={updateService.isPending}
              aria-invalid={error ? true : undefined}
            />
            {error ? (
              <p className="text-xs text-destructive">{error}</p>
            ) : (
              <p className="text-xs text-muted-foreground/70">
                Http(s) URL to a PNG, SVG, or JPG.
              </p>
            )}
            <div className="mt-1 flex items-center gap-2">
              <Button
                type="submit"
                size="sm"
                disabled={updateService.isPending || value.trim() === (iconUrl ?? "")}
              >
                {updateService.isPending ? "Saving…" : "Save"}
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={updateService.isPending || !iconUrl}
                onClick={() => submit("")}
              >
                Clear
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="ml-auto"
                onClick={() => setOpen(false)}
                disabled={updateService.isPending}
              >
                Cancel
              </Button>
            </div>
          </form>
        </div>
      ) : null}
    </div>
  );
}
