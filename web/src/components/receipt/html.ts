import type { Schemas } from "@/lib/api/client";
import { formatDateTime, formatKES, formatQty, maskKraPin, MINUS, moneyParts, THIN_SPACE } from "@/lib/format";

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

  const totalCents = isCN ? -Math.abs(rc.total_cents) : rc.total_cents;
  const subtotalCents = isCN ? -Math.abs(rc.subtotal_cents) : rc.subtotal_cents;
  const taxCents = isCN ? -Math.abs(rc.tax_cents) : rc.tax_cents;

  let customerMeta = "";
  if (rc.buyer_pin_masked && rc.buyer_name) {
    customerMeta += metaRow(t("billedTo"), rc.buyer_name);
    customerMeta += metaRow(t("buyerPin"), maskKraPin(rc.buyer_pin_masked));
    if (rc.payer_name) {
      customerMeta += metaRow(t("paidBy"), rc.payer_name);
    }
  } else if (rc.buyer_pin_masked) {
    customerMeta += metaRow(t("buyerPin"), maskKraPin(rc.buyer_pin_masked));
    if (rc.payer_name) {
      customerMeta += metaRow(t("paidBy"), rc.payer_name);
    }
  } else if (rc.payer_name) {
    customerMeta += metaRow(t("billedTo"), rc.payer_name);
  } else if (rc.buyer_name) {
    customerMeta += metaRow(t("billedTo"), rc.buyer_name);
  }

  const meta = [
    metaRow(t("invoiceNo"), rc.kra_invoice_no ?? "—"),
    metaRow(t("date"), formatDateTime(rc.issued_at, intl)),
    customerMeta,
    isCN && rc.credit_note_of ? metaRow(t("cancels", { kraNo: "" }).trim(), rc.credit_note_of) : "",
  ].join("");

  const lines = rc.lines
    .map((l) => {
      const lineTotal = isCN ? -Math.abs(l.line_total_cents) : l.line_total_cents;
      const label = `<span>${e(l.description)}<span class="rc-qty"> ×${formatQty(l.qty)} <span class="rc-cat">[${e(l.tax_category)}]</span></span></span>`;
      return `<li>${leaderHtml(label, moneyHtml(lineTotal, { bare: true, size: "sm" }))}</li>`;
    })
    .join("");

  const vat =
    rc.vat_by_category && rc.vat_by_category.length > 0
      ? rc.vat_by_category.map((v) => {
          const catTax = isCN ? -Math.abs(v.tax_cents) : v.tax_cents;
          return leaderHtml(e(t("vatCategory", { category: v.category })), moneyHtml(catTax, { bare: true, size: "sm" }));
        }).join("")
      : leaderHtml(e(t("vat")), moneyHtml(taxCents, { bare: true, size: "sm" }));

  const cnBanner = isCN
    ? `<div style="background:#FEE2E2;border:1px solid #F87171;color:#991B1B;padding:6px 10px;border-radius:4px;font-weight:700;font-size:11px;letter-spacing:0.05em;text-align:center;margin-bottom:12px;text-transform:uppercase;">CREDIT NOTE &middot; OFFICIAL KRA CANCELLATION</div>`
    : "";

  return (
    `<article class="rc${isCN ? " rc--credit-note" : ""} perforated-both" aria-label="${e(`${kind} ${rc.receipt_code}`)}"><div class="rc-inner">` +
    cnBanner +
    `<header class="rc-header"><div class="rc-head"><div class="rc-seller">${e(rc.seller.name)}</div><div>${e(t("pin"))} ${e(rc.seller.kra_pin)}</div><div class="rc-kind" style="${isCN ? "color:#991B1B;font-weight:700;" : ""}">${isCN ? "CREDIT NOTE (REFUND)" : e(kind)}</div></div>` +
    `<div class="rc-stamp"><span class="stamp stamp--${STAMP_TONE[rc.state]}" role="status">${e(stamp)}</span></div></header>` +
    `<dl class="rc-meta">${meta}</dl>` +
    `<hr class="rc-rule">` +
    `<ul class="rc-lines">${lines}</ul>` +
    `<hr class="rc-rule">` +
    `<div class="rc-totals">${leaderHtml(e(t("subtotal")), moneyHtml(subtotalCents, { bare: true, size: "sm" }))}${vat}` +
    leaderHtml(isCN ? "TOTAL CANCELLED / REFUNDED" : e(t("total")), moneyHtml(totalCents, { size: "xl" }), { strong: true, className: `rc-total${isCN ? " rc-total--credit-note" : ""}` }) +
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
    `<script src="/vendor/html2canvas.min.js" defer></script>` +
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
    // Dedicated Download PDF button
    `<button type="button" class="doc-save" id="btn-download-pdf" onclick="downloadReceiptPdf()">${e(t("downloadPdf"))}</button>` +
    // Dedicated Print button
    `<button type="button" class="doc-save" onclick="window.print()">${e(t("print"))}</button>` +
    `</div>` +
    (!rc.buyer_pin_masked && rc.kind !== "CREDIT_NOTE" ? (
      `<div class="doc-claim" style="margin: 1.5rem auto; max-width: 420px; width: 100%; text-align: left; background: #fff; padding: 1.25rem; border-radius: 4px; border: 1px solid rgba(18,17,15,0.12); box-shadow: 0 1px 3px rgba(0,0,0,0.04);">` +
      `<h3 style="font-size: 0.95rem; font-weight: 600; margin-bottom: 0.25rem; color: #12110F;">Claim input VAT</h3>` +
      `<p style="font-size: 0.8rem; color: #59544D; margin-bottom: 0.85rem; line-height: 1.4;">Add your KRA PIN to this receipt to claim VAT or include this in your official tax returns.</p>` +
      `<form id="claim-form" method="POST" action="/r/${e(rc.receipt_code)}" style="display: flex; flex-direction: column; gap: 0.6rem;">` +
      `<div>` +
      `<label style="display: block; font-size: 0.75rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: #59544D; margin-bottom: 0.25rem;">Your KRA PIN</label>` +
      `<input id="buyer-pin-input" type="text" name="buyer_pin" placeholder="A012345678X" required pattern="^[APap][0-9]{9}[A-Za-z]$" style="width: 100%; box-sizing: border-box; padding: 0.45rem 0.6rem; font-family: monospace; font-size: 0.95rem; text-transform: uppercase; border: 1px solid rgba(18,17,15,0.25); border-radius: 3px;" />` +
      `<div id="pin-validation-msg" style="display: none; margin-top: 0.35rem; font-size: 0.8rem;"></div>` +
      `</div>` +
      `<div>` +
      `<label style="display: block; font-size: 0.75rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: #59544D; margin-bottom: 0.25rem;">Business / Buyer Name (optional)</label>` +
      `<input id="buyer-name-input" type="text" name="buyer_name" placeholder="Acme Ltd" style="width: 100%; box-sizing: border-box; padding: 0.45rem 0.6rem; font-size: 0.9rem; border: 1px solid rgba(18,17,15,0.25); border-radius: 3px;" />` +
      `</div>` +
      `<button id="btn-submit-claim" type="submit" class="doc-save" style="margin-top: 0.25rem; width: 100%; cursor: pointer; text-align: center; border: none;">Add PIN &amp; Update Receipt</button>` +
      `</form>` +
      `</div>`
    ) : "") +
    `<footer class="doc-foot"><a href="https://ciftpay.co.ke/?utm_source=receipt&amp;utm_medium=footer&amp;utm_campaign=issue_yours">${e(t("cta"))}</a></footer>` +
    `</main>` +
    canvasScript(rc) +
    claimValidationScript() +
    `</body></html>`
  );
}

