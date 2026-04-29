import { useState } from "react";
import { ApiError } from "@/lib/api";
import type { Project } from "@/lib/queries";
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

interface ProjectDeleteDialogProps {
  project: Project | null;
  onClose: () => void;
  onDelete: () => Promise<void>;
}

export function ProjectDeleteDialog({
  project,
  onClose,
  onDelete,
}: ProjectDeleteDialogProps) {
  const [error, setError] = useState("");
  const [isDeleting, setIsDeleting] = useState(false);

  async function handleDelete() {
    if (!project) return;
    setError("");
    setIsDeleting(true);
    try {
      await onDelete();
    } catch (err) {
      setError(extractMessage(err));
    } finally {
      setIsDeleting(false);
    }
  }

  return (
    <Dialog
      open={!!project}
      onOpenChange={(open) => !open && !isDeleting && onClose()}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete project</DialogTitle>
          <DialogDescription>
            Permanently delete{" "}
            <code className="font-mono">{project?.name}</code>? This action
            cannot be undone.
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
            {isDeleting ? "Deleting…" : "Delete project"}
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
  return "Failed to delete project.";
}
