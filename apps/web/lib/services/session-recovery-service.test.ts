import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  branchRecoveryDetails,
  managedCloneRelocationRecoveryDetails,
  contextContinuationDetails,
  requestSessionRecover,
  sessionRecoveryGuardDetails,
} from "./session-recovery-service";
import { WebSocketRequestError } from "@/lib/ws/client";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mocks.request }),
}));

beforeEach(() => vi.clearAllMocks());

describe("session recovery service", () => {
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

it("sends the current stamp with an explicit managed clone relocation", async () => {
  mocks.request.mockResolvedValueOnce({ success: true });
  await requestSessionRecover({
    taskId: "task-1",
    sessionId: "session-1",
    action: "relocate_and_resume",
    failureMessage: "failed",
    errorStamp: "stamp-1",
  });
  expect(mocks.request).toHaveBeenCalledWith(
    "session.recover",
    {
      task_id: "task-1",
      session_id: "session-1",
      action: "relocate_and_resume",
      error_stamp: "stamp-1",
    },
    30 * 60 * 1000,
  );
});

it("sends provider-restored settings policy only with an explicit resume", async () => {
  mocks.request.mockResolvedValueOnce({ success: true });
  await requestSessionRecover({
    taskId: "task-1",
    sessionId: "session-1",
    action: "resume",
    failureMessage: "failed",
    settingsPolicy: "provider_restored",
  });
  expect(mocks.request).toHaveBeenCalledWith(
    "session.recover",
    {
      task_id: "task-1",
      session_id: "session-1",
      action: "resume",
      settings_policy: "provider_restored",
    },
    30_000,
  );
});

it("rejects provider-restored settings policy for actions other than resume", async () => {
  await expect(
    requestSessionRecover({
      taskId: "task-1",
      sessionId: "session-1",
      action: "fresh_start",
      failureMessage: "failed",
      settingsPolicy: "provider_restored",
    }),
  ).rejects.toThrow("failed");
  expect(mocks.request).not.toHaveBeenCalled();
});

it("waits for a relocation response that arrives after the previous 30 second deadline", async () => {
  vi.useFakeTimers();
  try {
    mocks.request.mockImplementationOnce(
      () => new Promise((resolve) => setTimeout(() => resolve({ success: true }), 35_000)),
    );
    const request = requestSessionRecover({
      taskId: "task-1",
      sessionId: "session-1",
      action: "relocate_and_resume",
      failureMessage: "failed",
      errorStamp: "stamp-1",
    });
    await vi.advanceTimersByTimeAsync(35_000);
    await expect(request).resolves.toBeUndefined();
  } finally {
    vi.useRealTimers();
  }
});

it("requires a stamp before requesting managed clone relocation", async () => {
  await expect(
    requestSessionRecover({
      taskId: "task-1",
      sessionId: "session-1",
      action: "relocate_and_resume",
      failureMessage: "failed",
    }),
  ).rejects.toThrow("failed");
  expect(mocks.request).not.toHaveBeenCalled();
});

it("recognizes only the typed, path-free managed clone error details", () => {
  const error = new WebSocketRequestError("workspace needs repair", "CONFLICT", {
    kind: "managed_clone_relocation_required",
    error_stamp: "stamp-2",
    recovery_action: "relocate_and_resume",
  });
  expect(managedCloneRelocationRecoveryDetails(error)?.error_stamp).toBe("stamp-2");
  expect(managedCloneRelocationRecoveryDetails(new Error("plain"))).toBeNull();
});
