import type { ReactNode } from "react";
import { Leader } from "@/components/ui/Leader";
import { Money } from "@/components/ui/Money";
import { Stamp } from "@/components/ui/Stamp";
import { formatDateTime, formatQty } from "@/lib/format";
import { cn } from "@/lib/cn";

export type ReceiptState = "verified" | "pending" | "cancelled";

export type ReceiptLine = {
  description: string;
  /** Decimal string, e.g. "1.000" (OpenAPI `Quantity`). */
  qty: string;
  unit_price_cents: number;
  tax_category: string;
  line_total_cents: number;
};

export type ReceiptCardProps = {
  state: ReceiptState;
  kind?: "INVOICE" | "CREDIT_NOTE";
  sellerName: string;
  sellerPin: string;
  kraInvoiceNo?: string | null;
  receiptCode: string;
  issuedAt: string;
  buyerPinMasked?: string | null;
  lines: ReceiptLine[];
  subtotalCents: number;
  taxCents: number;
  totalCents: number;
  vatByCategory?: Array<{ category: string; taxable_cents: number; tax_cents: number }>;
  creditNoteOf?: string | null;
  /** Pre-rendered QR (SVG markup string) — server-rendered so the page ships zero JS. */
  qrSvg?: string | null;
  /** Apply the print-reveal animation (first ack while on screen). */
  reveal?: boolean;
  labels: {
    taxInvoice: string;
    creditNote: string;
    pin: string;
    invoiceNo: string;
    date: string;
    buyerPin: string;
    subtotal: string;
    vat: string;
    total: string;
    stamp: string;
    cancels?: string;
    poweredBy: string;
  };
  footer?: ReactNode;
  className?: string;
};

const stampTone: Record<ReceiptState, "ok" | "pending" | "failed"> = { verified: "ok", pending: "pending", cancelled: "failed" };

/**
 * The one receipt. Identical in-app, on /r/<code>, and in the shared PDF.
 * Paper-2 surface, perforated top and bottom, mono header block, Leader rows,
 * QR bottom-left, Stamp top-right. Max width 360 px; never stretches.
 *
 * Styled by the semantic `.rc*` classes in styles/receipt.css. The public
 * document at /r/<code> renders the same markup as strings (receipt/html.ts)
 * so it can ship without React; keep the two in step.
 */
export function ReceiptCard(p: ReceiptCardProps) {
  const isCN = p.kind === "CREDIT_NOTE";
  return (
    <article className={cn("rc perforated-both", p.reveal && "print-reveal", p.className)} aria-label={`${isCN ? p.labels.creditNote : p.labels.taxInvoice} ${p.receiptCode}`}>
      <div className="rc-inner">
        <header className="rc-header">
          <div className="rc-head">
            <div className="rc-seller">{p.sellerName}</div>
            <div>
              {p.labels.pin} {p.sellerPin}
            </div>
            <div className="rc-kind">{isCN ? p.labels.creditNote : p.labels.taxInvoice}</div>
          </div>
          <div className="rc-stamp">
            <Stamp tone={stampTone[p.state]}>{p.labels.stamp}</Stamp>
          </div>
        </header>

        <dl className="rc-meta">
          <Row k={p.labels.invoiceNo} v={p.kraInvoiceNo ?? "—"} />
          <Row k={p.labels.date} v={formatDateTime(p.issuedAt)} />
          {p.buyerPinMasked && <Row k={p.labels.buyerPin} v={p.buyerPinMasked} />}
          {isCN && p.creditNoteOf && p.labels.cancels && <Row k={p.labels.cancels} v={p.creditNoteOf} />}
        </dl>

        <hr className="rc-rule" />

        <ul className="rc-lines">
          {p.lines.map((l, i) => (
            <li key={i}>
              <Leader
                label={
                  <span>
                    {l.description}
                    <span className="rc-qty">
                      {" "}
                      ×{formatQty(l.qty)} <span className="rc-cat">[{l.tax_category}]</span>
                    </span>
                  </span>
                }
                amount={<Money cents={l.line_total_cents} bare size="sm" />}
              />
            </li>
          ))}
        </ul>

        <hr className="rc-rule" />

        <div className="rc-totals">
          <Leader label={p.labels.subtotal} amount={<Money cents={p.subtotalCents} bare size="sm" />} />
          {p.vatByCategory && p.vatByCategory.length > 0 ? (
            p.vatByCategory.map((v) => (
              <Leader key={v.category} label={`${p.labels.vat} ${v.category}`} amount={<Money cents={v.tax_cents} bare size="sm" />} />
            ))
          ) : (
            <Leader label={p.labels.vat} amount={<Money cents={p.taxCents} bare size="sm" />} />
          )}
          <Leader strong className="rc-total" label={p.labels.total} amount={<Money cents={p.totalCents} size="xl" />} />
        </div>

        <footer className="rc-foot">
          <div className="rc-qr" aria-hidden dangerouslySetInnerHTML={p.qrSvg ? { __html: p.qrSvg } : undefined} />
          <div className="rc-code">
            <div>{p.receiptCode}</div>
            <div>{p.labels.poweredBy}</div>
          </div>
        </footer>
        {p.footer}
      </div>
    </article>
  );
}

function Row({ k, v }: { k: string; v: string }) {
  return (
    <div className="rc-meta-row">
      <dt>{k}</dt>
      <dd>{v}</dd>
    </div>
  );
}
