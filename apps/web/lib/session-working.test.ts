import { describe, expect, it } from "vitest";
import type { TaskSession } from "@/lib/types/http";
import { isSessionWorking } from "./session-working";

function session(
  state: TaskSession["state"],
  foreground_activity?: TaskSession["foreground_activity"],
) {
  return { state, foreground_activity } as TaskSession;
}

describe("isSessionWorking", () => {
  it.each([
    ["STARTING", undefined, true],
    ["RUNNING", undefined, true],
    ["RUNNING", "generating", true],
    ["RUNNING", "background", true],
    ["WAITING_FOR_INPUT", "background", true],
    ["IDLE", "background", true],
    ["CREATED", undefined, false],
    ["WAITING_FOR_INPUT", "generating", false],
    ["WAITING_FOR_INPUT", undefined, false],
    ["COMPLETED", undefined, false],
    ["FAILED", undefined, false],
    ["CANCELLED", undefined, false],
  ] as const)("returns %s for %s with %s foreground activity", (state, activity, expected) => {
    expect(isSessionWorking(session(state, activity))).toBe(expected);
  });

  it("returns false for an absent session", () => {
    expect(isSessionWorking(null)).toBe(false);
    expect(isSessionWorking(undefined)).toBe(false);
  });
});
