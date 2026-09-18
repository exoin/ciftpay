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
 * already knows (business name, masked KRA PIN, the short code, today's date)
 * is filled in cleanly with formal typography, official letterhead, and ready for
 * high-resolution print or PDF export.
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

  const todayDate = new Intl.DateTimeFormat("en-GB", {
    day: "numeric",
    month: "long",
    year: "numeric",
  }).format(new Date());

  return (
    <div className="mx-auto max-w-2xl py-6 print:py-0 print:max-w-none">
      <div className="mb-6 flex items-center justify-between print:hidden">
        <Link href="/onboarding" className="text-sm font-medium text-ink-2 underline underline-offset-2">
          {t("backLink")}
        </Link>
        <div className="flex items-center gap-3">
          <Button size="sm" variant="secondary" onClick={() => window.print()}>
            {t("print")}
          </Button>
        </div>
      </div>

      <article className="space-y-6 rounded-r2 border border-hairline bg-paper p-8 shadow-sm print:shadow-none font-mono text-sm leading-relaxed print:border-0 print:p-0">
        {/* Formal Header & Letterhead */}
        <div className="border-b border-hairline pb-4 flex items-start justify-between">
          <div>
            <h1 className="text-lg font-bold tracking-tight text-ink font-sans">
              {t("title")}
            </h1>
            <p className="text-xs text-muted mt-1">Ref: CIFT-AUTH-{shortcode || "MPESA"}</p>
          </div>
          <div className="text-right text-xs text-muted font-mono">
            <p className="font-semibold text-ink">Date:</p>
            <p>{todayDate}</p>
          </div>
        </div>

        {/* Recipient */}
        <div className="text-ink space-y-0.5">
          <p className="font-bold">{t("to")}</p>
          <p className="text-xs text-ink-2">M-Pesa Business Operations &amp; Partner Integrations</p>
          <p className="text-xs text-ink-2">Nairobi, Kenya</p>
        </div>

        {/* Subject */}
        <div className="bg-paper-2 rounded p-3 border-l-2 border-ink">
          <p className="font-bold text-ink text-sm">
            {t("subject", { shortcode: shortcode || "__________" })}
          </p>
        </div>

        {/* Body Paragraphs */}
        <p className="text-justify text-ink">
          {t("body1", { shortcode: shortcode || "__________", label: label ? ` (${label})` : "" })}
        </p>
        <p className="text-justify text-ink-2">
          {t("body2")}
        </p>

        {/* Business Particulars */}
        <dl className="space-y-2 border-t border-hairline pt-4 text-xs">
          <Row label={t("business")} value={org?.name} />
          <Row label={t("pin")} value={org?.kra_pin_masked} />
          <Row label={t("signatory")} />
          <Row label={t("idNumber")} />
        </dl>

        {/* Signature & Stamp Authorization Block */}
        <div className="grid grid-cols-2 gap-6 border-t border-hairline pt-6">
          <div className="space-y-6">
            <Row label={t("signature")} blank />
            <Row label={t("date")} value={todayDate} />
          </div>
          <div className="border border-dashed border-hairline rounded p-4 flex flex-col items-center justify-center min-h-[100px] text-center">
            <span className="text-xs text-muted uppercase tracking-wider">{t("stamp")}</span>
            <span className="text-[10px] text-muted/60 mt-1">(Affix official business stamp here)</span>
          </div>
        </div>

        <p className="text-xs text-muted print:hidden border-t border-hairline pt-4">{t("fillHint")}</p>
      </article>
    </div>
  );
}

function Row({ label, value, blank }: { label: string; value?: string | null; blank?: boolean }) {
  return (
    <div className="flex items-baseline gap-3">
      <dt className="w-36 shrink-0 text-ink-2 font-medium">{label}:</dt>
      <dd className={blank ? "flex-1 border-b border-ink/40" : "flex-1 font-semibold text-ink"}>
        {blank ? "\u00a0" : value || "__________"}
      </dd>
    </div>
  );
}
