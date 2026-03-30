import { type FormEvent, useEffect, useState } from "react";
import { Link, useNavigate } from "react-router";
import { api } from "../lib/api";
import { useAuth } from "../lib/auth";

export default function Register() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const [canRegister, setCanRegister] = useState<boolean | null>(null);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");

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
      navigate("/", { replace: true });
    } catch {
      setError("Registration failed.");
    }
  }

  if (canRegister === null) return <p>Loading...</p>;

  if (!canRegister) {
    return (
      <div>
        <h1>DeployKit</h1>
        <p>Registration is closed. An account already exists.</p>
        <Link to="/login">Login</Link>
      </div>
    );
  }

  return (
    <div>
      <h1>DeployKit</h1>
      <h2>Create Account</h2>
      <form onSubmit={handleSubmit}>
        <div>
          <label htmlFor="name">Name</label>
          <input
            id="name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
          />
        </div>
        <div>
          <label htmlFor="email">Email</label>
          <input
            id="email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
          />
        </div>
        <div>
          <label htmlFor="password">Password</label>
          <input
            id="password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />
        </div>
        {error && <p>{error}</p>}
        <button type="submit">Create Account</button>
      </form>
      <p>
        <Link to="/login">Already have an account? Login</Link>
      </p>
    </div>
  );
}
