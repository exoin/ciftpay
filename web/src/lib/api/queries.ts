"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, apiBaseUrl, getActiveOrgId, postForm, rawGet, rawPost, unwrap, type Schemas } from "./client";

/** Query keys, one namespace per resource so invalidation stays coarse and safe. */
export const qk = {
  session: ["session"] as const,
  org: ["org", "current"] as const,
  orgs: ["orgs"] as const,
  today: ["reports", "today"] as const,
  vat: (period: string) => ["reports", "vat", period] as const,
  payments: (params?: { status?: string; q?: string } | string) => {
    if (typeof params === "string") {
      return ["payments", params, ""] as const;
    }
    return ["payments", params?.status ?? "all", params?.q ?? ""] as const;
  },
  invoices: (params?: { state?: string; kind?: string; q?: string }) =>
    ["invoices", params?.state ?? "all", params?.kind ?? "all", params?.q ?? ""] as const,
  invoice: (id: string) => ["invoices", "detail", id] as const,
  items: ["items"] as const,
  shortcodes: ["shortcodes"] as const,
  shortcode: (id: string) => ["shortcodes", "detail", id] as const,
  attention: ["attention"] as const,
  entitlement: ["billing", "entitlement"] as const,
  etims: ["org", "etims"] as const,
};

/** The org's direct-KRA OSCU configuration (ADR-0009). Not yet in
 * api/openapi.yaml (contract debt, see docs/runbooks/local-dev.md). */
export type EtimsSettings = {
  status: "unconfigured" | "initialized" | "failed";
  kra_bhf_id: string | null;
  kra_device_serial: string | null;
  failed_reason: string | null;
  initialized_at: string | null;
};

export function useEtimsSettings() {
  return useQuery({
    queryKey: qk.etims,
    queryFn: () => rawGet<EtimsSettings>("/org/etims"),
  });
}

export function useConfigureEtims() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { kra_bhf_id?: string; kra_device_serial: string }) => rawPost<EtimsSettings>("/org/etims", body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.etims });
      void qc.invalidateQueries({ queryKey: qk.invoices() });
      void qc.invalidateQueries({ queryKey: qk.attention });
    },
  });
}

export function useToday() {
  return useQuery({
    queryKey: qk.today,
    queryFn: async () => unwrap(await api.GET("/reports/today")),
    refetchInterval: 30_000,
  });
}

export type PaymentFilterParams = {
  status?: Schemas["PaymentStatus"];
  q?: string;
};

export function usePayments(filters?: PaymentFilterParams | Schemas["PaymentStatus"]) {
  const params: PaymentFilterParams =
    typeof filters === "string" ? { status: filters } : filters ?? {};

  return useQuery({
    queryKey: qk.payments(params),
    queryFn: async () =>
      unwrap(
        await api.GET("/payments", {
          params: {
            query: {
              ...(params.status ? { status: params.status } : {}),
              ...(params.q ? { q: params.q } : {}),
            },
          },
        })
      ),
  });
}

export type InvoiceFilterParams = {
  state?: Schemas["InvoiceState"];
  kind?: "INVOICE" | "CREDIT_NOTE";
  q?: string;
  from?: string;
  to?: string;
};

export function useInvoices(filters?: InvoiceFilterParams | Schemas["InvoiceState"]) {
  const params: InvoiceFilterParams =
    typeof filters === "string" ? { state: filters } : filters ?? {};

  return useQuery({
    queryKey: qk.invoices(params),
    queryFn: async () =>
      unwrap(
        await api.GET("/invoices", {
          params: {
            query: {
              ...(params.state ? { state: params.state } : {}),
              ...(params.kind ? { kind: params.kind } : {}),
              ...(params.q ? { q: params.q } : {}),
              ...(params.from ? { from: params.from } : {}),
              ...(params.to ? { to: params.to } : {}),
            },
          },
        })
      ),
  });
}

export function useInvoice(id: string) {
  return useQuery({
    queryKey: qk.invoice(id),
    queryFn: async () => unwrap(await api.GET("/invoices/{id}", { params: { path: { id } } })),
    enabled: Boolean(id),
  });
}

