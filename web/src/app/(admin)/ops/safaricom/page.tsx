"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiBaseUrl, rawGet, rawPatch, rawPost } from "@/lib/api/client";

type OrgRow = {
  id: string;
  name: string;
  status: string;
  daraja_status: "pending_upload" | "ready_for_safaricom" | "sent_to_safaricom" | "live";
  vat_registered: boolean;
  kra_initialized: boolean;
  tier: string;
  created_at: string;
};

type AdminShortcode = {
  id: string;
  org_id: string;
  org_name: string;
  kind: string;
  shortcode: string;
  status: string;
  authorization_letter_uploaded: boolean;
  created_at: string;
};

type SafaricomExportRow = {
  org_id: string;
  org_name: string;
  daraja_status: string;
  shortcode_id: string;
  shortcode: string;
  shortcode_kind: string;
  authorization_letter_url: string;
};

export default function SafaricomPipelinePage() {
  const qc = useQueryClient();
  const [filter, setFilter] = useState<string>("all");
  const [exportModalData, setExportModalData] = useState<SafaricomExportRow[] | null>(null);
  const [statusMessage, setStatusMessage] = useState<string | null>(null);

  const orgsQuery = useQuery({
    queryKey: ["admin", "orgs"],
    queryFn: () => rawGet<{ data: OrgRow[] }>("/admin/orgs?limit=200"),
  });

  const shortcodesQuery = useQuery({
    queryKey: ["admin", "shortcodes", "all"],
    queryFn: () => rawGet<{ data: AdminShortcode[] }>("/admin/shortcodes?limit=200"),
  });

  const updateStatusMut = useMutation({
    mutationFn: ({ id, daraja_status }: { id: string; daraja_status: string }) =>
      rawPatch<{ data: OrgRow }>(`/admin/orgs/${id}`, { daraja_status }),
    onSuccess: (_, vars) => {
      setStatusMessage(`Updated status to ${vars.daraja_status}`);
      qc.invalidateQueries({ queryKey: ["admin", "orgs"] });
      setTimeout(() => setStatusMessage(null), 3000);
    },
    onError: (err: Error) => {
      alert(`Failed to update Daraja status: ${err.message}`);
    },
  });

  const exportMut = useMutation({
    mutationFn: () => rawPost<{ data: SafaricomExportRow[] }>("/admin/orgs/batch-safaricom-export"),
    onSuccess: (res) => {
      setExportModalData(res.data || []);
    },
    onError: (err: Error) => {
      alert(`Export failed: ${err.message}`);
    },
  });

  const shortcodesByOrg = new Map<string, AdminShortcode>();
  (shortcodesQuery.data?.data || []).forEach((sc) => {
    if (!shortcodesByOrg.has(sc.org_id) || sc.authorization_letter_uploaded) {
      shortcodesByOrg.set(sc.org_id, sc);
    }
  });

  const orgs = (orgsQuery.data?.data || []).filter((o) => {
    if (filter === "all") return true;
    return o.daraja_status === filter;
  });

  const readyCount = (orgsQuery.data?.data || []).filter((o) => o.daraja_status === "ready_for_safaricom").length;

  return (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-gray-200 pb-4 mb-6">
        <div>
          <h1 className="text-xl font-bold text-gray-900">Safaricom Daraja Onboarding Pipeline</h1>
          <p className="text-sm text-gray-500">
            Track authorization letters, update compliance states, and generate Safaricom batch exports.
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={() => exportMut.mutate()}
            disabled={exportMut.isPending}
            className="rounded-md bg-emerald-700 px-4 py-2 text-sm font-semibold text-white shadow-xs hover:bg-emerald-800 disabled:opacity-50"
          >
            {exportMut.isPending ? "Generating..." : `Export Ready Merchants (${readyCount})`}
          </button>
        </div>
      </div>

      {statusMessage && (
        <div className="mb-4 rounded-md bg-emerald-50 border border-emerald-200 p-3 text-sm text-emerald-800">
          {statusMessage}
        </div>
      )}

      {/* Filter Tabs */}
      <div className="mb-4 flex flex-wrap gap-2">
        {[
          { id: "all", label: "All Merchants" },
          { id: "pending_upload", label: "Pending Upload" },
          { id: "ready_for_safaricom", label: "Ready for Safaricom" },
          { id: "sent_to_safaricom", label: "Sent to Safaricom" },
          { id: "live", label: "Live" },
        ].map((tab) => (
          <button
            key={tab.id}
            type="button"
            onClick={() => setFilter(tab.id)}
            className={`rounded-md px-3 py-1.5 text-xs font-semibold ${
              filter === tab.id
                ? "bg-gray-900 text-white"
                : "bg-gray-100 text-gray-700 hover:bg-gray-200"
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Table */}
      <div className="overflow-x-auto border border-gray-200 rounded-lg">
        <table className="min-w-full divide-y divide-gray-200 text-left text-sm">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-4 py-3 font-semibold text-gray-900">Company Name</th>
              <th className="px-4 py-3 font-semibold text-gray-900">Shortcode</th>
              <th className="px-4 py-3 font-semibold text-gray-900">Daraja Status</th>
              <th className="px-4 py-3 font-semibold text-gray-900">Authorization Letter</th>
              <th className="px-4 py-3 font-semibold text-gray-900">KRA Initialized</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 bg-white">
            {orgsQuery.isLoading ? (
              <tr>
                <td colSpan={5} className="px-4 py-8 text-center text-gray-500">
                  Loading merchants...
                </td>
              </tr>
            ) : orgs.length === 0 ? (
              <tr>
                <td colSpan={5} className="px-4 py-8 text-center text-gray-500">
                  No merchants found matching the filter.
                </td>
              </tr>
            ) : (
              orgs.map((org) => {
                const sc = shortcodesByOrg.get(org.id);
                return (
                  <tr key={org.id} className="hover:bg-gray-50">
                    <td className="px-4 py-3 font-medium text-gray-900">
                      <div>{org.name}</div>
                      <div className="text-xs text-gray-500">{org.id}</div>
                    </td>
                    <td className="px-4 py-3 text-gray-600">
                      {sc ? (
                        <span>
                          <span className="font-mono font-medium text-gray-800">{sc.shortcode}</span>{" "}
                          <span className="text-xs text-gray-500">({sc.kind})</span>
                        </span>
                      ) : (
                        <span className="text-xs text-gray-400">None assigned</span>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <select
                        value={org.daraja_status}
                        onChange={(e) =>
                          updateStatusMut.mutate({ id: org.id, daraja_status: e.target.value })
                        }
                        disabled={updateStatusMut.isPending}
                        className="rounded border border-gray-300 bg-white px-2.5 py-1 text-xs font-medium text-gray-800 focus:border-gray-500 focus:outline-hidden"
                      >
                        <option value="pending_upload">Pending Upload</option>
                        <option value="ready_for_safaricom">Ready for Safaricom</option>
                        <option value="sent_to_safaricom">Sent to Safaricom</option>
                        <option value="live">Live</option>
                      </select>
                    </td>
                    <td className="px-4 py-3">
                      {sc?.authorization_letter_uploaded ? (
                        <a
                          href={`${apiBaseUrl()}/admin/shortcodes/${sc.id}/authorization`}
                          target="_blank"
                          rel="noreferrer"
                          className="inline-flex items-center gap-1 rounded bg-blue-50 px-2.5 py-1 text-xs font-semibold text-blue-700 hover:bg-blue-100"
                        >
                          📄 View PDF
                        </a>
                      ) : (
                        <span className="text-xs text-amber-600 font-medium">No letter</span>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      {org.kra_initialized ? (
                        <span className="inline-flex items-center rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700">
                          Initialized
                        </span>
                      ) : (
                        <span className="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600">
                          Unconfigured
                        </span>
                      )}
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      {/* Export Results Modal */}
      {exportModalData && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="max-h-[90vh] w-full max-w-2xl overflow-y-auto rounded-lg bg-white p-6 shadow-xl">
            <div className="flex items-center justify-between border-b pb-3 mb-4">
              <h2 className="text-lg font-bold text-gray-900">
                Safaricom Export Batch ({exportModalData.length} Merchants)
              </h2>
              <button
                type="button"
                onClick={() => setExportModalData(null)}
                className="text-gray-400 hover:text-gray-600 font-bold"
              >
                ✕
              </button>
            </div>

            {exportModalData.length === 0 ? (
              <p className="text-sm text-gray-600">No merchants currently marked as ready_for_safaricom.</p>
            ) : (
              <div className="space-y-4">
                <div className="flex justify-end">
                  <button
                    type="button"
                    onClick={() => {
                      navigator.clipboard.writeText(JSON.stringify(exportModalData, null, 2));
                      alert("Export JSON copied to clipboard!");
                    }}
                    className="rounded bg-gray-100 px-3 py-1 text-xs font-medium text-gray-700 hover:bg-gray-200"
                  >
                    Copy JSON Payload
                  </button>
                </div>
                <div className="border rounded divide-y max-h-80 overflow-y-auto text-xs">
                  {exportModalData.map((item) => (
                    <div key={item.shortcode_id} className="p-3 flex items-center justify-between">
                      <div>
                        <div className="font-semibold text-gray-900">{item.org_name}</div>
                        <div className="text-gray-500 font-mono">
                          Shortcode: {item.shortcode} ({item.shortcode_kind})
                        </div>
                      </div>
                      <a
                        href={`${apiBaseUrl()}${item.authorization_letter_url}`}
                        target="_blank"
                        rel="noreferrer"
                        className="rounded bg-emerald-50 px-2.5 py-1 font-semibold text-emerald-700 hover:bg-emerald-100"
                      >
                        Download PDF Letter
                      </a>
                    </div>
                  ))}
                </div>
              </div>
            )}

            <div className="mt-6 flex justify-end">
              <button
                type="button"
                onClick={() => setExportModalData(null)}
                className="rounded bg-gray-900 px-4 py-2 text-sm font-semibold text-white hover:bg-gray-800"
              >
                Done
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