function canvasScript(rc: PublicReceipt): string {
  const jsonRc = JSON.stringify(rc).replace(/</g, "\\u003c");
  return `<script>
var receiptData = ${jsonRc};

function loadVendorScript(src) {
  return new Promise(function(resolve, reject) {
    if (window.html2canvas && src.indexOf("html2canvas") !== -1) return resolve();
    var existing = document.querySelector('script[src="' + src + '"]');
    if (existing) {
      if (existing.getAttribute("data-loaded") === "true") return resolve();
      existing.addEventListener("load", resolve);
      existing.addEventListener("error", reject);
      return;
    }
    var s = document.createElement("script");
    s.src = src;
    s.onload = function() {
      s.setAttribute("data-loaded", "true");
      resolve();
    };
    s.onerror = function() {
      var cdn = src.indexOf("html2canvas") !== -1
        ? "https://cdnjs.cloudflare.com/ajax/libs/html2canvas/1.4.1/html2canvas.min.js"
        : "https://cdnjs.cloudflare.com/ajax/libs/jspdf/2.5.1/jspdf.umd.min.js";
      var s2 = document.createElement("script");
      s2.src = cdn;
      s2.onload = function() { resolve(); };
      s2.onerror = reject;
      document.head.appendChild(s2);
    };
    document.head.appendChild(s);
  });
}

async function captureReceiptCanvas() {
  if (!window.html2canvas) {
    await loadVendorScript("/vendor/html2canvas.min.js");
  }
  if (document.fonts && document.fonts.ready) {
    await document.fonts.ready;
  }
  var rcEl = document.querySelector(".rc");
  if (!rcEl) throw new Error("Receipt element not found");

  var origWidth = rcEl.style.width;
  var origMaxWidth = rcEl.style.maxWidth;
  var origBoxSizing = rcEl.style.boxSizing;

  rcEl.style.width = "420px";
  rcEl.style.maxWidth = "420px";
  rcEl.style.boxSizing = "border-box";

  try {
    var canvas = await window.html2canvas(rcEl, {
      scale: 3,
      useCORS: true,
      logging: false,
      backgroundColor: "#fffdf8",
      width: 420
    });
    return canvas;
  } finally {
    rcEl.style.width = origWidth;
    rcEl.style.maxWidth = origMaxWidth;
    rcEl.style.boxSizing = origBoxSizing;
  }
}

function buildSinglePagePdf(jpegBytes, widthPt, heightPt, imgWidth, imgHeight) {
  var chunks = [];
  var offsets = [];
  var byteOffset = 0;
  function writeChunk(str) {
    var len = str.length;
    var buf = new Uint8Array(len);
    for (var i = 0; i < len; i++) {
      buf[i] = str.charCodeAt(i) & 0xff;
    }
    chunks.push(buf);
    byteOffset += len;
  }
  writeChunk("%PDF-1.4\\n%\\xE2\\xE3\\xCF\\xD3\\n");
  offsets[1] = byteOffset;
  writeChunk("1 0 obj\\n<< /Type /Catalog /Pages 2 0 R >>\\nendobj\\n");
  offsets[2] = byteOffset;
  writeChunk("2 0 obj\\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\\nendobj\\n");
  offsets[3] = byteOffset;
  writeChunk("3 0 obj\\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 " + widthPt + " " + heightPt + "] /Contents 4 0 R /Resources << /XObject << /Im1 5 0 R >> >> >>\\nendobj\\n");
  offsets[4] = byteOffset;
  var contentStream = "q\\n" + widthPt + " 0 0 " + heightPt + " 0 0 cm\\n/Im1 Do\\nQ\\n";
  writeChunk("4 0 obj\\n<< /Length " + contentStream.length + " >>\\nstream\\n" + contentStream + "endstream\\nendobj\\n");
  offsets[5] = byteOffset;
  writeChunk("5 0 obj\\n<< /Type /XObject /Subtype /Image /Width " + imgWidth + " /Height " + imgHeight + " /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length " + jpegBytes.length + " >>\\nstream\\n");
  chunks.push(jpegBytes);
  byteOffset += jpegBytes.length;
  writeChunk("\\nendstream\\nendobj\\n");
  var startXref = byteOffset;
  writeChunk("xref\\n0 6\\n0000000000 65535 f \\n");
  for (var i = 1; i <= 5; i++) {
    var s = ("0000000000" + offsets[i]).slice(-10);
    writeChunk(s + " 00000 n \\n");
  }
  writeChunk("trailer\\n<< /Size 6 /Root 1 0 R >>\\nstartxref\\n" + startXref + "\\n%%EOF\\n");
  return new Blob(chunks, { type: "application/pdf" });
}

async function downloadReceiptImage() {
  var btn = document.getElementById("btn-save-img");
  var origText = btn ? btn.innerText : "";
  if (btn) {
    btn.disabled = true;
    btn.innerText = "Generating...";
  }
  try {
    var canvas = await captureReceiptCanvas();
    canvas.toBlob(function(blob) {
      if (!blob) return;
      var url = URL.createObjectURL(blob);
      var a = document.createElement("a");
      a.href = url;
      a.download = "ciftpay-" + receiptData.receipt_code + ".png";
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    }, "image/png");
  } catch (err) {
    console.error("Failed to generate receipt image:", err);
    alert("Could not generate image on this browser. You can use the Print button to print or save as PDF.");
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.innerText = origText;
    }
  }
}

async function downloadReceiptPdf() {
  var btn = document.getElementById("btn-download-pdf");
  var origText = btn ? btn.innerText : "";
  if (btn) {
    btn.disabled = true;
    btn.innerText = "Generating...";
  }
  try {
    var canvas = await captureReceiptCanvas();
    var imgDataUrl = canvas.toDataURL("image/jpeg", 0.96);
    var base64 = imgDataUrl.split(",")[1];
    var raw = window.atob(base64);
    var rawLen = raw.length;
    var jpegBytes = new Uint8Array(rawLen);
    for (var i = 0; i < rawLen; i++) {
      jpegBytes[i] = raw.charCodeAt(i);
    }
    var pdfWidth = 240;
    var pdfHeight = Math.round((canvas.height * pdfWidth) / canvas.width);
    var pdfBlob = buildSinglePagePdf(jpegBytes, pdfWidth, pdfHeight, canvas.width, canvas.height);
    var url = URL.createObjectURL(pdfBlob);
    var a = document.createElement("a");
    a.href = url;
    a.download = "ciftpay-receipt-" + receiptData.receipt_code + ".pdf";
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  } catch (err) {
    console.error("Failed to generate receipt PDF:", err);
    alert("Could not generate PDF on this browser. You can use the Print button to print or save as PDF.");
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.innerText = origText;
    }
  }
}
</script>`;
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
  const customerLines: string[] = [];
  if (rc.buyer_pin_masked && rc.buyer_name) {
    customerLines.push(`Billed to: ${rc.buyer_name}`);
    customerLines.push(`Buyer PIN: ${maskKraPin(rc.buyer_pin_masked)}`);
    if (rc.payer_name) {
      customerLines.push(`Paid by: ${rc.payer_name}`);
    }
  } else if (rc.buyer_pin_masked) {
    customerLines.push(`Buyer PIN: ${maskKraPin(rc.buyer_pin_masked)}`);
    if (rc.payer_name) {
      customerLines.push(`Paid by: ${rc.payer_name}`);
    }
  } else if (rc.payer_name) {
    customerLines.push(`Billed to: ${rc.payer_name}`);
  } else if (rc.buyer_name) {
    customerLines.push(`Billed to: ${rc.buyer_name}`);
  }

  const lines = [
    rc.seller.name.toUpperCase(),
    `PIN ${rc.seller.kra_pin}`,
    rc.kind === "CREDIT_NOTE" ? "CREDIT NOTE" : "TAX INVOICE",
    rc.kra_invoice_no ? `KRA ${rc.kra_invoice_no}` : "",
    rc.issued_at,
    ...customerLines,
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

function claimValidationScript(): string {
  return `<script>
(function() {
  var pinInput = document.getElementById("buyer-pin-input");
  var nameInput = document.getElementById("buyer-name-input");
  var msg = document.getElementById("pin-validation-msg");
  var btn = document.getElementById("btn-submit-claim");
  var form = document.getElementById("claim-form");
  if (!pinInput || !form || !btn || !msg) return;

  var pinRegex = /^[AP][0-9]{9}[A-Z]$/i;
  var debounceTimer = null;
  var isValidated = false;

  pinInput.addEventListener("input", function() {
    clearTimeout(debounceTimer);
    var val = pinInput.value.trim().toUpperCase();
    if (!pinRegex.test(val)) {
      msg.style.display = "none";
      isValidated = false;
      return;
    }
    debounceTimer = setTimeout(function() {
      msg.style.display = "block";
      msg.style.color = "#59544D";
      msg.textContent = "Verifying PIN with KRA...";
      fetch("/kra/pin/" + encodeURIComponent(val))
        .then(function(r) { return r.json(); })
        .then(function(data) {
          if (data && data.taxpayer_name) {
            msg.style.display = "block";
            msg.style.color = "#10b981";
            msg.innerHTML = "<strong>Claiming as:</strong> " + data.taxpayer_name;
            if (nameInput && !nameInput.value) {
              nameInput.value = data.taxpayer_name;
            }
            isValidated = true;
          } else {
            msg.style.display = "block";
            msg.style.color = "#ef4444";
            msg.textContent = "Invalid KRA PIN. Not found in official tax registry.";
            isValidated = false;
          }
        })
        .catch(function() {
          // If offline or network fails, let form submission attempt server validation
          msg.style.display = "none";
          isValidated = true;
        });
    }, 400);
  });

  form.addEventListener("submit", function(e) {
    var val = pinInput.value.trim().toUpperCase();
    if (!pinRegex.test(val)) {
      e.preventDefault();
      msg.style.display = "block";
      msg.style.color = "#ef4444";
      msg.textContent = "Please enter a valid KRA PIN format (e.g. A012345678X).";
      return;
    }
    if (msg.style.color === "rgb(239, 68, 68)") {
      e.preventDefault();
      return;
    }
  });
})();
</script>`;
}
