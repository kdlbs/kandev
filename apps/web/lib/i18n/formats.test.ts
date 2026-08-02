import { beforeAll, describe, expect, it } from "vitest";

import { formatDate, formatNumber, formatRelative } from "./formats";
import { activateLocale } from "./index";

beforeAll(async () => {
  // Activate en so the relative-time buckets resolve from the real catalog,
  // matching the former timeAgo behavior byte-for-byte.
  await activateLocale("en");
});

describe("formatRelative (en, timeAgo-compatible)", () => {
  const now = new Date("2026-07-27T12:00:00Z").getTime();
  const ago = (ms: number) => new Date(now - ms).toISOString();

  it("returns empty string for empty or invalid input", () => {
    expect(formatRelative("", now)).toBe("");
    expect(formatRelative("not-a-date", now)).toBe("");
  });

  it("returns 'just now' under a minute", () => {
    expect(formatRelative(ago(30_000), now)).toBe("just now");
  });

  it("formats minutes, hours, and days", () => {
    expect(formatRelative(ago(5 * 60_000), now)).toBe("5m ago");
    expect(formatRelative(ago(3 * 3_600_000), now)).toBe("3h ago");
    expect(formatRelative(ago(2 * 86_400_000), now)).toBe("2d ago");
  });
});

describe("locale-aware Intl wrappers", () => {
  it("formats numbers using the active locale", async () => {
    await activateLocale("en");
    expect(formatNumber(1234567.89)).toBe("1,234,567.89");
  });

  it("maps the pseudo locale to en for Intl", async () => {
    await activateLocale("pseudo");
    // pseudo has no CLDR data; wrappers fall back to en formatting.
    expect(formatNumber(1000)).toBe("1,000");
    expect(formatDate("2026-07-27T00:00:00Z", { year: "numeric" })).toBe("2026");
    await activateLocale("en");
  });
});
