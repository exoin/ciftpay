"use client";

import type { ReactNode } from "react";
import { useActiveMembership } from "@/lib/auth";

/** Page title in Fraunces, org name in mono, optional right action. Sticky. */
export function TopBar({ title, action }: { title: ReactNode; action?: ReactNode }) {
  const m = useActiveMembership();
  return (
    <header className="sticky top-0 z-30 -mx-4 mb-4 flex min-h-14 items-center justify-between gap-4 border-b border-hairline bg-paper/95 px-4 backdrop-blur-none lg:mx-0 lg:px-0 lg:border-0">
      <div className="min-w-0">
        <h1 className="truncate">{title}</h1>
        {m && <div className="truncate font-mono text-xs text-muted">{m.name}</div>}
      </div>
      {action && <div className="shrink-0">{action}</div>}
    </header>
  );
}
