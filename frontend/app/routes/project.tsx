import { useParams } from "react-router";
import { RequireAuth } from "../lib/auth";
import { useProject } from "../lib/queries";
import { DashboardLayout } from "@/components/dashboard-layout";
import { ProjectFlow } from "@/components/project-flow";

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
    <DashboardLayout fluid>
      <ProjectFlow />
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
