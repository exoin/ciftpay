"use client";

import { Suspense, useRef, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/Button";
import { useCurrentOrg } from "@/lib/api/queries";
import { exportElementToPdf } from "@/lib/pdf";

/**
 * The pre-filled *CiftPay Safaricom Authorization Letter* (ADR-0008).
 * Opened from AuthorizationStep in a new tab; what CiftPay already knows
 * (business name, unmasked KRA PIN, the short code, today's date) is filled in
 * cleanly with formal typography and official letterhead, ready for direct single-page PDF download.
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

  const letterRef = useRef<HTMLDivElement>(null);
  const [isExportingPdf, setIsExportingPdf] = useState(false);

  const todayDate = new Intl.DateTimeFormat("en-GB", {
    day: "numeric",
    month: "long",
    year: "numeric",
  }).format(new Date());

  const currentYear = new Date().getFullYear();
  const refCode = `CP/SAF-AUTH/${shortcode || "MPESA"}/${currentYear}`;

  async function handleDownloadPdf() {
    if (!letterRef.current || isExportingPdf) return;
    setIsExportingPdf(true);
    try {
      await exportElementToPdf(letterRef.current, {
        filename: `safaricom-authorization-letter-${shortcode || "mpesa"}.pdf`,
        format: "a4",
        scale: 3,
        captureWidth: 794,
        marginMm: 10,
      });
    } catch (err) {
      console.error("Failed to generate PDF:", err);
    } finally {
      setIsExportingPdf(false);
    }
  }

  return (
    <div className="w-full overflow-x-auto py-6 px-4">
      {/* Interactive Top Actions Bar */}
      <div className="w-[794px] min-w-[794px] max-w-[794px] mx-auto mb-6 flex flex-row items-center justify-between flex-nowrap">
        <Link
          href="/onboarding"
          className="text-sm font-medium text-ink-2 hover:text-ink underline underline-offset-2 shrink-0"
        >
          &larr; {t("backLink")}
        </Link>
        <div className="flex flex-row items-center gap-3 shrink-0 flex-nowrap">
          <Button
            size="sm"
            variant="primary"
            loading={isExportingPdf}
            onClick={handleDownloadPdf}
          >
            {isExportingPdf ? t("generatingPdf") : t("downloadPdf")}
          </Button>
        </div>
      </div>

      {/* Document Sheet - Strictly locked to 794px A4 canvas */}
      <div
        ref={letterRef}
        className="w-[794px] min-w-[794px] max-w-[794px] mx-auto bg-white text-ink border border-hairline rounded-r2 p-10 shadow-sm font-sans leading-normal box-border"
        style={{ color: "#14130F" }}
      >
        {/* Official CiftPay Letterhead & Branding */}
        <header className="border-b-2 border-ledger-green pb-3 mb-4">
          <div className="flex flex-row items-center justify-between flex-nowrap">
            <div className="flex flex-row items-center gap-3.5 flex-nowrap">
              {/* High-res CiftPay Monogram Logo */}
              <div className="h-10 w-10 shrink-0">
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  viewBox="0 0 512 512"
                  className="h-full w-full"
                  aria-hidden="true"
                >
                  <defs>
                    <mask id="perf-auth-letter">
                      <rect width="512" height="512" fill="#fff" />
                      <g fill="#000">
                        <circle cx="32" cy="0" r="14" />
                        <circle cx="96" cy="0" r="14" />
                        <circle cx="160" cy="0" r="14" />
                        <circle cx="224" cy="0" r="14" />
                        <circle cx="288" cy="0" r="14" />
                        <circle cx="352" cy="0" r="14" />
                        <circle cx="416" cy="0" r="14" />
                        <circle cx="480" cy="0" r="14" />
                        <circle cx="32" cy="512" r="14" />
                        <circle cx="96" cy="512" r="14" />
                        <circle cx="160" cy="512" r="14" />
                        <circle cx="224" cy="512" r="14" />
                        <circle cx="288" cy="512" r="14" />
                        <circle cx="352" cy="512" r="14" />
                        <circle cx="416" cy="512" r="14" />
                        <circle cx="480" cy="512" r="14" />
                      </g>
                    </mask>
                  </defs>
                  <rect width="512" height="512" fill="#F6F1E7" mask="url(#perf-auth-letter)" />
                  <rect x="40" y="40" width="432" height="432" rx="8" fill="#0B3D2E" mask="url(#perf-auth-letter)" />
                  <path
                    d="M338 194c-18-30-50-48-86-48-58 0-100 46-100 110s42 110 100 110c36 0 68-18 86-48"
                    fill="none"
                    stroke="#F6F1E7"
                    strokeWidth="44"
                    strokeLinecap="square"
                  />
                  <rect x="318" y="248" width="70" height="16" fill="#C97B12" />
                </svg>
              </div>

              <div>
                <div className="font-display text-lg font-bold tracking-tight text-ledger-green uppercase">
                  CiftPay
                </div>
                <div className="text-[10.5px] font-medium tracking-wide text-ink-2 uppercase whitespace-nowrap">
                  Automated M-Pesa &amp; KRA eTIMS Fiscalization Platform
                </div>
              </div>
            </div>

            <div className="text-right text-[11px] text-muted font-mono space-y-0.5 shrink-0 whitespace-nowrap">
              <p>Nairobi, Kenya</p>
              <p>support@ciftpay.co.ke</p>
              <p>www.ciftpay.co.ke</p>
            </div>
          </div>
        </header>

        {/* Reference & Date Header - Strict non-wrapping flex row */}
        <div className="flex flex-row items-center justify-between flex-nowrap text-xs font-mono mb-3.5">
          <div className="whitespace-nowrap">
            <span className="font-semibold text-ink-2">REF: </span>
            <span className="font-bold text-ink">{refCode}</span>
          </div>
          <div className="text-right whitespace-nowrap">
            <span className="font-semibold text-ink-2">DATE: </span>
            <span className="font-bold text-ink">{todayDate}</span>
          </div>
        </div>

        {/* Addressee Block */}
        <div className="mb-3.5 text-xs text-ink space-y-0.5">
          <p className="font-bold text-xs text-ink uppercase">To:</p>
          <p className="font-semibold">Safaricom PLC</p>
          <p className="font-semibold text-ledger-green">M-Pesa API Integration Team</p>
          <p className="text-muted">Safaricom House, Waiyaki Way &middot; P.O. Box 66827 - 00800, Nairobi, Kenya</p>
        </div>

        {/* Subject Header Banner */}
        <div className="mb-3.5 rounded border-l-4 border-ledger-green bg-paper-2 p-2.5 border border-hairline">
          <p className="text-[11px] font-bold uppercase tracking-wide text-ink">
            SUBJECT: FORMAL AUTHORIZATION TO MAP M-PESA SHORT CODE {shortcode || "__________"} TO CIFTPAY DARAJA GATEWAY
          </p>
        </div>

        {/* Formal Letter Body */}
        <div className="space-y-2.5 text-xs leading-relaxed text-ink text-justify mb-3.5">
          <p>
            Dear M-Pesa Integration Team,
          </p>
          <p>
            I, the undersigned authorized legal representative of{" "}
            <strong className="text-ink">{org?.name || "the Merchant Business"}</strong>, hereby
            formally authorize Safaricom PLC to link and map our Lipa na M-Pesa Short Code{" "}
            <strong className="font-mono font-bold text-ledger-green">
              {shortcode || "__________"}{label ? ` (${label})` : ""}
            </strong>{" "}
            to the <strong className="text-ink">CiftPay</strong> Daraja C2B integration gateway.
          </p>
          <p>
            This authorization allows CiftPay to receive real-time C2B payment validation and
            confirmation webhook notifications strictly on behalf of our registered business.
            These notifications are utilized solely to generate and record automated electronic
            tax invoices with the Kenya Revenue Authority (KRA eTIMS) in full compliance with
            Kenyan tax statutes.
          </p>
          <p className="text-ink-2">
            We confirm and understand that CiftPay operates strictly as a read-only payment notification
            and fiscal data processor. CiftPay does not hold, move, disburse, or exercise custody
            over our M-Pesa funds or settlement balances.
          </p>
        </div>

        {/* Dedicated Merchant Details Block - Strict 2-column table grid */}
        <div className="mb-3.5 border border-hairline rounded-r1 overflow-hidden bg-paper-2">
          <div className="bg-ledger-green/10 border-b border-hairline px-3.5 py-1.5 text-[10.5px] font-bold uppercase tracking-wider text-ledger-green">
            Merchant Business Particulars
          </div>
          <table className="w-full table-fixed text-xs font-mono border-collapse">
            <tbody>
              <tr className="border-b border-hairline/60">
                <td className="w-1/2 p-2.5 align-top border-r border-hairline/60">
                  <span className="block text-muted text-[9.5px] uppercase font-sans">Business Name</span>
                  <span className="block font-bold text-ink font-sans mt-0.5 truncate">{org?.name || "__________"}</span>
                </td>
                <td className="w-1/2 p-2.5 align-top">
                  <span className="block text-muted text-[9.5px] uppercase font-sans">Merchant KRA PIN</span>
                  <span className="block font-bold text-ink mt-0.5 whitespace-nowrap">{org?.kra_pin || org?.kra_pin_masked || "__________"}</span>
                </td>
              </tr>
              <tr className="border-b border-hairline/60">
                <td className="w-1/2 p-2.5 align-top border-r border-hairline/60">
                  <span className="block text-muted text-[9.5px] uppercase font-sans">M-Pesa Short Code</span>
                  <span className="block font-bold text-ledger-green mt-0.5 whitespace-nowrap">{shortcode || "__________"}</span>
                </td>
                <td className="w-1/2 p-2.5 align-top">
                  <span className="block text-muted text-[9.5px] uppercase font-sans">Classification</span>
                  <span className="block font-medium text-ink mt-0.5 truncate">{label || "Till (Buy Goods) / Paybill"}</span>
                </td>
              </tr>
              <tr>
                <td className="w-1/2 p-2.5 align-top border-r border-hairline/60">
                  <span className="block text-muted text-[9.5px] uppercase font-sans">Authorized Signatory</span>
                  <span className="block font-medium text-ink mt-0.5 border-b border-ink/40 pb-0.5">&nbsp;</span>
                </td>
                <td className="w-1/2 p-2.5 align-top">
                  <span className="block text-muted text-[9.5px] uppercase font-sans">National ID / Passport No.</span>
                  <span className="block font-medium text-ink mt-0.5 border-b border-ink/40 pb-0.5">&nbsp;</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        {/* Authorized Signatory & Official Stamp Block - Strict 2-column grid */}
        <div className="border-t border-hairline pt-3 mt-2.5">
          <p className="text-[10.5px] font-bold uppercase tracking-wider text-ink mb-2">
            Authorized Signatory &amp; Official Rubber Stamp
          </p>

          <div className="grid grid-cols-2 gap-8 items-end">
            <div className="space-y-2 text-xs font-mono">
              <div>
                <p className="text-[9.5px] text-muted uppercase font-sans mb-0.5">Signature:</p>
                <div className="border-b-2 border-ink h-7 w-full"></div>
              </div>
              <div className="flex flex-row items-baseline gap-2 flex-nowrap">
                <span className="text-muted w-24 shrink-0 font-sans">Full Name:</span>
                <span className="border-b border-ink/40 flex-1">&nbsp;</span>
              </div>
              <div className="flex flex-row items-baseline gap-2 flex-nowrap">
                <span className="text-muted w-24 shrink-0 font-sans">Designation:</span>
                <span className="font-semibold text-ink font-sans">Director / Proprietor</span>
              </div>
              <div className="flex flex-row items-baseline gap-2 flex-nowrap">
                <span className="text-muted w-24 shrink-0 font-sans">Date:</span>
                <span className="font-semibold text-ink font-sans whitespace-nowrap">{todayDate}</span>
              </div>
            </div>

            <div className="border-2 border-dashed border-hairline rounded p-3 flex flex-col items-center justify-center min-h-[105px] text-center bg-paper-2">
              <span className="text-xs font-bold text-muted uppercase tracking-wider">
                {t("stamp")}
              </span>
              <span className="text-[9.5px] text-muted mt-0.5">
                (Affix official business rubber stamp or seal here)
              </span>
            </div>
          </div>
        </div>

        {/* Small formal footer */}
        <footer className="mt-3.5 border-t border-hairline pt-2 flex flex-row items-center justify-between flex-nowrap text-[9.5px] text-muted font-mono">
          <span>CiftPay Automated eTIMS Fiscal Architecture</span>
          <span>Verified Merchant Integration Document</span>
        </footer>

        <p className="text-xs text-muted mt-4 border-t border-hairline pt-3">
          {t("fillHint")}
        </p>
      </div>
    </div>
  );
}
