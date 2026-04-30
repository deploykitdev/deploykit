import { type FormEvent, useState } from "react";
import { Button } from "@/components/ui/button";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";

interface AccountStepProps {
  onSubmit: (values: {
    name: string;
    email: string;
    password: string;
  }) => Promise<void>;
  onBack: () => void;
}

export function AccountStep({ onSubmit, onBack }: AccountStepProps) {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      await onSubmit({ name, email, password });
    } catch {
      setError("Could not create account. Please try again.");
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-6">
      <div className="flex flex-col items-center gap-2 text-center">
        <h1 className="text-2xl font-bold">Create your admin account</h1>
        <p className="text-balance text-sm text-muted-foreground">
          This will be the first user and will have full admin access.
        </p>
      </div>
      <div className="grid gap-4">
        <Field>
          <FieldLabel htmlFor="name">Name</FieldLabel>
          <Input
            id="name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            className="bg-background dark:bg-background"
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="email">Email</FieldLabel>
          <Input
            id="email"
            type="email"
            placeholder="you@example.com"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            className="bg-background dark:bg-background"
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="password">Password</FieldLabel>
          <Input
            id="password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            className="bg-background dark:bg-background"
          />
          {error && <FieldError>{error}</FieldError>}
        </Field>
        <div className="flex gap-2">
          <Button
            type="button"
            variant="outline"
            className="flex-1"
            onClick={onBack}
            disabled={submitting}
          >
            Back
          </Button>
          <Button type="submit" className="flex-1" disabled={submitting}>
            {submitting ? "Creating..." : "Create account"}
          </Button>
        </div>
      </div>
    </form>
  );
}
