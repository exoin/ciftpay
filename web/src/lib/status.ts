import type { ChipTone } from "@/components/ui/StatusChip";
import type { Schemas } from "@/lib/api/client";

/** Maps API enums to chip tones and message keys under `status.*`. */
export function invoiceChip(state: Schemas["InvoiceState"]): { tone: ChipTone; key: "acked" | "pending" | "failed" | "needsReview" | "taxPending" | "syncing" } {
  switch (state) {
    case "ACKED":
      return { tone: "acked", key: "acked" };
    case "NEEDS_REVIEW":
      return { tone: "failed", key: "needsReview" };
    case "FAILED_TERMINAL":
      return { tone: "failed", key: "failed" };
    case "FAILED_RETRYABLE":
    case "SUBMITTED":
      return { tone: "pending", key: "syncing" };
    // Recorded, but withheld from KRA until the org configures eTIMS
    // (ADR-0009) — distinct from an ordinary in-flight submission.
    case "TAX_PENDING":
      return { tone: "neutral", key: "taxPending" };
    default:
      return { tone: "pending", key: "syncing" };
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

/** Administrative gate: amber while Safaricom's paperwork is pending, green once an operator confirmed the mapping, red when the letter was refused. */
export function shortcodeChip(status: Schemas["ShortcodeStatus"]): { tone: ChipTone; key: "pendingAuthorization" | "verified" | "rejected" } {
  switch (status) {
    case "verified":
      return { tone: "acked", key: "verified" };
    case "rejected":
      return { tone: "failed", key: "rejected" };
    default:
      return { tone: "pending", key: "pendingAuthorization" };
  }
}

/** What the merchant can do with a shortcode row: upload (or re-upload) the letter, wait for Safaricom, or nothing. */
export function shortcodeAction(sc: Pick<Schemas["Shortcode"], "status" | "authorization_letter_uploaded">): "upload" | "waiting" | null {
  if (sc.status === "verified") return null;
  if (sc.status === "rejected" || !sc.authorization_letter_uploaded) return "upload";
  return "waiting";
}

export function receiptState(state: Schemas["InvoiceState"], kind?: string): "verified" | "pending" | "cancelled" {
  if (kind === "CREDIT_NOTE") return "cancelled";
  return state === "ACKED" ? "verified" : "pending";
}
