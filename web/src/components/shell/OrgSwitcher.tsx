"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { ChevronsUpDown } from "lucide-react";
import { Sheet } from "@/components/ui/Sheet";
import { StatusChip } from "@/components/ui/StatusChip";
import { switchOrg, useActiveMembership, useSession } from "@/lib/auth";
import { cn } from "@/lib/cn";

/** For accountants/admins: a Sheet listing orgs. Hidden when the user has one org. */
export function OrgSwitcher() {
  const t = useTranslations("orgSwitcher");
  const tc = useTranslations("common");
  const qc = useQueryClient();
  const { data: session } = useSession();
  const active = useActiveMembership();
  const [open, setOpen] = useState(false);

  if (!session || session.orgs.length < 2 || !active) return null;

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="flex min-h-[var(--touch)] w-full items-center justify-between gap-2 rounded-r2 border border-hairline bg-paper-2 px-3 text-left text-sm hover:bg-paper-3"
      >
        <span className="min-w-0">
          <span className="block truncate font-medium">{active.name}</span>
          <span className="block font-mono text-xs text-muted">{t(`role.${active.role}`)}</span>
        </span>
        <ChevronsUpDown size={16} strokeWidth={1.5} aria-hidden />
      </button>
      <Sheet open={open} onClose={() => setOpen(false)} title={t("title")} closeLabel={tc("close")}>
        <ul className="ruled -mx-4">
          {session.orgs.map((o) => {
            const cur = o.org_id === active.org_id;
            return (
              <li key={o.org_id}>
                <button
                  type="button"
                  onClick={() => {
                    switchOrg(qc, o.org_id);
                    setOpen(false);
                  }}
                  className={cn("flex min-h-[var(--row)] w-full items-center justify-between gap-3 px-4 text-left hover:bg-paper-3", cur && "bg-paper-3")}
                >
                  <span className="min-w-0">
                    <span className="block truncate">{o.name}</span>
                    <span className="block font-mono text-xs text-muted">{t(`role.${o.role}`)}</span>
                  </span>
                  {cur && <StatusChip tone="acked">{t("current")}</StatusChip>}
                </button>
              </li>
            );
          })}
        </ul>
      </Sheet>
    </>
  );
}
