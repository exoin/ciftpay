import type { ReactNode } from "react";
import { cn } from "@/lib/cn";

export type StampTone = "ok" | "pending" | "failed";

const tones: Record<StampTone, string> = {
  ok: "text-ok",
  pending: "text-ochre",
  failed: "text-red",
};

/** Rotated outline label. Only on receipt detail and the public receipt page. */
export function Stamp({ tone, children, className }: { tone: StampTone; children: ReactNode; className?: string }) {
  return (
    <span className={cn("stamp", tones[tone], className)} role="status">
      {children}
    </span>
  );
}
