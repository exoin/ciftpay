"use client";

import { useEffect, type ReactNode } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { BottomNav } from "./BottomNav";
import { OrgSwitcher } from "./OrgSwitcher";
import { hasNoOrg, readSession } from "@/lib/auth";
import { useOnline } from "@/lib/useOnline";

/**
 * Authenticated shell: left rail on desktop, bottom tabs on mobile, content
 * column max 880 px left-anchored (design-system §9). Redirects to /login
 * when there is no client-side session copy, and to /onboarding when the
 * session has no business yet.
 */
export function AppShell({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const online = useOnline();
  const t = useTranslations("common");

  useEffect(() => {
    const s = readSession();
    if (!s) router.replace(`/login?next=${encodeURIComponent(pathname)}`);
    else if (hasNoOrg(s)) router.replace("/onboarding");
  }, [router, pathname]);

  return (
    <div className="min-h-dvh lg:pl-[var(--rail)]">
      <BottomNav />
      <div className="hidden lg:block fixed left-0 top-0 w-[var(--rail)] p-4 z-50">
        <div className="font-display text-xl font-semibold text-green">CiftPay</div>
      </div>
      <div className="hidden lg:block fixed left-0 top-[calc(100dvh-88px)] w-[var(--rail)] px-4">
        <OrgSwitcher />
      </div>
      <main className="mx-auto w-full max-w-[var(--content-max)] px-4 pb-24 pt-2 lg:mx-0 lg:px-8 lg:pb-8 lg:pt-6">
        {!online && (
          <p role="status" className="mb-3 rounded-r2 border border-ochre bg-warn-bg px-3 py-2 text-sm text-ink">
            {t("offline")}
          </p>
        )}
        {children}
      </main>
    </div>
  );
}
