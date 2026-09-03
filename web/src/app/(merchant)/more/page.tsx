"use client";

import Link from "next/link";
import { useTranslations } from "next-intl";
import { ChevronRight } from "lucide-react";
import { TopBar } from "@/components/shell/TopBar";
import { useSession } from "@/lib/auth";

/** The fifth tab: Items, Settings, and role-gated portals. */
export default function MorePage() {
  const t = useTranslations("nav");
  const { data: session } = useSession();
  const roles = new Set(session?.orgs.map((o) => o.role) ?? []);

  const links = [
    { href: "/items", label: t("items") },
    { href: "/settings", label: t("settings") },
    ...(roles.has("accountant") ? [{ href: "/clients", label: t("clients") }] : []),
    ...(roles.has("admin") ? [{ href: "/ops", label: t("ops") }] : []),
  ];

  return (
    <>
      <TopBar title={t("more")} />
      <ul className="ruled">
        {links.map((l) => (
          <li key={l.href}>
            <Link href={l.href} className="flex min-h-[var(--row)] items-center justify-between gap-3 py-2 hover:bg-paper-3 -mx-4 px-4">
              <span>{l.label}</span>
              <ChevronRight size={20} strokeWidth={1.5} aria-hidden className="text-muted" />
            </Link>
          </li>
        ))}
      </ul>
    </>
  );
}
