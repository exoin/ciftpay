import type { ReactNode } from "react";
import { cn } from "@/lib/cn";

export type LeaderProps = {
  label: ReactNode;
  amount: ReactNode;
  strong?: boolean;
  className?: string;
};

/** `label · · · · · amount` — the ledger row. Amount should usually be <Money />. */
export function Leader({ label, amount, strong = false, className }: LeaderProps) {
  return (
    <div className={cn("leader", strong && "font-medium", className)}>
      <span className="shrink-0 max-w-[65%] truncate">{label}</span>
      <span className="leader-dots" aria-hidden />
      <span className="shrink-0 font-mono">{amount}</span>
    </div>
  );
}
