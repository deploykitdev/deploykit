import { ActivityIcon } from "lucide-react";
import { ComingSoon } from "./service-variables-tab";

export function ServiceMetricsTab() {
  return (
    <ComingSoon
      icon={<ActivityIcon className="size-6" />}
      title="Metrics"
      description="CPU, memory, and network metrics will appear here."
    />
  );
}
