"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { Search, X, ArrowRight } from "lucide-react";
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
import { PaymentDetailSheet } from "@/components/sale/PaymentDetailSheet";

type Filter = "all" | "unmatched" | "cash_sale" | "matched";

export default function PaymentsPage() {
  const t = useTranslations("payments");
  const ts = useTranslations("status");
  const tc = useTranslations("common");

  const [filter, setFilter] = useState<Filter>("all");
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [selected, setSelected] = useState<Schemas["Payment"] | null>(null);

  useEffect(() => {
    const handler = setTimeout(() => {
      setDebouncedSearch(search.trim());
    }, 300);
    return () => clearTimeout(handler);
  }, [search]);

  const { data, isPending } = usePayments({
    status: filter === "all" ? undefined : filter,
    q: debouncedSearch || undefined,
  });

  const rows = data?.data ?? [];

  return (
    <>
      <TopBar title={t("title")} />

      {/* Universal Search Bar */}
      <div className="relative mb-4">
        <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3 text-muted">
          <Search className="h-4 w-4" />
        </div>
        <input
          type="search"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t("searchPlaceholder")}
          className="w-full rounded-md border border-edge bg-surface py-2 pl-9 pr-9 text-sm text-foreground placeholder:text-muted focus:border-accent focus:outline-none focus:ring-1 focus:ring-accent"
        />
        {search && (
          <button
            type="button"
            onClick={() => setSearch("")}
            className="absolute inset-y-0 right-0 flex items-center pr-3 text-muted hover:text-foreground"
          >
            <X className="h-4 w-4" />
          </button>
        )}
      </div>

      {/* Filter Tabs */}
      <div className="overflow-x-auto pb-1">
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
      </div>

      {/* Payment Table / List */}
      <div className="mt-4">
        {isPending ? (
          <LoadingRows rows={6} label={tc("loading")} />
        ) : rows.length === 0 ? (
          <EmptyState>{t("empty")}</EmptyState>
        ) : (
          <DataTable
            rows={rows}
            rowKey={(p) => p.id}
            onRowClick={(p) => setSelected(p)}
            primary={(p) => (
              <div className="flex items-center gap-2">
                <span className="font-semibold text-ink truncate">
                  {p.payer_name || p.payer_msisdn_masked}
                </span>
                {p.payer_name && (
                  <span className="font-mono text-xs text-muted shrink-0">
                    {p.payer_msisdn_masked}
                  </span>
                )}
              </div>
            )}
            secondary={(p) => {
              const c = paymentChip(p.status);
              return (
                <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
                  <span className="font-mono font-medium text-ink-2">{p.trans_id}</span>
                  {p.bill_ref && (
                    <span className="text-muted">
                      <span>{"· "}</span>
                      <span>{t("accPrefix")}</span>
                      <span>{": "}</span>
                      <span>{p.bill_ref}</span>
                    </span>
                  )}
                  <span className="text-muted">
                    <span>{"· "}</span>
                    <span>{formatDateTime(p.paid_at)}</span>
                  </span>
                  <StatusChip tone={c.tone}>{ts(c.key)}</StatusChip>
                </div>
              );
            }}
            trailing={(p) => (
              <div className="flex flex-col items-end">
                <Money cents={p.amount_cents} />
                {p.invoice_id ? (
                  <span className="mt-1 inline-flex items-center gap-0.5 text-[11px] font-semibold text-leaf">
                    <span>{t("invoiceAction")}</span>
                    <ArrowRight className="h-3 w-3" />
                  </span>
                ) : p.status === "unmatched" ? (
                  <span className="mt-1 inline-flex items-center gap-0.5 text-[11px] font-semibold text-ochre">
                    <span>{t("matchAction")}</span>
                    <ArrowRight className="h-3 w-3" />
                  </span>
                ) : null}
              </div>
            )}
            columns={[
              {
                key: "when",
                header: t("paidAt"),
                cell: (p) => (
                  <span className="font-mono text-xs text-muted">
                    {formatDateTime(p.paid_at)}
                  </span>
                ),
              },
              {
                key: "trans",
                header: t("mpesaReceipt"),
                cell: (p) => (
                  <span className="font-mono font-bold text-ink">
                    {p.trans_id}
                  </span>
                ),
              },
              {
                key: "payer",
                header: t("customer"),
                cell: (p) => (
                  <div>
                    <div className="font-medium text-ink">
                      {p.payer_name || "—"}
                    </div>
                    <div className="font-mono text-xs text-muted">
                      {p.payer_msisdn_masked}
                    </div>
                  </div>
                ),
              },
              {
                key: "ref",
                header: t("account"),
                cell: (p) => (
                  <span className="font-mono text-xs text-muted">
                    {p.bill_ref || "—"}
                  </span>
                ),
              },
              {
                key: "status",
                header: t("statusHeader"),
                cell: (p) => {
                  const c = paymentChip(p.status);
                  return <StatusChip tone={c.tone}>{ts(c.key)}</StatusChip>;
                },
              },
              {
                key: "amount",
                header: t("amountHeader"),
                numeric: true,
                cell: (p) => <Money cents={p.amount_cents} bare />,
              },
              {
                key: "invoice",
                header: t("viewInvoice"),
                cell: (p) =>
                  p.invoice_id ? (
                    <Link
                      href={`/invoices/${p.invoice_id}`}
                      onClick={(e) => e.stopPropagation()}
                      className="inline-flex items-center gap-1 text-xs font-semibold text-leaf hover:underline active:opacity-80"
                    >
                      <span>{t("invoiceAction")}</span>
                      <ArrowRight className="h-3 w-3" />
                    </Link>
                  ) : p.status === "unmatched" ? (
                    <span className="inline-flex items-center gap-1 text-xs font-semibold text-ochre">
                      <span>{t("matchAction")}</span>
                      <ArrowRight className="h-3 w-3" />
                    </span>
                  ) : (
                    <span className="text-xs text-muted">{"—"}</span>
                  ),
              },
            ]}
          />
        )}
      </div>

      <PaymentDetailSheet payment={selected} onClose={() => setSelected(null)} />
    </>
  );
}
