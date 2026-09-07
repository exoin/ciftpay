import type { Schemas } from "@/lib/api/client";
import { formatDateTime, formatKES, formatQty, MINUS, moneyParts, THIN_SPACE } from "@/lib/format";

/**
 * The public receipt (/r/<code>) as plain HTML strings. The document must work
 * with JavaScript disabled and stay under 30 KB (design-system §11, N6), so it
 * bypasses React entirely: no RSC payload, no hydration, no framework chunks.
 * Markup mirrors <ReceiptCard> and is styled by the same receipt.css classes.
 */

export type PublicReceipt = Schemas["PublicReceipt"];

/** Translator for the "receipt" namespace (next-intl `getTranslations`). */
export type ReceiptT = (key: string, values?: Record<string, string | number>) => string;

export type ReceiptDocument = {
  rc: PublicReceipt;
  /** Pre-rendered inline SVG for the KRA QR (see qr.ts), or null. */
  qrSvg: string | null;
  t: ReceiptT;
  locale: string;
  css: string;
};

export function escapeHtml(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#39;");
}

const e = escapeHtml;

const STAMP_TONE: Record<PublicReceipt["state"], "ok" | "pending" | "failed"> = { verified: "ok", pending: "pending", cancelled: "failed" };

function moneyHtml(cents: number, opts: { bare?: boolean; size?: "sm" | "md" | "xl" } = {}): string {
  const { bare = false, size = "md" } = opts;
  const p = moneyParts(cents);
  const label = `${p.negative ? MINUS : ""}${bare ? "" : `KES${THIN_SPACE}`}${p.whole}.${p.cents}`;
  return (
    `<span class="money money--${size}${p.negative ? " money--neg" : ""}" aria-label="${e(label)}">` +
    (p.negative ? `<span aria-hidden="true">${MINUS}</span>` : "") +
    (bare ? "" : `<span class="money-cur" aria-hidden="true">KES</span>`) +
    `<span aria-hidden="true">${p.whole}</span><span class="money-frac" aria-hidden="true">.${p.cents}</span></span>`
  );
}

function leaderHtml(label: string, amount: string, opts: { strong?: boolean; className?: string } = {}): string {
  const cls = ["leader", opts.strong && "leader--strong", opts.className].filter(Boolean).join(" ");
  return `<div class="${cls}"><span class="leader-label">${label}</span><span class="leader-dots" aria-hidden="true"></span><span class="leader-amount">${amount}</span></div>`;
}

function metaRow(k: string, v: string): string {
  return `<div class="rc-meta-row"><dt>${e(k)}</dt><dd>${e(v)}</dd></div>`;
}

/** The receipt card itself: <article class="rc">…</article>. */
export function receiptCardHtml({ rc, qrSvg, t, locale }: Omit<ReceiptDocument, "css">): string {
  const isCN = rc.kind === "CREDIT_NOTE";
  const kind = isCN ? t("creditNote") : t("taxInvoice");
  const stamp = rc.state === "verified" ? t("verified") : rc.state === "cancelled" ? t("cancelled") : t("pending");
  const intl = locale === "sw" ? "sw-KE" : "en-KE";

  const meta = [
    metaRow(t("invoiceNo"), rc.kra_invoice_no ?? "—"),
    metaRow(t("date"), formatDateTime(rc.issued_at, intl)),
    rc.buyer_pin_masked ? metaRow(t("buyerPin"), rc.buyer_pin_masked) : "",
    isCN && rc.credit_note_of ? metaRow(t("cancels", { kraNo: "" }).trim(), rc.credit_note_of) : "",
  ].join("");

  const lines = rc.lines
    .map((l) => {
      const label = `<span>${e(l.description)}<span class="rc-qty"> ×${formatQty(l.qty)} <span class="rc-cat">[${e(l.tax_category)}]</span></span></span>`;
      return `<li>${leaderHtml(label, moneyHtml(l.line_total_cents, { bare: true, size: "sm" }))}</li>`;
    })
    .join("");

  const vat =
    rc.vat_by_category && rc.vat_by_category.length > 0
      ? rc.vat_by_category.map((v) => leaderHtml(e(t("vatCategory", { category: v.category })), moneyHtml(v.tax_cents, { bare: true, size: "sm" }))).join("")
      : leaderHtml(e(t("vat")), moneyHtml(rc.tax_cents, { bare: true, size: "sm" }));

  return (
    `<article class="rc perforated-both" aria-label="${e(`${kind} ${rc.receipt_code}`)}"><div class="rc-inner">` +
    `<header class="rc-header"><div class="rc-head"><div class="rc-seller">${e(rc.seller.name)}</div><div>${e(t("pin"))} ${e(rc.seller.kra_pin)}</div><div class="rc-kind">${e(kind)}</div></div>` +
    `<div class="rc-stamp"><span class="stamp stamp--${STAMP_TONE[rc.state]}" role="status">${e(stamp)}</span></div></header>` +
    `<dl class="rc-meta">${meta}</dl>` +
    `<hr class="rc-rule">` +
    `<ul class="rc-lines">${lines}</ul>` +
    `<hr class="rc-rule">` +
    `<div class="rc-totals">${leaderHtml(e(t("subtotal")), moneyHtml(rc.subtotal_cents, { bare: true, size: "sm" }))}${vat}` +
    leaderHtml(e(t("total")), moneyHtml(rc.total_cents, { size: "xl" }), { strong: true, className: "rc-total" }) +
    `</div>` +
    `<footer class="rc-foot"><div class="rc-qr" aria-hidden="true">${qrSvg ?? ""}</div>` +
    `<div class="rc-code"><div>${e(rc.receipt_code)}</div><div>${e(t("poweredBy"))}</div></div></footer>` +
    `</div></article>`
  );
}

