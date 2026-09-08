"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { BusinessForm } from "@/components/onboarding/BusinessForm";
import { ShortcodeForm } from "@/components/onboarding/ShortcodeForm";
import { StepIndicator, type OnboardingStep } from "@/components/onboarding/StepIndicator";
import { VerifyShortcode } from "@/components/onboarding/VerifyShortcode";
import { Button } from "@/components/ui/Button";
import { LoadingRows } from "@/components/ui/LoadingRows";
import type { Schemas } from "@/lib/api/client";
import { useShortcodes } from "@/lib/api/queries";
import { readSession, useSession } from "@/lib/auth";

/**
 * login → business → shortcode → pay-KES-1 → /today. Needs a session (else
 * /login?next=/onboarding). Skips the business step when the user already has
 * an org, and leaves for /today when a verified shortcode already exists.
 */
export function OnboardingFlow() {
  const t = useTranslations("onboarding");
  const router = useRouter();
  const { data: session, isPending: sessionPending } = useSession();
  const [step, setStep] = useState<OnboardingStep | null>(null);
  const [shortcode, setShortcode] = useState<Schemas["Shortcode"] | null>(null);

  useEffect(() => {
    if (!readSession()) router.replace(`/login?next=${encodeURIComponent("/onboarding")}`);
  }, [router]);

  const hasOrg = Boolean(session && session.orgs.length > 0);
  const { data: shortcodes, isPending: shortcodesPending } = useShortcodes({ enabled: hasOrg });
  const shortcodesReady = !hasOrg || !shortcodesPending;

  // Decide the first step once we know what the account already has.
  useEffect(() => {
    if (step !== null || sessionPending || !session || !shortcodesReady) return;
    if (!hasOrg) {
      setStep("business");
      return;
    }
    const list = shortcodes?.data ?? [];
    if (list.some((s) => s.verified)) {
      router.replace("/today");
      return;
    }
    const unverified = list[0];
    if (unverified) {
      setShortcode(unverified);
      setStep("verify");
    } else {
      setStep("shortcode");
    }
  }, [step, sessionPending, session, hasOrg, shortcodes, shortcodesReady, router]);

  if (!session || step === null) {
    return <LoadingRows rows={3} />;
  }

  return (
    <>
      <StepIndicator current={step} />
      {step === "business" && <BusinessForm onCreated={() => setStep("shortcode")} />}
      {step === "shortcode" && (
        <div className="space-y-4">
          <ShortcodeForm
            onCreated={(sc) => {
              setShortcode(sc);
              setStep("verify");
            }}
          />
          <Button type="button" variant="ghost" block onClick={() => router.replace("/today")}>
            {t("later")}
          </Button>
        </div>
      )}
      {step === "verify" && shortcode && (
        <VerifyShortcode key={shortcode.id} shortcode={shortcode} onVerified={setShortcode} onDone={() => router.replace("/today")} onLater={() => router.replace("/today")} />
      )}
    </>
  );
}
