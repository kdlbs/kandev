import { describe, it, expect } from "vitest";

import {
  buildWatchPayload,
  isWatchFormReady,
  makeEmptyForm,
  maxInflightTasksString,
  parseMaxInflightTasks,
  type FormState,
} from "./linear-issue-watch-form";

describe("parseMaxInflightTasks", () => {
  it("treats blank / whitespace as uncapped (null)", () => {
    expect(parseMaxInflightTasks("")).toBeNull();
    expect(parseMaxInflightTasks("   ")).toBeNull();
  });

  it("accepts positive integers", () => {
    expect(parseMaxInflightTasks("5")).toBe(5);
    expect(parseMaxInflightTasks(" 12 ")).toBe(12);
  });

  it("rejects zero, negatives, and non-integers", () => {
    expect(parseMaxInflightTasks("0")).toBe("invalid");
    expect(parseMaxInflightTasks("-1")).toBe("invalid");
    expect(parseMaxInflightTasks("1.5")).toBe("invalid");
    expect(parseMaxInflightTasks("abc")).toBe("invalid");
  });
});

describe("maxInflightTasksString", () => {
  it("renders null/undefined and non-positive values as blank (uncapped)", () => {
    expect(maxInflightTasksString(null)).toBe("");
    expect(maxInflightTasksString(undefined)).toBe("");
    expect(maxInflightTasksString(0)).toBe("");
    expect(maxInflightTasksString(-3)).toBe("");
  });

  it("renders positive values as the integer string", () => {
    expect(maxInflightTasksString(5)).toBe("5");
  });
});

// makeEmptyForm seeds a valid filter-less skeleton; tests add a filter field
// and a destination so isWatchFormReady's other gates pass and we isolate the
// cap behaviour.
function readyForm(overrides: Partial<FormState> = {}): FormState {
  return {
    ...makeEmptyForm("ws-1"),
    teamKey: "ENG",
    workflowId: "wf-1",
    workflowStepId: "step-1",
    prompt: "do the thing",
    ...overrides,
  };
}

describe("isWatchFormReady", () => {
  it("is true for a complete form with a valid cap", () => {
    expect(isWatchFormReady(readyForm({ maxInflightTasks: "5" }))).toBe(true);
  });

  it("is true when the cap is blank (uncapped)", () => {
    expect(isWatchFormReady(readyForm({ maxInflightTasks: "" }))).toBe(true);
  });

  it("is false when the cap is invalid", () => {
    expect(isWatchFormReady(readyForm({ maxInflightTasks: "0" }))).toBe(false);
  });

  it("is false when the filter is empty", () => {
    expect(isWatchFormReady(readyForm({ teamKey: "", maxInflightTasks: "5" }))).toBe(false);
  });
});

describe("buildWatchPayload", () => {
  it("maps a blank cap to null", () => {
    const payload = buildWatchPayload(readyForm({ maxInflightTasks: "" }));
    expect(payload).not.toBeNull();
    expect(payload?.maxInflightTasks).toBeNull();
  });

  it("maps a positive cap to the integer", () => {
    const payload = buildWatchPayload(readyForm({ maxInflightTasks: "7" }));
    expect(payload?.maxInflightTasks).toBe(7);
  });

  it("returns null when the cap is invalid", () => {
    expect(buildWatchPayload(readyForm({ maxInflightTasks: "-1" }))).toBeNull();
  });
});
