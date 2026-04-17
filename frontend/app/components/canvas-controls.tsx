import { useReactFlow } from "@xyflow/react";
import { PlusIcon, MinusIcon, Minimize2Icon } from "lucide-react";

const btnClass =
  "flex size-9 items-center justify-center text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground";

const groupClass =
  "flex flex-col items-center rounded-lg bg-popover shadow-md ring-1 ring-foreground/10";

export function CanvasControls() {
  const { zoomIn, zoomOut, fitView } = useReactFlow();

  return (
    <div className="absolute bottom-4 left-4 z-10 flex flex-col gap-2">
      <div className={groupClass}>
        <button
          onClick={() => zoomIn()}
          className={`${btnClass} rounded-t-lg`}
          title="Zoom in"
        >
          <PlusIcon className="size-4" />
        </button>
        <button
          onClick={() => zoomOut()}
          className={btnClass}
          title="Zoom out"
        >
          <MinusIcon className="size-4" />
        </button>
        <button
          onClick={() => fitView({ padding: 0.2 })}
          className={`${btnClass} rounded-b-lg`}
          title="Fit to view"
        >
          <Minimize2Icon className="size-4" />
        </button>
      </div>
    </div>
  );
}
