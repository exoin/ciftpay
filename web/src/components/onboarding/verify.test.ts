import { describe, expect, it } from "vitest";
import { formatCountdown, payInstructions, remainingSeconds, verifyPhase } from "./verify";

describe("payInstructions", () => {
  it("Till goes through Buy Goods with the till number and no account", () => {
    const steps = payInstructions("till", "600123");
    expect(steps.map((s) => s.key)).toEqual(["open", "menuBuyGoods", "enterTill", "amount", "pin"]);
    expect(steps.find((s) => s.key === "enterTill")?.value).toBe("600123");
  });

  it("Paybill needs the business number and the CIFTPAY account reference", () => {
    const steps = payInstructions("paybill", "400200", "CIFTPAY");
    expect(steps.map((s) => s.key)).toEqual(["open", "menuPayBill", "enterBusiness", "enterAccount", "amount", "pin"]);
    expect(steps.find((s) => s.key === "enterBusiness")?.value).toBe("400200");
    expect(steps.find((s) => s.key === "enterAccount")?.value).toBe("CIFTPAY");
  });

  it("Pochi is a Send Money to the phone number", () => {
    const steps = payInstructions("pochi", "254712345678");
    expect(steps.map((s) => s.key)).toEqual(["open", "menuPochi", "enterPhone", "amount", "pin"]);
    expect(steps.find((s) => s.key === "enterPhone")?.value).toBe("254712345678");
  });
});

describe("remainingSeconds", () => {
  const now = Date.parse("2026-09-08T12:00:00Z");

  it("floors to whole seconds", () => {
    expect(remainingSeconds("2026-09-08T12:09:59.900Z", now)).toBe(599);
  });

  it("never goes negative once expired", () => {
    expect(remainingSeconds("2026-09-08T11:59:00Z", now)).toBe(0);
  });

  it("treats an unparsable date as expired", () => {
    expect(remainingSeconds("not a date", now)).toBe(0);
  });
});

describe("formatCountdown", () => {
  it("renders m:ss with a padded second", () => {
    expect(formatCountdown(599)).toBe("9:59");
    expect(formatCountdown(65)).toBe("1:05");
    expect(formatCountdown(0)).toBe("0:00");
    expect(formatCountdown(-4)).toBe("0:00");
  });
});

describe("verifyPhase", () => {
  const pending = { status: "pending" as const };

  it("is starting until the challenge answer arrives", () => {
    expect(verifyPhase({ challenge: null, polled: null, secondsLeft: 0, errorCode: null })).toBe("starting");
  });

  it("is pending while the countdown runs", () => {
    expect(verifyPhase({ challenge: pending, polled: { verified: false, verification: { status: "pending" } }, secondsLeft: 120, errorCode: null })).toBe("pending");
  });

  it("flips to verified from either the challenge or the poll", () => {
    expect(verifyPhase({ challenge: { status: "verified" }, polled: null, secondsLeft: 0, errorCode: null })).toBe("verified");
    expect(verifyPhase({ challenge: pending, polled: { verified: true }, secondsLeft: 12, errorCode: null })).toBe("verified");
  });

  it("expires when the server says so or the local clock runs out", () => {
    expect(verifyPhase({ challenge: pending, polled: { verified: false, verification: { status: "expired" } }, secondsLeft: 30, errorCode: null })).toBe("expired");
    expect(verifyPhase({ challenge: pending, polled: { verified: false }, secondsLeft: 0, errorCode: null })).toBe("expired");
  });

  it("shows claimed for a 409 or a failed challenge", () => {
    expect(verifyPhase({ challenge: null, polled: null, secondsLeft: 0, errorCode: "shortcode_claimed" })).toBe("claimed");
    expect(verifyPhase({ challenge: pending, polled: { verified: false, verification: { status: "failed" } }, secondsLeft: 100, errorCode: null })).toBe("claimed");
  });

  it("any other error is a plain error", () => {
    expect(verifyPhase({ challenge: null, polled: null, secondsLeft: 0, errorCode: "internal" })).toBe("error");
  });
});
