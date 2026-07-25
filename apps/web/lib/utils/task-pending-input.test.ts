import { describe, expect, it } from "vitest";
import { aggregateTaskPendingInput, isInputCapableSessionState } from "./task-pending-input";

describe("isInputCapableSessionState", () => {
  it.each([
    ["RUNNING", true],
    ["WAITING_FOR_INPUT", true],
    ["COMPLETED", false],
    ["FAILED", false],
    ["CANCELLED", false],
    ["STARTING", false],
    ["CREATED", false],
  ] as const)("%s -> %s", (state, expected) => {
    expect(isInputCapableSessionState(state)).toBe(expected);
  });
});

describe("aggregateTaskPendingInput", () => {
  it("ignores sessions that are not input-capable", () => {
    const sessions = [{ id: "s1", state: "COMPLETED" }];
    const result = aggregateTaskPendingInput(
      sessions,
      () => ({
        clarification: true,
        permission: true,
      }),
      undefined,
    );
    expect(result).toEqual({ clarification: false, permission: false, hasUnloadedMessages: false });
  });

  it("ORs flags across every input-capable session", () => {
    const sessions = [
      { id: "s1", state: "RUNNING" },
      { id: "s2", state: "WAITING_FOR_INPUT" },
    ];
    const flagsBySession: Record<string, { clarification: boolean; permission: boolean }> = {
      s1: { clarification: true, permission: false },
      s2: { clarification: false, permission: true },
    };
    const result = aggregateTaskPendingInput(
      sessions,
      (session) => flagsBySession[session.id],
      undefined,
    );
    expect(result).toEqual({ clarification: true, permission: true, hasUnloadedMessages: false });
  });

  it("falls back to taskPendingAction when a session's messages are not loaded", () => {
    const sessions = [{ id: "s1", state: "RUNNING" }];
    const result = aggregateTaskPendingInput(sessions, () => undefined, "clarification");
    expect(result).toEqual({ clarification: true, permission: false, hasUnloadedMessages: true });
  });

  it("falls back to taskPendingAction when there are no sessions", () => {
    const result = aggregateTaskPendingInput([], () => undefined, "permission");
    expect(result).toEqual({ clarification: false, permission: true, hasUnloadedMessages: false });
  });

  it("does not apply the taskPendingAction fallback once every session's messages are loaded", () => {
    const sessions = [{ id: "s1", state: "RUNNING" }];
    const result = aggregateTaskPendingInput(
      sessions,
      () => ({ clarification: false, permission: false }),
      "clarification",
    );
    expect(result).toEqual({ clarification: false, permission: false, hasUnloadedMessages: false });
  });
});
