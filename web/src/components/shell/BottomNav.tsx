"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import { AlertTriangle, FileText, MoreHorizontal, Receipt, Sun } from "lucide-react";
import { cn } from "@/lib/cn";
import { useAttention } from "@/lib/api/queries";

const tabs = [
  { href: "/today", key: "today", Icon: Sun },
  { href: "/payments", key: "payments", Icon: Receipt },
  { href: "/invoices", key: "invoices", Icon: FileText },
  { href: "/attention", key: "attention", Icon: AlertTriangle },
  { href: "/more", key: "more", Icon: MoreHorizontal },
] as const;

/**
 * Five tabs: Today · Payments · Invoices · Attention · More. Active tab is
 * ledger-green with a 2 px top rule; Attention carries a kra-red count badge.
 * Fixed on mobile, becomes the 240 px left rail on ≥ 1024 px.
 */
export function BottomNav() {
  const t = useTranslations("nav");
  const pathname = usePathname();
  const { data: attention } = useAttention();
  const badge = attention?.actionable_count ?? 0;

  return (
    <nav
      aria-label="Primary"
      className={cn(
        "fixed inset-x-0 bottom-0 z-40 border-t border-hairline bg-paper pb-[env(safe-area-inset-bottom)]",
        "lg:inset-y-0 lg:right-auto lg:w-[var(--rail)] lg:border-r lg:border-t-0 lg:pt-20",
      )}
    >
      <ul className="flex lg:flex-col">
        {tabs.map(({ href, key, Icon }) => {
          const active = pathname === href || pathname.startsWith(`${href}/`) || (key === "more" && ["/items", "/settings"].some((p) => pathname.startsWith(p)));
          return (
            <li key={key} className="flex-1 lg:flex-none">
              <Link
                href={href}
                aria-current={active ? "page" : undefined}
                className={cn(
                  "relative flex min-h-[56px] flex-col items-center justify-center gap-1 px-2 text-xs font-medium",
                  "lg:min-h-[48px] lg:flex-row lg:justify-start lg:gap-3 lg:px-6 lg:text-base",
                  active ? "text-green" : "text-muted hover:text-ink-2",
                )}
              >
                {active && <span aria-hidden className="absolute inset-x-3 top-0 h-0.5 bg-green lg:inset-y-2 lg:left-0 lg:h-auto lg:w-0.5" />}
                <span className="relative">
                  <Icon size={20} strokeWidth={1.5} aria-hidden />
                  {key === "attention" && badge > 0 && (
                    <span className="absolute -right-2 -top-1.5 min-w-4 rounded-r1 bg-red px-1 text-center font-mono text-[10px] leading-4 text-paper" aria-label={`${badge}`}>
                      {badge > 99 ? "99+" : badge}
                    </span>
                  )}
                </span>
                <span>{t(key)}</span>
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
