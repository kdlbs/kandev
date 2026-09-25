import { describe, expect, it } from "vitest";
import { estimateDurationComponents } from "./duration";

describe("estimateDurationComponents", () => {
  it.each([
    ["zero floors to one second", 0, 1, false],
    ["sub-second floors to one second", 250, 1, false],
    ["exact second stays whole", 1000, 1, false],
    ["partial second rounds up", 1001, 2, false],
    ["boundary of 60000ms renders in seconds", 60000, 60, false],
    ["just over the boundary renders in minutes", 60001, 2, true],
    ["exact minute stays whole", 120000, 2, true],
    ["partial minute rounds up", 120001, 3, true],
  ] as const)("%s", (_name, ms, wantValue, wantMinutes) => {
    expect(estimateDurationComponents(ms)).toEqual({ value: wantValue, minutes: wantMinutes });
  });
});
