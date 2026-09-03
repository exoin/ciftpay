import type { ReactNode } from "react";

/** One sentence of copy (design-system §7) and at most one action. No illustration. */
export function EmptyState({ children, action }: { children: ReactNode; action?: ReactNode }) {
  return (
    <div className="border border-dashed border-hairline rounded-r3 px-4 py-6 text-ink-2">
      <p className="max-w-prose">{children}</p>
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}
