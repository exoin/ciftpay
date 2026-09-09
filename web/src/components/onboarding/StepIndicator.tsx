import { useTranslations } from "next-intl";
import { cn } from "@/lib/cn";

export type OnboardingStep = "business" | "shortcode" | "authorization";

export const ONBOARDING_STEPS: OnboardingStep[] = ["business", "shortcode", "authorization"];

/** Three short segments with the current step's name under them; no icons, no colour-only state. */
export function StepIndicator({ current }: { current: OnboardingStep }) {
  const t = useTranslations("onboarding");
  const idx = ONBOARDING_STEPS.indexOf(current);
  return (
    <div aria-label={t("stepLabel", { n: idx + 1, total: ONBOARDING_STEPS.length })} role="group" className="mb-6">
      <ol className="flex gap-1.5" aria-hidden>
        {ONBOARDING_STEPS.map((s, i) => (
          <li key={s} className={cn("h-1 flex-1 rounded-r1", i <= idx ? "bg-green" : "bg-paper-3")} />
        ))}
      </ol>
      <p className="mt-2 font-mono text-xs text-muted">
        {t("stepLabel", { n: idx + 1, total: ONBOARDING_STEPS.length })} · {t(`steps.${current}`)}
      </p>
    </div>
  );
}
