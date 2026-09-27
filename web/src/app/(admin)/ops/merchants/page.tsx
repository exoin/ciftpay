"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { rawGet, rawPatch, rawPost } from "@/lib/api/client";

type OrgRow = {
  id: string;
  name: string;
  status: string;
  daraja_status: string;
  vat_registered: boolean;
  kra_initialized: boolean;
  tier: "hustler" | "duka" | "biashara" | "accountant" | string;
  created_at: string;
};

export default function MerchantCRMPage() {
  const qc = useQueryClient();
  const [search, setSearch] = useState("");
  const [notifyOrg, setNotifyOrg] = useState<OrgRow | null>(null);
  const [notifyMessage, setNotifyMessage] = useState("");
  const [notifyChannel, setNotifyChannel] = useState<"sms" | "email">("sms");
  const [banner, setBanner] = useState<{ type: "success" | "error"; text: string } | null>(null);

  const orgsQuery = useQuery({
    queryKey: ["admin", "orgs"],
    queryFn: () => rawGet<{ data: OrgRow[] }>("/admin/orgs?limit=250"),
  });

  const updateTierMut = useMutation({
    mutationFn: ({ id, tier }: { id: string; tier: string }) =>
      rawPatch<{ data: OrgRow }>(`/admin/orgs/${id}`, { tier }),
    onSuccess: (_, vars) => {
      setBanner({ type: "success", text: `Merchant tier updated to ${vars.tier}` });
      qc.invalidateQueries({ queryKey: ["admin", "orgs"] });
      setTimeout(() => setBanner(null), 3500);
    },
    onError: (err: Error) => {
      setBanner({ type: "error", text: `Failed to update tier: ${err.message}` });
    },
  });

  const notifyMut = useMutation({
    mutationFn: ({ id, message, channel }: { id: string; message: string; channel: string }) =>
      rawPost(`/admin/orgs/${id}/notify`, { message, channel }),
    onSuccess: () => {
      setBanner({ type: "success", text: `Notification dispatched successfully via ${notifyChannel.toUpperCase()}` });
      setNotifyOrg(null);
      setNotifyMessage("");
      setTimeout(() => setBanner(null), 3500);
    },
    onError: (err: Error) => {
      alert(`Notification error: ${err.message}`);
    },
  });

  const orgs = (orgsQuery.data?.data || []).filter((o) => {
    if (!search.trim()) return true;
    const q = search.toLowerCase();
    return o.name.toLowerCase().includes(q) || o.id.toLowerCase().includes(q);
  });

  return (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-gray-200 pb-4 mb-6">
        <div>
          <h1 className="text-xl font-bold text-gray-900">Merchant CRM & Tier Entitlements</h1>
          <p className="text-sm text-gray-500">
            Manage merchant subscription plans, monitor KRA registration, and send direct operator broadcasts.
          </p>
        </div>
        <div>
          <input
            type="text"
            placeholder="Search by company name or ID..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-72 rounded-md border border-gray-300 px-3 py-1.5 text-sm text-gray-900 placeholder-gray-400 focus:border-gray-500 focus:outline-hidden"
          />
        </div>
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

      {/* CRM Table */}
      <div className="overflow-x-auto border border-gray-200 rounded-lg">
        <table className="min-w-full divide-y divide-gray-200 text-left text-sm">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-4 py-3 font-semibold text-gray-900">Merchant Name</th>
              <th className="px-4 py-3 font-semibold text-gray-900">Account Status</th>
              <th className="px-4 py-3 font-semibold text-gray-900">Subscription Tier</th>
              <th className="px-4 py-3 font-semibold text-gray-900">KRA Initialization</th>
              <th className="px-4 py-3 font-semibold text-gray-900">Daraja Status</th>
              <th className="px-4 py-3 font-semibold text-gray-900 text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 bg-white">
            {orgsQuery.isLoading ? (
              <tr>
                <td colSpan={6} className="px-4 py-8 text-center text-gray-500">
                  Loading CRM data...
                </td>
              </tr>
            ) : orgs.length === 0 ? (
              <tr>
                <td colSpan={6} className="px-4 py-8 text-center text-gray-500">
                  No organizations found matching search criteria.
                </td>
              </tr>
            ) : (
              orgs.map((org) => (
                <tr key={org.id} className="hover:bg-gray-50">
                  <td className="px-4 py-3 font-medium text-gray-900">
                    <div>{org.name}</div>
                    <div className="text-xs text-gray-500 font-mono">{org.id}</div>
                  </td>
                  <td className="px-4 py-3">
                    <span
                      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-semibold ${
                        org.status === "active"
                          ? "bg-emerald-100 text-emerald-800"
                          : "bg-gray-100 text-gray-700"
                      }`}
                    >
                      {org.status}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    <select
                      value={org.tier || "hustler"}
                      onChange={(e) => updateTierMut.mutate({ id: org.id, tier: e.target.value })}
                      disabled={updateTierMut.isPending}
                      className="rounded border border-gray-300 bg-white px-2.5 py-1 text-xs font-medium text-gray-800 focus:border-gray-500 focus:outline-hidden"
                    >
                      <option value="hustler">Hustler (Free / Pay-as-you-go)</option>
                      <option value="duka">Duka (Basic Retail)</option>
                      <option value="biashara">Biashara (Commercial / Automated)</option>
                      <option value="accountant">Accountant Multi-Org</option>
                    </select>
                  </td>
                  <td className="px-4 py-3">
                    {org.kra_initialized ? (
                      <span className="inline-flex items-center rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700">
                        ✓ Etims Ready
                      </span>
                    ) : (
                      <span className="inline-flex items-center rounded-full bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700">
                        ⚠ Unconfigured
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-xs text-gray-600 font-mono">
                    {org.daraja_status}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <button
                      type="button"
                      onClick={() => {
                        setNotifyOrg(org);
                        setNotifyMessage("");
                      }}
                      className="rounded-md border border-gray-300 bg-white px-3 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50"
                    >
                      💬 Notify
                    </button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Notify Merchant Modal */}
      {notifyOrg && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="w-full max-w-lg rounded-lg bg-white p-6 shadow-xl">
            <div className="flex items-center justify-between border-b pb-3 mb-4">
              <div>
                <h2 className="text-lg font-bold text-gray-900">Notify Merchant Owner</h2>
                <p className="text-xs text-gray-500">{notifyOrg.name} ({notifyOrg.id})</p>
              </div>
              <button
                type="button"
                onClick={() => setNotifyOrg(null)}
                className="text-gray-400 hover:text-gray-600 font-bold"
              >
                ✕
              </button>
            </div>

            <div className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-gray-700 mb-1">Dispatch Channel</label>
                <div className="flex gap-4 text-sm">
                  <label className="flex items-center gap-1.5 cursor-pointer">
                    <input
                      type="radio"
                      name="channel"
                      value="sms"
                      checked={notifyChannel === "sms"}
                      onChange={() => setNotifyChannel("sms")}
                    />
                    <span>SMS (Africa&apos;s Talking)</span>
                  </label>
                  <label className="flex items-center gap-1.5 cursor-pointer">
                    <input
                      type="radio"
                      name="channel"
                      value="email"
                      checked={notifyChannel === "email"}
                      onChange={() => setNotifyChannel("email")}
                    />
                    <span>Email Broadcast</span>
                  </label>
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-gray-700 mb-1">
                  Message Content
                </label>
                <textarea
                  rows={4}
                  value={notifyMessage}
                  onChange={(e) => setNotifyMessage(e.target.value)}
                  placeholder="e.g. Action required: Please sign and upload your Daraja authorization letter..."
                  className="w-full rounded-md border border-gray-300 p-2.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-hidden"
                />
              </div>
            </div>

            <div className="mt-6 flex justify-end gap-3">
              <button
                type="button"
                onClick={() => setNotifyOrg(null)}
                className="rounded border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
              >
                Cancel
              </button>
              <button
                type="button"
                disabled={notifyMut.isPending || !notifyMessage.trim()}
                onClick={() =>
                  notifyMut.mutate({
                    id: notifyOrg.id,
                    message: notifyMessage.trim(),
                    channel: notifyChannel,
                  })
                }
                className="rounded bg-emerald-700 px-4 py-2 text-sm font-semibold text-white shadow-xs hover:bg-emerald-800 disabled:opacity-50"
              >
                {notifyMut.isPending ? "Sending..." : "Send Message"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
