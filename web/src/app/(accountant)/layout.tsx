"use client";

import { useEffect, type ReactNode } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { Building2, Mail, Settings, LogOut, ShieldCheck } from "lucide-react";
import { Providers } from "@/components/Providers";
import { readSession, useLogout } from "@/lib/auth";
import { useAccountantInvites } from "@/lib/api/queries";
import { cn } from "@/lib/cn";

function AccountantNavigation() {
  const pathname = usePathname();
  const router = useRouter();
  const logout = useLogout();
  const { data: invitesData } = useAccountantInvites();
  const pendingCount = invitesData?.data?.length ?? 0;

  const navItems = [
    {
      href: "/clients",
      label: "Clients",
      icon: Building2,
      active: pathname === "/clients" || pathname.startsWith("/client/"),
    },
    {
      href: "/clients#invites",
      label: "Pending Invites",
      icon: Mail,
      badge: pendingCount > 0 ? pendingCount : undefined,
      active: pathname === "/clients" && typeof window !== "undefined" && window.location.hash === "#invites",
    },
    {
      href: "/settings",
      label: "Settings",
      icon: Settings,
      active: pathname.startsWith("/settings"),
    },
  ];

  return (
    <header className="sticky top-0 z-40 border-b border-hairline bg-paper/95 backdrop-blur">
      <div className="mx-auto flex max-w-6xl items-center justify-between px-4 py-3 sm:px-6">
        {/* Brand & Portal Badge */}
        <div className="flex items-center gap-3">
          <Link href="/clients" className="flex items-center gap-2">
            <span className="font-display text-xl font-bold tracking-tight text-green">CiftPay</span>
            <span className="inline-flex items-center gap-1 rounded-full bg-green/10 px-2.5 py-0.5 text-xs font-semibold text-green">
              <ShieldCheck className="size-3" />
              Accountant Portal
            </span>
          </Link>
        </div>

        {/* Nav Links */}
        <nav className="flex items-center gap-1 sm:gap-2">
          {navItems.map((item) => {
            const Icon = item.icon;
            return (
              <Link
                key={item.href}
                href={item.href}
                className={cn(
                  "relative flex items-center gap-2 rounded-r2 px-3 py-1.5 text-sm font-medium transition-colors",
                  item.active
                    ? "bg-paper-2 text-ink font-semibold"
                    : "text-ink-2 hover:bg-paper-2/60 hover:text-ink"
                )}
              >
                <Icon className="size-4" />
                <span className="hidden sm:inline">{item.label}</span>
                {item.badge !== undefined && (
                  <span className="ml-1 flex size-5 items-center justify-center rounded-full bg-ochre font-mono text-[10px] font-bold text-paper">
                    {item.badge}
                  </span>
                )}
              </Link>
            );
          })}

          {/* Sign Out Action */}
          <button
            type="button"
            onClick={async () => {
              await logout.mutateAsync();
              router.replace("/login");
            }}
            disabled={logout.isPending}
            className="ml-2 flex items-center gap-1.5 rounded-r2 px-3 py-1.5 text-sm font-medium text-muted transition-colors hover:bg-paper-2 hover:text-danger"
            title="Sign Out"
          >
            <LogOut className="size-4" />
            <span className="hidden sm:inline">Sign Out</span>
          </button>
        </nav>
      </div>
    </header>
  );
}

function AccountantShellInner({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    const s = readSession();
    if (!s) {
      router.replace(`/login?next=${encodeURIComponent(pathname)}`);
    }
  }, [router, pathname]);

  return (
    <div className="min-h-screen bg-sand text-ink">
      <AccountantNavigation />
      <main className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
        {children}
      </main>
    </div>
  );
}

export default function AccountantLayout({ children }: { children: ReactNode }) {
  return (
    <Providers>
      <AccountantShellInner>{children}</AccountantShellInner>
    </Providers>
  );
}
