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
import { useToday } from "@/lib/api/queries";
import { formatTime } from "@/lib/format";
import { paymentChip } from "@/lib/status";
import { RecordSaleSheet } from "@/components/sale/RecordSaleSheet";

export default function TodayPage() {
  const t = useTranslations("today");
  const ts = useTranslations("status");
  const tc = useTranslations("common");
  const { data, isPending } = useToday();
  const [saleOpen, setSaleOpen] = useState(false);

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

      {/* Live receipt strip: the number first (design-system §12). */}
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
              {data.recent_payments.map((p) => {
                const chip = paymentChip(p.status);
                return (
                  <li key={p.id} className="flex min-h-[var(--row)] items-center gap-3 py-2">
                    <div className="min-w-0 flex-1">
                      <div className="truncate font-mono">{p.payer_msisdn_masked}</div>
                      <div className="flex items-center gap-2 text-sm text-muted">
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
