"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { Search, X } from "lucide-react";
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

type StateFilter = "all" | Schemas["InvoiceState"];
type KindFilter = "ALL" | "INVOICE" | "CREDIT_NOTE";

export default function InvoicesPage() {
  const t = useTranslations("invoices");
  const ts = useTranslations("status");
  const tc = useTranslations("common");
  const router = useRouter();

  const [stateFilter, setStateFilter] = useState<StateFilter>("all");
  const [kindFilter, setKindFilter] = useState<KindFilter>("ALL");
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");

  useEffect(() => {
    const handler = setTimeout(() => {
      setDebouncedSearch(search.trim());
    }, 300);
    return () => clearTimeout(handler);
  }, [search]);

  const { data, isPending } = useInvoices({
    state: stateFilter === "all" ? undefined : stateFilter,
    kind: kindFilter === "ALL" ? undefined : kindFilter,
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

      {/* Primary Kind Filter Tabs: All, Sales, Credit Notes */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <Tabs<KindFilter>
          ariaLabel="Document Type"
          value={kindFilter}
          onChange={setKindFilter}
          items={[
            { value: "ALL", label: t("tabAll") },
            { value: "INVOICE", label: t("tabInvoices") },
            { value: "CREDIT_NOTE", label: t("tabCreditNotes") },
          ]}
        />

        {/* State Filter Tabs: All, KRA verified, Pending, Needs Review */}
        <div className="overflow-x-auto">
          <Tabs<StateFilter>
            ariaLabel="Invoice State"
            value={stateFilter}
            onChange={setStateFilter}
            items={[
              { value: "all", label: t("filterAll") },
              { value: "ACKED", label: t("filterAcked") },
              { value: "QUEUED", label: t("filterPending") },
              { value: "NEEDS_REVIEW", label: t("filterReview") },
            ]}
          />
        </div>
      </div>

      <div className="mt-4">
        {isPending ? (
          <LoadingRows rows={6} label={tc("loading")} />
        ) : rows.length === 0 ? (
          <EmptyState>
            {debouncedSearch
              ? `No documents matching "${debouncedSearch}"`
              : t("empty")}
          </EmptyState>
        ) : (
          <DataTable
            rows={rows}
            rowKey={(i) => i.id}
            onRowClick={(i) => router.push(`/invoices/${i.id}`)}
            primary={(i) => (
              <span className="flex items-center gap-2">
                <span className="font-mono">
                  {i.kra_invoice_no ?? (
                    <span className="text-muted">{t("noKraNo")}</span>
                  )}
                </span>
                {i.kind === "CREDIT_NOTE" && (
                  <StatusChip tone="pending">{t("creditNote")}</StatusChip>
                )}
              </span>
            )}
            secondary={(i) => {
              const c = invoiceChip(i.state);
              return (
                <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <span className="font-mono">{formatDateTime(i.created_at)}</span>
                  <StatusChip tone={c.tone}>{ts(c.key)}</StatusChip>
                </span>
              );
            }}
            trailing={(i) => (
              <span
                className={
                  i.kind === "CREDIT_NOTE"
                    ? "font-semibold text-danger"
                    : "font-semibold text-foreground"
                }
              >
                <Money cents={i.total_cents} />
              </span>
            )}
            columns={[
              {
                key: "when",
                header: "Date",
                cell: (i) => (
                  <span className="font-mono">{formatDateTime(i.created_at)}</span>
                ),
              },
              {
                key: "kra",
                header: t("kraNo"),
                cell: (i) => (
                  <span className="flex items-center gap-2">
                    <span className="font-mono">{i.kra_invoice_no ?? "—"}</span>
                    {i.kind === "CREDIT_NOTE" && (
                      <span className="rounded bg-amber-100 px-1.5 py-0.5 text-xs font-semibold text-amber-800 dark:bg-amber-950 dark:text-amber-200">
                        CN
                      </span>
                    )}
                  </span>
                ),
              },
              {
                key: "buyer",
                header: "Buyer / Customer",
                cell: (i) => (
                  <div className="flex flex-col">
                    {i.buyer_name && (
                      <span className="text-sm font-medium">{i.buyer_name}</span>
                    )}
                    <span className="font-mono text-xs text-muted">
                      {i.buyer_pin_masked ?? i.buyer_msisdn_masked ?? "—"}
                    </span>
                  </div>
                ),
              },
              {
                key: "state",
                header: "State",
                cell: (i) => {
                  const c = invoiceChip(i.state);
                  return <StatusChip tone={c.tone}>{ts(c.key)}</StatusChip>;
                },
              },
              {
                key: "total",
                header: "KES",
                numeric: true,
                cell: (i) => (
                  <span
                    className={
                      i.kind === "CREDIT_NOTE"
                        ? "font-semibold text-danger"
                        : "font-semibold text-foreground"
                    }
                  >
                    <Money cents={i.total_cents} bare />
                  </span>
                ),
              },
            ]}
          />
        )}
      </div>
    </>
  );
}
