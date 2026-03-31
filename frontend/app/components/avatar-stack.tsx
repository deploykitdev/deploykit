import type { ConnectedUser } from "@/lib/use-canvas-sync";
import { getUserColor } from "@/lib/user-colors";
import { Avatar } from "@/components/ui/avatar";

interface AvatarStackProps {
  users: ConnectedUser[];
  maxVisible?: number;
}

export function AvatarStack({
  users,
  maxVisible = 3,
}: AvatarStackProps) {
  if (users.length === 0) return null;

  const visible = users.slice(0, maxVisible);
  const remaining = users.length - visible.length;

  return (
    <div className="flex -space-x-2">
      {visible.map((user) => (
        <Avatar
          key={user.user_id}
          name={user.user_name}
          color={getUserColor(user.user_id)}
        />
      ))}
      {remaining > 0 && (
        <div
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-medium text-muted-foreground ring-2 ring-background"
          title={`${remaining} more`}
        >
          +{remaining}
        </div>
      )}
    </div>
  );
}
