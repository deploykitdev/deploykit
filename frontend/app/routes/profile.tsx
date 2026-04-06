import { FormEvent, useEffect, useState } from "react";
import { toast } from "sonner";
import { RequireAuth, useAuth } from "../lib/auth";
import { ApiError } from "../lib/api";
import { useUpdateProfile } from "../lib/queries";
import { DashboardLayout } from "@/components/dashboard-layout";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Field,
  FieldLabel,
  FieldError,
  FieldDescription,
} from "@/components/ui/field";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
} from "@/components/ui/card";

export default function ProfilePage() {
  return (
    <RequireAuth>
      <DashboardLayout>
        <h1 className="mt-4 mb-8 text-2xl font-semibold">Profile</h1>
        <div className="grid gap-6">
          <ProfileInfoCard />
          <ChangePasswordCard />
        </div>
      </DashboardLayout>
    </RequireAuth>
  );
}

type FormStatus =
  | { kind: "idle" }
  | { kind: "error"; errors: Record<string, string> };

function parseApiError(err: unknown, fallback: string): Record<string, string> {
  if (err instanceof ApiError) {
    try {
      const body = JSON.parse(err.message);
      if (body.errors && typeof body.errors === "object") {
        const fieldErrors: Record<string, string> = {};
        for (const [field, msgs] of Object.entries(body.errors)) {
          fieldErrors[field] = Array.isArray(msgs)
            ? (msgs[0] as string)
            : String(msgs);
        }
        if (body.message) fieldErrors._form = body.message;
        return fieldErrors;
      }
      return { _form: body.message || fallback };
    } catch {
      // Non-JSON error body — surface it directly so we can diagnose.
      const raw = err.message?.trim();
      return { _form: raw ? `${fallback} (${err.status}: ${raw})` : fallback };
    }
  }
  if (err instanceof Error) {
    return { _form: `${fallback} ${err.message}` };
  }
  return { _form: fallback };
}

function ProfileInfoCard() {
  const { user, refreshUser } = useAuth();
  const updateProfile = useUpdateProfile();

  const [name, setName] = useState(user?.name ?? "");
  const [email, setEmail] = useState(user?.email ?? "");
  const [status, setStatus] = useState<FormStatus>({ kind: "idle" });

  useEffect(() => {
    if (user) {
      setName(user.name);
      setEmail(user.email);
    }
  }, [user]);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!user) return;
    setStatus({ kind: "idle" });

    const data: {
      name?: string;
      email?: string;
      current_password: string;
    } = { current_password: "" };

    if (name !== user.name) data.name = name;
    if (email !== user.email) data.email = email;

    try {
      if (data.name || data.email) {
        await updateProfile.mutateAsync(data);
        await refreshUser();
      }
      setStatus({ kind: "idle" });
      toast.success("Profile information was updated successfully");
    } catch (err) {
      setStatus({
        kind: "error",
        errors: parseApiError(err, "Failed to update profile."),
      });
    }
  }

  const errors = status.kind === "error" ? status.errors : {};

  if (!user) return null;

  return (
    <form onSubmit={handleSubmit}>
      <Card>
        <CardHeader>
          <CardTitle>Profile Information</CardTitle>
          <CardDescription>
            Update your name and email address.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {errors._form && (
            <div className="mb-4">
              <FieldError>{errors._form}</FieldError>
            </div>
          )}

          <div className="grid gap-4 sm:grid-cols-2">
            <Field data-invalid={errors.name ? true : undefined}>
              <FieldLabel htmlFor="profile-name">Name</FieldLabel>
              <Input
                id="profile-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Your name"
              />
              <FieldError>{errors.name}</FieldError>
            </Field>

            <Field data-invalid={errors.email ? true : undefined}>
              <FieldLabel htmlFor="profile-email">Email</FieldLabel>
              <Input
                id="profile-email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="you@example.com"
              />
              <FieldError>{errors.email}</FieldError>
            </Field>
          </div>
        </CardContent>
        <CardFooter className="justify-end">
          <Button type="submit" disabled={updateProfile.isPending}>
            {updateProfile.isPending ? "Saving..." : "Save Changes"}
          </Button>
        </CardFooter>
      </Card>
    </form>
  );
}

function ChangePasswordCard() {
  const { user } = useAuth();
  const updateProfile = useUpdateProfile();

  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [status, setStatus] = useState<FormStatus>({ kind: "idle" });

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!user) return;
    setStatus({ kind: "idle" });

    if (!newPassword) {
      setStatus({
        kind: "error",
        errors: { new_password: "New password is required." },
      });
      return;
    }

    if (newPassword !== confirmPassword) {
      setStatus({
        kind: "error",
        errors: { confirm_password: "Passwords do not match." },
      });
      return;
    }

    try {
      await updateProfile.mutateAsync({
        new_password: newPassword,
        current_password: currentPassword,
      });
      setNewPassword("");
      setConfirmPassword("");
      setCurrentPassword("");
      setStatus({ kind: "idle" });
      toast.success("Password was updated successfully");
    } catch (err) {
      setStatus({
        kind: "error",
        errors: parseApiError(err, "Failed to update password."),
      });
    }
  }

  const errors = status.kind === "error" ? status.errors : {};

  if (!user) return null;

  return (
    <form onSubmit={handleSubmit}>
      <Card>
        <CardHeader>
          <CardTitle>Change Password</CardTitle>
          <CardDescription>
            Choose a new password. You'll need to enter your current password
            to confirm.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {errors._form && (
            <div className="mb-4">
              <FieldError>{errors._form}</FieldError>
            </div>
          )}

          <div className="grid gap-4 sm:grid-cols-2">
            <Field data-invalid={errors.new_password ? true : undefined}>
              <FieldLabel htmlFor="new-password">New Password</FieldLabel>
              <Input
                id="new-password"
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                placeholder="Min. 8 characters"
                autoComplete="new-password"
              />
              <FieldDescription>Minimum 8 characters.</FieldDescription>
              <FieldError>{errors.new_password}</FieldError>
            </Field>

            <Field data-invalid={errors.confirm_password ? true : undefined}>
              <FieldLabel htmlFor="confirm-password">
                Confirm New Password
              </FieldLabel>
              <Input
                id="confirm-password"
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                placeholder="Re-enter new password"
                autoComplete="new-password"
              />
              <FieldError>{errors.confirm_password}</FieldError>
            </Field>

            <Field
              className="sm:col-span-2"
              data-invalid={errors.current_password ? true : undefined}
            >
              <FieldLabel htmlFor="password-current">
                Current Password
              </FieldLabel>
              <Input
                id="password-current"
                type="password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                placeholder="Required to save changes"
                autoComplete="current-password"
              />
              <FieldError>{errors.current_password}</FieldError>
            </Field>
          </div>
        </CardContent>
        <CardFooter className="justify-end">
          <Button type="submit" disabled={updateProfile.isPending}>
            {updateProfile.isPending ? "Saving..." : "Update Password"}
          </Button>
        </CardFooter>
      </Card>
    </form>
  );
}
