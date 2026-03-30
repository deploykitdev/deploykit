import { useEffect, useState } from "react";
import { Link } from "react-router";
import { RequireAuth, useAuth } from "../lib/auth";
import { api } from "../lib/api";

interface Project {
  id: string;
  name: string;
  slug: string;
  created_at: string;
}

export default function Dashboard() {
  return (
    <RequireAuth>
      <DashboardContent />
    </RequireAuth>
  );
}

function DashboardContent() {
  const { user, logout } = useAuth();
  const [projects, setProjects] = useState<Project[]>([]);

  useEffect(() => {
    api<{ projects: Project[] }>("/projects").then((data) =>
      setProjects(data.projects ?? []),
    );
  }, []);

  return (
    <div>
      <header>
        <h1>DeployKit</h1>
        <nav>
          <span>
            {user?.name} ({user?.email})
          </span>
          <button onClick={logout}>Logout</button>
        </nav>
      </header>
      <main>
        <h2>Projects</h2>
        {projects.length === 0 ? (
          <p>No projects yet.</p>
        ) : (
          <ul>
            {projects.map((p) => (
              <li key={p.id}>
                <Link to={`/projects`}>{p.name}</Link>
              </li>
            ))}
          </ul>
        )}
      </main>
    </div>
  );
}
