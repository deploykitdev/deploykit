import { SettingsIcon } from "lucide-react";
import { ComingSoon } from "./service-variables-tab";

export function ServiceSettingsTab() {
  return (
    <ComingSoon
      icon={<SettingsIcon className="size-6" />}
      title="Settings"
      description="Rename, configure, and delete this service here."
    />
  );
}
