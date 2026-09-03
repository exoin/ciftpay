"use client";

import { useState, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { ApiRequestError } from "@/lib/api/client";
import { writeSession } from "@/lib/auth";
import { ToastProvider } from "@/components/ui/Toast";

export function Providers({ children }: { children: ReactNode }) {
  const router = useRouter();
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            staleTime: 15_000,
            retry: (count, err) => !(err instanceof ApiRequestError && err.status < 500) && count < 2,
            // Offline shell: keep showing what we last saw.
            networkMode: "offlineFirst",
            throwOnError: (err) => {
              if (err instanceof ApiRequestError && err.status === 401) {
                writeSession(null);
                router.replace("/login");
              }
              return false;
            },
          },
          mutations: { networkMode: "offlineFirst" },
        },
      }),
  );
  return (
    <QueryClientProvider client={client}>
      <ToastProvider>{children}</ToastProvider>
    </QueryClientProvider>
  );
}
