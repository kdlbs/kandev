import { describe, expect, it } from "vitest";
import { formatWait, isRetireItem, knownCondition, measureHasValue } from "./learning";

describe("learning helpers", () => {
  it("knows only the closed set of waiting conditions", () => {
    expect(knownCondition("spacing")).toBe("spacing");
    expect(knownCondition("made_up")).toBeNull();
    expect(knownCondition(undefined)).toBeNull();
  });

  it("lists retire, update and supersede kinds under retire or supersede", () => {
    for (const kind of ["standing_order_retire", "note_retire", "note_update"]) {
      expect(isRetireItem({ kind })).toBe(true);
    }
    for (const kind of ["note_add", "standing_order_add", "context_diff"]) {
      expect(isRetireItem({ kind })).toBe(false);
    }
  });

  it("treats a measure with a null reason as having no value", () => {
    const base = { numerator: 0, denominator: 0, capped: false };
    expect(measureHasValue({ ...base, value: null, null_reason: "no_data" })).toBe(false);
    expect(measureHasValue({ ...base, value: 0 })).toBe(true);
  });

  it("splits seconds into days, hours and minutes", () => {
    expect(formatWait(2 * 3600 + 10 * 60)).toEqual({ days: 0, hours: 2, minutes: 10 });
    expect(formatWait(90_000)).toEqual({ days: 1, hours: 1, minutes: 0 });
  });
});
