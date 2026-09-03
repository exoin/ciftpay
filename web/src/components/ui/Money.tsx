import { moneyParts, MINUS, THIN_SPACE } from "@/lib/format";
import { cn } from "@/lib/cn";

type Size = "sm" | "md" | "lg" | "xl" | "2xl" | "3xl";

const sizeClass: Record<Size, string> = {
  sm: "text-sm",
  md: "text-base",
  lg: "text-lg",
  xl: "text-xl",
  "2xl": "text-2xl",
  "3xl": "text-3xl",
};

export type MoneyProps = {
  cents: number;
  currency?: "KES";
  size?: Size;
  /** Hide the currency label (e.g. in dense tables where the header says KES). */
  bare?: boolean;
  className?: string;
};

/**
 * `KES 12 400.00` in tabular mono, right-aligned. Negative amounts are kra-red
 * with a leading U+2212. Never renders floats — input is integer cents.
 */
export function Money({ cents, currency = "KES", size = "md", bare = false, className }: MoneyProps) {
  const p = moneyParts(cents);
  const label = `${p.negative ? MINUS : ""}${bare ? "" : `${currency}${THIN_SPACE}`}${p.whole}.${p.cents}`;
  return (
    <span
      className={cn("inline-flex items-baseline justify-end whitespace-nowrap font-mono tabular-nums text-right", sizeClass[size], p.negative && "text-red", className)}
      aria-label={label}
    >
      {p.negative && <span aria-hidden>{MINUS}</span>}
      {!bare && (
        <span aria-hidden className="kes mr-[0.35em] text-[0.8em]">
          {currency}
        </span>
      )}
      <span aria-hidden>{p.whole}</span>
      <span aria-hidden className="text-[0.85em] opacity-80">
        .{p.cents}
      </span>
    </span>
  );
}
