import { describe, expect, it } from "vitest";
import { formatAge, formatEvidenceTime, formatLocalTime, truncatePreview } from "./format";

describe("formatAge", () => {
  it("renders under an hour as minutes only", () => {
    expect(formatAge(5 * 60_000)).toBe("5m");
  });

  it("renders an hour or more as hours and minutes", () => {
    expect(formatAge(4 * 3_600_000 + 12 * 60_000)).toBe("4h 12m");
  });

  it("renders a future reference time as 0m", () => {
    expect(formatAge(-1000)).toBe("0m");
  });

  it("renders an absent age as 0m", () => {
    expect(formatAge(undefined)).toBe("0m");
  });

  it("renders exactly zero as 0m", () => {
    expect(formatAge(0)).toBe("0m");
  });
});

describe("formatLocalTime", () => {
  it("formats an epoch millisecond timestamp as local HH:mm", () => {
    const date = new Date(2026, 8, 27, 8, 5);
    expect(formatLocalTime(date.getTime())).toBe("08:05");
  });
});

describe("formatEvidenceTime", () => {
  it("formats a strict RFC3339 timestamp as local HH:mm", () => {
    const date = new Date(2026, 8, 27, 8, 5);
    const iso = date.toISOString();
    expect(formatEvidenceTime(iso)).toBe(formatLocalTime(date.getTime()));
  });

  it("returns undefined for a missing or malformed value", () => {
    expect(formatEvidenceTime(undefined)).toBeUndefined();
    expect(formatEvidenceTime("not-a-timestamp")).toBeUndefined();
  });
});

describe("truncatePreview", () => {
  it("leaves a short preview unchanged", () => {
    expect(truncatePreview("short")).toBe("short");
  });

  it("truncates a preview longer than 140 characters", () => {
    const long = "a".repeat(150);
    expect(truncatePreview(long)).toHaveLength(140);
  });
});
