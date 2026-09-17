"use client";

import { Suspense } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/Button";
import { useCurrentOrg } from "@/lib/api/queries";

/**
 * The pre-filled, printable *CiftPay Safaricom Authorization Letter*
 * (ADR-0008). Opened from AuthorizationStep in a new tab; what CiftPay
 * already knows (business name, masked KRA PIN, the short code) is filled
 * in, everything only the merchant has (signatory, ID number, signature,
 * stamp, date) is left blank for them to complete by hand before signing.
 */
export default function AuthorizationLetterPage() {
  return (
    <Suspense>
      <Letter />
    </Suspense>
  );
}

function Letter() {
  const t = useTranslations("onboarding.letter");
  const params = useSearchParams();
  const shortcode = params.get("shortcode") ?? "";
  const label = params.get("label") ?? "";
  const { data: org } = useCurrentOrg();

  return (
    <div className="mx-auto max-w-2xl print:max-w-none">
      <div className="mb-6 flex items-center justify-between print:hidden">
        <Link href="/onboarding" className="text-sm font-medium text-ink-2 underline underline-offset-2">
          {t("backLink")}
        </Link>
        <Button size="sm" onClick={() => window.print()}>
          {t("print")}
        </Button>
      </div>

      <article className="space-y-6 rounded-r2 border border-hairline bg-paper p-6 font-mono text-sm leading-relaxed print:border-0 print:p-0">
        <h1 className="text-base font-semibold">{t("title")}</h1>
        <p>{t("to")}</p>
        <p>{t("subject", { shortcode: shortcode || "__________" })}</p>

        <p>{t("body1", { shortcode: shortcode || "__________", label: label ? ` (${label})` : "" })}</p>
        <p>{t("body2")}</p>

        <dl className="space-y-3 border-t border-hairline pt-4">
          <Row label={t("business")} value={org?.name} />
          <Row label={t("pin")} value={org?.kra_pin_masked} />
          <Row label={t("signatory")} />
          <Row label={t("idNumber")} />
        </dl>

        <dl className="space-y-6 border-t border-hairline pt-6">
          <Row label={t("signature")} blank />
          <Row label={t("stamp")} blank />
          <Row label={t("date")} blank />
        </dl>

        <p className="text-xs text-muted print:hidden">{t("fillHint")}</p>
      </article>
    </div>
  );
}

function Row({ label, value, blank }: { label: string; value?: string | null; blank?: boolean }) {
  return (
    <div className="flex items-baseline gap-3">
      <dt className="w-40 shrink-0 text-ink-2">{label}:</dt>
      <dd className={blank ? "flex-1 border-b border-ink" : "flex-1"}>{blank ? "\u00a0" : value || "__________"}</dd>
    </div>
  );
}
