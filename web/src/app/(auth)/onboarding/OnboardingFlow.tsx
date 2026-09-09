"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { AuthorizationStep } from "@/components/onboarding/AuthorizationStep";
import { BusinessForm } from "@/components/onboarding/BusinessForm";
import { ShortcodeForm } from "@/components/onboarding/ShortcodeForm";
import { StepIndicator, type OnboardingStep } from "@/components/onboarding/StepIndicator";
import { Button } from "@/components/ui/Button";
import { LoadingRows } from "@/components/ui/LoadingRows";
import type { Schemas } from "@/lib/api/client";
import { useShortcodes } from "@/lib/api/queries";
import { readSession, useSession } from "@/lib/auth";

/**
 * login → business → shortcode → authorization letter → /today. Needs a
 * session (else /login?next=/onboarding). Skips the business step when the
 * user already has an org, resumes at the letter step for a shortcode that is
 * still waiting for one, and leaves for /today once a letter is uploaded or a
 * shortcode is verified.
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
    if (list.length === 0) {
      setStep("shortcode");
      return;
    }
    const needsLetter = list.find((s) => !s.verified && !s.authorization_letter_uploaded);
    if (list.some((s) => s.verified) || !needsLetter) {
      router.replace("/today");
      return;
    }
    setShortcode(needsLetter);
    setStep("authorization");
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
              setStep("authorization");
            }}
          />
          <Button type="button" variant="ghost" block onClick={() => router.replace("/today")}>
            {t("later")}
          </Button>
        </div>
      )}
      {step === "authorization" && shortcode && (
        <AuthorizationStep key={shortcode.id} shortcode={shortcode} onSubmitted={() => router.replace("/today")} onLater={() => router.replace("/today")} />
      )}
    </>
  );
}
