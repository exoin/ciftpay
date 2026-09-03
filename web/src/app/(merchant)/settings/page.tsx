"use client";

import type { ReactNode } from "react";
import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { TopBar } from "@/components/shell/TopBar";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { StatusChip } from "@/components/ui/StatusChip";
import { OrgSwitcher } from "@/components/shell/OrgSwitcher";
import { qk, useCurrentOrg, useShortcodes } from "@/lib/api/queries";
import { rawGet } from "@/lib/api/client";
import { useLogout } from "@/lib/auth";
import { setLocale } from "@/lib/i18n/actions";
import { cn } from "@/lib/cn";

export default function SettingsPage() {
  const t = useTranslations("settings");
  const ts = useTranslations("status");
  const locale = useLocale();
  const router = useRouter();
  const { data: org } = useCurrentOrg();
  const { data: shortcodes } = useShortcodes();
  const { data: ent } = useQuery({
    queryKey: qk.entitlement,
    queryFn: () => rawGet<{ plan?: string; used?: number; limit?: number | null }>("/billing/entitlement"),
  });
  const logout = useLogout();

  return (
    <>
      <TopBar title={t("title")} />
      <div className="space-y-10">
        <Section title={t("business")}>
          <dl className="font-mono text-sm">
            <div className="flex justify-between gap-4 py-2">
              <dt className="text-muted">{t("business")}</dt>
              <dd>{org?.name ?? "—"}</dd>
            </div>
            <div className="flex justify-between gap-4 py-2">
              <dt className="text-muted">{t("kraPin")}</dt>
              <dd>{org?.kra_pin_masked ?? "—"}</dd>
            </div>
          </dl>
          <div className="lg:hidden">
            <OrgSwitcher />
          </div>
        </Section>

        <Section title={t("shortcodes")}>
          {!shortcodes || shortcodes.data.length === 0 ? (
            <EmptyState>{t("addShortcode")}</EmptyState>
          ) : (
            <ul className="ruled">
              {shortcodes.data.map((s) => (
                <li key={s.id} className="flex min-h-[var(--row)] items-center justify-between gap-3 py-2">
                  <span className="min-w-0">
                    <span className="block font-mono">{s.shortcode}</span>
                    <span className="block text-sm text-muted">
                      {s.kind}
                      {s.label ? ` · ${s.label}` : ""}
                    </span>
                  </span>
                  <StatusChip tone={s.verified ? "acked" : "pending"}>{s.verified ? ts("verified") : ts("unverified")}</StatusChip>
                </li>
              ))}
            </ul>
          )}
        </Section>

        <Section title={t("language")}>
          <div className="flex gap-2" role="radiogroup" aria-label={t("language")}>
            {(["en", "sw"] as const).map((l) => (
              <button
                key={l}
                type="button"
                role="radio"
                aria-checked={locale === l}
                onClick={async () => {
                  await setLocale(l);
                  router.refresh();
                }}
                className={cn("min-h-[var(--touch)] rounded-r2 border px-4 text-sm", locale === l ? "border-ink bg-paper-3" : "border-hairline bg-paper-2 hover:bg-paper-3")}
              >
                {l === "en" ? t("english") : t("swahili")}
              </button>
            ))}
          </div>
        </Section>

        <Section title={t("plan")}>
          <p className="font-mono text-sm">
            {ent?.limit == null
              ? t("planUnlimited", { plan: ent?.plan ?? "—" })
              : t("planLead", { plan: ent.plan ?? "—", used: ent.used ?? 0, limit: ent.limit })}
          </p>
        </Section>

        <Section title={t("receiptTemplate")}>
          <p className="text-sm text-ink-2">{t("receiptLead")}</p>
        </Section>

        <Section title={t("account")}>
          <Button variant="danger" onClick={() => logout.mutate()} loading={logout.isPending}>
            {t("signOut")}
          </Button>
        </Section>
      </div>
    </>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section>
      <h2 className="border-b border-hairline pb-2">{title}</h2>
      <div className="mt-3">{children}</div>
    </section>
  );
}
