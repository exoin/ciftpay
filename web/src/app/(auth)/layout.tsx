import type { ReactNode } from "react";
import { Providers } from "@/components/Providers";

export default function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <Providers>
      <div className="w-full min-h-screen">
        {children}
      </div>
    </Providers>
  );
}
