import type { ReactNode } from "react";
import { Link } from "react-router";
import { AppLogo } from "./app-logo";

export function SetupLayout({ children }: { children: ReactNode }) {
  return (
    <div className="bg-canvas-dots flex min-h-svh flex-col p-6 md:p-10">
      <div className="flex justify-center md:justify-start">
        <Link to="/">
          <AppLogo className="h-8 w-auto" />
        </Link>
      </div>
      <div className="flex flex-1 items-center justify-center">
        <div className="w-full max-w-md">{children}</div>
      </div>
    </div>
  );
}
