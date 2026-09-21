"use client";

import { use, useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { TopBar } from "@/components/shell/TopBar";
import { Button } from "@/components/ui/Button";
import { LoadingRows } from "@/components/ui/LoadingRows";
import { StatusChip } from "@/components/ui/StatusChip";
import { useToast } from "@/components/ui/Toast";
import { ReceiptCard } from "@/components/receipt/ReceiptCard";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { useInvoice, useReissueInvoice, useResendReceipt, useRetryInvoice } from "@/lib/api/queries";
import { ApiRequestError } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import { invoiceChip, receiptState } from "@/lib/status";

export default function InvoiceDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const t = useTranslations("invoices");
  const tr = useTranslations("receipt");
  const ts = useTranslations("status");
  const tc = useTranslations("common");
  const toast = useToast();
  const { data: inv, isPending } = useInvoice(id);
  const retry = useRetryInvoice();
  const resend = useResendReceipt();
  const reissue = useReissueInvoice();
  const router = useRouter();
  const [showAmend, setShowAmend] = useState(false);
  const [amendPin, setAmendPin] = useState("");
  const [amendName, setAmendName] = useState("");

  // Print-reveal only when the invoice flips to ACKED while on screen.
  const prev = useRef<string | undefined>(undefined);
  const [reveal, setReveal] = useState(false);
  useEffect(() => {
    if (!inv) return;
    if (prev.current && prev.current !== "ACKED" && inv.state === "ACKED") setReveal(true);
    prev.current = inv.state;
  }, [inv]);

  if (isPending || !inv) {
    return (
      <>
        <TopBar title={t("detailTitle")} />
        <LoadingRows rows={8} label={tc("loading")} />
      </>
    );
  }

  const chip = invoiceChip(inv.state);
  const state = receiptState(inv.state, inv.kind);
  const stampLabel = state === "verified" ? tr("verified") : state === "cancelled" ? tr("cancelled") : tr("pending");

  async function onRetry() {
    try {
      await retry.mutateAsync(inv!.id);
      toast.push(t("retried"));
    } catch (e) {
      toast.push(e instanceof ApiRequestError ? tc("errorGeneric", { message: e.message }) : tc("noConnection"), "error");
    }
  }
  async function onResend() {
    try {
      await resend.mutateAsync(inv!.id);
      toast.push(t("resent"));
    } catch (e) {
      toast.push(e instanceof ApiRequestError ? tc("errorGeneric", { message: e.message }) : tc("noConnection"), "error");
    }
  }

  async function onAmend(e: React.FormEvent) {
    e.preventDefault();
    if (!amendPin.trim()) return;
    try {
      const updated = await reissue.mutateAsync({
        id: inv!.id,
        buyer_pin: amendPin.trim().toUpperCase(),
        buyer_name: amendName.trim() || undefined,
      });
      toast.push(t("reissued"));
      setShowAmend(false);
      router.push(`/invoices/${updated.id}`);
    } catch (err) {
      toast.push(err instanceof ApiRequestError ? tc("errorGeneric", { message: err.message }) : tc("noConnection"), "error");
    }
  }

  return (
    <>
      <TopBar title={t("detailTitle")} action={<StatusChip tone={chip.tone}>{ts(chip.key)}</StatusChip>} />
      <div className="grid gap-8 lg:grid-cols-[1fr_var(--receipt-w)]">
        <div className="space-y-6">
          {inv.last_error && (inv.state === "NEEDS_REVIEW" || inv.state === "FAILED_TERMINAL") && (
            <p role="alert" className="rounded-r2 border border-red bg-bad-bg px-3 py-2 text-sm text-ink">
              {inv.last_error}
            </p>
          )}
          {inv.superseded_by_id && (
            <div className="rounded-r2 border border-ochre/40 bg-warn-bg px-4 py-3 text-sm flex flex-wrap items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <span className="font-semibold text-ink">
                  {inv.kind === "CREDIT_NOTE" ? "Offsetting Credit Note:" : "Superseded Invoice:"}
                </span>
                <span className="text-ink-2">
                  {inv.kind === "CREDIT_NOTE"
                    ? "This credit note reversed the transaction. It is closed and cannot be amended."
                    : "This invoice was replaced by an amended version and cannot be amended again."}
                </span>
              </div>
              <Link href={`/invoices/${inv.superseded_by_id}`} className="font-semibold underline text-ink">
                View Active Invoice &rarr;
              </Link>
            </div>
          )}
          <div className="flex flex-wrap gap-2">
            {inv.state === "NEEDS_REVIEW" && (
              <Button onClick={onRetry} loading={retry.isPending}>
                {t("retry")}
              </Button>
            )}
            {inv.state === "ACKED" && (
              <Button variant="secondary" onClick={onResend} loading={resend.isPending}>
                {t("resend")}
              </Button>
            )}
            {inv.receipt_url && (
              <a href={inv.receipt_url} target="_blank" rel="noreferrer" className="inline-flex min-h-[var(--touch)] items-center px-2 text-sm underline">
                {t("openPublic")}
              </a>
            )}
            {inv.kind === "INVOICE" && !inv.superseded_by_id && (
              <Button variant="secondary" onClick={() => setShowAmend(!showAmend)}>
                {inv.buyer_pin_masked ? t("amendPin") : t("addBuyerPin")}
              </Button>
            )}
          </div>

          {showAmend && (
            <div className="rounded-r2 border border-border bg-paper-2 p-4 space-y-3">
              <div>
                <h3 className="font-medium text-sm text-ink">{t("amendTitle")}</h3>
                <p className="text-xs text-muted">{t("amendLead")}</p>
              </div>
              <form onSubmit={onAmend} className="space-y-3">
                <div>
                  <label className="block text-xs font-semibold uppercase text-muted mb-1">Buyer KRA PIN</label>
                  <input
                    type="text"
                    value={amendPin}
                    onChange={(e) => setAmendPin(e.target.value.toUpperCase())}
                    placeholder="A012345678X"
                    required
                    pattern="^[APap][0-9]{9}[A-Za-z]$"
                    className="w-full rounded-r1 border border-border px-3 py-1.5 font-mono text-sm uppercase bg-paper"
                  />
                </div>
                <div>
                  <label className="block text-xs font-semibold uppercase text-muted mb-1">Buyer Name (optional)</label>
                  <input
                    type="text"
                    value={amendName}
                    onChange={(e) => setAmendName(e.target.value)}
                    placeholder="Acme Ltd"
                    className="w-full rounded-r1 border border-border px-3 py-1.5 text-sm bg-paper"
                  />
                </div>
                <div className="flex gap-2">
                  <Button type="submit" loading={reissue.isPending}>Submit Amendment</Button>
                  <Button type="button" variant="secondary" onClick={() => setShowAmend(false)}>Cancel</Button>
                </div>
              </form>
            </div>
          )}

          {inv.submissions && inv.submissions.length > 0 && (
            <section>
              <h2>{t("submissions")}</h2>
              <ul className="ruled mt-2 text-sm">
                {inv.submissions.map((s) => (
                  <li key={s.attempt} className="flex min-h-12 items-center justify-between gap-3 py-2">
                    <span className="font-mono">
                      {t("attempt", { n: s.attempt })} · {s.adapter}
                    </span>
                    <span className="flex items-center gap-2">
                      <span className="font-mono text-muted">{formatDateTime(s.started_at)}</span>
                      <StatusChip tone={s.classification === "ok" ? "acked" : s.classification === "retryable" ? "pending" : "failed"}>{s.classification}</StatusChip>
                    </span>
                  </li>
                ))}
              </ul>
            </section>
          )}
        </div>

        <ReceiptCard
          state={state}
          kind={inv.kind}
          sellerName={inv.seller.name}
          sellerPin={inv.seller.kra_pin}
          kraInvoiceNo={inv.kra_invoice_no}
          receiptCode={inv.receipt_code}
          issuedAt={inv.acked_at ?? inv.created_at}
          buyerPinMasked={inv.buyer_pin_masked}
          lines={inv.lines}
          subtotalCents={inv.total_cents - inv.tax_cents}
          taxCents={inv.tax_cents}
          totalCents={inv.total_cents}
          reveal={reveal}
          labels={{
            taxInvoice: tr("taxInvoice"),
            creditNote: tr("creditNote"),
            pin: tr("pin"),
            invoiceNo: tr("invoiceNo"),
            date: tr("date"),
            buyerPin: tr("buyerPin"),
            subtotal: tr("subtotal"),
            vat: tr("vat"),
            total: tr("total"),
            stamp: stampLabel,
            poweredBy: tr("poweredBy"),
          }}
        />
      </div>
    </>
  );
}
