"use client";

import Link from "next/link";
import { useState } from "react";
import { useTranslations } from "next-intl";
import { TopBar } from "@/components/shell/TopBar";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { LoadingRows } from "@/components/ui/LoadingRows";
import { Money } from "@/components/ui/Money";
import { StatusChip } from "@/components/ui/StatusChip";
import { Tabs } from "@/components/ui/Tabs";
import { useAnalyticsSummary, useToday } from "@/lib/api/queries";
import type { Schemas } from "@/lib/api/client";
import { formatTime } from "@/lib/format";
import { paymentChip } from "@/lib/status";
import { RecordSaleSheet } from "@/components/sale/RecordSaleSheet";

export default function TodayPage() {
  const t = useTranslations("today");
  const ts = useTranslations("status");
  const tc = useTranslations("common");

  const [period, setPeriod] = useState<"today" | "month">("today");
  const { data, isPending } = useToday();
  const { data: analytics } = useAnalyticsSummary(period);
  const [saleOpen, setSaleOpen] = useState(false);

  const hasNoActivity = (data?.payments_count ?? 0) === 0 && (data?.received_cents ?? 0) === 0;

  return (
    <>
      <TopBar
        title={t("title")}
        action={
          <Button size="sm" onClick={() => setSaleOpen(true)}>
            {t("recordSale")}
          </Button>
        }
      />

      {/* Live receipt strip: the number first (hero card matching design-system §12). */}
      <section aria-live="polite" className="perforated-top bg-paper-2 px-5 pb-5 pt-6">
        <div className="receipt-head text-xs text-muted">{t("received")}</div>
        <div className="mt-1">
          <Money cents={data?.received_cents ?? 0} size="3xl" className="w-full justify-start" />
        </div>
        <div className="mt-1 font-mono text-sm text-muted">{t("payments", { count: data?.payments_count ?? 0 })}</div>
        <div className="mt-4 flex flex-wrap gap-2">
          <StatusChip tone="acked">
            {data?.invoices_acked ?? 0} {t("acked")}
          </StatusChip>
          <StatusChip tone="pending">
            {data?.invoices_pending ?? 0} {t("pending")}
          </StatusChip>
          {(data?.attention_count ?? 0) > 0 && (
            <Link href="/attention">
              <StatusChip tone="failed">
                {data?.attention_count} {t("attention")}
              </StatusChip>
            </Link>
          )}
        </div>
      </section>

      {/* Thin Analytics Strip below the hero receipt */}
      <section className="mt-3 rounded-r1 border border-hairline bg-paper-2 p-3 text-xs">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-hairline/60 pb-2">
          <Tabs<"today" | "month">
            ariaLabel="Period"
            value={period}
            onChange={setPeriod}
            items={[
              { value: "today", label: t("periodToday") },
              { value: "month", label: t("periodMonth") },
            ]}
          />
          <span className="font-mono text-[11px] text-muted">
            {period === "today" ? data?.date : new Date().toLocaleDateString(undefined, { month: "short", year: "numeric" })}
          </span>
        </div>

        <div className="mt-2.5 grid grid-cols-1 divide-y divide-hairline/60 sm:grid-cols-3 sm:divide-x sm:divide-y-0">
          {/* Gross Sales */}
          <div className="py-1.5 sm:px-3 first:pl-0">
            <div className="text-[11px] font-medium uppercase tracking-wide text-muted">{t("grossSales")}</div>
            <div className="mt-1 flex items-baseline gap-2">
              <Money cents={analytics?.gross_sales_cents ?? 0} size="md" />
              <span className="font-mono text-[10px] text-muted">
                ({analytics?.payments_count ?? 0})
              </span>
            </div>
          </div>

          {/* Net Sales */}
          <div className="py-1.5 sm:px-3">
            <div className="flex items-center justify-between text-[11px] font-medium uppercase tracking-wide text-muted">
              <span>{t("netSales")}</span>
              {(analytics?.daraja_fees_cents ?? 0) > 0 && (
                <span className="font-mono text-[10px] text-ink-2">
                  -{(analytics?.daraja_fees_cents ?? 0) / 100} fee
                </span>
              )}
            </div>
            <div className="mt-1">
              <Money cents={analytics?.net_sales_cents ?? 0} size="md" />
            </div>
          </div>

          {/* Est. VAT Liability */}
          <div className="py-1.5 sm:px-3 last:pr-0">
            <div className="flex items-center justify-between text-[11px] font-medium uppercase tracking-wide text-muted">
              <span className="text-ochre">{t("vatLiability")}</span>
              <span className="rounded bg-ochre/15 px-1 font-mono text-[9px] font-semibold text-ochre">{"KRA"}</span>
            </div>
            <div className="mt-1">
              <Money cents={analytics?.vat_liability_cents ?? 0} size="md" />
            </div>
          </div>
        </div>
      </section>

      {/* First-action empty state / setup checklist for new merchants */}
      {hasNoActivity && !isPending && (
        <section className="mt-6 rounded-r2 border border-hairline bg-paper p-5">
          <h2 className="text-base font-semibold text-ink">{t("quickStartTitle")}</h2>
          <p className="mt-1 text-xs text-ink-2 leading-relaxed">{t("quickStartLead")}</p>

          <div className="mt-4 grid gap-3 sm:grid-cols-3">
            <div className="flex flex-col justify-between rounded-r1 border border-hairline bg-paper-2 p-3.5">
              <div>
                <span className="inline-flex size-5 items-center justify-center rounded-full bg-ochre/20 font-mono text-xs font-bold text-ochre">
                  {"1"}
                </span>
                <p className="mt-2 text-xs font-semibold text-ink">{t("step1Title")}</p>
                <p className="mt-1 text-[11px] text-muted leading-normal">{t("step1Desc")}</p>
              </div>
              <div className="mt-3">
                <Link href="/items">
                  <Button size="sm" variant="secondary" block>
                    {t("step1Action")}
                  </Button>
                </Link>
              </div>
            </div>

            <div className="flex flex-col justify-between rounded-r1 border border-hairline bg-paper-2 p-3.5">
              <div>
                <span className="inline-flex size-5 items-center justify-center rounded-full bg-ochre/20 font-mono text-xs font-bold text-ochre">
                  {"2"}
                </span>
                <p className="mt-2 text-xs font-semibold text-ink">{t("step2Title")}</p>
                <p className="mt-1 text-[11px] text-muted leading-normal">{t("step2Desc")}</p>
              </div>
              <div className="mt-3">
                <Link href="/settings">
                  <Button size="sm" variant="secondary" block>
                    {t("step2Action")}
                  </Button>
                </Link>
              </div>
            </div>

            <div className="flex flex-col justify-between rounded-r1 border border-hairline bg-paper-2 p-3.5">
              <div>
                <span className="inline-flex size-5 items-center justify-center rounded-full bg-ochre/20 font-mono text-xs font-bold text-ochre">
                  {"3"}
                </span>
                <p className="mt-2 text-xs font-semibold text-ink">{t("step3Title")}</p>
                <p className="mt-1 text-[11px] text-muted leading-normal">{t("step3Desc")}</p>
              </div>
              <div className="mt-3">
                <Button size="sm" block onClick={() => setSaleOpen(true)}>
                  {t("step3Action")}
                </Button>
              </div>
            </div>
          </div>
        </section>
      )}

      <section className="mt-8">
        <div className="flex items-baseline justify-between">
          <h2>{t("recent")}</h2>
          <Link href="/payments" className="text-sm text-ink-2 underline">
            {t("seeAll")}
          </Link>
        </div>
        <div className="mt-3">
          {isPending ? (
            <LoadingRows rows={5} label={tc("loading")} />
          ) : !data || data.recent_payments.length === 0 ? (
            <EmptyState>{t("emptyPayments")}</EmptyState>
          ) : (
            <ul className="ruled">
              {data.recent_payments.map((p: Schemas["Payment"]) => {
                const chip = paymentChip(p.status);
                return (
                  <li key={p.id} className="flex min-h-[var(--row)] items-center gap-3 py-2">
                    <div className="min-w-0 flex-1">
                      <div className="truncate font-mono">{p.payer_msisdn_masked}</div>
                      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-muted">
                        <span className="font-mono">{formatTime(p.paid_at)}</span>
                        <StatusChip tone={chip.tone}>{ts(chip.key)}</StatusChip>
                      </div>
                    </div>
                    <Money cents={p.amount_cents} />
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      </section>

      <RecordSaleSheet open={saleOpen} onClose={() => setSaleOpen(false)} />
    </>
  );
}
