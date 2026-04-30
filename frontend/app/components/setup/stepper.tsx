import { cn } from "@/lib/utils";

interface StepperProps {
  total: number;
  current: number;
}

export function Stepper({ total, current }: StepperProps) {
  if (total <= 1) return null;
  return (
    <div className="flex items-center justify-center gap-2">
      {Array.from({ length: total }).map((_, i) => (
        <span
          key={i}
          aria-hidden
          className={cn(
            "h-1.5 w-1.5 rounded-full transition-colors",
            i === current
              ? "bg-primary"
              : i < current
                ? "bg-primary/60"
                : "bg-muted-foreground/30",
          )}
        />
      ))}
    </div>
  );
}
