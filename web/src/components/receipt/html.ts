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
    `<div class="doc-actions">` +
    // Primary action: Save to phone as plain text download (works everywhere, zero JS)
    `<a class="doc-save" href="${e(save)}" download="ciftpay-${e(rc.receipt_code)}.txt">${e(t("save"))}</a>` +
    // Image download button via canvas script
    `<button type="button" class="doc-save" id="btn-save-img" onclick="downloadReceiptImage()">${e(t("saveImage"))}</button>` +
    // Print button
    `<button type="button" class="doc-save" onclick="window.print()">${e(t("print"))}</button>` +
    `</div>` +
    (!rc.buyer_pin_masked && rc.kind !== "CREDIT_NOTE" ? (
      `<div class="doc-claim" style="margin: 1.5rem auto; max-width: 420px; width: 100%; text-align: left; background: #fff; padding: 1.25rem; border-radius: 4px; border: 1px solid rgba(18,17,15,0.12); box-shadow: 0 1px 3px rgba(0,0,0,0.04);">` +
      `<h3 style="font-size: 0.95rem; font-weight: 600; margin-bottom: 0.25rem; color: #12110F;">Claim input VAT</h3>` +
      `<p style="font-size: 0.8rem; color: #59544D; margin-bottom: 0.85rem; line-height: 1.4;">Add your KRA PIN to this receipt to claim VAT or include this in your official tax returns.</p>` +
      `<form method="POST" action="/r/${e(rc.receipt_code)}" style="display: flex; flex-direction: column; gap: 0.6rem;">` +
      `<div>` +
      `<label style="display: block; font-size: 0.75rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: #59544D; margin-bottom: 0.25rem;">Your KRA PIN</label>` +
      `<input type="text" name="buyer_pin" placeholder="A012345678X" required pattern="^[APap][0-9]{9}[A-Za-z]$" style="width: 100%; box-sizing: border-box; padding: 0.45rem 0.6rem; font-family: monospace; font-size: 0.95rem; text-transform: uppercase; border: 1px solid rgba(18,17,15,0.25); border-radius: 3px;" />` +
      `</div>` +
      `<div>` +
      `<label style="display: block; font-size: 0.75rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: #59544D; margin-bottom: 0.25rem;">Business / Buyer Name (optional)</label>` +
      `<input type="text" name="buyer_name" placeholder="Acme Ltd" style="width: 100%; box-sizing: border-box; padding: 0.45rem 0.6rem; font-size: 0.9rem; border: 1px solid rgba(18,17,15,0.25); border-radius: 3px;" />` +
      `</div>` +
      `<button type="submit" class="doc-save" style="margin-top: 0.25rem; width: 100%; cursor: pointer; text-align: center; border: none;">Add PIN &amp; Update Receipt</button>` +
      `</form>` +
      `</div>`
    ) : "") +
    `<footer class="doc-foot"><a href="https://ciftpay.co.ke/?utm_source=receipt&amp;utm_medium=footer&amp;utm_campaign=issue_yours">${e(t("cta"))}</a></footer>` +
    `</main>` +
    canvasScript(rc) +
    `</body></html>`
  );
}

