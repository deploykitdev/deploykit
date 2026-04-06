import { FormEvent, useState } from "react";
import { useAuth } from "../../lib/auth";
import { ApiError } from "../../lib/api";
import {
  useUsers,
  useCreateUser,
  useUpdateUser,
  useDeleteUser,
  type User,
} from "../../lib/queries";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, FieldLabel, FieldError } from "@/components/ui/field";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardAction,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Plus, Pencil, Trash2, X } from "lucide-react";

export default function SettingsUsers() {
  const { user: currentUser } = useAuth();
  const { data: users = [] } = useUsers();
  const [showAddForm, setShowAddForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [deletingUser, setDeletingUser] = useState<User | null>(null);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Users</CardTitle>
        <CardDescription>
          Manage who has access to this DeployKit instance.
        </CardDescription>
        <CardAction>
          {!showAddForm && (
            <Button size="sm" onClick={() => setShowAddForm(true)}>
              <Plus className="mr-1.5 size-3.5" />
              Add User
            </Button>
          )}
        </CardAction>
      </CardHeader>
      <CardContent>
        {showAddForm && (
          <AddUserForm
            onCreated={() => setShowAddForm(false)}
            onCancel={() => setShowAddForm(false)}
          />
        )}

        {users.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">
            No users found.
          </p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Email</TableHead>
                <TableHead>Role</TableHead>
                <TableHead className="w-[100px]" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {users.map((u) =>
                editingId === u.id ? (
                  <EditUserRow
                    key={u.id}
                    user={u}
                    onDone={() => setEditingId(null)}
                  />
                ) : (
                  <TableRow key={u.id}>
                    <TableCell className="font-medium">{u.name}</TableCell>
                    <TableCell className="text-muted-foreground">
                      {u.email}
                    </TableCell>
                    <TableCell>
                      <RoleBadge role={u.role} />
                    </TableCell>
                    <TableCell className="text-right">
                      {u.id !== currentUser?.id && (
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="icon-xs"
                            onClick={() => setEditingId(u.id)}
                            title="Edit user"
                          >
                            <Pencil className="size-3.5" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon-xs"
                            onClick={() => setDeletingUser(u)}
                            title="Delete user"
                          >
                            <Trash2 className="size-3.5 text-destructive" />
                          </Button>
                        </div>
                      )}
                      {u.id === currentUser?.id && (
                        <span className="text-xs text-muted-foreground">
                          You
                        </span>
                      )}
                    </TableCell>
                  </TableRow>
                ),
              )}
            </TableBody>
          </Table>
        )}
      </CardContent>

      <DeleteUserDialog
        user={deletingUser}
        onClose={() => setDeletingUser(null)}
      />
    </Card>
  );
}

