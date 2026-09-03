"use client";

import { cn } from "@/lib/cn";

export type TabItem<T extends string> = { value: T; label: string; count?: number };

export type TabsProps<T extends string> = {
  items: TabItem<T>[];
  value: T;
  onChange: (v: T) => void;
  ariaLabel: string;
};

/** Underline tabs: 2 px ink underline, no pill background. */
export function Tabs<T extends string>({ items, value, onChange, ariaLabel }: TabsProps<T>) {
  return (
    <div role="tablist" aria-label={ariaLabel} className="flex gap-1 overflow-x-auto border-b border-hairline -mx-4 px-4 md:mx-0 md:px-0">
      {items.map((it) => {
        const active = it.value === value;
        return (
          <button
            key={it.value}
            role="tab"
            type="button"
            aria-selected={active}
            onClick={() => onChange(it.value)}
            className={cn(
              "relative min-h-[var(--touch)] shrink-0 px-3 text-sm font-medium transition-colors duration-[var(--dur-1)]",
              active ? "text-ink" : "text-muted hover:text-ink-2",
            )}
          >
            {it.label}
            {typeof it.count === "number" && <span className="ml-1.5 font-mono text-xs text-muted">{it.count}</span>}
            {active && <span aria-hidden className="absolute inset-x-3 -bottom-px h-0.5 bg-ink" />}
          </button>
        );
      })}
    </div>
  );
}
