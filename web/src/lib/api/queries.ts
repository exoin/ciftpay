"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "./client";

/** Query keys, one namespace per resource so invalidation stays coarse and safe. */
export const qk = {
  session: ["session"] as const,
  org: ["org", "current"] as const,
  orgs: ["orgs"] as const,
  today: ["reports", "today"] as const,
  vat: (period: string) => ["reports", "vat", period] as const,
  payments: (status?: string) => ["payments", status ?? "all"] as const,
  invoices: (state?: string) => ["invoices", state ?? "all"] as const,
  invoice: (id: string) => ["invoices", "detail", id] as const,
  items: ["items"] as const,
  shortcodes: ["shortcodes"] as const,
  shortcode: (id: string) => ["shortcodes", "detail", id] as const,
  attention: ["attention"] as const,
  entitlement: ["billing", "entitlement"] as const,
};

export function useToday() {
  return useQuery({
    queryKey: qk.today,
    queryFn: async () => unwrap(await api.GET("/reports/today")),
    refetchInterval: 30_000,
  });
}

export function usePayments(status?: Schemas["PaymentStatus"]) {
  return useQuery({
    queryKey: qk.payments(status),
    queryFn: async () => unwrap(await api.GET("/payments", { params: { query: status ? { status } : {} } })),
  });
}

export function useInvoices(state?: Schemas["InvoiceState"]) {
  return useQuery({
    queryKey: qk.invoices(state),
    queryFn: async () => unwrap(await api.GET("/invoices", { params: { query: state ? { state } : {} } })),
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
  return useQuery({
    queryKey: qk.org,
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

export function useVerifyShortcode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => unwrap(await api.POST("/shortcodes/{id}/verify", { params: { path: { id } } })),
    onSuccess: () => void qc.invalidateQueries({ queryKey: qk.shortcodes }),
  });
}
