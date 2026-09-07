import { moneyParts, MINUS, THIN_SPACE } from "@/lib/format";
import { cn } from "@/lib/cn";

type Size = "sm" | "md" | "lg" | "xl" | "2xl" | "3xl";

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
 * Styled by the semantic `.money` classes in styles/receipt.css, shared with
 * the zero-JS public receipt (components/receipt/html.ts).
 */
export function Money({ cents, currency = "KES", size = "md", bare = false, className }: MoneyProps) {
  const p = moneyParts(cents);
  const label = `${p.negative ? MINUS : ""}${bare ? "" : `${currency}${THIN_SPACE}`}${p.whole}.${p.cents}`;
  return (
    <span className={cn("money", `money--${size}`, p.negative && "money--neg", className)} aria-label={label}>
      {p.negative && <span aria-hidden>{MINUS}</span>}
      {!bare && (
        <span aria-hidden className="money-cur">
          {currency}
        </span>
      )}
      <span aria-hidden>{p.whole}</span>
      <span aria-hidden className="money-frac">
        .{p.cents}
      </span>
    </span>
  );
}
