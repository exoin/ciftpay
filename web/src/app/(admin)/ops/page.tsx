"use client";

import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { TopBar } from "@/components/shell/TopBar";
import { EmptyState } from "@/components/ui/EmptyState";
import { LoadingRows } from "@/components/ui/LoadingRows";
import { StatusChip } from "@/components/ui/StatusChip";
import { rawGet } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";

type OrgRow = { id: string; name: string; status: string; vat_registered: boolean; created_at: string };
type DeadWebhook = { id: string; provider: string; kind: string; external_id: string; received_at: string; error: string | null };
type Flag = { key: string; enabled: boolean; org_allowlist: string[] };

/** Back-office (admin role): dead-letter webhooks, newest orgs, feature flags. Read-only in Phase 0. */
export default function OpsPage() {
  const t = useTranslations("admin");
  const tc = useTranslations("common");
  const dead = useQuery({ queryKey: ["admin", "dead"], queryFn: () => rawGet<{ data: DeadWebhook[] }>("/admin/webhooks/dead?limit=50") });
  const orgs = useQuery({ queryKey: ["admin", "orgs"], queryFn: () => rawGet<{ data: OrgRow[] }>("/admin/orgs?limit=50") });
  const flags = useQuery({ queryKey: ["admin", "flags"], queryFn: () => rawGet<{ data: Flag[] }>("/admin/flags") });

  return (
    <>
      <TopBar title={t("title")} />
      <p className="mb-6 text-sm text-ink-2">{t("lead")}</p>

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
