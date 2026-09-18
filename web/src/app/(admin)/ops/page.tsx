"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { TopBar } from "@/components/shell/TopBar";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { LoadingRows } from "@/components/ui/LoadingRows";
import { StatusChip } from "@/components/ui/StatusChip";
import { rawGet, rawPatch } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";

type AdminShortcode = {
  id: string;
  org_id: string;
  org_name: string;
  org_kra_pin_masked: string;
  kind: string;
  shortcode: string;
  label: string | null;
  status: "pending_authorization" | "verified" | "rejected";
  authorization_letter_uploaded: boolean;
  authorization_submitted_at: string | null;
  verified_at: string | null;
  reviewed_by: string | null;
  reviewed_at: string | null;
  rejection_reason: string | null;
  created_at: string;
};

type OrgRow = { id: string; name: string; status: string; vat_registered: boolean; created_at: string };
type DeadWebhook = { id: string; provider: string; kind: string; external_id: string; received_at: string; error: string | null };
type Flag = { key: string; enabled: boolean; org_allowlist: string[] };

/** Back-office (admin role): Safaricom letter queue, dead-letter webhooks, newest orgs, feature flags. */
export default function OpsPage() {
  const t = useTranslations("admin");
  const tc = useTranslations("common");
  const qc = useQueryClient();

  const [statusFilter, setStatusFilter] = useState<string>("pending_authorization");
  const [actionMsg, setActionMsg] = useState<string | null>(null);

  const shortcodes = useQuery({
    queryKey: ["admin", "shortcodes", statusFilter],
    queryFn: () => rawGet<{ data: AdminShortcode[] }>(`/admin/shortcodes?status=${statusFilter}&limit=50`),
  });

  const dead = useQuery({ queryKey: ["admin", "dead"], queryFn: () => rawGet<{ data: DeadWebhook[] }>("/admin/webhooks/dead?limit=50") });
  const orgs = useQuery({ queryKey: ["admin", "orgs"], queryFn: () => rawGet<{ data: OrgRow[] }>("/admin/orgs?limit=50") });
  const flags = useQuery({ queryKey: ["admin", "flags"], queryFn: () => rawGet<{ data: Flag[] }>("/admin/flags") });

  const verifyMut = useMutation({
    mutationFn: (id: string) => rawPatch(`/admin/shortcodes/${id}/verify`, { note: "Verified in ops dashboard" }),
    onSuccess: () => {
      setActionMsg(t("verifySuccess"));
      qc.invalidateQueries({ queryKey: ["admin", "shortcodes"] });
    },
  });

  const rejectMut = useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) => rawPatch(`/admin/shortcodes/${id}/reject`, { reason }),
    onSuccess: () => {
      setActionMsg(t("rejectSuccess"));
      qc.invalidateQueries({ queryKey: ["admin", "shortcodes"] });
    },
  });

  const handleReject = (id: string) => {
    const reason = window.prompt(t("rejectPrompt"));
    if (reason && reason.trim().length >= 3) {
      rejectMut.mutate({ id, reason: reason.trim() });
    }
  };

  return (
    <>
      <TopBar title={t("title")} />
      <p className="mb-6 text-sm text-ink-2">{t("lead")}</p>

      {/* M-Pesa Shortcodes Pending Authorization Queue (ADR-0008) */}
      <section className="mb-10 rounded-r2 border border-hairline bg-paper p-5">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-hairline pb-3">
          <div>
            <h2 className="text-base font-semibold text-ink">{t("shortcodesTitle")}</h2>
            <p className="text-xs text-muted">{t("shortcodesLead")}</p>
          </div>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={() => setStatusFilter("pending_authorization")}
              className={`rounded px-2.5 py-1 text-xs font-medium ${
                statusFilter === "pending_authorization" ? "bg-ink text-paper" : "bg-paper-2 text-ink-2"
              }`}
            >
              {t("pendingOnly")}
            </button>
            <button
              type="button"
              onClick={() => setStatusFilter("")}
              className={`rounded px-2.5 py-1 text-xs font-medium ${
                statusFilter === "" ? "bg-ink text-paper" : "bg-paper-2 text-ink-2"
              }`}
            >
              {t("allShortcodes")}
            </button>
          </div>
        </div>

        {actionMsg && (
          <div className="mt-3 rounded bg-green/10 p-2 text-xs font-medium text-green">
            {actionMsg}
          </div>
        )}

        <div className="mt-4">
          {shortcodes.isPending ? (
            <LoadingRows rows={3} label={tc("loading")} />
          ) : !shortcodes.data || shortcodes.data.data.length === 0 ? (
            <EmptyState>{t("emptyShortcodes")}</EmptyState>
          ) : (
            <ul className="divide-y divide-hairline">
              {shortcodes.data.data.map((sc) => (
                <li key={sc.id} className="flex flex-wrap items-center justify-between gap-4 py-3">
                  <div className="min-w-0 space-y-1">
                    <div className="flex items-center gap-2">
                      <span className="font-mono font-bold text-ink">{sc.shortcode}</span>
                      <span className="text-xs uppercase text-muted font-mono">({sc.kind})</span>
                      <StatusChip
                        tone={
                          sc.status === "verified"
                            ? "acked"
                            : sc.status === "rejected"
                            ? "failed"
                            : "pending"
                        }
                      >
                        {sc.status}
                      </StatusChip>
                    </div>
                    <div className="text-xs text-ink-2">
                      <span className="font-medium text-ink">{sc.org_name}</span> · PIN: {sc.org_kra_pin_masked}
                      {sc.label ? ` · ${sc.label}` : ""}
                    </div>
                    {sc.authorization_submitted_at && (
                      <div className="text-[11px] text-muted">
                        Submitted: {formatDateTime(sc.authorization_submitted_at)}
                      </div>
                    )}
                    {sc.rejection_reason && (
                      <div className="text-xs text-red">Rejection: {sc.rejection_reason}</div>
                    )}
                  </div>

                  <div className="flex flex-wrap items-center gap-2">
                    {sc.authorization_letter_uploaded ? (
                      <a
                        href={`/api/proxy/admin/shortcodes/${sc.id}/authorization`}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="rounded border border-hairline bg-paper-2 px-3 py-1.5 text-xs font-medium text-ink hover:bg-paper-3"
                      >
                        {t("viewLetterBtn")}
                      </a>
                    ) : (
                      <span className="text-xs text-muted">{t("noLetter")}</span>
                    )}

                    {sc.status !== "verified" && (
                      <Button
                        size="sm"
                        loading={verifyMut.isPending}
                        onClick={() => verifyMut.mutate(sc.id)}
                      >
                        {t("verifyBtn")}
                      </Button>
                    )}

                    {sc.status !== "rejected" && (
                      <Button
                        size="sm"
                        variant="secondary"
                        loading={rejectMut.isPending}
                        onClick={() => handleReject(sc.id)}
                      >
                        {t("rejectBtn")}
                      </Button>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </section>

      <section>
        <h2 className="border-b border-hairline pb-2">{t("deadLetter")}</h2>
        <div className="mt-3">
          {dead.isPending ? (
            <LoadingRows rows={3} label={tc("loading")} />
          ) : !dead.data || dead.data.data.length === 0 ? (
            <EmptyState>{t("empty")}</EmptyState>
          ) : (
            <ul className="ruled font-mono text-sm">
              {dead.data.data.map((w) => (
                <li key={w.id} className="flex min-h-12 flex-wrap items-center justify-between gap-2 py-2">
                  <span>
                    {w.provider}/{w.kind} · {w.external_id}
                  </span>
                  <span className="flex items-center gap-2 text-muted">
                    {formatDateTime(w.received_at)}
                    <StatusChip tone="failed">{w.error ?? "unprocessed"}</StatusChip>
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </section>

      <section className="mt-10">
        <h2 className="border-b border-hairline pb-2">{t("orgSearch")}</h2>
        <div className="mt-3">
          {orgs.isPending ? (
            <LoadingRows rows={3} label={tc("loading")} />
          ) : (
            <ul className="ruled text-sm">
              {orgs.data?.data.map((o) => (
                <li key={o.id} className="flex min-h-12 items-center justify-between gap-3 py-2">
                  <span className="min-w-0 truncate">{o.name}</span>
                  <span className="flex items-center gap-2 font-mono text-muted">
                    {formatDateTime(o.created_at)}
                    <StatusChip tone={o.status === "active" ? "acked" : "neutral"}>{o.status}</StatusChip>
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </section>

      <section className="mt-10">
        <h2 className="border-b border-hairline pb-2">{t("flags")}</h2>
        <ul className="ruled mt-3 font-mono text-sm">
          {flags.data?.data.map((f) => (
            <li key={f.key} className="flex min-h-12 items-center justify-between gap-3 py-2">
              <span>{f.key}</span>
              <StatusChip tone={f.enabled ? "acked" : "neutral"}>{f.enabled ? "on" : `${f.org_allowlist.length} orgs`}</StatusChip>
            </li>
          ))}
        </ul>
      </section>
    </>
  );
}
