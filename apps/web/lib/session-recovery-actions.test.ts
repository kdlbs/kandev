import { describe, expect, it } from "vitest";
import { sessionRecoveryAction } from "@/components/task/chat/messages/action-message-recovery";

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.12
// The projection must keep the existing transport actions distinct.
describe("recovery operation identity", () => {
  it.each(["resume", "fresh_start", "runtime_retry", "resume_new_branch"])(
    "preserves %s",
    (action) => {
      expect(
        sessionRecoveryAction({
          type: "ws_request",
          label: action,
          params: { method: "session.recover", payload: { action } },
        }),
      ).toBe(action);
    },
  );
  it("does not treat arbitrary websocket actions as recovery", () => {
    expect(
      sessionRecoveryAction({
        type: "ws_request",
        label: "restore",
        params: {
          method: "session.launch",
          payload: { action: "resume" },
        },
      }),
    ).toBeNull();
  });
});

import { selectPrimaryRecoveryAction } from "./session-recovery-actions";

describe("primary recovery selection", () => {
  it.each([
    [["fresh_start", "resume"], "resume"],
    [["resume", "runtime_retry"], "runtime_retry"],
    [["restore", "resume_new_branch", "resume"], "resume_new_branch"],
    [["restore"], "restore"],
    [["fresh_start"], "fresh_start"],
    [[], null],
  ] as const)("selects one eligible action from %j", (actions, expected) => {
    expect(selectPrimaryRecoveryAction(actions)).toBe(expected);
  });
  it("offers no bypass for a non-retryable guard", () => {
    expect(selectPrimaryRecoveryAction(["resume", "restore", "fresh_start"], true)).toBeNull();
  });
});
