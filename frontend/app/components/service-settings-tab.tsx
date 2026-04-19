import { FormEvent, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { ApiError } from "@/lib/api";
import {
  usePendingChanges,
  useService,
  useUpdateService,
} from "@/lib/queries";
import { collectServiceOverride } from "@/lib/pending-changes-diff";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Field, FieldError, FieldLabel } from "./ui/field";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "./ui/card";

interface ServiceSettingsTabProps {
  projectId: string;
  serviceId: string;
}

export function ServiceSettingsTab({
  projectId,
  serviceId,
}: ServiceSettingsTabProps) {
  const { data: service } = useService(projectId, serviceId);
  const { data: pendingChanges } = usePendingChanges(projectId);
  const updateService = useUpdateService(projectId, serviceId);

  // The effective name is the applied name with the latest staged rename
  // layered on top. Staging another rename to the same value is a no-op.
  const override = useMemo(
    () => collectServiceOverride(pendingChanges, serviceId),
    [pendingChanges, serviceId],
  );
  const effectiveName = override?.name ?? service?.name ?? "";
  const [name, setName] = useState(effectiveName);
  const [error, setError] = useState("");

  // Keep the input synced with the effective name when server state or
  // pending changes shift (e.g. a collaborator stages a rename).
  useEffect(() => {
    setName(effectiveName);
  }, [effectiveName]);

  if (!service) return null;

  const trimmed = name.trim();
  const dirty = trimmed !== effectiveName;
  const canSubmit = dirty && trimmed !== "" && !updateService.isPending;

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    if (!canSubmit) return;

    try {
      await updateService.mutateAsync({ name: trimmed });
      toast.success("Rename staged", {
        description: "Deploy the project to apply the change.",
      });
    } catch (err) {
      if (err instanceof ApiError) {
        try {
          const body = JSON.parse(err.message);
          if (body.errors?.name) {
            setError(body.errors.name[0]);
          } else {
            setError(body.message || "Failed to rename service.");
          }
        } catch {
          setError("Failed to rename service.");
        }
      } else {
        setError("Failed to rename service.");
      }
    }
  }

  return (
    <form onSubmit={handleSubmit}>
      <Card>
        <CardHeader>
          <CardTitle>Service name</CardTitle>
          <CardDescription>
            A human-friendly name for this service.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Field data-invalid={error ? true : undefined}>
            <FieldLabel htmlFor="service-name">Name</FieldLabel>
            <Input
              id="service-name"
              value={name}
              onChange={(e) => {
                setName(e.target.value);
                if (error) setError("");
              }}
            />
            <FieldError>{error || undefined}</FieldError>
          </Field>
        </CardContent>
        <CardFooter>
          <Button type="submit" disabled={!canSubmit}>
            {updateService.isPending ? "Saving..." : "Save"}
          </Button>
        </CardFooter>
      </Card>
    </form>
  );
}
