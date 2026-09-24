"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { api, getActiveOrgId, setActiveOrgId, setCsrfToken, unwrap, type Schemas } from "./api/client";
export { getActiveOrgId, setActiveOrgId } from "./api/client";
import { qk } from "./api/queries";

export type Session = Schemas["Session"];

const SESSION_KEY = "ciftpay.session";

/**
 * The session cookie is HttpOnly, so the client keeps a copy of the
 * `Session` body (user id, CSRF token, memberships) in sessionStorage.
 * 401 from any endpoint clears it and sends the user to /login.
 */
/**
 * Completely flushes all client-side state caches across TanStack Query,
 * window state (Redux/Zustand if registered), localStorage and sessionStorage.
 * Guarantees zero multi-tenant state bleed on login, account/org switch, or logout.
 */
export function flushAllClientState(qc?: ReturnType<typeof useQueryClient> | null) {
  if (typeof window !== "undefined") {
    try {
      window.sessionStorage.clear();
    } catch {}

    try {
      const keysToRemove: string[] = [];
      for (let i = 0; i < window.localStorage.length; i++) {
        const key = window.localStorage.key(i);
        if (
          key &&
          (key.startsWith("ciftpay") ||
            key.includes("org") ||
            key.includes("user") ||
            key.includes("profile") ||
            key.includes("session"))
        ) {
          keysToRemove.push(key);
        }
      }
      keysToRemove.forEach((k) => window.localStorage.removeItem(k));
      window.localStorage.removeItem("ciftpay.org");
      window.localStorage.removeItem("ciftpay.session");
      window.localStorage.removeItem("ciftpay.csrf");
    } catch {}

    const win = window as unknown as Record<string, unknown>;
    if (typeof win.__REDUX_STORE__ === "object" && win.__REDUX_STORE__ !== null) {
      try {
        (win.__REDUX_STORE__ as { dispatch: (action: unknown) => void }).dispatch({ type: "RESET" });
      } catch {}
    }
  }

  if (qc) {
    try {
      qc.clear();
    } catch {}
  }
}

export function readSession(): Session | null {
  if (typeof window === "undefined") return null;
  const raw = window.sessionStorage.getItem(SESSION_KEY);
  if (!raw) return null;
  try {
    const s = JSON.parse(raw) as Session;
    if (new Date(s.expires_at).getTime() < Date.now()) return null;
    return s;
  } catch {
    return null;
  }
}

export function writeSession(s: Session | null) {
  if (typeof window === "undefined") return;
  if (!s) {
    flushAllClientState();
    return;
  }
  window.sessionStorage.setItem(SESSION_KEY, JSON.stringify(s));
  setCsrfToken(s.csrf_token);
  const current = getActiveOrgId();
  const orgs = s.orgs ?? [];
  if (!current || !orgs.some((o) => o.org_id === current)) {
    const def = orgs.find((o) => o.is_default) ?? orgs[0];
    setActiveOrgId(def?.org_id ?? null);
  }
}

export function useSession() {
  return useQuery({
    queryKey: qk.session,
    queryFn: async () => readSession(),
    staleTime: Infinity,
  });
}

export function useRequestOtp() {
  return useMutation({
    mutationFn: async (vars: { msisdn: string; locale?: Schemas["Locale"] }) =>
      unwrap(await api.POST("/auth/otp/request", { body: vars })),
  });
}

export function useVerifyOtp() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { msisdn: string; code: string }) => unwrap(await api.POST("/auth/otp/verify", { body: vars })),
    onSuccess: (s) => {
      // Flush previous account state completely before establishing new session
      flushAllClientState(qc);
      writeSession(s);
      qc.setQueryData(qk.session, s);
    },
  });
}

/**
 * Creates the business and makes it the active org. The api returns the `Org`,
 * not a new session, so the sessionStorage copy gets the membership appended
 * (the caller is the owner; the first org is the default, as on the server).
 */
export function useCreateOrg() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["OrgCreate"]) => unwrap(await api.POST("/orgs", { body })),
    onSuccess: (org) => {
      const s = readSession();
      const orgs = s?.orgs ?? [];
      if (s && !orgs.some((o) => o.org_id === org.id)) {
        const next: Session = { ...s, orgs: [...orgs, { org_id: org.id, name: org.name, role: "owner", is_default: orgs.length === 0 }] };
        writeSession(next);
        qc.setQueryData(qk.session, next);
      }
      switchOrg(qc, org.id);
      void qc.invalidateQueries({ queryKey: qk.orgs });
    },
  });
}

/** True when the signed-in user has not created or joined a business yet. */
export function hasNoOrg(s: Session | null | undefined): boolean {
  return !!s && (!s.orgs || s.orgs.length === 0);
}

export function useLogout() {
  const qc = useQueryClient();
  const router = useRouter();
  return useMutation({
    mutationFn: async () => {
      await api.POST("/auth/logout");
    },
    onSettled: () => {
      flushAllClientState(qc);
      router.replace("/login");
    },
  });
}

export function useActiveMembership(): Schemas["OrgMembership"] | null {
  const { data } = useSession();
  const id = getActiveOrgId();
  if (!data) return null;
  return data.orgs.find((o) => o.org_id === id) ?? data.orgs.find((o) => o.is_default) ?? data.orgs[0] ?? null;
}

export function switchOrg(qc: ReturnType<typeof useQueryClient>, orgId: string) {
  // Clear all query cache to prevent any data bleed from previous org
  qc.clear();
  setActiveOrgId(orgId);
  void qc.invalidateQueries();
}
