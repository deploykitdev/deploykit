import { useParams } from "react-router";
import { RequireAuth } from "../lib/auth";
import { DashboardLayout } from "@/components/dashboard-layout";

function ProjectDetail() {
  const { id } = useParams();

  return (
    <DashboardLayout>
      <h1 className="text-2xl font-bold">Project: {id}</h1>
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
