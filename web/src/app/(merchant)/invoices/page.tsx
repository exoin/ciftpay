"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { Download, Search, X } from "lucide-react";
import { TopBar } from "@/components/shell/TopBar";
import { Button } from "@/components/ui/Button";
import { DataTable } from "@/components/ui/DataTable";
import { EmptyState } from "@/components/ui/EmptyState";
import { LoadingRows } from "@/components/ui/LoadingRows";
import { Money } from "@/components/ui/Money";
import { Sheet } from "@/components/ui/Sheet";
import { StatusChip } from "@/components/ui/StatusChip";
import { Tabs } from "@/components/ui/Tabs";
import { useToast } from "@/components/ui/Toast";
import { downloadItaxReport, useInvoices } from "@/lib/api/queries";
import type { Schemas } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import { invoiceChip } from "@/lib/status";

type StateFilter = "all" | Schemas["InvoiceState"];
type KindFilter = "ALL" | "INVOICE" | "CREDIT_NOTE";

export default function InvoicesPage() {
  const t = useTranslations("invoices");
  const ts = useTranslations("status");
  const tc = useTranslations("common");
  const toast = useToast();
  const router = useRouter();

  const [stateFilter, setStateFilter] = useState<StateFilter>("all");
  const [kindFilter, setKindFilter] = useState<KindFilter>("ALL");
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");

  // Export iTax CSV state
  const [exportOpen, setExportOpen] = useState(false);
  const [selectedMonth, setSelectedMonth] = useState(() => {
    const now = new Date();
    const y = now.getFullYear();
    const m = String(now.getMonth() + 1).padStart(2, "0");
    return `${y}-${m}`;
  });
  const [isExporting, setIsExporting] = useState(false);

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

  async function handleExport(e: React.FormEvent) {
    e.preventDefault();
    if (!selectedMonth) return;
    try {
      setIsExporting(true);
      await downloadItaxReport(selectedMonth);
      toast.push("iTax CSV export downloaded successfully.");
      setExportOpen(false);
    } catch (err: unknown) {
      toast.push(err instanceof Error ? err.message : "Failed to download iTax export", "error");
    } finally {
      setIsExporting(false);
    }
  }

  return (
    <>
      <TopBar
        title={t("title")}
        action={
          <Button size="sm" variant="secondary" onClick={() => setExportOpen(true)}>
            <span className="flex items-center gap-1.5">
              <Download className="size-4" />
              {t("exportItax")}
            </span>
          </Button>
        }
      />

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

      {/* Export iTax CSV Modal Sheet */}
      <Sheet
        open={exportOpen}
        onClose={() => setExportOpen(false)}
        title={t("exportDialogTitle")}
        closeLabel={tc("close")}
      >
        <form onSubmit={handleExport} className="space-y-4">
          <p className="text-sm text-ink-2 leading-relaxed">
            {t("exportDialogLead")}
          </p>
          <div>
            <label htmlFor="itax-month-select" className="block text-xs font-semibold uppercase text-muted mb-1.5">
              {t("selectMonth")}
            </label>
            <input
              id="itax-month-select"
              type="month"
              value={selectedMonth}
              onChange={(e) => setSelectedMonth(e.target.value)}
              required
              className="w-full rounded-r2 border border-hairline bg-paper px-3 py-2 font-mono text-sm text-ink focus:border-ochre focus:outline-none"
            />
          </div>

          <div className="rounded-r2 border border-hairline bg-paper-2 p-3 text-xs text-muted space-y-1">
            <p className="font-semibold text-ink">Included Columns:</p>
            <p className="font-mono">Date, Invoice/Receipt No, PIN of Purchaser, Total Amount, Taxable Amount, VAT Amount, eTIMS Class Code, Tax Category</p>
          </div>

          <div className="pt-2 flex justify-end gap-2">
            <Button
              type="button"
              variant="secondary"
              onClick={() => setExportOpen(false)}
            >
              {tc("cancel")}
            </Button>
            <Button
              type="submit"
              loading={isExporting}
            >
              <span className="flex items-center gap-1.5">
                <Download className="size-4" />
                {isExporting ? t("downloading") : t("downloadCsv")}
              </span>
            </Button>
          </div>
        </form>
      </Sheet>
    </>
  );
}
