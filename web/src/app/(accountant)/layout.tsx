import type { ReactNode } from "react";
import { Providers } from "@/components/Providers";
import { AppShell } from "@/components/shell/AppShell";

export default function AccountantLayout({ children }: { children: ReactNode }) {
  return (
    <Providers>
      <AppShell>{children}</AppShell>
    </Providers>
  );
}
