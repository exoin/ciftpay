"use client";

import { useState, type ReactNode } from "react";
import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { TopBar } from "@/components/shell/TopBar";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Sheet } from "@/components/ui/Sheet";
import { StatusChip } from "@/components/ui/StatusChip";
import { OrgSwitcher } from "@/components/shell/OrgSwitcher";
import { ShortcodeForm } from "@/components/onboarding/ShortcodeForm";
import { VerifyShortcode } from "@/components/onboarding/VerifyShortcode";
import { qk, useCurrentOrg, useShortcodes } from "@/lib/api/queries";
import { rawGet, type Schemas } from "@/lib/api/client";
import { useLogout } from "@/lib/auth";
import { setLocale } from "@/lib/i18n/actions";
import { cn } from "@/lib/cn";

export default function SettingsPage() {
  const t = useTranslations("settings");
  const tc = useTranslations("common");
  const ts = useTranslations("status");
  const locale = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const { data: org } = useCurrentOrg();
  const { data: shortcodes } = useShortcodes();
  // The add/verify sheet: `add` shows the form, `verify` the KES 1 control check for one row.
  const [sheet, setSheet] = useState<{ mode: "add" } | { mode: "verify"; shortcode: Schemas["Shortcode"] } | null>(null);
  const closeSheet = () => {
    setSheet(null);
    void qc.invalidateQueries({ queryKey: qk.shortcodes });
  };
  const { data: ent } = useQuery({
    queryKey: qk.entitlement,
    // Shape of internal/billing Entitlement (not yet in api/openapi.yaml; see local-dev.md contract debt).
    queryFn: () => rawGet<{ plan_code: string; plan_name: string; invoices_acked: number; invoice_cap: number | null }>("/billing/entitlement"),
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

        <Section
          title={t("shortcodes")}
          action={
            <Button size="sm" variant="secondary" onClick={() => setSheet({ mode: "add" })}>
              {t("addAction")}
            </Button>
          }
        >
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
                    <span className="block text-xs text-muted">{s.c2b_urls_registered_at ? t("c2bRegistered") : t("c2bNotRegistered")}</span>
                  </span>
                  <span className="flex shrink-0 items-center gap-2">
                    <StatusChip tone={s.verified ? "acked" : "pending"}>{s.verified ? ts("verified") : ts("unverified")}</StatusChip>
                    {!s.verified && (
                      <Button size="sm" variant="secondary" onClick={() => setSheet({ mode: "verify", shortcode: s })}>
                        {t("verifyAction")}
                      </Button>
                    )}
                  </span>
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
            {!ent
              ? "—"
              : ent.invoice_cap == null
                ? t("planUnlimited", { plan: ent.plan_name })
                : t("planLead", { plan: ent.plan_name, used: ent.invoices_acked, limit: ent.invoice_cap })}
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

      <Sheet open={sheet !== null} onClose={closeSheet} title={sheet?.mode === "verify" ? t("verifySheetTitle") : t("addSheetTitle")} closeLabel={tc("close")}>
        {sheet?.mode === "add" && <ShortcodeForm heading={false} onCreated={(sc) => setSheet({ mode: "verify", shortcode: sc })} />}
        {sheet?.mode === "verify" && (
          <VerifyShortcode
            key={sheet.shortcode.id}
            shortcode={sheet.shortcode}
            heading="compact"
            onVerified={() => void qc.invalidateQueries({ queryKey: qk.shortcodes })}
            onDone={closeSheet}
            doneLabel={t("done")}
          />
        )}
      </Sheet>
    </>
  );
}

function Section({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <section>
      <div className="flex items-end justify-between gap-3 border-b border-hairline pb-2">
        <h2>{title}</h2>
        {action}
      </div>
      <div className="mt-3">{children}</div>
    </section>
  );
}