export function useItems() {
  return useQuery({
    queryKey: qk.items,
    queryFn: async () => unwrap(await api.GET("/items")),
  });
}

export function useShortcodes(opts: { enabled?: boolean } = {}) {
  return useQuery({
    queryKey: qk.shortcodes,
    queryFn: async () => unwrap(await api.GET("/shortcodes")),
    enabled: opts.enabled ?? true,
  });
}

export function useShortcode(id: string | null | undefined) {
  return useQuery({
    queryKey: qk.shortcode(id ?? ""),
    queryFn: async () => unwrap(await api.GET("/shortcodes/{id}", { params: { path: { id: id! } } })),
    enabled: Boolean(id),
  });
}

export function useAttention() {
  return useQuery({
    queryKey: qk.attention,
    queryFn: async () => unwrap(await api.GET("/attention")),
    refetchInterval: 60_000,
  });
}

export function useCurrentOrg() {
  const orgId = getActiveOrgId();
  return useQuery({
    queryKey: [...qk.org, orgId ?? "default"],
    queryFn: async () => unwrap(await api.GET("/orgs/current")),
  });
}

export function useOrgs() {
  return useQuery({
    queryKey: qk.orgs,
    queryFn: async () => unwrap(await api.GET("/orgs")),
  });
}

export function useVatReport(period: string) {
  return useQuery({
    queryKey: qk.vat(period),
    queryFn: async () => unwrap(await api.GET("/reports/vat", { params: { query: { period } } })),
    enabled: /^\d{4}-\d{2}$/.test(period),
  });
}

export function useReissueInvoice() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, buyer_pin, buyer_name }: { id: string; buyer_pin: string; buyer_name?: string }) =>
      unwrap(
        await api.POST("/invoices/{id}/reissue", {
          params: { path: { id } },
          body: { buyer_pin, buyer_name },
        }),
      ),
    onSuccess: (inv) => {
      void qc.invalidateQueries({ queryKey: ["invoices"] });
      void qc.invalidateQueries({ queryKey: qk.invoice(inv.id) });
      void qc.invalidateQueries({ queryKey: qk.attention });
      void qc.invalidateQueries({ queryKey: qk.today });
    },
  });
}

export function useRetryInvoice() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => unwrap(await api.POST("/invoices/{id}/retry", { params: { path: { id } } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["invoices"] });
      void qc.invalidateQueries({ queryKey: qk.attention });
    },
  });
}

export function useResendReceipt() {
  return useMutation({
    mutationFn: async (id: string) => unwrap(await api.POST("/invoices/{id}/resend", { params: { path: { id } } })),
  });
}

export function useCreateCreditNote() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, reason }: { id: string; reason?: string }) =>
      unwrap(
        await api.POST("/invoices/{id}/credit-note", {
          params: { path: { id } },
          body: { reason },
        }),
      ),
    onSuccess: (inv) => {
      void qc.invalidateQueries({ queryKey: ["invoices"] });
      void qc.invalidateQueries({ queryKey: qk.invoice(inv.id) });
      void qc.invalidateQueries({ queryKey: qk.attention });
      void qc.invalidateQueries({ queryKey: qk.today });
    },
  });
}

export function useConvertPayment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { id: string; body: { lines: Schemas["SaleLineInput"][]; buyer_pin?: string; buyer_name?: string } }) =>
      unwrap(await api.POST("/payments/{id}/convert", { params: { path: { id: vars.id } }, body: vars.body })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["payments"] });
      void qc.invalidateQueries({ queryKey: ["invoices"] });
      void qc.invalidateQueries({ queryKey: qk.attention });
      void qc.invalidateQueries({ queryKey: qk.today });
    },
  });
}

export function useCreateSale() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["SaleCreate"]) => unwrap(await api.POST("/sales", { body })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["invoices"] });
      void qc.invalidateQueries({ queryKey: qk.today });
    },
  });
}

export function useCreateItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["ItemCreate"]) => unwrap(await api.POST("/items", { body })),
    onSuccess: () => void qc.invalidateQueries({ queryKey: qk.items }),
  });
}

