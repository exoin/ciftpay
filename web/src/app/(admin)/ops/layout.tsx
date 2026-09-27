"use client";

import { type ReactNode } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useSession } from "@/lib/auth";

const NAV_ITEMS = [
  { href: "/ops/safaricom", label: "Safaricom Pipeline", icon: "🛰️" },
  { href: "/ops/merchants", label: "Merchant CRM & Tiers", icon: "🏢" },
  { href: "/ops/kra-dlq", label: "KRA Fiscal DLQ", icon: "⚡" },
  { href: "/ops/system", label: "Engineering Health", icon: "🛠️" },
  { href: "/ops", label: "Shortcodes Legacy", icon: "📜" },
];

export default function OpsLayout({ children }: { children: ReactNode }) {
  const { data: session, isLoading } = useSession();
  const pathname = usePathname();

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <p className="text-sm text-gray-500">Verifying administrator credentials...</p>
      </div>
    );
  }

  const isAdmin = session?.orgs?.some((o) => o.role === "admin");
  if (!isAdmin) {
    return (
      <div className="mx-auto mt-16 max-w-md rounded-lg border border-red-300 bg-red-50 p-6 text-center shadow-sm">
        <div className="text-3xl mb-2">🚫</div>
        <h1 className="text-lg font-bold text-red-800">403 - Forbidden</h1>
        <p className="mt-2 text-sm text-red-700">
          Access to CiftPay Ops-Core is restricted to internal operations administrators.
        </p>
        <div className="mt-6">
          <Link
            href="/dashboard"
            className="inline-block rounded bg-red-700 px-4 py-2 text-sm font-semibold text-white hover:bg-red-800"
          >
            Return to Merchant Dashboard
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-[calc(100vh-4rem)] flex-col md:flex-row gap-6">
      {/* Sidebar Navigation */}
      <aside className="w-full md:w-64 shrink-0 rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
        <div className="mb-4 pb-3 border-b border-gray-100 flex items-center justify-between">
          <div>
            <h2 className="text-base font-bold text-gray-900">Ops-Core Portal</h2>
            <p className="text-xs text-gray-500">Internal Administration</p>
          </div>
          <span className="rounded bg-emerald-100 px-2 py-0.5 text-xs font-semibold text-emerald-800">
            ADMIN
          </span>
        </div>

        <nav className="flex flex-col space-y-1">
          {NAV_ITEMS.map((item) => {
            const active = pathname === item.href;
            return (
              <Link
                key={item.href}
                href={item.href}
                className={`flex items-center gap-2.5 rounded-md px-3 py-2 text-sm font-medium transition-colors ${
                  active
                    ? "bg-gray-900 text-white shadow-xs"
                    : "text-gray-700 hover:bg-gray-100 hover:text-gray-900"
                }`}
              >
                <span>{item.icon}</span>
                <span>{item.label}</span>
              </Link>
            );
          })}
        </nav>
      </aside>

      {/* Main Module Content */}
      <main className="flex-1 min-w-0 rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        {children}
      </main>
    </div>
  );
}
