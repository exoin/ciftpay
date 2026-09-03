import type { ReactNode } from "react";
import { cn } from "@/lib/cn";

export type ChipTone = "acked" | "pending" | "failed" | "unmatched" | "cash" | "neutral";

const tones: Record<ChipTone, string> = {
  acked: "text-ok bg-ok-bg",
  pending: "text-ochre bg-warn-bg",
  failed: "text-red bg-bad-bg",
  unmatched: "text-ink bg-paper-3",
  cash: "text-ink bg-paper-3",
  neutral: "text-ink-2 bg-paper-3",
};

export type StatusChipProps = { tone: ChipTone; children: ReactNode; className?: string };

/** 3 px radius, mono, dot before label. Colour is never the only signal — the label is required. */
export function StatusChip({ tone, children, className }: StatusChipProps) {
  return (
    <span className={cn("inline-flex items-center gap-1.5 rounded-r1 px-2 py-0.5 font-mono text-sm leading-[1.45] whitespace-nowrap", tones[tone], className)}>
      <span aria-hidden className="inline-block size-1.5 rounded-full bg-current" />
      {children}
    </span>
  );
}
