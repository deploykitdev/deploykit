import { useState } from "react";
import { ContainerIcon, Loader2Icon } from "lucide-react";
import {
  fetchDatabasePreset,
  useDatabasePresets,
  type Preset,
} from "@/lib/queries";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "./ui/dialog";

interface AddDatabaseDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onPick: (preset: Preset) => void;
}

export function AddDatabaseDialog({
  open,
  onOpenChange,
  onPick,
}: AddDatabaseDialogProps) {
  const { data: presets, isLoading, error } = useDatabasePresets();
  const [pendingId, setPendingId] = useState<string | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  async function handlePick(id: string) {
    setPendingId(id);
    setErrorMessage(null);
    try {
      const materialized = await fetchDatabasePreset(id);
      onPick(materialized);
      onOpenChange(false);
    } catch (e) {
      setErrorMessage(e instanceof Error ? e.message : "Failed to load preset.");
    } finally {
      setPendingId(null);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Add database</DialogTitle>
          <DialogDescription>
            Pick a preset. We'll fill in the Docker image and the env vars the
            container needs to start. You can edit any value before committing.
          </DialogDescription>
        </DialogHeader>

        {error ? (
          <p className="text-sm text-destructive">
            Failed to load presets: {error.message}
          </p>
        ) : null}

        {isLoading ? (
          <div className="flex items-center justify-center py-12 text-muted-foreground">
            <Loader2Icon className="size-5 animate-spin" />
          </div>
        ) : (
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
            {(presets ?? []).map((preset) => (
              <PresetCard
                key={preset.id}
                preset={preset}
                disabled={pendingId !== null}
                loading={pendingId === preset.id}
                onClick={() => handlePick(preset.id)}
              />
            ))}
          </div>
        )}

        {errorMessage ? (
          <p className="text-sm text-destructive">{errorMessage}</p>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

interface PresetCardProps {
  preset: Preset;
  disabled: boolean;
  loading: boolean;
  onClick: () => void;
}

function PresetCard({ preset, disabled, loading, onClick }: PresetCardProps) {
  const [iconBroken, setIconBroken] = useState(false);
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className="group flex flex-col items-center gap-2 rounded-lg border bg-card p-4 text-card-foreground transition-colors hover:border-primary/40 hover:bg-accent disabled:cursor-not-allowed disabled:opacity-60"
    >
      <div className="flex size-12 items-center justify-center">
        {loading ? (
          <Loader2Icon className="size-7 animate-spin text-muted-foreground" />
        ) : iconBroken || !preset.icon_url ? (
          <ContainerIcon className="size-7 text-muted-foreground" />
        ) : (
          <img
            src={preset.icon_url}
            alt=""
            className="size-12 object-contain"
            onError={() => setIconBroken(true)}
          />
        )}
      </div>
      <div className="flex flex-col items-center gap-0.5">
        <span className="text-sm font-medium">{preset.name}</span>
        <span className="font-mono text-[10px] text-muted-foreground">
          {preset.image}
        </span>
      </div>
    </button>
  );
}
