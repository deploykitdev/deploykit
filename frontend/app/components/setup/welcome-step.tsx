import { BoxIcon, ServerIcon, WorkflowIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { WelcomeHero } from "./welcome-hero";

interface WelcomeStepProps {
  onNext: () => void;
}

const features = [
  {
    icon: BoxIcon,
    title: "Deploy any container",
    description: "Bring your Docker image. We'll handle the rest.",
  },
  {
    icon: WorkflowIcon,
    title: "Wire it up visually",
    description: "Connect services on a real-time canvas.",
  },
  {
    icon: ServerIcon,
    title: "Self-hosted, on your VM",
    description: "Your infrastructure, your data, no lock-in.",
  },
];

export function WelcomeStep({ onNext }: WelcomeStepProps) {
  return (
    <div className="flex flex-col gap-8">
      <WelcomeHero />

      <div className="flex flex-col items-center gap-2 text-center">
        <h1 className="font-heading text-4xl font-bold tracking-tight">
          Welcome aboard
        </h1>
        <p className="text-balance text-sm text-muted-foreground">
          Let&apos;s get you set up. It only takes a minute.
        </p>
      </div>

      <ul className="mt-4 flex flex-col gap-4">
        {features.map(({ icon: Icon, title, description }, i) => (
          <li
            key={title}
            className="flex items-start gap-3 animate-in fade-in slide-in-from-bottom-1 duration-500 fill-mode-backwards"
            style={{ animationDelay: `${150 + i * 100}ms` }}
          >
            <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary ring-1 ring-primary/15">
              <Icon className="size-4" />
            </span>
            <div className="flex flex-col gap-0.5">
              <p className="text-sm font-medium leading-none">{title}</p>
              <p className="text-xs text-muted-foreground">{description}</p>
            </div>
          </li>
        ))}
      </ul>

      <Button type="button" className="mt-4 w-full" onClick={onNext}>
        Get started
      </Button>
    </div>
  );
}
