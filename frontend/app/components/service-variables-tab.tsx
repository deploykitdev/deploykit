import { KeyIcon } from "lucide-react";

export function ServiceVariablesTab() {
  return <ComingSoon icon={<KeyIcon className="size-6" />} title="Variables" description="Environment variables will be editable here." />;
}

function ComingSoon({
  icon,
  title,
  description,
}: {
  icon: React.ReactNode;
  title: string;
  description: string;
}) {
  return (
    <div className="flex flex-col items-center gap-2 py-16 text-center">
      <div className="flex size-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
        {icon}
      </div>
      <p className="text-sm font-medium">{title} — coming soon</p>
      <p className="max-w-[280px] text-xs text-muted-foreground">
        {description}
      </p>
    </div>
  );
}

export { ComingSoon };