function RoleBadge({ role }: { role: string }) {
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${
        role === "admin"
          ? "bg-primary/10 text-primary"
          : "bg-muted text-muted-foreground"
      }`}
    >
      {role}
    </span>
  );
}

function AddUserForm({
  onCreated,
  onCancel,
}: {
  onCreated: () => void;
  onCancel: () => void;
}) {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("member");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const createUser = useCreateUser();

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setErrors({});

    try {
      await createUser.mutateAsync({ name, email, password, role });
      onCreated();
    } catch (err) {
      if (err instanceof ApiError) {
        try {
          const body = JSON.parse(err.message);
          if (body.errors) {
            const fieldErrors: Record<string, string> = {};
            for (const [field, msgs] of Object.entries(body.errors)) {
              fieldErrors[field] = (msgs as string[])[0];
            }
            setErrors(fieldErrors);
          } else {
            setErrors({ _form: body.message || "Failed to create user." });
          }
        } catch {
          setErrors({ _form: "Failed to create user." });
        }
      } else {
        setErrors({ _form: "Failed to create user." });
      }
    }
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="mb-6 rounded-lg border border-border bg-muted/30 p-4"
    >
      <div className="mb-4 flex items-center justify-between">
        <h3 className="text-sm font-medium">New User</h3>
        <Button
          variant="ghost"
          size="icon-xs"
          type="button"
          onClick={onCancel}
        >
          <X className="size-3.5" />
        </Button>
      </div>
      {errors._form && (
        <div className="mb-4">
          <FieldError>{errors._form}</FieldError>
        </div>
      )}
      <div className="grid gap-4 sm:grid-cols-2">
        <Field data-invalid={errors.name ? true : undefined}>
          <FieldLabel htmlFor="new-user-name">Name</FieldLabel>
          <Input
            id="new-user-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Jane Doe"
          />
          <FieldError>{errors.name}</FieldError>
        </Field>
        <Field data-invalid={errors.email ? true : undefined}>
          <FieldLabel htmlFor="new-user-email">Email</FieldLabel>
          <Input
            id="new-user-email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="jane@example.com"
          />
          <FieldError>{errors.email}</FieldError>
        </Field>
        <Field data-invalid={errors.password ? true : undefined}>
          <FieldLabel htmlFor="new-user-password">Password</FieldLabel>
          <Input
            id="new-user-password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="Min. 8 characters"
          />
          <FieldError>{errors.password}</FieldError>
        </Field>
        <Field data-invalid={errors.role ? true : undefined}>
          <FieldLabel htmlFor="new-user-role">Role</FieldLabel>
          <select
            id="new-user-role"
            value={role}
            onChange={(e) => setRole(e.target.value)}
            className="h-9 w-full rounded-md border border-input bg-transparent px-2.5 py-1 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
          >
            <option value="member">Member</option>
            <option value="admin">Admin</option>
          </select>
          <FieldError>{errors.role}</FieldError>
        </Field>
      </div>
      <div className="mt-4 flex justify-end gap-2">
        <Button variant="outline" size="sm" type="button" onClick={onCancel}>
          Cancel
        </Button>
        <Button size="sm" type="submit" disabled={createUser.isPending}>
          {createUser.isPending ? "Creating..." : "Create User"}
        </Button>
      </div>
    </form>
  );
}

function EditUserRow({ user, onDone }: { user: User; onDone: () => void }) {
  const [name, setName] = useState(user.name);
  const [email, setEmail] = useState(user.email);
  const [role, setRole] = useState(user.role);
  const [error, setError] = useState("");
  const updateUser = useUpdateUser();

  async function handleSave() {
    setError("");

    const changes: Record<string, string> = {};
    if (name !== user.name) changes.name = name;
    if (email !== user.email) changes.email = email;
    if (role !== user.role) changes.role = role;

    if (Object.keys(changes).length === 0) {
      onDone();
      return;
    }

    try {
      await updateUser.mutateAsync({ id: user.id, ...changes });
      onDone();
    } catch (err) {
      if (err instanceof ApiError) {
        try {
          const body = JSON.parse(err.message);
          setError(body.message || "Failed to update user.");
        } catch {
          setError("Failed to update user.");
        }
      } else {
        setError("Failed to update user.");
      }
    }
  }

  return (
    <TableRow>
      <TableCell>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="h-7 text-sm"
        />
      </TableCell>
      <TableCell>
        <Input
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="h-7 text-sm"
        />
      </TableCell>
      <TableCell>
        <select
          value={role}
          onChange={(e) => setRole(e.target.value)}
          className="h-7 rounded-md border border-input bg-transparent px-2 text-xs shadow-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
        >
          <option value="member">member</option>
          <option value="admin">admin</option>
        </select>
      </TableCell>
      <TableCell className="text-right">
        <div className="flex flex-col items-end gap-1">
          <div className="flex items-center gap-1">
            <Button
              variant="outline"
              size="xs"
              onClick={onDone}
              type="button"
            >
              Cancel
            </Button>
            <Button
              size="xs"
              onClick={handleSave}
              disabled={updateUser.isPending}
              type="button"
            >
              {updateUser.isPending ? "..." : "Save"}
            </Button>
          </div>
          {error && (
            <span className="text-xs text-destructive">{error}</span>
          )}
        </div>
      </TableCell>
    </TableRow>
  );
}

function DeleteUserDialog({
  user,
  onClose,
}: {
  user: User | null;
  onClose: () => void;
}) {
  const deleteUser = useDeleteUser();
  const [error, setError] = useState("");

  async function handleDelete() {
    if (!user) return;
    setError("");

    try {
      await deleteUser.mutateAsync(user.id);
      onClose();
    } catch (err) {
      if (err instanceof ApiError) {
        try {
          const body = JSON.parse(err.message);
          setError(body.message || "Failed to delete user.");
        } catch {
          setError("Failed to delete user.");
        }
      } else {
        setError("Failed to delete user.");
      }
    }
  }

  return (
    <Dialog open={!!user} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete User</DialogTitle>
          <DialogDescription>
            Are you sure you want to delete <strong>{user?.name}</strong> (
            {user?.email})? This action cannot be undone.
          </DialogDescription>
        </DialogHeader>
        {error && <FieldError>{error}</FieldError>}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={handleDelete}
            disabled={deleteUser.isPending}
          >
            {deleteUser.isPending ? "Deleting..." : "Delete User"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
