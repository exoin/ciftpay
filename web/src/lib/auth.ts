"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { api, getActiveOrgId, setActiveOrgId, setCsrfToken, unwrap, type Schemas } from "./api/client";
import { qk } from "./api/queries";

export type Session = Schemas["Session"];

const SESSION_KEY = "ciftpay.session";

/**
 * The session cookie is HttpOnly, so the client keeps a copy of the
 * `Session` body (user id, CSRF token, memberships) in sessionStorage.
 * 401 from any endpoint clears it and sends the user to /login.
 */
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
    window.sessionStorage.removeItem(SESSION_KEY);
    setCsrfToken(null);
    setActiveOrgId(null);
    return;
  }
  window.sessionStorage.setItem(SESSION_KEY, JSON.stringify(s));
  setCsrfToken(s.csrf_token);
  const current = getActiveOrgId();
  if (!current || !s.orgs.some((o) => o.org_id === current)) {
    const def = s.orgs.find((o) => o.is_default) ?? s.orgs[0];
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
      if (s && !s.orgs.some((o) => o.org_id === org.id)) {
        const next: Session = { ...s, orgs: [...s.orgs, { org_id: org.id, name: org.name, role: "owner", is_default: s.orgs.length === 0 }] };
        writeSession(next);
        qc.setQueryData(qk.session, next);
      }
      switchOrg(qc, org.id);
      void qc.invalidateQueries({ queryKey: qk.orgs });
    },
  });
}

/** True when the signed-in user has not created or joined a business yet. */
export function hasNoOrg(s: Session | null): boolean {
  return s !== null && s.orgs.length === 0;
}

export function useLogout() {
  const qc = useQueryClient();
  const router = useRouter();
  return useMutation({
    mutationFn: async () => {
      await api.POST("/auth/logout");
    },
    onSettled: () => {
      writeSession(null);
      qc.clear();
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
  setActiveOrgId(orgId);
  // Everything except the session is org-scoped.
  qc.removeQueries({ predicate: (q) => q.queryKey[0] !== "session" });
}
