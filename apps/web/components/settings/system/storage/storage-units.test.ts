import { describe, expect, it } from "vitest";
import { bytesToGigabytes, formatGigabytes, gigabytesToBytes } from "./storage-units";

describe("storage units", () => {
  it("converts between API bytes and editable gigabytes", () => {
    expect(bytesToGigabytes(16_106_127_360)).toBe(15);
    expect(gigabytesToBytes(2.5)).toBe(2_684_354_560);
  });

  it("formats normal byte values as gigabytes", () => {
    expect(formatGigabytes(0)).toBe("0 GB");
    expect(formatGigabytes(1)).toBe("<0.01 GB");
    expect(formatGigabytes(16_106_127_360)).toBe("15 GB");
  });

  it("returns a dash for missing or non-finite values", () => {
    expect(formatGigabytes(null)).toBe("-");
    expect(formatGigabytes(undefined)).toBe("-");
    expect(formatGigabytes(Infinity)).toBe("-");
    expect(formatGigabytes(NaN)).toBe("-");
  });
});
