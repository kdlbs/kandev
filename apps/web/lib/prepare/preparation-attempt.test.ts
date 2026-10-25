import { describe, expect, it } from "vitest";
import { comparePreparationStartedAt } from "./preparation-attempt";

describe("comparePreparationStartedAt", () => {
  it("orders RFC3339Nano timestamps that differ below millisecond precision", () => {
    expect(
      comparePreparationStartedAt(
        "2026-09-28T18:00:00.123456789Z",
        "2026-09-28T18:00:00.123456788Z",
      ),
    ).toBe(1);
  });

  it("normalizes offsets and fractional precision before comparing", () => {
    expect(
      comparePreparationStartedAt("2026-09-28T19:00:00.12+01:00", "2026-09-28T18:00:00.120000000Z"),
    ).toBe(0);
  });

  it("rejects malformed timestamps by returning no ordering", () => {
    expect(comparePreparationStartedAt("2026-02-30T18:00:00Z", "2026-03-01T18:00:00Z")).toBeNull();
  });
});
