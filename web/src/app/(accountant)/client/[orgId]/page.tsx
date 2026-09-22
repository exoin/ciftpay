"use client";

import { useEffect, useState, use } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  Calendar,
  CheckCircle2,
  ChevronDown,
  Download,
  FileSpreadsheet,
  FileText,
  Search,
  ShieldAlert,
  ShieldCheck,
  TrendingUp,
  X,
} from "lucide-react";
import { Money } from "@/components/ui/Money";
import { Button } from "@/components/ui/Button";
import { useToast } from "@/components/ui/Toast";
import {
  useAccountantClients,
  useAnalyticsSummary,
  useCurrentOrg,
  useInvoices,
  useToday,
  downloadItaxReport,
} from "@/lib/api/queries";
import { switchOrg, getActiveOrgId } from "@/lib/auth";

export default function ClientWorkspacePage({
  params,
}: {
  params: Promise<{ orgId: string }>;
}) {
  const { orgId } = use(params);
  const router = useRouter();
  const qc = useQueryClient();
  const toast = useToast();

  // Active org synchronization
  useEffect(() => {
    if (orgId && getActiveOrgId() !== orgId) {
      switchOrg(qc, orgId);
    }
  }, [orgId, qc]);

  // Client Portfolio for quick switcher
  const { data: clientsData } = useAccountantClients();
  const clients = clientsData?.data ?? [];
  const currentClientMeta = clients.find((c) => c.org_id === orgId);

  // Queries scoped to active client
  const { data: orgData } = useCurrentOrg();
  const currentOrg = orgData;

  // Historical quarterly & annual VAT aggregates
  const [auditPeriod, setAuditPeriod] = useState<string>("month");
  const { data: summaryData, isPending: isSummaryPending } = useAnalyticsSummary(auditPeriod);
  const summary = summaryData;

  const { data: todayData } = useToday();
  const today = todayData;

  // iTax Export state
  const [exportMonth, setExportMonth] = useState(() => {
    const now = new Date();
    const y = now.getFullYear();
    const m = String(now.getMonth() + 1).padStart(2, "0");
    return `${y}-${m}`;
  });
  const [isExporting, setIsExporting] = useState(false);

  // Ledger Browser state with date-range filtering
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [kindFilter, setKindFilter] = useState<"ALL" | "INVOICE" | "CREDIT_NOTE">("ALL");
  const [dateFrom, setDateFrom] = useState<string>("");
  const [dateTo, setDateTo] = useState<string>("");

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedSearch(search.trim()), 300);
    return () => clearTimeout(timer);
  }, [search]);

  const { data: invoicesData, isPending: isInvoicesPending } = useInvoices({
    kind: kindFilter === "ALL" ? undefined : kindFilter,
    q: debouncedSearch || undefined,
    from: dateFrom || undefined,
    to: dateTo || undefined,
  });
  const invoices = invoicesData?.data ?? [];

  async function handleDownloadCsv(e: React.FormEvent) {
    e.preventDefault();
    if (!exportMonth) return;
    try {
      setIsExporting(true);
      await downloadItaxReport(exportMonth, orgId);
      toast.push(`iTax VAT Return CSV for ${exportMonth} downloaded successfully.`);
    } catch (err: unknown) {
      toast.push(err instanceof Error ? err.message : "Failed to download iTax CSV", "error");
    } finally {
      setIsExporting(false);
    }
  }

  const clientName = currentOrg?.name || currentClientMeta?.name || "Client Workspace";
  const kraPin = currentClientMeta?.kra_pin || currentOrg?.kra_pin_masked || "—";

  return (
    <div className="space-y-6">
      {/* Top Breadcrumb & Switcher Bar */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between border-b border-hairline pb-4">
        <div className="flex items-center gap-3">
          <Link
            href="/clients"
            className="flex items-center gap-1 text-xs font-semibold text-muted hover:text-ink transition-colors"
          >
            <ArrowLeft className="size-3.5" />
            All Clients
          </Link>
          <span className="text-muted">/</span>
          <div className="relative inline-block text-left">
            <select
              aria-label="Switch Client Workspace"
              value={orgId}
              onChange={(e) => {
                const targetId = e.target.value;
                if (targetId) {
                  switchOrg(qc, targetId);
                  router.push(`/client/${targetId}`);
                }
              }}
              className="appearance-none rounded-r2 border border-hairline bg-paper py-1 pl-3 pr-8 text-sm font-semibold text-ink shadow-sm focus:border-green focus:outline-none"
            >
              {clients.map((c) => (
                <option key={c.org_id} value={c.org_id}>
                  {c.name} ({c.kra_pin || "No PIN"})
                </option>
              ))}
            </select>
            <ChevronDown className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 size-3.5 text-muted" />
          </div>
        </div>

        <div className="flex items-center gap-2">
          <span className="text-xs text-muted">KRA PIN:</span>
          <span className="font-mono text-xs font-bold text-ink bg-paper-2 px-2 py-0.5 rounded border border-hairline">
            {kraPin}
          </span>
          <span className="inline-flex items-center gap-1 rounded-full bg-green/10 px-2 py-0.5 text-xs font-medium text-green border border-green/20">
            <ShieldCheck className="size-3" />
            Authorized Accountant
          </span>
        </div>
      </div>

      {/* Client Overview Card */}
      <div className="flex flex-col gap-4 rounded-r2 border border-hairline bg-paper p-5 shadow-sm sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="font-display text-2xl font-bold tracking-tight text-ink">
            {clientName}
          </h1>
          <p className="mt-1 text-xs text-ink-2">
            Workspace scoped to Client Org ID: <span className="font-mono">{orgId}</span>
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            variant="secondary"
            onClick={() => {
              const el = document.getElementById("itax-section");
              if (el) el.scrollIntoView({ behavior: "smooth" });
            }}
          >
            <Download className="mr-1.5 size-3.5" />
            iTax CSV Export
          </Button>
        </div>
      </div>

      {/* Grid: iTax Export Card & Reconciliation Audit View */}
      <div className="grid gap-6 md:grid-cols-2">
        {/* 1. iTax Export Card (The Killer Feature) */}
        <div id="itax-section" className="rounded-r2 border border-hairline bg-paper p-5 shadow-sm space-y-4">
          <div className="flex items-center gap-2 text-ink">
            <FileSpreadsheet className="size-5 text-green" />
            <h2 className="text-base font-semibold">KRA iTax VAT Return Export</h2>
          </div>
          <p className="text-xs text-ink-2 leading-relaxed">
            Generate and stream the official CSV file formatted for KRA iTax VAT monthly return submissions.
          </p>

          <form onSubmit={handleDownloadCsv} className="space-y-4 pt-1">
            <div>
              <label htmlFor="itax-month" className="block text-xs font-medium uppercase text-muted mb-1.5">
                Tax Filing Period (Month)
              </label>
              <div className="relative">
                <input
                  id="itax-month"
                  type="month"
                  value={exportMonth}
                  onChange={(e) => setExportMonth(e.target.value)}
                  required
                  className="w-full rounded-r2 border border-hairline bg-paper px-3 py-2 font-mono text-sm text-ink focus:border-green focus:outline-none"
                />
              </div>
            </div>

            <div className="rounded-r2 border border-hairline bg-paper-2 p-3 text-xs space-y-1">
              <span className="font-semibold text-ink">KRA Standard Formatted Columns:</span>
              <p className="font-mono text-[11px] text-muted leading-relaxed">
                Date, Invoice/Receipt No, PIN of Purchaser, Total Amount, Taxable Amount, VAT Amount, eTIMS Class Code, Tax Category
              </p>
            </div>

            <Button
              type="submit"
              block
              loading={isExporting}
              className="mt-2"
            >
              <Download className="mr-2 size-4" />
              {isExporting ? "Streaming iTax CSV..." : "Download iTax VAT Return CSV"}
            </Button>
          </form>
        </div>

        {/* 2. Reconciliation & Audit View */}
        <div className="rounded-r2 border border-hairline bg-paper p-5 shadow-sm space-y-4">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between border-b border-hairline pb-3">
            <div className="flex items-center gap-2 text-ink">
              <TrendingUp className="size-5 text-ochre" />
              <h2 className="text-base font-semibold">Ledger & Tax Audit</h2>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-xs text-muted">Period:</span>
              <select
                aria-label="Audit Period"
                value={auditPeriod}
                onChange={(e) => setAuditPeriod(e.target.value)}
                className="rounded-r2 border border-hairline bg-paper-2 px-2.5 py-1 text-xs font-semibold text-ink focus:border-green focus:outline-none"
              >
                <optgroup label="Standard Ranges">
                  <option value="month">Current Month</option>
                  <option value="quarter">Current Quarter</option>
                  <option value="year">Current Year (Annual)</option>
                </optgroup>
                <optgroup label="Quarterly Historical">
                  <option value="2026-Q1">2026 Q1 (Jan - Mar)</option>
                  <option value="2026-Q2">2026 Q2 (Apr - Jun)</option>
                  <option value="2026-Q3">2026 Q3 (Jul - Sep)</option>
                  <option value="2026-Q4">2026 Q4 (Oct - Dec)</option>
                </optgroup>
                <optgroup label="Annual Historical">
                  <option value="2025">2025 Full Year</option>
                </optgroup>
              </select>
            </div>
          </div>
          <p className="text-xs text-ink-2 leading-relaxed">
            Real-time aggregates calculated from PostgreSQL ledger transactions and fiscal acknowledgements for the selected period.
          </p>

          {isSummaryPending ? (
            <div className="py-8 text-center text-xs text-muted">Calculating monthly audit...</div>
          ) : (
            <div className="grid grid-cols-2 gap-3 pt-1">
              <div className="rounded-r2 border border-hairline bg-paper-2 p-3">
                <span className="text-xs text-muted block">M-Pesa Gross Volume</span>
                <span className="font-mono text-base font-bold text-ink">
                  <Money cents={summary?.gross_sales_cents ?? 0} />
                </span>
                <span className="text-[10px] text-muted block mt-0.5">
                  {summary?.payments_count ?? 0} total payments
                </span>
              </div>

              <div className="rounded-r2 border border-hairline bg-paper-2 p-3">
                <span className="text-xs text-muted block">Estimated VAT Liability</span>
                <span className="font-mono text-base font-bold text-danger">
                  <Money cents={summary?.vat_liability_cents ?? 0} />
                </span>
                <span className="text-[10px] text-muted block mt-0.5">
                  Standard 16% Output VAT
                </span>
              </div>

              <div className="rounded-r2 border border-hairline bg-paper-2 p-3">
                <span className="text-xs text-muted block">Daraja Processing Fees</span>
                <span className="font-mono text-sm font-semibold text-muted">
                  <Money cents={summary?.daraja_fees_cents ?? 0} />
                </span>
              </div>

              <div className="rounded-r2 border border-hairline bg-paper-2 p-3">
                <span className="text-xs text-muted block">Net M-Pesa Settlement</span>
                <span className="font-mono text-sm font-semibold text-green">
                  <Money cents={summary?.net_sales_cents ?? 0} />
                </span>
              </div>
            </div>
          )}

          {/* Sync Health Counters */}
          <div className="border-t border-hairline pt-3">
            <span className="text-xs font-semibold text-ink block mb-2">eTIMS Submission Health:</span>
            <div className="flex items-center gap-4 text-xs font-medium">
              <span className="flex items-center gap-1.5 text-green">
                <CheckCircle2 className="size-3.5" />
                <span>{today?.invoices_acked ?? 0} Acked</span>
              </span>
              <span className="flex items-center gap-1.5 text-ochre">
                <FileText className="size-3.5" />
                <span>{today?.invoices_pending ?? 0} Queued</span>
              </span>
              <span className="flex items-center gap-1.5 text-danger">
                <ShieldAlert className="size-3.5" />
                <span>{today?.attention_count ?? 0} Needs Review</span>
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* 3. Ledger Browser */}
      <section className="space-y-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 className="text-lg font-semibold text-ink">Client Transaction Ledger</h2>
            <p className="text-xs text-ink-2">
              Browse fiscal invoices, credit notes, eTIMS classification codes, and buyer PINs.
            </p>
          </div>

          {/* Kind Filter Tabs */}
          <div className="flex items-center gap-1 rounded-r2 border border-hairline bg-paper p-1 text-xs">
            {(["ALL", "INVOICE", "CREDIT_NOTE"] as const).map((k) => (
              <button
                key={k}
                type="button"
                onClick={() => setKindFilter(k)}
                className={`rounded px-2.5 py-1 font-medium transition-colors ${
                  kindFilter === k ? "bg-paper-2 text-ink font-semibold" : "text-muted hover:text-ink"
                }`}
              >
                {k === "ALL" ? "All Documents" : k === "INVOICE" ? "Invoices" : "Credit Notes"}
              </button>
            ))}
          </div>
        </div>

        {/* Filter Controls Bar (Search + Date Range) */}
        <div className="grid grid-cols-1 md:grid-cols-12 gap-3">
          {/* Universal Search Bar */}
          <div className="relative md:col-span-6">
            <Search className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted" />
            <input
              type="search"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search by receipt code, buyer PIN, or amount..."
              className="w-full rounded-r2 border border-hairline bg-paper py-2 pl-9 pr-9 text-sm text-ink placeholder:text-muted focus:border-green focus:outline-none"
            />
            {search && (
              <button
                type="button"
                onClick={() => setSearch("")}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-muted hover:text-ink"
              >
                <X className="size-4" />
              </button>
            )}
          </div>

          {/* Date Range Inputs */}
          <div className="flex items-center gap-2 md:col-span-6">
            <Calendar className="size-4 text-muted hidden sm:inline shrink-0" />
            <div className="flex items-center gap-1.5 flex-1">
              <span className="text-xs font-medium text-muted">From:</span>
              <input
                type="date"
                value={dateFrom}
                onChange={(e) => setDateFrom(e.target.value)}
                className="w-full rounded-r2 border border-hairline bg-paper px-2 py-1.5 font-mono text-xs text-ink focus:border-green focus:outline-none"
              />
            </div>
            <div className="flex items-center gap-1.5 flex-1">
              <span className="text-xs font-medium text-muted">To:</span>
              <input
                type="date"
                value={dateTo}
                onChange={(e) => setDateTo(e.target.value)}
                className="w-full rounded-r2 border border-hairline bg-paper px-2 py-1.5 font-mono text-xs text-ink focus:border-green focus:outline-none"
              />
            </div>
            {(dateFrom || dateTo) && (
              <button
                type="button"
                onClick={() => {
                  setDateFrom("");
                  setDateTo("");
                }}
                className="rounded border border-hairline bg-paper-2 px-2 py-1.5 text-xs text-muted hover:text-ink"
                title="Clear date range"
              >
                <X className="size-3.5" />
              </button>
            )}
          </div>
        </div>

        {/* Ledger Table */}
        {isInvoicesPending ? (
          <div className="rounded-r2 border border-hairline bg-paper p-8 text-center text-sm text-muted">
            Loading ledger transactions...
          </div>
        ) : invoices.length === 0 ? (
          <div className="rounded-r2 border border-hairline bg-paper p-8 text-center text-sm text-muted">
            No transactions found for this filter.
          </div>
        ) : (
          <div className="overflow-hidden rounded-r2 border border-hairline bg-paper shadow-sm">
            <div className="overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead className="border-b border-hairline bg-paper-2 text-xs font-semibold uppercase text-muted">
                  <tr>
                    <th scope="col" className="px-4 py-3 sm:px-6">Date</th>
                    <th scope="col" className="px-4 py-3">Receipt / Doc No</th>
                    <th scope="col" className="px-4 py-3">Buyer PIN</th>
                    <th scope="col" className="px-4 py-3 text-center">Class / Tax</th>
                    <th scope="col" className="px-4 py-3 text-right">Total Amount</th>
                    <th scope="col" className="px-4 py-3 text-right sm:px-6">Status</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-hairline">
                  {invoices.map((inv) => (
                    <tr key={inv.id} className="hover:bg-paper-2/40 transition-colors">
                      <td className="px-4 py-3 text-xs text-muted sm:px-6 whitespace-nowrap">
                        {new Date(inv.created_at).toLocaleDateString()} {new Date(inv.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                      </td>
                      <td className="px-4 py-3 font-mono text-xs font-semibold text-ink">
                        {inv.kra_invoice_no || inv.receipt_code || "—"}
                      </td>
                      <td className="px-4 py-3 font-mono text-xs text-ink-2">
                        {inv.buyer_pin_masked || inv.buyer_name || <span className="text-muted italic">Walk-in</span>}
                      </td>
                      <td className="px-4 py-3 text-center">
                        <span className="font-mono text-xs bg-paper-2 px-1.5 py-0.5 rounded border border-hairline text-ink">
                          {inv.kind === "CREDIT_NOTE" ? "CN (16%)" : "INV (16%)"}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-right font-mono font-medium">
                        <Money
                          cents={inv.total_cents}
                          className={inv.kind === "CREDIT_NOTE" ? "text-danger" : "text-ink"}
                        />
                      </td>
                      <td className="px-4 py-3 text-right sm:px-6">
                        <span
                          className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ${
                            inv.state === "ACKED"
                              ? "bg-green/10 text-green"
                              : inv.state === "QUEUED" || inv.state === "SUBMITTED"
                              ? "bg-ochre/10 text-ochre"
                              : "bg-danger/10 text-danger"
                          }`}
                        >
                          {inv.state === "ACKED" ? (
                            <CheckCircle2 className="size-3" />
                          ) : (
                            <FileText className="size-3" />
                          )}
                          {inv.state}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </section>
    </div>
  );
}
