import { useState } from "react";
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
import { FieldError } from "./ui/field";

interface EnvVarDeleteDialogProps {
  envVar: EnvVar | null;
  onClose: () => void;
  onDelete: (id: string) => Promise<void>;
  redeployMessage: string;
}

export function EnvVarDeleteDialog({
  envVar,
  onClose,
  onDelete,
  redeployMessage,
}: EnvVarDeleteDialogProps) {
  const [error, setError] = useState("");
  const [isDeleting, setIsDeleting] = useState(false);

  async function handleDelete() {
    if (!envVar) return;
    setError("");
    setIsDeleting(true);
    try {
      await onDelete(envVar.id);
      onClose();
    } catch (err) {
      setError(extractMessage(err));
    } finally {
      setIsDeleting(false);
    }
  }

  return (
    <Dialog open={!!envVar} onOpenChange={(open) => !open && !isDeleting && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete variable</DialogTitle>
          <DialogDescription>
            Delete <code className="font-mono">{envVar?.key}</code>?{" "}
            {redeployMessage}
          </DialogDescription>
        </DialogHeader>
        {error && <FieldError>{error}</FieldError>}
        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={isDeleting}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={handleDelete}
            disabled={isDeleting}
          >
            {isDeleting ? "Deleting…" : "Delete"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function extractMessage(err: unknown): string {
  if (err instanceof ApiError) {
    try {
      const body = JSON.parse(err.message);
      if (body.message) return body.message;
    } catch {
      // fall through
    }
  }
  return "Failed to delete variable.";
}
