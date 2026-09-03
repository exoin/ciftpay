import type { ReactNode } from "react";
import { Providers } from "@/components/Providers";

export default function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <Providers>
      <main className="mx-auto w-full max-w-md px-4 py-10 lg:mx-0 lg:ml-[var(--rail)] lg:py-16">{children}</main>
    </Providers>
  );
}
