import { describe, expect, it, vi } from "vitest";
import { WebSocketRequestError } from "@/lib/ws/client";
import {
  branchRecoveryDetails,
  contextContinuationDetails,
  requestSessionRecover,
  sessionRecoveryGuardDetails,
} from "./session-recovery-service";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mocks.request }),
}));

describe("session recovery service", () => {
  it("recognizes typed native-state loss without authorizing generic failures", () => {
    const error = new WebSocketRequestError(
      "native state is unavailable",
      "SESSION_RESTORE_REQUIRED",
      {
        kind: "session_restore_required",
        recovery_action: "continue_from_history",
        reason: "native_state_missing",
        generation: 3,
      },
    );
    expect(contextContinuationDetails(error)).toMatchObject({
      kind: "session_restore_required",
      recovery_action: "continue_from_history",
      reason: "native_state_missing",
    });
    expect(
      contextContinuationDetails(new WebSocketRequestError("unknown", "INTERNAL_ERROR")),
    ).toBeNull();
  });

  it("accepts the explicit continuation action as a distinct protocol action", async () => {
    mocks.request.mockResolvedValue({ success: true });
    await expect(
      requestSessionRecover("task-1", "session-1", "continue_from_history", "failed"),
    ).resolves.toBeUndefined();
    expect(mocks.request).toHaveBeenCalledWith(
      "session.recover",
      { task_id: "task-1", session_id: "session-1", action: "continue_from_history" },
      30_000,
    );
  });
});

describe("sessionRecoveryGuardDetails", () => {
  it("returns the details for a retryable in-progress recovery refusal", () => {
    const error = new WebSocketRequestError("blocked", "CONFLICT", {
      kind: "session_recovery_in_progress",
      retryable: true,
      session_id: "session-1",
    });

    expect(sessionRecoveryGuardDetails(error)).toEqual({
      kind: "session_recovery_in_progress",
      retryable: true,
      session_id: "session-1",
    });
  });

  it("returns the details for a non-retryable unstoppable-agent refusal", () => {
    const error = new WebSocketRequestError("blocked", "UNAVAILABLE", {
      kind: "session_recovery_unstoppable",
      retryable: false,
      session_id: "session-2",
    });

    expect(sessionRecoveryGuardDetails(error)).toEqual({
      kind: "session_recovery_unstoppable",
      retryable: false,
      session_id: "session-2",
    });
  });

  it("returns null for an unrelated WebSocketRequestError", () => {
    const error = new WebSocketRequestError("nope", "CONFLICT", {
      kind: "branch_unrecoverable",
      recovery_action: "resume_new_branch",
    });

    expect(sessionRecoveryGuardDetails(error)).toBeNull();
  });

  it("returns null for a non-WebSocketRequestError", () => {
    expect(sessionRecoveryGuardDetails(new Error("plain"))).toBeNull();
  });

  it("does not match a branch recovery error", () => {
    const error = new WebSocketRequestError("branch gone", "CONFLICT", {
      kind: "branch_unrecoverable",
      recovery_action: "resume_new_branch",
      original_branch: "feature/lost",
    });

    expect(branchRecoveryDetails(error)).not.toBeNull();
    expect(sessionRecoveryGuardDetails(error)).toBeNull();
  });
});
