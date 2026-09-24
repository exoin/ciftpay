"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { readSession } from "@/lib/auth";
import { OnboardingFlow } from "./OnboardingFlow";
import { AccountantForm } from "@/components/onboarding/AccountantForm";

export default function OnboardingPage() {
  const t = useTranslations();
  const router = useRouter();
  const [mode, setMode] = useState<"merchant" | "accountant">("merchant");

  useEffect(() => {
    if (!readSession()) {
      router.replace(`/login?next=${encodeURIComponent("/onboarding")}`);
    }
  }, [router]);

  return (
    <main className="mx-auto w-full max-w-md px-4 py-10 lg:mx-0 lg:ml-[var(--rail)] lg:py-16">
      <div className="font-display text-2xl font-semibold text-green">{t("app.name")}</div>
      <p className="mt-1 text-sm text-muted">{t("app.tagline")}</p>

      <div className="mt-8">
        {/* Dual-Track Onboarding Mode Selector */}
        <div className="mb-6 flex rounded-r2 border border-hairline bg-paper-2 p-1" role="tablist">
          <button
            type="button"
            role="tab"
            aria-selected={mode === "merchant"}
            onClick={() => setMode("merchant")}
            className={`flex-1 rounded py-2 text-center text-sm font-medium transition-all ${
              mode === "merchant"
                ? "bg-paper text-ink shadow-sm font-semibold"
                : "text-muted hover:text-ink"
            }`}
          >
            Business Owner
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={mode === "accountant"}
            onClick={() => setMode("accountant")}
            className={`flex-1 rounded py-2 text-center text-sm font-medium transition-all ${
              mode === "accountant"
                ? "bg-paper text-ink shadow-sm font-semibold"
                : "text-muted hover:text-ink"
            }`}
          >
            Tax Professional
          </button>
        </div>

        {mode === "merchant" ? (
          <OnboardingFlow />
        ) : (
          <AccountantForm onCreated={() => router.replace("/clients")} />
        )}
      </div>
    </main>
  );
}