function head(title: string, description: string, locale: string, css: string): string {
  return (
    `<!doctype html><html lang="${e(locale)}"><head><meta charset="utf-8">` +
    `<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">` +
    `<meta name="robots" content="noindex, nofollow"><meta name="theme-color" content="#F6F1E7">` +
    `<title>${e(title)} · CiftPay</title><meta name="description" content="${e(description)}">` +
    `<meta property="og:title" content="${e(title)}"><meta property="og:type" content="article">` +
    `<link rel="icon" href="/icons/icon.svg" type="image/svg+xml">` +
    `<link rel="preload" href="/fonts/ibm-plex-mono-latin-400-normal.woff2" as="font" type="font/woff2" crossorigin>` +
    `<style>${css}</style></head><body>`
  );
}

/** Full 200 document. */
export function receiptDocumentHtml(doc: ReceiptDocument): string {
  const { rc, t, locale, css } = doc;
  const title = t("title", { code: rc.receipt_code });
  const lead = rc.state === "verified" ? t("verifiedLead") : rc.state === "cancelled" ? t("cancelledLead") : t("pendingLead");
  const save = `data:text/plain;charset=utf-8,${encodeURIComponent(plainText(rc))}`;
  return (
    head(title, `${rc.seller.name} · ${formatKES(rc.total_cents)}`, locale, css) +
    `<main class="doc">${receiptCardHtml(doc)}` +
    `<p class="doc-lead">${e(lead)}</p>` +
    // "Save to phone" is a plain download link; no JS required.
    `<a class="doc-save" href="${e(save)}" download="ciftpay-${e(rc.receipt_code)}.txt">${e(t("save"))}</a>` +
    `<footer class="doc-foot"><a href="https://ciftpay.co.ke/?utm_source=receipt&amp;utm_medium=footer&amp;utm_campaign=issue_yours">${e(t("cta"))}</a></footer>` +
    `</main></body></html>`
  );
}

/** Receipt-shaped 404 document. */
export function notFoundDocumentHtml({ t, locale, css }: Pick<ReceiptDocument, "t" | "locale" | "css">): string {
  return (
    head(t("notFound"), t("notFoundLead"), locale, css) +
    `<main class="doc"><div class="rc perforated-both doc-404"><h1>${e(t("notFound"))}</h1><p>${e(t("notFoundLead"))}</p></div></main></body></html>`
  );
}

/** Thermal-style plain text twin of the receipt for "Save to phone". */
export function plainText(rc: PublicReceipt): string {
  const w = 32;
  const row = (l: string, r: string) => `${l}${" ".repeat(Math.max(1, w - l.length - r.length))}${r}`;
  const lines = [
    rc.seller.name.toUpperCase(),
    `PIN ${rc.seller.kra_pin}`,
    rc.kind === "CREDIT_NOTE" ? "CREDIT NOTE" : "TAX INVOICE",
    rc.kra_invoice_no ? `KRA ${rc.kra_invoice_no}` : "",
    rc.issued_at,
    "-".repeat(w),
    ...rc.lines.map((l) => row(`${l.description} x${l.qty}`.slice(0, 22), formatKES(l.line_total_cents, { symbol: false }))),
    "-".repeat(w),
    row("Subtotal", formatKES(rc.subtotal_cents, { symbol: false })),
    row("VAT", formatKES(rc.tax_cents, { symbol: false })),
    row("TOTAL", formatKES(rc.total_cents)),
    "",
    `${rc.state.toUpperCase()} · ${rc.receipt_code}`,
    "Issued through CiftPay",
  ];
  return lines.filter((l) => l !== "").join("\n");
}
