"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { rawGet, rawPost } from "@/lib/api/client";
import { formatKES } from "@/lib/format";

type DLQInvoiceRow = {
  id: string;
  org_id: string;
  org_name: string;
  state: "NEEDS_REVIEW" | "FAILED_TERMINAL" | string;
  total_cents: number;
  receipt_code: string;
  last_error: string;
  created_at: string;
  updated_at: string;
  attempt?: number;
  submission_error?: string;
  raw_response?: unknown;
  classification?: string;
};

export default function KraDLQPage() {
  const qc = useQueryClient();
  const [selectedJson, setSelectedJson] = useState<{ id: string; data: unknown } | null>(null);
  const [forceAckInvoice, setForceAckInvoice] = useState<DLQInvoiceRow | null>(null);
  const [receiptNo, setReceiptNo] = useState("");
  const [receiptSign, setReceiptSign] = useState("");
  const [banner, setBanner] = useState<{ type: "success" | "error"; text: string } | null>(null);

  const dlqQuery = useQuery({
    queryKey: ["admin", "invoices", "dlq"],
    queryFn: () => rawGet<{ data: DLQInvoiceRow[] }>("/admin/invoices/dlq?limit=150"),
  });

  const requeueMut = useMutation({
    mutationFn: (id: string) => rawPost(`/admin/invoices/${id}/requeue`),
    onSuccess: () => {
      setBanner({ type: "success", text: "Invoice re-queued successfully for fiscal submission." });
      qc.invalidateQueries({ queryKey: ["admin", "invoices", "dlq"] });
      setTimeout(() => setBanner(null), 3500);
    },
    onError: (err: Error) => {
      setBanner({ type: "error", text: `Failed to re-queue: ${err.message}` });
    },
  });

  const forceAckMut = useMutation({
    mutationFn: ({ id, kra_receipt_no, kra_receipt_sign }: { id: string; kra_receipt_no: string; kra_receipt_sign: string }) =>
      rawPost(`/admin/invoices/${id}/force-acked`, { kra_receipt_no, kra_receipt_sign }),
    onSuccess: () => {
      setBanner({ type: "success", text: "Invoice manually transitioned to ACKED with valid signature." });
      setForceAckInvoice(null);
      setReceiptNo("");
      setReceiptSign("");
      qc.invalidateQueries({ queryKey: ["admin", "invoices", "dlq"] });
      setTimeout(() => setBanner(null), 3500);
    },
    onError: (err: Error) => {
      alert(`Force ACKED error: ${err.message}`);
    },
  });

  const invoices = dlqQuery.data?.data || [];

  return (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-gray-200 pb-4 mb-6">
        <div>
          <h1 className="text-xl font-bold text-gray-900">KRA Dead-Letter Queue (DLQ) & Overrides</h1>
          <p className="text-sm text-gray-500">
            Investigate upstream OSCU/eTIMS transmission failures, review raw error payloads, or manually resolve transactions.
          </p>
        </div>
        <button
          type="button"
          onClick={() => dlqQuery.refetch()}
          disabled={dlqQuery.isFetching}
          className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-xs font-semibold text-gray-700 hover:bg-gray-50"
        >
          {dlqQuery.isFetching ? "Refreshing..." : "↻ Refresh Queue"}
        </button>
      </div>

      {banner && (
        <div
          className={`mb-4 rounded-md p-3 text-sm border ${
            banner.type === "success"
              ? "bg-emerald-50 border-emerald-200 text-emerald-800"
              : "bg-red-50 border-red-200 text-red-800"
          }`}
        >
          {banner.text}
        </div>
      )}

      {/* Invoices DLQ Table */}
      <div className="overflow-x-auto border border-gray-200 rounded-lg">
        <table className="min-w-full divide-y divide-gray-200 text-left text-sm">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-4 py-3 font-semibold text-gray-900">Invoice / Org</th>
              <th className="px-4 py-3 font-semibold text-gray-900">State</th>
              <th className="px-4 py-3 font-semibold text-gray-900">Amount</th>
              <th className="px-4 py-3 font-semibold text-gray-900">Failure Reason</th>
              <th className="px-4 py-3 font-semibold text-gray-900">Attempts</th>
              <th className="px-4 py-3 font-semibold text-gray-900 text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 bg-white">
            {dlqQuery.isLoading ? (
              <tr>
                <td colSpan={6} className="px-4 py-8 text-center text-gray-500">
                  Loading DLQ entries...
                </td>
              </tr>
            ) : invoices.length === 0 ? (
              <tr>
                <td colSpan={6} className="px-4 py-8 text-center text-emerald-700 font-medium">
                  ✓ DLQ is clean. No stuck invoices requiring intervention.
                </td>
              </tr>
            ) : (
              invoices.map((inv) => (
                <tr key={inv.id} className="hover:bg-gray-50">
                  <td className="px-4 py-3">
                    <div className="font-mono font-medium text-gray-900">{inv.receipt_code || inv.id.slice(0, 8)}</div>
                    <div className="text-xs text-gray-500">{inv.org_name}</div>
                  </td>
                  <td className="px-4 py-3">
                    <span
                      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-semibold ${
                        inv.state === "FAILED_TERMINAL"
                          ? "bg-red-100 text-red-800"
                          : "bg-amber-100 text-amber-800"
                      }`}
                    >
                      {inv.state}
                    </span>
                  </td>
                  <td className="px-4 py-3 font-mono text-gray-900">
                    {formatKES(inv.total_cents)}
                  </td>
                  <td className="px-4 py-3 max-w-xs truncate text-xs text-red-600" title={inv.last_error || inv.submission_error || ""}>
                    {inv.last_error || inv.submission_error || "Unknown submission error"}
                  </td>
                  <td className="px-4 py-3 text-xs text-gray-600 font-mono">
                    {inv.attempt ?? 1}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-2">
                      <button
                        type="button"
                        onClick={() => setSelectedJson({ id: inv.id, data: inv.raw_response || inv.submission_error || inv.last_error })}
                        className="rounded border border-gray-300 bg-white px-2 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50"
                      >
                        {`{ } JSON`}
                      </button>
                      <button
                        type="button"
                        disabled={requeueMut.isPending}
                        onClick={() => requeueMut.mutate(inv.id)}
                        className="rounded bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-800 hover:bg-gray-200"
                      >
                        Re-Queue
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          setForceAckInvoice(inv);
                          setReceiptNo("");
                          setReceiptSign("");
                        }}
                        className="rounded bg-red-600 px-2.5 py-1 text-xs font-semibold text-white hover:bg-red-700"
                      >
                        Force ACKED
                      </button>
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Raw JSON Payload Modal */}
      {selectedJson && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="max-h-[85vh] w-full max-w-2xl overflow-y-auto rounded-lg bg-white p-6 shadow-xl">
            <div className="flex items-center justify-between border-b pb-3 mb-4">
              <h2 className="text-base font-bold text-gray-900">
                Raw Upstream KRA Response (Invoice {selectedJson.id})
              </h2>
              <button
                type="button"
                onClick={() => setSelectedJson(null)}
                className="text-gray-400 hover:text-gray-600 font-bold"
              >
                ✕
              </button>
            </div>

            <pre className="max-h-96 overflow-x-auto rounded bg-gray-900 p-4 font-mono text-xs text-emerald-400">
              {typeof selectedJson.data === "object"
                ? JSON.stringify(selectedJson.data, null, 2)
                : String(selectedJson.data || "No raw response payload recorded")}
            </pre>

            <div className="mt-6 flex justify-end">
              <button
                type="button"
                onClick={() => setSelectedJson(null)}
                className="rounded bg-gray-900 px-4 py-2 text-sm font-semibold text-white hover:bg-gray-800"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Force ACKED Modal */}
      {forceAckInvoice && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="w-full max-w-md rounded-lg bg-white p-6 shadow-xl border-t-4 border-red-600">
            <div className="flex items-center justify-between border-b pb-3 mb-4">
              <div>
                <h2 className="text-lg font-bold text-gray-900">Manual Override: Force ACKED</h2>
                <p className="text-xs text-gray-500">Invoice: {forceAckInvoice.id}</p>
              </div>
              <button
                type="button"
                onClick={() => setForceAckInvoice(null)}
                className="text-gray-400 hover:text-gray-600 font-bold"
              >
                ✕
              </button>
            </div>

            <p className="mb-4 text-xs text-red-700 bg-red-50 p-2.5 rounded border border-red-200">
              ⚠ Caution: This manually transitions the invoice to ACKED and permanently locks financial amounts.
              Provide the verified KRA Receipt Number and cryptographic signature from KRA support.
            </p>

            <div className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-gray-700 mb-1">
                  KRA Receipt Number (e.g. KRAMOVA220000000001)
                </label>
                <input
                  type="text"
                  value={receiptNo}
                  onChange={(e) => setReceiptNo(e.target.value)}
                  placeholder="KRA..."
                  className="w-full rounded-md border border-gray-300 p-2 text-sm font-mono text-gray-900 focus:border-red-500 focus:outline-hidden"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-gray-700 mb-1">
                  KRA Receipt Signature (Base64)
                </label>
                <textarea
                  rows={3}
                  value={receiptSign}
                  onChange={(e) => setReceiptSign(e.target.value)}
                  placeholder="Signature string from KRA..."
                  className="w-full rounded-md border border-gray-300 p-2 text-xs font-mono text-gray-900 focus:border-red-500 focus:outline-hidden"
                />
              </div>
            </div>

            <div className="mt-6 flex justify-end gap-3">
              <button
                type="button"
                onClick={() => setForceAckInvoice(null)}
                className="rounded border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
              >
                Cancel
              </button>
              <button
                type="button"
                disabled={forceAckMut.isPending || !receiptNo.trim() || !receiptSign.trim()}
                onClick={() =>
                  forceAckMut.mutate({
                    id: forceAckInvoice.id,
                    kra_receipt_no: receiptNo.trim(),
                    kra_receipt_sign: receiptSign.trim(),
                  })
                }
                className="rounded bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-700 disabled:opacity-50"
              >
                {forceAckMut.isPending ? "Confirming..." : "Confirm Force ACKED"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
