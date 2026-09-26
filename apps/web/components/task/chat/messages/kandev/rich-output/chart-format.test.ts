import { describe, expect, it } from "vitest";
import { createXAxisTickFormatter, formatYAxisTick } from "./chart-format";

const AUGUST_12_TIMESTAMP = "2026-08-12T10:00:00Z";

describe("createXAxisTickFormatter", () => {
  it("keeps short categories and clips long categories", () => {
    const format = createXAxisTickFormatter(["/api", "very-long-category-name"], "en-US");

    expect(format("/api")).toBe("/api");
    expect(format("very-long-category-name")).toBe("very-long-categ…");
  });

  it("formats ISO timestamps on distinct days as compact dates", () => {
    const format = createXAxisTickFormatter([AUGUST_12_TIMESTAMP, "2026-08-13T10:00:00Z"], "en-US");

    expect(format(AUGUST_12_TIMESTAMP)).toBe("Aug 12");
    expect(format("2026-08-13T10:00:00Z")).toBe("Aug 13");
  });

  it("includes time when timestamps share a calendar day", () => {
    const first = AUGUST_12_TIMESTAMP;
    const second = "2026-08-12T14:30:00Z";
    const format = createXAxisTickFormatter([first, second], "en-US");

    expect(format(first)).toContain("Aug 12");
    expect(format(first)).toContain("10:00");
    expect(format(second)).toContain("14:30");
    expect(format(first)).not.toBe(format(second));
  });

  it("falls back safely for the pseudo locale", () => {
    const format = createXAxisTickFormatter([AUGUST_12_TIMESTAMP], "pseudo");

    expect(format(AUGUST_12_TIMESTAMP)).toBe("Aug 12");
    expect(formatYAxisTick(2_400, "pseudo")).toBe("2.4K");
  });
});

describe("formatYAxisTick", () => {
  it("uses compact notation for large values", () => {
    expect(formatYAxisTick(2_400, "en-US")).toBe("2.4K");
    expect(formatYAxisTick(-12_500, "en-US")).toBe("-12.5K");
  });

  it("preserves useful precision for ordinary and small values", () => {
    expect(formatYAxisTick(29.4, "en-US")).toBe("29.4");
    expect(formatYAxisTick(0.0042, "en-US")).toBe("0.0042");
  });
});
