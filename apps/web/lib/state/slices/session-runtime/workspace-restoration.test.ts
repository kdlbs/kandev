import { describe, expect, it } from "vitest";
import {
  beginWorkspaceRestoration,
  clearWorkspaceRestoration,
  completeWorkspaceRestoration,
  failWorkspaceRestoration,
  sanitizeWorkspaceRestorationDetails,
  type WorkspaceRestorationState,
} from "./workspace-restoration";

const INPUT = { taskId: "task-1", sessionId: "session-1", environmentId: "env-1" };

function makeState(): WorkspaceRestorationState {
  return { byEnvironmentId: {} };
}

describe("workspace restoration attempt state", () => {
  it("rejects a duplicate pending attempt for the same workspace identity", () => {
    const state = makeState();

    const first = beginWorkspaceRestoration(state, INPUT);
    const duplicate = beginWorkspaceRestoration(state, INPUT);

    expect(first).toMatchObject({ ...INPUT, revision: 1, status: "pending" });
    expect(duplicate).toBeNull();
  });

  it("supersedes a pending attempt from another session in the same environment", () => {
    const state = makeState();
    const first = beginWorkspaceRestoration(state, INPUT);

    const second = beginWorkspaceRestoration(state, {
      ...INPUT,
      sessionId: "session-2",
    });

    expect(second).toMatchObject({ sessionId: "session-2", revision: 2, status: "pending" });
    expect(first).not.toBeNull();
    expect(failWorkspaceRestoration(state, first!, "stale")).toBe(false);
    expect(state.byEnvironmentId[INPUT.environmentId]).toMatchObject({
      sessionId: "session-2",
      status: "pending",
    });
  });

  it("accepts success and clears the matching failure details", () => {
    const state = makeState();
    const attempt = beginWorkspaceRestoration(state, INPUT)!;

    expect(failWorkspaceRestoration(state, attempt, "backend detail")).toBe(true);
    expect(state.byEnvironmentId[INPUT.environmentId]).toMatchObject({
      status: "error",
      details: "backend detail",
    });
    expect(completeWorkspaceRestoration(state, attempt)).toBe(true);
    expect(state.byEnvironmentId[INPUT.environmentId]).toMatchObject({
      status: "ready",
    });
    expect(state.byEnvironmentId[INPUT.environmentId].details).toBeUndefined();
  });

  it("does not let an old result clear a newer attempt", () => {
    const state = makeState();
    const first = beginWorkspaceRestoration(state, INPUT)!;
    expect(failWorkspaceRestoration(state, first, "first attempt failed")).toBe(true);
    const second = beginWorkspaceRestoration(state, INPUT)!;
    expect(second.revision).toBe(first.revision + 1);

    expect(clearWorkspaceRestoration(state, first)).toBe(false);
    expect(state.byEnvironmentId[INPUT.environmentId]).toMatchObject({
      revision: second.revision,
      status: "pending",
    });
  });

  it("removes only the matching attempt", () => {
    const state = makeState();
    const attempt = beginWorkspaceRestoration(state, INPUT)!;

    expect(clearWorkspaceRestoration(state, attempt)).toBe(true);
    expect(state.byEnvironmentId[INPUT.environmentId]).toBeUndefined();
  });

  it("strips control characters and bounds diagnostics", () => {
    const details = sanitizeWorkspaceRestorationDetails(
      new Error(`before\u0000${"x".repeat(600)}`),
    );

    expect(details).toHaveLength(512);
    expect(details).not.toContain("\u0000");
  });

  it("does not leave a trailing high surrogate in bounded diagnostics", () => {
    const details = sanitizeWorkspaceRestorationDetails("x".repeat(511) + "😀");

    expect(details).toHaveLength(511);
    expect(details).toBe("x".repeat(511));
  });
});
