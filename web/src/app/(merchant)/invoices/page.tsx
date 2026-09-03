"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { TopBar } from "@/components/shell/TopBar";
import { DataTable } from "@/components/ui/DataTable";
import { EmptyState } from "@/components/ui/EmptyState";
import { LoadingRows } from "@/components/ui/LoadingRows";
import { Money } from "@/components/ui/Money";
import { StatusChip } from "@/components/ui/StatusChip";
import { Tabs } from "@/components/ui/Tabs";
import { useInvoices } from "@/lib/api/queries";
import type { Schemas } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import { invoiceChip } from "@/lib/status";

type Filter = "all" | Schemas["InvoiceState"];

export default function InvoicesPage() {
  const t = useTranslations("invoices");
  const ts = useTranslations("status");
  const tc = useTranslations("common");
  const router = useRouter();
  const [filter, setFilter] = useState<Filter>("all");
  const { data, isPending } = useInvoices(filter === "all" ? undefined : filter);
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
          { value: "ACKED", label: t("filterAcked") },
          { value: "QUEUED", label: t("filterPending") },
          { value: "NEEDS_REVIEW", label: t("filterReview") },
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
            rowKey={(i) => i.id}
            onRowClick={(i) => router.push(`/invoices/${i.id}`)}
            primary={(i) => <span className="font-mono">{i.kra_invoice_no ?? <span className="text-muted">{t("noKraNo")}</span>}</span>}
            secondary={(i) => {
              const c = invoiceChip(i.state);
              return (
                <span className="flex items-center gap-2">
                  <span className="font-mono">{formatDateTime(i.created_at)}</span>
                  <StatusChip tone={c.tone}>{i.kind === "CREDIT_NOTE" ? t("creditNote") : ts(c.key)}</StatusChip>
                </span>
              );
            }}
            trailing={(i) => <Money cents={i.kind === "CREDIT_NOTE" ? -i.total_cents : i.total_cents} />}
            columns={[
              { key: "when", header: "Date", cell: (i) => <span className="font-mono">{formatDateTime(i.created_at)}</span> },
              { key: "kra", header: t("kraNo"), cell: (i) => <span className="font-mono">{i.kra_invoice_no ?? "—"}</span> },
              { key: "buyer", header: "Buyer", cell: (i) => <span className="font-mono text-muted">{i.buyer_pin_masked ?? i.buyer_msisdn_masked ?? "—"}</span> },
              {
                key: "state",
                header: "State",
                cell: (i) => {
                  const c = invoiceChip(i.state);
                  return <StatusChip tone={c.tone}>{ts(c.key)}</StatusChip>;
                },
              },
              { key: "total", header: "KES", numeric: true, cell: (i) => <Money cents={i.kind === "CREDIT_NOTE" ? -i.total_cents : i.total_cents} bare /> },
            ]}
          />
        )}
      </div>
    </>
  );
}
