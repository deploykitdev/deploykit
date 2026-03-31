import { useParams } from "react-router";
import { RequireAuth } from "../lib/auth";
import { useProject } from "../lib/queries";
import { DashboardLayout } from "@/components/dashboard-layout";

function ProjectDetail() {
  const { id } = useParams();
  const { data: project, isLoading } = useProject(id!);

  if (isLoading) {
    return (
      <DashboardLayout>
        <p>Loading...</p>
      </DashboardLayout>
    );
  }

  if (!project) {
    return (
      <DashboardLayout>
        <p>Project not found</p>
      </DashboardLayout>
    );
  }

  return (
    <DashboardLayout>
      <h1 className="text-2xl font-bold">Project: {project.name}</h1>
    </DashboardLayout>
  );
}

export default function Project() {
  return (
    <RequireAuth>
      <ProjectDetail />
    </RequireAuth>
  );
}
