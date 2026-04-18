import { useEffect, useRef, useState } from "react";
import { ApiError } from "@/lib/api";
import type { EnvVar } from "@/lib/queries";
import { Button } from "./ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "./ui/dialog";
import { Field, FieldError, FieldLabel } from "./ui/field";
import { Input } from "./ui/input";

type SubmitFn =
  | { mode: "create"; submit: (data: { key: string; value: string }) => Promise<void> }
  | { mode: "edit"; envVar: EnvVar; submit: (data: { id: string; value: string }) => Promise<void> };

interface EnvVarFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  action: SubmitFn;
}

export function EnvVarFormDialog({
  open,
  onOpenChange,
  action,
}: EnvVarFormDialogProps) {
  const initialKey = action.mode === "edit" ? action.envVar.key : "";
  const initialValue = action.mode === "edit" ? action.envVar.value : "";

  const [key, setKey] = useState(initialKey);
  const [value, setValue] = useState(initialValue);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [isSubmitting, setIsSubmitting] = useState(false);
  const keyInputRef = useRef<HTMLInputElement>(null);
  const valueInputRef = useRef<HTMLInputElement>(null);

  // Reset form state when the dialog opens or the target env var changes.
  useEffect(() => {
    if (open) {
      setKey(initialKey);
      setValue(initialValue);
      setErrors({});
      setIsSubmitting(false);
      const target = action.mode === "edit" ? valueInputRef : keyInputRef;
      requestAnimationFrame(() => target.current?.focus());
    }
  }, [open, initialKey, initialValue, action.mode]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setErrors({});

    const trimmedKey = key.trim();
    if (action.mode === "create" && !trimmedKey) {
      setErrors({ key: "Key is required." });
      return;
    }

    setIsSubmitting(true);
    try {
      if (action.mode === "create") {
        await action.submit({ key: trimmedKey, value });
      } else {
        await action.submit({ id: action.envVar.id, value });
      }
      onOpenChange(false);
    } catch (err) {
      setErrors(parseApiErrors(err));
    } finally {
      setIsSubmitting(false);
    }
  }

  const isEdit = action.mode === "edit";
  const canSubmit = isEdit ? !isSubmitting : !isSubmitting && key.trim().length > 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={handleSubmit} className="flex flex-col gap-6">
          <DialogHeader>
            <DialogTitle>{isEdit ? "Edit variable" : "Add variable"}</DialogTitle>
            <DialogDescription>
              {isEdit
                ? "Services will be redeployed with the new value."
                : "All services in scope will be redeployed with the new variable."}
            </DialogDescription>
          </DialogHeader>

          {errors._form && <FieldError>{errors._form}</FieldError>}

          <div className="flex flex-col gap-4">
            <Field data-invalid={errors.key ? true : undefined}>
              <FieldLabel htmlFor="env-var-key">Key</FieldLabel>
              <Input
                id="env-var-key"
                ref={keyInputRef}
                value={key}
                onChange={(e) => setKey(e.target.value)}
                placeholder="DATABASE_URL"
                disabled={isEdit || isSubmitting}
                autoComplete="off"
                spellCheck={false}
                className="font-mono"
              />
              {errors.key && <FieldError>{errors.key}</FieldError>}
            </Field>

            <Field data-invalid={errors.value ? true : undefined}>
              <FieldLabel htmlFor="env-var-value">Value</FieldLabel>
              <Input
                id="env-var-value"
                ref={valueInputRef}
                value={value}
                onChange={(e) => setValue(e.target.value)}
                placeholder=""
                disabled={isSubmitting}
                autoComplete="off"
                spellCheck={false}
                className="font-mono"
              />
              {errors.value && <FieldError>{errors.value}</FieldError>}
            </Field>
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={isSubmitting}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={!canSubmit}>
              {isSubmitting ? "Saving…" : isEdit ? "Save" : "Add variable"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

// parseApiErrors maps a backend ValidationError (422) or Error envelope into
// {field: message} shape. Falls back to _form: generic message.
function parseApiErrors(err: unknown): Record<string, string> {
  if (!(err instanceof ApiError)) {
    return { _form: "Something went wrong." };
  }
  try {
    const body = JSON.parse(err.message);
    if (body.errors && typeof body.errors === "object") {
      const out: Record<string, string> = {};
      for (const [field, msgs] of Object.entries(body.errors)) {
        if (Array.isArray(msgs) && msgs.length > 0) {
          out[field] = String(msgs[0]);
        }
      }
      if (Object.keys(out).length > 0) return out;
    }
    return { _form: body.message || "Failed to save variable." };
  } catch {
    return { _form: "Failed to save variable." };
  }
}
