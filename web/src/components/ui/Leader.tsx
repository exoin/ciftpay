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
    <div className={cn("leader", strong && "leader--strong", className)}>
      <span className="leader-label">{label}</span>
      <span className="leader-dots" aria-hidden />
      <span className="leader-amount">{amount}</span>
    </div>
  );
}
