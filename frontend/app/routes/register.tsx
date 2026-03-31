import { type FormEvent, useEffect, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { api } from "../lib/api";
import { useAuth } from "../lib/auth";
import { AuthLayout } from "@/components/auth-layout";
import { Button, buttonVariants } from "@/components/ui/button";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";

export default function Register() {
  const { user, loading, login } = useAuth();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const redirectTo = searchParams.get("redirect") || "/";
  const [canRegister, setCanRegister] = useState<boolean | null>(null);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!loading && user) {
      navigate(redirectTo, { replace: true });
    }
  }, [user, loading, navigate, redirectTo]);

  useEffect(() => {
    api<{ can_register: boolean }>("/auth/register")
      .then((data) => setCanRegister(data.can_register))
      .catch(() => setCanRegister(false));
  }, []);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    try {
      await api("/auth/register", {
        method: "POST",
        body: JSON.stringify({ name, email, password }),
      });
      await login(email, password);
      navigate(redirectTo, { replace: true });
    } catch {
      setError("Registration failed.");
    }
  }

  if (loading || user) return null;

  if (canRegister === null) {
    return (
      <AuthLayout>
        <p className="text-center text-muted-foreground">Loading...</p>
      </AuthLayout>
    );
  }

  if (!canRegister) {
    return (
      <AuthLayout>
        <div className="flex flex-col items-center gap-6 text-center">
          <div className="flex flex-col gap-2">
            <h1 className="text-2xl font-bold">Registration closed</h1>
            <p className="text-balance text-sm text-muted-foreground">
              An account already exists. Registration is not available.
            </p>
          </div>
          <Link to="/login" className={buttonVariants({ className: "w-full" })}>
            Login
          </Link>
        </div>
      </AuthLayout>
    );
  }

  return (
    <AuthLayout>
      <form onSubmit={handleSubmit} className="flex flex-col gap-6">
        <div className="flex flex-col items-center gap-2 text-center">
          <h1 className="text-2xl font-bold">Create your account</h1>
          <p className="text-balance text-sm text-muted-foreground">
            Enter your details below to get started
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
            />
            {error && <FieldError>{error}</FieldError>}
          </Field>
          <Button type="submit" className="w-full">
            Create Account
          </Button>
        </div>
        <div className="text-center text-sm">
          Already have an account?{" "}
          <Link to="/login" className="underline underline-offset-4">
            Login
          </Link>
        </div>
      </form>
    </AuthLayout>
  );
}
