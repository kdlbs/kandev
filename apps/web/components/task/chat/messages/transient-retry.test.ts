import { describe, expect, it } from "vitest";
import { parseRetryAt, retryCountdownLabel } from "./transient-retry";

describe("transient retry countdown", () => {
  it("formats a bounded countdown as minutes and seconds", () => {
    expect(retryCountdownLabel(65_000)).toBe("1:05");
    expect(retryCountdownLabel(1_000)).toBe("0:01");
    expect(retryCountdownLabel(-1)).toBe("0:00");
  });

  it("rejects malformed retry deadlines", () => {
    expect(parseRetryAt(undefined)).toBeUndefined();
    expect(parseRetryAt("not-a-date")).toBeUndefined();
    expect(parseRetryAt("2026-08-08T12:00:00.000Z")).toBe(Date.parse("2026-08-08T12:00:00.000Z"));
  });
});
