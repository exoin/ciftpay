"use client";

import { useState } from "react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { Check, Copy, FileText, ArrowRight } from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Field, SelectField } from "@/components/ui/Field";
import { Money } from "@/components/ui/Money";
import { Sheet } from "@/components/ui/Sheet";
import { StatusChip } from "@/components/ui/StatusChip";
import { useToast } from "@/components/ui/Toast";
import { useConvertPayment, useItems } from "@/lib/api/queries";
import { ApiRequestError, type Schemas } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import { paymentChip } from "@/lib/status";

export function PaymentDetailSheet({
  payment,
  onClose,
}: {
  payment: Schemas["Payment"] | null;
  onClose: () => void;
}) {
  const t = useTranslations("payments");
  const ts = useTranslations("status");
  const tc = useTranslations("common");
  const toast = useToast();
  const { data: items } = useItems();
  const convert = useConvertPayment();

  const [copied, setCopied] = useState(false);
  const [itemId, setItemId] = useState("");
  const [buyerPin, setBuyerPin] = useState("");

  if (!payment) return null;

  const chip = paymentChip(payment.status);

  async function handleCopy() {
    if (!payment?.trans_id) return;
    try {
      await navigator.clipboard.writeText(payment.trans_id);
      setCopied(true);
      toast.push(t("copied"));
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Fallback
    }
  }

  async function handleConvert() {
    if (!payment || !itemId) return;
    try {
      await convert.mutateAsync({
        id: payment.id,
        body: {
          lines: [{ item_id: itemId, qty: "1", unit_price_cents: payment.amount_cents }],
          ...(buyerPin ? { buyer_pin: buyerPin.toUpperCase() } : {}),
        },
      });
      toast.push(t("converted"));
      setItemId("");
      setBuyerPin("");
      onClose();
    } catch (e) {
      toast.push(
        e instanceof ApiRequestError ? tc("errorGeneric", { message: e.message }) : tc("noConnection"),
        "error"
      );
    }
  }

  function getRuleLabel(rule: Schemas["Payment"]["match_rule"]) {
    switch (rule) {
      case "auto_invoice":
        return t("ruleAuto");
      case "stk_window":
        return t("ruleStk");
      case "bill_ref":
        return t("ruleBill");
      case "manual":
        return t("ruleManual");
      default:
        return t("ruleUnknown");
    }
  }

  return (
    <Sheet
      open={payment !== null}
      onClose={onClose}
      title={t("detailsTitle")}
      closeLabel={tc("close")}
      footer={
        payment.status === "unmatched" ? (
          <Button block disabled={!itemId} loading={convert.isPending} onClick={handleConvert}>
            {t("convertSubmit")}
          </Button>
        ) : undefined
      }
    >
      <div className="space-y-6">
        {/* Top Summary Card */}
        <div className="rounded-lg border border-edge bg-paper-2 p-4 text-center sm:text-left">
          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2">
            <div>
              <div className="text-xs font-semibold uppercase tracking-wider text-muted">
                {t("title")}
              </div>
              <div className="mt-1 text-2xl sm:text-3xl font-bold tracking-tight text-ink">
                <Money cents={payment.amount_cents} />
              </div>
            </div>
            <div className="self-center sm:self-auto">
              <StatusChip tone={chip.tone}>{ts(chip.key)}</StatusChip>
            </div>
          </div>
          <div className="mt-3 font-mono text-xs text-muted">
            {formatDateTime(payment.paid_at)}
          </div>
        </div>

        {/* Transaction Details Grid */}
        <div className="space-y-3 rounded-lg border border-edge p-4 text-sm">
          {/* M-Pesa Receipt Code with Copy */}
          <div className="flex items-center justify-between border-b border-hairline pb-2.5">
            <span className="text-muted">{t("mpesaReceipt")}</span>
            <div className="flex items-center gap-1.5">
              <span className="font-mono font-bold text-ink">{payment.trans_id}</span>
              <button
                type="button"
                onClick={handleCopy}
                title={t("copy")}
                className="inline-flex items-center justify-center rounded p-1 text-muted hover:bg-paper-3 hover:text-ink active:scale-95 transition-all"
              >
                {copied ? <Check className="h-3.5 w-3.5 text-leaf" /> : <Copy className="h-3.5 w-3.5" />}
              </button>
            </div>
          </div>

          {/* Customer Name */}
          <div className="flex items-center justify-between border-b border-hairline pb-2.5">
            <span className="text-muted">{t("customer")}</span>
            <span className="font-medium text-ink text-right truncate max-w-[200px]">
              {payment.payer_name || "—"}
            </span>
          </div>

          {/* Customer Phone */}
          <div className="flex items-center justify-between border-b border-hairline pb-2.5">
            <span className="text-muted">{tc("phone")}</span>
            <span className="font-mono text-ink">{payment.payer_msisdn_masked}</span>
          </div>

          {/* Bill / Account Ref */}
          {payment.bill_ref && (
            <div className="flex items-center justify-between border-b border-hairline pb-2.5">
              <span className="text-muted">{t("account")}</span>
              <span className="font-mono text-ink">{payment.bill_ref}</span>
            </div>
          )}

          {/* Reconciliation Rule */}
          <div className="flex items-center justify-between">
            <span className="text-muted">{t("matchRule")}</span>
            <span className="text-xs font-medium text-ink-2">{getRuleLabel(payment.match_rule)}</span>
          </div>
        </div>

        {/* Linked Invoice Section */}
        {payment.invoice_id ? (
          <div className="rounded-lg border border-leaf/30 bg-leaf/5 p-4">
            <div className="flex items-start gap-3">
              <div className="rounded-full bg-leaf/10 p-2 text-leaf shrink-0 mt-0.5">
                <FileText className="h-4 w-4" />
              </div>
              <div className="flex-1 min-w-0">
                <h4 className="text-sm font-semibold text-ink">{t("invoiceGenerated")}</h4>
                <p className="mt-0.5 text-xs text-muted">{t("invoiceGeneratedDesc")}</p>
                <div className="mt-3">
                  <Link
                    href={`/invoices/${payment.invoice_id}`}
                    onClick={onClose}
                    className="inline-flex items-center gap-1.5 text-xs font-semibold text-leaf hover:underline active:opacity-80"
                  >
                    <span>{t("viewInvoice")}</span>
                    <ArrowRight className="h-3.5 w-3.5" />
                  </Link>
                </div>
              </div>
            </div>
          </div>
        ) : payment.status === "unmatched" ? (
          /* Unmatched Conversion Form */
          <div className="space-y-4 rounded-lg border border-ochre/30 bg-ochre/5 p-4">
            <div>
              <h4 className="text-sm font-semibold text-ink">{t("convertTitle")}</h4>
              <p className="mt-1 text-xs text-muted">{t("unmatchedLead")}</p>
            </div>
            <div className="space-y-3">
              <SelectField
                label={t("item")}
                value={itemId}
                onChange={(e) => setItemId(e.target.value)}
              >
                <option value="" />
                {items?.data
                  .filter((it) => it.is_active)
                  .map((it) => (
                    <option key={it.id} value={it.id}>
                      {it.name}
                    </option>
                  ))}
              </SelectField>
              <Field
                label={t("buyerPin")}
                hint={t("buyerPinHint")}
                mono
                placeholder="A123456789B"
                value={buyerPin}
                onChange={(e) => setBuyerPin(e.target.value)}
              />
            </div>
          </div>
        ) : null}
      </div>
    </Sheet>
  );
}
