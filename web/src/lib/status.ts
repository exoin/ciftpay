import type { ChipTone } from "@/components/ui/StatusChip";
import type { Schemas } from "@/lib/api/client";

/** Maps API enums to chip tones and message keys under `status.*`. */
export function invoiceChip(state: Schemas["InvoiceState"]): { tone: ChipTone; key: "acked" | "pending" | "failed" | "needsReview" } {
  switch (state) {
    case "ACKED":
      return { tone: "acked", key: "acked" };
    case "NEEDS_REVIEW":
      return { tone: "failed", key: "needsReview" };
    case "FAILED_TERMINAL":
      return { tone: "failed", key: "failed" };
    default:
      return { tone: "pending", key: "pending" };
  }
}

export function paymentChip(status: Schemas["PaymentStatus"]): { tone: ChipTone; key: "matched" | "cash" | "unmatched" | "reversed" | "partial" } {
  switch (status) {
    case "matched":
      return { tone: "acked", key: "matched" };
    case "cash_sale":
      return { tone: "cash", key: "cash" };
    case "reversed":
      return { tone: "failed", key: "reversed" };
    case "partial":
      return { tone: "pending", key: "partial" };
    default:
      return { tone: "unmatched", key: "unmatched" };
  }
}

export function receiptState(state: Schemas["InvoiceState"], kind?: string): "verified" | "pending" | "cancelled" {
  if (kind === "CREDIT_NOTE") return "cancelled";
  return state === "ACKED" ? "verified" : "pending";
}
