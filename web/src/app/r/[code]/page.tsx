import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { ReceiptCard } from "@/components/receipt/ReceiptCard";
import { qrSvg } from "@/components/receipt/qr";
import { serverApi, type Schemas } from "@/lib/api/client";
import { formatKES } from "@/lib/format";

/**
 * Public buyer receipt. Server-rendered, no client JS, < 30 KB. The only
 * centred layout in the product (it is a document, not an app screen).
 */
export const dynamic = "force-dynamic";

type Params = { params: Promise<{ code: string }> };

const CODE_RE = /^[A-Z0-9]{6,12}$/i;

async function load(code: string): Promise<Schemas["PublicReceipt"] | null> {
  if (!CODE_RE.test(code)) return null;
  const { data, response } = await serverApi().GET("/r/{code}", { params: { path: { code: code.toUpperCase() } } });
  if (response.status === 404 || !data) return null;
  return data;
}

export async function generateMetadata({ params }: Params): Promise<Metadata> {
  const { code } = await params;
  const t = await getTranslations("receipt");
  const rc = await load(code);
  const title = t("title", { code: code.toUpperCase() });
  return {
    title,
    description: rc ? `${rc.seller.name} · ${formatKES(rc.total_cents)}` : t("notFound"),
    robots: { index: false, follow: false },
    openGraph: { title, type: "article" },
  };
}

export default async function PublicReceiptPage({ params }: Params) {
  const { code } = await params;
  const t = await getTranslations("receipt");
  const rc = await load(code);
  if (!rc) notFound();

  const stamp = rc.state === "verified" ? t("verified") : rc.state === "cancelled" ? t("cancelled") : t("pending");
  const lead = rc.state === "verified" ? t("verifiedLead") : rc.state === "cancelled" ? t("cancelledLead") : t("pendingLead");
  const qr = rc.kra_qr_payload ? await qrSvg(rc.kra_qr_payload) : null;

  return (
    <main className="mx-auto flex min-h-dvh w-full max-w-[var(--receipt-w)] flex-col items-center px-4 py-8">
      <ReceiptCard
        state={rc.state}
        kind={rc.kind}
        sellerName={rc.seller.name}
        sellerPin={rc.seller.kra_pin}
        kraInvoiceNo={rc.kra_invoice_no}
        receiptCode={rc.receipt_code}
        issuedAt={rc.issued_at}
        buyerPinMasked={rc.buyer_pin_masked}
        lines={rc.lines}
        subtotalCents={rc.subtotal_cents}
        taxCents={rc.tax_cents}
        totalCents={rc.total_cents}
        vatByCategory={rc.vat_by_category}
        creditNoteOf={rc.credit_note_of}
        qrSvg={qr}
        labels={{
          taxInvoice: t("taxInvoice"),
          creditNote: t("creditNote"),
          pin: t("pin"),
          invoiceNo: t("invoiceNo"),
          date: t("date"),
          buyerPin: t("buyerPin"),
          subtotal: t("subtotal"),
          vat: t("vat"),
          total: t("total"),
          stamp,
          cancels: rc.credit_note_of ? t("cancels", { kraNo: "" }).trim() : undefined,
          poweredBy: t("poweredBy"),
        }}
      />

      <p className="mt-6 max-w-prose text-center text-sm text-ink-2">{lead}</p>

      {/* "Save to phone" is the browser's print/share; no JS required. */}
      <a
        href={`data:text/plain;charset=utf-8,${encodeURIComponent(plainText(rc))}`}
        download={`ciftpay-${rc.receipt_code}.txt`}
        className="mt-6 inline-flex min-h-[var(--touch)] items-center justify-center rounded-r2 border border-ink bg-paper-2 px-4 text-base font-medium hover:bg-paper-3"
      >
        {t("save")}
      </a>

      <footer className="mt-12 text-center text-sm text-muted">
        <a href="https://ciftpay.co.ke/?utm_source=receipt&utm_medium=footer&utm_campaign=issue_yours" className="underline">
          {t("cta")}
        </a>
      </footer>
    </main>
  );
}

/** Thermal-style plain text twin of the receipt for "Save to phone". */
function plainText(rc: Schemas["PublicReceipt"]): string {
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
