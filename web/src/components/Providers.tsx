"use client";

import { useEffect, useState, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { ApiRequestError, getActiveOrgId } from "@/lib/api/client";
import { flushAllClientState, readSession } from "@/lib/auth";
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
            networkMode: "offlineFirst",
            throwOnError: (err) => {
              if (err instanceof ApiRequestError) {
                if (err.status === 401) {
                  flushAllClientState(client);
                  router.replace("/login");
                  return true;
                }
                if (err.status === 403 && (err.code === "forbidden" || err.code === "no_org")) {
                  // Mismatch between requested tenant and active session membership
                  flushAllClientState(client);
                  router.replace("/login");
                  return true;
                }
              }
              return false;
            },
          },
          mutations: { networkMode: "offlineFirst" },
        },
      }),
  );

  // Synchronize multi-tab session and org changes:
  // If user signs into another account in another tab or changes active org,
  // sync the client cache immediately so state never bleeds between accounts.
  useEffect(() => {
    const handleStorageChange = (e: StorageEvent) => {
      if (e.key === "ciftpay.session" || e.key === "ciftpay.org") {
        const sess = readSession();
        if (!sess) {
          flushAllClientState(client);
          router.replace("/login");
        } else {
          const currentOrg = getActiveOrgId();
          if (currentOrg && !sess.orgs.some((o) => o.org_id === currentOrg)) {
            // Active org in this tab is not valid for the new session in other tab!
            flushAllClientState(client);
            router.replace("/login");
          } else {
            // Org changed in another tab, wipe query cache to prevent cross-tenant data mix
            client.clear();
            void client.invalidateQueries();
          }
        }
      }
    };

    window.addEventListener("storage", handleStorageChange);
    return () => window.removeEventListener("storage", handleStorageChange);
  }, [client, router]);

  return (
    <QueryClientProvider client={client}>
      <ToastProvider>{children}</ToastProvider>
    </QueryClientProvider>
  );
}
