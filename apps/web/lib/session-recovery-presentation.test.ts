import { describe, expect, it } from "vitest";
import {
  isBootstrapSessionRecoveryError,
  ownsSessionRecoveryChat,
  selectSessionRecoveryError,
} from "./session-recovery-presentation";

const bootstrapError = {
  session_id: "session-1",
  stamp: "bootstrap-1",
  occurred_at: "2026-09-11T10:00:00Z",
  preview: "The agent could not start.",
  phase: "bootstrap",
  category: "generic_launch_failure",
} as const;

describe("session recovery presentation", () => {
  it("selects only a bootstrap error owned by the selected session", () => {
    expect(selectSessionRecoveryError(bootstrapError, "session-1")).toEqual(bootstrapError);
    expect(selectSessionRecoveryError(bootstrapError, "session-2")).toBeNull();
    expect(ownsSessionRecoveryChat(bootstrapError, "session-1")).toBe(true);
  });

  it("selects an older session's persisted bootstrap error instead of the task-wide newest error", () => {
    const selectedSessionMetadata = {
      last_agent_error: {
        message: "The selected session could not start.",
        occurred_at: "2026-09-11T09:00:00Z",
        stamp: "bootstrap-session-1",
        phase: "bootstrap",
        code: "permission_denied",
        details: "safe structured details",
        execution_id: "execution-session-1",
      },
    };

    expect(
      selectSessionRecoveryError(
        { ...bootstrapError, session_id: "session-2", stamp: "bootstrap-2" },
        "session-1",
        selectedSessionMetadata,
      ),
    ).toEqual({
      session_id: "session-1",
      stamp: "bootstrap-session-1",
      occurred_at: "2026-09-11T09:00:00Z",
      preview: "The selected session could not start.",
      phase: "bootstrap",
      category: "permission_denied",
      details: "safe structured details",
      execution_id: "execution-session-1",
    });
  });

  it.each([
    { ...bootstrapError, phase: "agent" },
    { ...bootstrapError, session_id: undefined },
    { ...bootstrapError, stamp: "" },
    null,
  ])("does not claim non-bootstrap or incomplete errors", (error) => {
    expect(isBootstrapSessionRecoveryError(error)).toBe(false);
    expect(selectSessionRecoveryError(error, "session-1")).toBeNull();
    expect(ownsSessionRecoveryChat(error, "session-1")).toBe(false);
  });
});
