/**
 * Formatting helpers. Money is always integer cents; never floats.
 * docs/design-system.md §1 rule 9: `KES 12 400.00` — thin-space (U+2009)
 * thousands separator, two decimals, U+2212 minus for negatives.
 */

export const THIN_SPACE = "\u2009";
export const MINUS = "\u2212";

export type MoneyParts = {
  negative: boolean;
  whole: string; // "12 400" with thin spaces
  cents: string; // "00"
};

export function moneyParts(cents: number): MoneyParts {
  if (!Number.isFinite(cents)) {
    throw new TypeError(`moneyParts: not a finite number: ${cents}`);
  }
  const n = Math.trunc(cents);
  const negative = n < 0;
  const abs = Math.abs(n);
  const whole = Math.floor(abs / 100).toString();
  const frac = (abs % 100).toString().padStart(2, "0");
  return { negative, whole: groupThousands(whole), cents: frac };
}

export function groupThousands(digits: string, sep: string = THIN_SPACE): string {
  let out = "";
  for (let i = 0; i < digits.length; i++) {
    const fromEnd = digits.length - i;
    if (i > 0 && fromEnd % 3 === 0) out += sep;
    out += digits[i];
  }
  return out;
}

/** `formatKES(1240000)` → `KES 12 400.00`; `formatKES(-500)` → `−KES 5.00`. */
export function formatKES(cents: number, opts: { symbol?: boolean; decimals?: boolean } = {}): string {
  const { symbol = true, decimals = true } = opts;
  const p = moneyParts(cents);
  let s = p.whole;
  if (decimals) s += `.${p.cents}`;
  if (symbol) s = `KES${THIN_SPACE}${s}`;
  return p.negative ? `${MINUS}${s}` : s;
}

/** Masks a Kenyan MSISDN to first 4 and last 3 digits: 2547•••••345. */
export function maskMsisdn(msisdn: string): string {
  const d = msisdn.replace(/\D/g, "");
  if (d.length < 8) return d.replace(/./g, "•");
  return `${d.slice(0, 4)}${"•".repeat(d.length - 7)}${d.slice(-3)}`;
}

/** Normalises local input (07xx, +2547xx, 2547xx) to E.164 without plus. */
export function normaliseMsisdn(input: string): string | null {
  const d = input.replace(/[^\d]/g, "");
  let n = d;
  if (n.startsWith("0") && n.length === 10) n = `254${n.slice(1)}`;
  else if (n.length === 9 && /^[17]/.test(n)) n = `254${n}`;
  return /^254[17]\d{8}$/.test(n) ? n : null;
}

export function formatDateTime(iso: string, locale: string = "en-KE"): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return new Intl.DateTimeFormat(locale, {
    timeZone: "Africa/Nairobi",
    day: "2-digit",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(d);
}

export function formatTime(iso: string, locale: string = "en-KE"): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return new Intl.DateTimeFormat(locale, { timeZone: "Africa/Nairobi", hour: "2-digit", minute: "2-digit", hour12: false }).format(d);
}

export function formatQty(qty: number): string {
  return Number.isInteger(qty) ? qty.toString() : qty.toFixed(3).replace(/\.?0+$/, "");
}
