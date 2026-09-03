"use client";

import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { TopBar } from "@/components/shell/TopBar";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { LoadingRows } from "@/components/ui/LoadingRows";
import { StatusChip } from "@/components/ui/StatusChip";
import { useOrgs } from "@/lib/api/queries";
import { switchOrg } from "@/lib/auth";

/** Accountant portal: one row per client org; opening one switches the active org. */
export default function ClientsPage() {
  const t = useTranslations("accountant");
  const to = useTranslations("orgSwitcher");
  const tc = useTranslations("common");
  const router = useRouter();
  const qc = useQueryClient();
  const { data, isPending } = useOrgs();
  const clients = (data?.data ?? []).filter((o) => o.role === "accountant");

  return (
    <>
      <TopBar title={t("title")} />
      <p className="mb-4 text-sm text-ink-2">{t("lead")}</p>
      {isPending ? (
        <LoadingRows rows={4} label={tc("loading")} />
      ) : clients.length === 0 ? (
        <EmptyState>{t("empty")}</EmptyState>
      ) : (
        <ul className="ruled">
          {clients.map((o) => (
            <li key={o.org_id} className="flex min-h-[var(--row)] items-center justify-between gap-3 py-2">
              <span className="min-w-0">
                <span className="block truncate">{o.name}</span>
                <StatusChip tone="neutral">{to(`role.${o.role}`)}</StatusChip>
              </span>
              <Button
                size="sm"
                variant="secondary"
                onClick={() => {
                  switchOrg(qc, o.org_id);
                  router.push("/today");
                }}
              >
                {t("open")}
              </Button>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