export function useCreateShortcode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["ShortcodeCreate"]) => unwrap(await api.POST("/shortcodes", { body })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.shortcodes });
      void qc.invalidateQueries({ queryKey: qk.attention });
    },
  });
}

/**
 * Uploads the signed Safaricom authorization letter (multipart field `letter`).
 * Resolves to the shortcode, still `pending_authorization` but with
 * `authorization_letter_uploaded: true`. 409 `conflict` (already verified),
 * 409 `shortcode_claimed` and 422 `validation` surface as `ApiRequestError`.
 */
export function useSubmitShortcodeAuthorization() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, file }: { id: string; file: File }): Promise<Schemas["Shortcode"]> => {
      const form = new FormData();
      form.append("letter", file, file.name);
      return postForm(`/shortcodes/${encodeURIComponent(id)}/authorization`, form);
    },
    onSuccess: (_sc, { id }) => {
      void qc.invalidateQueries({ queryKey: qk.shortcodes });
      void qc.invalidateQueries({ queryKey: qk.shortcode(id) });
      void qc.invalidateQueries({ queryKey: qk.attention });
    },
  });
}

export function useAnalyticsSummary(period: string = "today") {
  const orgId = getActiveOrgId();
  return useQuery({
    queryKey: ["analytics", "summary", orgId, period] as const,
    queryFn: async () =>
      unwrap(
        await api.GET("/analytics/summary", {
          params: { query: { period } },
        }),
      ),
    refetchInterval: 30_000,
  });
}

export async function downloadItaxReport(month: string, overrideOrgId?: string): Promise<void> {
  const base = apiBaseUrl();
  const url = `${base}/reports/itax/export?month=${encodeURIComponent(month)}`;
  const headers: HeadersInit = {};
  const orgId = overrideOrgId ?? getActiveOrgId();
  if (orgId) headers["X-Org-Id"] = orgId;

  const res = await fetch(url, {
    method: "GET",
    credentials: "include",
    headers,
  });
  if (!res.ok) {
    throw new Error(`Failed to download report: ${res.statusText}`);
  }
  const blob = await res.blob();
  const downloadUrl = window.URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = downloadUrl;
  a.download = `ciftpay-itax-${month}.csv`;
  document.body.appendChild(a);
  a.click();
  window.URL.revokeObjectURL(downloadUrl);
  a.remove();
}

export function useOrgMembers() {
  return useQuery({
    queryKey: ["org", "members"] as const,
    queryFn: async () => unwrap(await api.GET("/org/members")),
  });
}

export function useOrgInvites() {
  return useQuery({
    queryKey: ["org", "invites"] as const,
    queryFn: async () => unwrap(await api.GET("/org/invites")),
  });
}

export function useCreateInvite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: Schemas["OrgInviteInput"]) =>
      unwrap(await api.POST("/org/invites", { body })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["org", "invites"] });
      void qc.invalidateQueries({ queryKey: ["org", "members"] });
    },
  });
}

export function useRevokeInvite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) =>
      unwrap(await api.DELETE("/org/invites/{id}", { params: { path: { id } } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["org", "invites"] });
    },
  });
}

export function useRemoveMember() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (userId: string) =>
      unwrap(await api.DELETE("/org/members/{userId}", { params: { path: { userId } } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["org", "members"] });
    },
  });
}

export function useAccountantInvites() {
  return useQuery({
    queryKey: ["accountant", "invites"] as const,
    queryFn: async () => unwrap(await api.GET("/accountant/invites")),
    refetchInterval: 30_000,
  });
}

export function useAcceptAccountantInvite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) =>
      unwrap(await api.POST("/accountant/invites/{id}/accept", { params: { path: { id } } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["accountant"] });
      void qc.invalidateQueries({ queryKey: ["orgs"] });
    },
  });
}

export function useRejectAccountantInvite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) =>
      unwrap(await api.POST("/accountant/invites/{id}/reject", { params: { path: { id } } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["accountant", "invites"] });
    },
  });
}

export function useAccountantClients() {
  return useQuery({
    queryKey: ["accountant", "clients"] as const,
    queryFn: async () => unwrap(await api.GET("/accountant/clients")),
    refetchInterval: 60_000,
  });
}
