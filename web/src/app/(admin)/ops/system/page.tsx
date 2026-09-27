"use client";

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { rawGet, rawPost } from "@/lib/api/client";

type WebhookRow = {
  id: string;
  provider: string;
  kind: string;
  external_id: string;
  payload: Record<string, unknown>;
  received_at: string;
  processed_at?: string | null;
  error?: string | null;
};

export default function SystemHealthPage() {
  const qc = useQueryClient();
  const [cidrText, setCidrText] = useState("");
  const [transIdQuery, setTransIdQuery] = useState("");
  const [activeSearch, setActiveSearch] = useState("");
  const [selectedPayload, setSelectedPayload] = useState<Record<string, unknown> | null>(null);
  const [banner, setBanner] = useState<{ type: "success" | "error"; text: string } | null>(null);

  // 1. Daraja IP Allowlist
  const ipQuery = useQuery({
    queryKey: ["admin", "system", "daraja-ips"],
    queryFn: () => rawGet<{ data: { cidrs: string[] } }>("/admin/system/daraja-ips"),
  });

  useEffect(() => {
    if (ipQuery.data?.data?.cidrs) {
      setCidrText(ipQuery.data.data.cidrs.join("\n"));
    }
  }, [ipQuery.data]);

  const updateIpsMut = useMutation({
    mutationFn: (cidrs: string[]) => rawPost("/admin/system/daraja-ips", cidrs),
    onSuccess: () => {
      setBanner({ type: "success", text: "Daraja dynamic CIDR allowlist updated in memory." });
      qc.invalidateQueries({ queryKey: ["admin", "system", "daraja-ips"] });
      setTimeout(() => setBanner(null), 3500);
    },
    onError: (err: Error) => {
      setBanner({ type: "error", text: `Failed to update IPs: ${err.message}` });
    },
  });

  // 2. Webhooks Inspector
  const webhooksQuery = useQuery({
    queryKey: ["admin", "webhooks", activeSearch],
    queryFn: () => {
      const q = activeSearch.trim() ? `?trans_id=${encodeURIComponent(activeSearch.trim())}` : "";
      return rawGet<{ data: WebhookRow[] }>(`/admin/webhooks${q}`);
    },
  });

  const replayMut = useMutation({
    mutationFn: (id: string) => rawPost(`/admin/webhooks/${id}/replay`),
    onSuccess: () => {
      setBanner({ type: "success", text: "Webhook payload replayed successfully through ingestion engine." });
      qc.invalidateQueries({ queryKey: ["admin", "webhooks"] });
      setTimeout(() => setBanner(null), 3500);
    },
    onError: (err: Error) => {
      alert(`Replay failed: ${err.message}`);
    },
  });

  const handleSaveCidrs = () => {
    const lines = cidrText
      .split("\n")
      .map((l) => l.trim())
      .filter((l) => l.length > 0);
    updateIpsMut.mutate(lines);
  };

  const webhooks = webhooksQuery.data?.data || [];

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-xl font-bold text-gray-900">Engineering Health & System Controls</h1>
        <p className="text-sm text-gray-500">
          Zero-downtime infrastructure operations, dynamic Safaricom IP management, and webhook replay debugging.
        </p>
      </div>

      {banner && (
        <div
          className={`rounded-md p-3 text-sm border ${
            banner.type === "success"
              ? "bg-emerald-50 border-emerald-200 text-emerald-800"
              : "bg-red-50 border-red-200 text-red-800"
          }`}
        >
          {banner.text}
        </div>
      )}

      {/* Section 1: Dynamic Daraja IP Allowlist */}
      <section className="rounded-lg border border-gray-200 p-5 bg-white shadow-xs">
        <div className="mb-4">
          <h2 className="text-base font-bold text-gray-900">Dynamic Safaricom Daraja IP Allowlist</h2>
          <p className="text-xs text-gray-500">
            CIDR blocks checked dynamically on incoming M-Pesa webhooks. Changes apply instantly in-memory without server restart.
          </p>
        </div>

        <div className="space-y-3">
          <textarea
            rows={5}
            value={cidrText}
            onChange={(e) => setCidrText(e.target.value)}
            placeholder="196.201.214.0/24&#10;196.201.213.0/24"
            className="w-full rounded-md border border-gray-300 p-3 font-mono text-xs text-gray-900 focus:border-gray-500 focus:outline-hidden"
          />

          <div className="flex items-center justify-between">
            <span className="text-xs text-gray-500">
              One CIDR notation network per line (e.g. 196.201.214.0/24)
            </span>
            <button
              type="button"
              disabled={updateIpsMut.isPending}
              onClick={handleSaveCidrs}
              className="rounded-md bg-gray-900 px-4 py-2 text-xs font-semibold text-white hover:bg-gray-800 disabled:opacity-50"
            >
              {updateIpsMut.isPending ? "Updating Allowlist..." : "Apply Dynamic Allowlist"}
            </button>
          </div>
        </div>
      </section>

      {/* Section 2: Webhook Inspector & Replay */}
      <section className="rounded-lg border border-gray-200 p-5 bg-white shadow-xs">
        <div className="flex flex-wrap items-center justify-between gap-4 mb-4">
          <div>
            <h2 className="text-base font-bold text-gray-900">Webhook Inspector & Ingest Replay</h2>
            <p className="text-xs text-gray-500">
              Query recent webhook receipts or search by Safaricom TransID to re-run ingestion through IngestC2B.
            </p>
          </div>

          <div className="flex items-center gap-2">
            <input
              type="text"
              placeholder="Search TransID (e.g. QK12345678)..."
              value={transIdQuery}
              onChange={(e) => setTransIdQuery(e.target.value)}
              className="rounded-md border border-gray-300 px-3 py-1.5 text-xs text-gray-900 placeholder-gray-400 focus:border-gray-500 focus:outline-hidden font-mono"
            />
            <button
              type="button"
              onClick={() => setActiveSearch(transIdQuery)}
              className="rounded-md bg-gray-900 px-3 py-1.5 text-xs font-semibold text-white hover:bg-gray-800"
            >
              Filter
            </button>
            {activeSearch && (
              <button
                type="button"
                onClick={() => {
                  setTransIdQuery("");
                  setActiveSearch("");
                }}
                className="rounded-md border border-gray-300 px-2 py-1.5 text-xs text-gray-600 hover:bg-gray-50"
              >
                Clear
              </button>
            )}
          </div>
        </div>

        {/* Webhooks Table */}
        <div className="overflow-x-auto border border-gray-200 rounded-lg">
          <table className="min-w-full divide-y divide-gray-200 text-left text-xs">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-3 py-2.5 font-semibold text-gray-900">Event ID / Kind</th>
                <th className="px-3 py-2.5 font-semibold text-gray-900">External ID / TransID</th>
                <th className="px-3 py-2.5 font-semibold text-gray-900">Received At</th>
                <th className="px-3 py-2.5 font-semibold text-gray-900">Status / Result</th>
                <th className="px-3 py-2.5 font-semibold text-gray-900 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 bg-white">
              {webhooksQuery.isLoading ? (
                <tr>
                  <td colSpan={5} className="px-3 py-8 text-center text-gray-500">
                    Loading webhooks...
                  </td>
                </tr>
              ) : webhooks.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-3 py-8 text-center text-gray-500">
                    No webhook events found.
                  </td>
                </tr>
              ) : (
                webhooks.map((event) => (
                  <tr key={event.id} className="hover:bg-gray-50">
                    <td className="px-3 py-2 font-mono">
                      <div className="font-semibold text-gray-900">{event.kind}</div>
                      <div className="text-[10px] text-gray-400">{event.id}</div>
                    </td>
                    <td className="px-3 py-2 font-mono text-gray-800 font-medium">
                      {event.external_id || String((event.payload as { TransID?: string })?.TransID || "-")}
                    </td>
                    <td className="px-3 py-2 text-gray-600">
                      {new Date(event.received_at).toLocaleTimeString([], {
                        hour: "2-digit",
                        minute: "2-digit",
                        second: "2-digit",
                      })}
                    </td>
                    <td className="px-3 py-2">
                      {event.error ? (
                        <span className="inline-flex items-center rounded bg-red-100 px-1.5 py-0.5 text-[10px] font-medium text-red-800 truncate max-w-xs">
                          {event.error}
                        </span>
                      ) : event.processed_at ? (
                        <span className="inline-flex items-center rounded bg-emerald-100 px-1.5 py-0.5 text-[10px] font-medium text-emerald-800">
                          Processed
                        </span>
                      ) : (
                        <span className="inline-flex items-center rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-700">
                          Pending
                        </span>
                      )}
                    </td>
                    <td className="px-3 py-2 text-right">
                      <div className="flex items-center justify-end gap-1.5">
                        <button
                          type="button"
                          onClick={() => setSelectedPayload(event.payload)}
                          className="rounded border border-gray-300 bg-white px-2 py-0.5 text-[11px] font-medium text-gray-700 hover:bg-gray-50"
                        >
                          Payload
                        </button>
                        <button
                          type="button"
                          disabled={replayMut.isPending}
                          onClick={() => replayMut.mutate(event.id)}
                          className="rounded bg-emerald-700 px-2 py-0.5 text-[11px] font-semibold text-white hover:bg-emerald-800 disabled:opacity-50"
                        >
                          Replay
                        </button>
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </section>

      {/* Raw Payload Modal */}
      {selectedPayload ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="max-h-[85vh] w-full max-w-xl overflow-y-auto rounded-lg bg-white p-5 shadow-xl">
            <div className="flex items-center justify-between border-b pb-3 mb-4">
              <h3 className="text-sm font-bold text-gray-900">Webhook Raw Ingestion Payload</h3>
              <button
                type="button"
                onClick={() => setSelectedPayload(null)}
                className="text-gray-400 hover:text-gray-600 font-bold"
              >
                ✕
              </button>
            </div>

            <pre className="max-h-96 overflow-x-auto rounded bg-gray-900 p-4 font-mono text-xs text-emerald-400">
              {JSON.stringify(selectedPayload, null, 2)}
            </pre>

            <div className="mt-5 flex justify-end">
              <button
                type="button"
                onClick={() => setSelectedPayload(null)}
                className="rounded bg-gray-900 px-3 py-1.5 text-xs font-semibold text-white hover:bg-gray-800"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
