"use client";

import Link from "next/link";
import { useState } from "react";
import { useTranslations } from "next-intl";
import { CheckCircle2, Clock, AlertTriangle } from "lucide-react";
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
  const { data: todayData, isPending: isTodayPending } = useToday();
  const { data: analytics, isPending: isAnalyticsPending } = useAnalyticsSummary(period);
  const [saleOpen, setSaleOpen] = useState(false);

  const hasNoActivity =
    (analytics?.payments_count ?? todayData?.payments_count ?? 0) === 0 &&
    (analytics?.gross_sales_cents ?? todayData?.received_cents ?? 0) === 0;

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

      {/* Analytics Interval Switcher & Live Metric Cards */}
      <section className="space-y-4">
        <div className="flex items-center justify-between">
          <Tabs<"today" | "month">
            ariaLabel="Analytics Period"
            value={period}
            onChange={setPeriod}
            items={[
              { value: "today", label: t("periodToday") },
              { value: "month", label: t("periodMonth") },
            ]}
          />
          <span className="font-mono text-xs text-muted">
            {period === "today" ? todayData?.date : new Date().toLocaleDateString(undefined, { month: "short", year: "numeric" })}
          </span>
        </div>

        {/* Real Metrics Grid */}
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          {/* Gross Sales */}
          <div className="rounded-r2 border border-hairline bg-paper-2 p-4 shadow-sm">
            <div className="flex items-center justify-between text-xs text-muted">
              <span className="font-medium uppercase tracking-wider">{t("grossSales")}</span>
              <span className="font-mono text-[11px] text-ink-2">
                {analytics?.payments_count ?? todayData?.payments_count ?? 0} {t("payments", { count: analytics?.payments_count ?? todayData?.payments_count ?? 0 })}
              </span>
            </div>
            <div className="mt-2">
              <Money cents={analytics?.gross_sales_cents ?? todayData?.received_cents ?? 0} size="2xl" />
            </div>
            <p className="mt-1 text-[11px] text-muted">Total M-Pesa volume received</p>
          </div>

          {/* Net Sales */}
          <div className="rounded-r2 border border-hairline bg-paper-2 p-4 shadow-sm">
            <div className="flex items-center justify-between text-xs text-muted">
              <span className="font-medium uppercase tracking-wider">{t("netSales")}</span>
              {(analytics?.daraja_fees_cents ?? 0) > 0 && (
                <span className="font-mono text-[11px] text-ink-2">
                  -{(analytics?.daraja_fees_cents ?? 0) / 100} fee
                </span>
              )}
            </div>
            <div className="mt-2">
              <Money cents={analytics?.net_sales_cents ?? todayData?.received_cents ?? 0} size="2xl" />
            </div>
            <p className="mt-1 text-[11px] text-muted">{t("netSalesHint")}</p>
          </div>

          {/* Estimated VAT Liability */}
          <div className="rounded-r2 border border-hairline bg-paper-2 p-4 shadow-sm">
            <div className="flex items-center justify-between text-xs text-muted">
              <span className="font-medium uppercase tracking-wider text-ochre">{t("vatLiability")}</span>
              <span className="rounded bg-ochre/15 px-1.5 py-0.5 font-mono text-[10px] font-semibold text-ochre">KRA</span>
            </div>
            <div className="mt-2">
              <Money cents={analytics?.vat_liability_cents ?? 0} size="2xl" />
            </div>
            <p className="mt-1 text-[11px] text-muted">{t("vatLiabilityHint")}</p>
          </div>
        </div>

        {/* Live status chips */}
        <div className="flex flex-wrap items-center gap-2 pt-1">
          <StatusChip tone="acked">
            <span className="flex items-center gap-1">
              <CheckCircle2 className="size-3.5" />
              {analytics?.invoices_acked_count ?? todayData?.invoices_acked ?? 0} {t("acked")}
            </span>
          </StatusChip>
          <StatusChip tone="pending">
            <span className="flex items-center gap-1">
              <Clock className="size-3.5" />
              {analytics?.invoices_pending_count ?? todayData?.invoices_pending ?? 0} {t("pending")}
            </span>
          </StatusChip>
          {(analytics?.attention_count ?? todayData?.attention_count ?? 0) > 0 && (
            <Link href="/attention">
              <StatusChip tone="failed">
                <span className="flex items-center gap-1">
                  <AlertTriangle className="size-3.5" />
                  {analytics?.attention_count ?? todayData?.attention_count} {t("attention")}
                </span>
              </StatusChip>
            </Link>
          )}
        </div>
      </section>

      {/* First-action empty state / setup checklist for new merchants */}
      {hasNoActivity && !isTodayPending && !isAnalyticsPending && (
        <section className="mt-6 rounded-r2 border border-hairline bg-paper p-5">
          <h2 className="text-base font-semibold text-ink">{t("quickStartTitle")}</h2>
          <p className="mt-1 text-xs text-ink-2 leading-relaxed">{t("quickStartLead")}</p>

          <div className="mt-4 grid gap-3 sm:grid-cols-3">
            <div className="flex flex-col justify-between rounded-r1 border border-hairline bg-paper-2 p-3.5">
              <div>
                <span className="inline-flex size-5 items-center justify-center rounded-full bg-ochre/20 font-mono text-xs font-bold text-ochre">
                  1
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
                  2
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
                  3
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

      {/* Latest Payments */}
      <section className="mt-8">
        <div className="flex items-baseline justify-between">
          <h2>{t("recent")}</h2>
          <Link href="/payments" className="text-sm text-ink-2 underline">
            {t("seeAll")}
          </Link>
        </div>
        <div className="mt-3">
          {isTodayPending ? (
            <LoadingRows rows={5} label={tc("loading")} />
          ) : !todayData || todayData.recent_payments.length === 0 ? (
            <EmptyState>{t("emptyPayments")}</EmptyState>
          ) : (
            <ul className="ruled">
              {todayData.recent_payments.map((p: Schemas["Payment"]) => {
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
