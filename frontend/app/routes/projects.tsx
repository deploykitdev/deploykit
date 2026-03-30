import { useEffect, useState } from "react";
import { Link } from "react-router";
import { RequireAuth } from "../lib/auth";
import { api } from "../lib/api";

interface Project {
  id: string;
  name: string;
  slug: string;
  created_at: string;
}

export default function Projects() {
  return (
    <RequireAuth>
      <ProjectsList />
    </RequireAuth>
  );
}

function ProjectsList() {
  const [projects, setProjects] = useState<Project[]>([]);

  useEffect(() => {
    api<{ projects: Project[] }>("/projects").then((data) =>
      setProjects(data.projects ?? []),
    );
  }, []);

  return (
    <div>
      <header>
        <h1>
          <Link to="/">DeployKit</Link>
        </h1>
      </header>
      <main>
        <h2>Projects</h2>
        {projects.length === 0 ? (
          <p>No projects yet.</p>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Slug</th>
                <th>Created</th>
              </tr>
            </thead>
            <tbody>
              {projects.map((p) => (
                <tr key={p.id}>
                  <td>{p.name}</td>
                  <td>{p.slug}</td>
                  <td>{new Date(p.created_at).toLocaleDateString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </main>
    </div>
  );
}
