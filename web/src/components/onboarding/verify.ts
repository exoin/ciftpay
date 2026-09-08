/**
 * Pure helpers for the "pay KES 1 to your own Till" control check. Kept free
 * of React so the instruction card and the countdown are unit-testable.
 */

export type ShortcodeKind = "till" | "paybill" | "pochi";

/** One line of the instruction card; `key` is looked up under `onboarding.verify.steps`. */
export type PayStep = { key: string; value?: string };

/**
 * The M-Pesa menu path a merchant follows on their own phone to send exactly
 * KES 1 to the number they are proving control of. Till and Pochi are paid via
 * Buy Goods / Send Money with no account number; Paybill needs `accountRef`.
 */
export function payInstructions(kind: ShortcodeKind, shortcode: string, accountRef = "CIFTPAY"): PayStep[] {
  switch (kind) {
    case "paybill":
      return [{ key: "open" }, { key: "menuPayBill" }, { key: "enterBusiness", value: shortcode }, { key: "enterAccount", value: accountRef }, { key: "amount" }, { key: "pin" }];
    case "pochi":
      return [{ key: "open" }, { key: "menuPochi" }, { key: "enterPhone", value: shortcode }, { key: "amount" }, { key: "pin" }];
    case "till":
    default:
      return [{ key: "open" }, { key: "menuBuyGoods" }, { key: "enterTill", value: shortcode }, { key: "amount" }, { key: "pin" }];
  }
}

/** Whole seconds until `expiresAt`, never negative; `NaN` dates count as expired. */
export function remainingSeconds(expiresAt: string | Date, now: number = Date.now()): number {
  const t = typeof expiresAt === "string" ? Date.parse(expiresAt) : expiresAt.getTime();
  if (!Number.isFinite(t)) return 0;
  return Math.max(0, Math.floor((t - now) / 1000));
}

/** `m:ss` for the countdown, e.g. `9:07`, `0:00`. */
export function formatCountdown(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  const m = Math.floor(s / 60);
  return `${m}:${String(s % 60).padStart(2, "0")}`;
}

export type VerifyPhase = "starting" | "pending" | "verified" | "expired" | "claimed" | "error";

/**
 * Folds what the api told us into one UI phase. `challenge` is the last
 * `POST /verify` answer, `polled` the latest `GET /shortcodes/{id}`.
 */
export function verifyPhase(input: {
  challenge: { status: "pending" | "verified" } | null;
  polled: { verified: boolean; verification?: { status: "pending" | "verified" | "expired" | "failed" } } | null;
  secondsLeft: number;
  errorCode: string | null;
}): VerifyPhase {
  if (input.errorCode === "shortcode_claimed") return "claimed";
  if (input.errorCode) return "error";
  if (input.challenge?.status === "verified" || input.polled?.verified) return "verified";
  if (input.polled?.verification?.status === "failed") return "claimed";
  if (!input.challenge) return "starting";
  if (input.polled?.verification?.status === "expired" || input.secondsLeft <= 0) return "expired";
  return "pending";
}