function canvasScript(rc: PublicReceipt): string {
  const jsonRc = JSON.stringify(rc).replace(/</g, "\\u003c");
  return `<script>
function downloadReceiptImage() {
  var rc = ${jsonRc};
  var width = 420;
  var padding = 24;
  var itemsHeight = (rc.lines ? rc.lines.length : 0) * 24;
  var height = 360 + itemsHeight;
  var canvas = document.createElement("canvas");
  var dpr = window.devicePixelRatio || 2;
  canvas.width = width * dpr;
  canvas.height = height * dpr;
  var ctx = canvas.getContext("2d");
  ctx.scale(dpr, dpr);
  ctx.fillStyle = "#F6F1E7";
  ctx.fillRect(0, 0, width, height);
  ctx.strokeStyle = "rgba(18, 17, 15, 0.15)";
  ctx.lineWidth = 1;
  ctx.strokeRect(8, 8, width - 16, height - 16);
  var y = 36;
  ctx.fillStyle = "#12110F";
  ctx.font = "bold 18px monospace, sans-serif";
  ctx.textAlign = "center";
  ctx.fillText(rc.seller.name, width / 2, y);
  y += 20;
  ctx.font = "12px monospace, sans-serif";
  ctx.fillStyle = "#59544D";
  ctx.fillText(rc.kind === "CREDIT_NOTE" ? "CREDIT NOTE" : "TAX INVOICE", width / 2, y);
  y += 18;
  ctx.fillText("PIN " + rc.seller.kra_pin + (rc.kra_invoice_no ? " · KRA " + rc.kra_invoice_no : ""), width / 2, y);
  y += 16;
  ctx.fillText(rc.issued_at, width / 2, y);
  y += 14;
  ctx.setLineDash([4, 4]);
  ctx.beginPath();
  ctx.moveTo(padding, y);
  ctx.lineTo(width - padding, y);
  ctx.stroke();
  ctx.setLineDash([]);
  y += 20;
  ctx.font = "13px monospace, sans-serif";
  for (var i = 0; i < rc.lines.length; i++) {
    var l = rc.lines[i];
    ctx.textAlign = "left";
    ctx.fillStyle = "#12110F";
    var lineDesc = (l.description + " x" + l.qty).slice(0, 26);
    ctx.fillText(lineDesc, padding, y);
    ctx.textAlign = "right";
    var amt = "KES " + (l.line_total_cents / 100).toFixed(2);
    ctx.fillText(amt, width - padding, y);
    y += 22;
  }
  y += 6;
  ctx.setLineDash([4, 4]);
  ctx.beginPath();
  ctx.moveTo(padding, y);
  ctx.lineTo(width - padding, y);
  ctx.stroke();
  ctx.setLineDash([]);
  y += 22;
  ctx.textAlign = "left";
  ctx.fillStyle = "#59544D";
  ctx.fillText("Subtotal", padding, y);
  ctx.textAlign = "right";
  ctx.fillStyle = "#12110F";
  ctx.fillText("KES " + (rc.subtotal_cents / 100).toFixed(2), width - padding, y);
  y += 20;
  ctx.textAlign = "left";
  ctx.fillStyle = "#59544D";
  ctx.fillText("VAT", padding, y);
  ctx.textAlign = "right";
  ctx.fillStyle = "#12110F";
  ctx.fillText("KES " + (rc.tax_cents / 100).toFixed(2), width - padding, y);
  y += 24;
  ctx.font = "bold 15px monospace, sans-serif";
  ctx.textAlign = "left";
  ctx.fillStyle = "#12110F";
  ctx.fillText("TOTAL", padding, y);
  ctx.textAlign = "right";
  ctx.fillText("KES " + (rc.total_cents / 100).toFixed(2), width - padding, y);
  y += 26;
  ctx.setLineDash([4, 4]);
  ctx.beginPath();
  ctx.moveTo(padding, y);
  ctx.lineTo(width - padding, y);
  ctx.stroke();
  ctx.setLineDash([]);
  y += 24;
  ctx.font = "11px monospace, sans-serif";
  ctx.textAlign = "center";
  ctx.fillStyle = "#78726A";
  ctx.fillText((rc.state || "").toUpperCase() + " · " + rc.receipt_code, width / 2, y);
  y += 16;
  ctx.fillText("Issued through CiftPay · Verifiable with KRA", width / 2, y);
  canvas.toBlob(function(blob) {
    if (!blob) return;
    var url = URL.createObjectURL(blob);
    var a = document.createElement("a");
    a.href = url;
    a.download = "ciftpay-" + rc.receipt_code + ".png";
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }, "image/png");
}
<\/script>`;
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
