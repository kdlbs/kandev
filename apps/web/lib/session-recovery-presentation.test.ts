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
