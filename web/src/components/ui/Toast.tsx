"use client";

import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from "react";
import { cn } from "@/lib/cn";

type Toast = { id: number; message: string; tone: "info" | "error" };

const ToastCtx = createContext<{ push: (message: string, tone?: Toast["tone"]) => void } | null>(null);

/** Top, paper-2 with ink border, auto-dismiss 4 s, never more than 2 stacked. */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<Toast[]>([]);
  const seq = useRef(0);
  const push = useCallback((message: string, tone: Toast["tone"] = "info") => {
    const id = ++seq.current;
    setItems((cur) => [...cur.slice(-1), { id, message, tone }]);
    window.setTimeout(() => setItems((cur) => cur.filter((t) => t.id !== id)), 4000);
  }, []);
  const value = useMemo(() => ({ push }), [push]);
  return (
    <ToastCtx.Provider value={value}>
      {children}
      <div aria-live="polite" className="pointer-events-none fixed inset-x-0 top-2 z-50 flex flex-col items-center gap-2 px-4">
        {items.map((t) => (
          <div
            key={t.id}
            className={cn(
              "pointer-events-auto max-w-md rounded-r2 border bg-paper-2 px-4 py-3 text-sm [box-shadow:var(--shadow-sheet)]",
              t.tone === "error" ? "border-red text-red" : "border-ink text-ink",
            )}
          >
            {t.message}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}

export function useToast() {
  const ctx = useContext(ToastCtx);
  if (!ctx) throw new Error("useToast must be used inside <ToastProvider>");
  return ctx;
}
