"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { TopBar } from "@/components/shell/TopBar";
import { DataTable } from "@/components/ui/DataTable";
import { EmptyState } from "@/components/ui/EmptyState";
import { LoadingRows } from "@/components/ui/LoadingRows";
import { Money } from "@/components/ui/Money";
import { StatusChip } from "@/components/ui/StatusChip";
import { Tabs } from "@/components/ui/Tabs";
import { usePayments } from "@/lib/api/queries";
import type { Schemas } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import { paymentChip } from "@/lib/status";
import { ConvertPaymentSheet } from "@/components/sale/ConvertPaymentSheet";

type Filter = "all" | "unmatched" | "cash_sale" | "matched";

export default function PaymentsPage() {
  const t = useTranslations("payments");
  const ts = useTranslations("status");
  const tc = useTranslations("common");
  const [filter, setFilter] = useState<Filter>("all");
  const { data, isPending } = usePayments(filter === "all" ? undefined : filter);
  const [selected, setSelected] = useState<Schemas["Payment"] | null>(null);

  const rows = data?.data ?? [];

  return (
    <>
      <TopBar title={t("title")} />
      <Tabs<Filter>
        ariaLabel={t("title")}
        value={filter}
        onChange={setFilter}
        items={[
          { value: "all", label: t("filterAll") },
          { value: "unmatched", label: t("filterUnmatched") },
          { value: "cash_sale", label: t("filterCash") },
          { value: "matched", label: t("filterMatched") },
        ]}
      />
      <div className="mt-4">
        {isPending ? (
          <LoadingRows rows={6} label={tc("loading")} />
        ) : rows.length === 0 ? (
          <EmptyState>{t("empty")}</EmptyState>
        ) : (
          <DataTable
            rows={rows}
            rowKey={(p) => p.id}
            onRowClick={(p) => p.status === "unmatched" && setSelected(p)}
            primary={(p) => <span className="font-mono">{p.payer_msisdn_masked}</span>}
            secondary={(p) => {
              const c = paymentChip(p.status);
              return (
                <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <span className="font-mono">{formatDateTime(p.paid_at)}</span>
                  <StatusChip tone={c.tone}>{ts(c.key)}</StatusChip>
                </span>
              );
            }}
            trailing={(p) => <Money cents={p.amount_cents} />}
            columns={[
              { key: "when", header: "Time", cell: (p) => <span className="font-mono">{formatDateTime(p.paid_at)}</span> },
              { key: "from", header: "From", cell: (p) => <span className="font-mono">{p.payer_msisdn_masked}</span> },
              { key: "ref", header: "Ref", cell: (p) => <span className="font-mono text-muted">{p.bill_ref || p.trans_id}</span> },
              {
                key: "status",
                header: "Status",
                cell: (p) => {
                  const c = paymentChip(p.status);
                  return <StatusChip tone={c.tone}>{ts(c.key)}</StatusChip>;
                },
              },
              { key: "amount", header: "KES", numeric: true, cell: (p) => <Money cents={p.amount_cents} bare /> },
            ]}
          />
        )}
      </div>
      <ConvertPaymentSheet payment={selected} onClose={() => setSelected(null)} />
    </>
  );
}
