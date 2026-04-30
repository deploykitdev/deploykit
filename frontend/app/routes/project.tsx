import { useParams } from "react-router";
import { RequireAuth } from "../lib/auth";
import { useProject } from "../lib/queries";
import { DashboardLayout } from "@/components/dashboard-layout";
import { ProjectFlow } from "@/components/project-flow";

function ProjectDetail() {
  const { id } = useParams();
  const { data: project, isLoading, isError } = useProject(id!);

  if (!isLoading && (isError || !project)) {
    return (
      <DashboardLayout>
        <p>Project not found</p>
      </DashboardLayout>
    );
  }

  return (
    <DashboardLayout fluid>
      <ProjectFlow projectId={id!} />
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
