import { describe, expect, it } from "vitest";
import { formatKES, groupThousands, maskMsisdn, moneyParts, normaliseMsisdn, THIN_SPACE, MINUS } from "./format";

describe("formatKES", () => {
  it("renders cents with thin-space thousands and two decimals", () => {
    expect(formatKES(1240000)).toBe(`KES${THIN_SPACE}12${THIN_SPACE}400.00`);
  });
  it("renders small amounts", () => {
    expect(formatKES(500)).toBe(`KES${THIN_SPACE}5.00`);
    expect(formatKES(0)).toBe(`KES${THIN_SPACE}0.00`);
    expect(formatKES(7)).toBe(`KES${THIN_SPACE}0.07`);
  });
  it("uses U+2212 for negatives", () => {
    expect(formatKES(-240050)).toBe(`${MINUS}KES${THIN_SPACE}2${THIN_SPACE}400.50`);
  });
  it("groups millions", () => {
    expect(formatKES(123456789)).toBe(`KES${THIN_SPACE}1${THIN_SPACE}234${THIN_SPACE}567.89`);
  });
  it("can drop symbol and decimals", () => {
    expect(formatKES(240000, { symbol: false })).toBe(`2${THIN_SPACE}400.00`);
    expect(formatKES(240000, { decimals: false })).toBe(`KES${THIN_SPACE}2${THIN_SPACE}400`);
  });
  it("never renders floats", () => {
    expect(formatKES(1e12 + 1)).toBe(`KES${THIN_SPACE}10${THIN_SPACE}000${THIN_SPACE}000${THIN_SPACE}000.01`);
    expect(() => formatKES(Number.NaN)).toThrow(TypeError);
  });
});

describe("moneyParts / groupThousands", () => {
  it("splits whole and cents", () => {
    expect(moneyParts(240050)).toEqual({ negative: false, whole: `2${THIN_SPACE}400`, cents: "50" });
  });
  it("groups with a custom separator", () => {
    expect(groupThousands("1234567", ",")).toBe("1,234,567");
    expect(groupThousands("999", ",")).toBe("999");
  });
});

describe("msisdn helpers", () => {
  it("masks first 4 and last 3", () => {
    expect(maskMsisdn("254712345678")).toBe("2547•••••678");
  });
  it("normalises local formats", () => {
    expect(normaliseMsisdn("0712 345 678")).toBe("254712345678");
    expect(normaliseMsisdn("+254 712 345678")).toBe("254712345678");
    expect(normaliseMsisdn("712345678")).toBe("254712345678");
    expect(normaliseMsisdn("0112345678")).toBe("254112345678");
    expect(normaliseMsisdn("12345")).toBeNull();
  });
});
