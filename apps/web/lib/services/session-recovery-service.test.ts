import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  branchRecoveryDetails,
  managedCloneRelocationRecoveryDetails,
  requestSessionRecover,
  sessionRecoveryGuardDetails,
} from "./session-recovery-service";
import { WebSocketRequestError } from "@/lib/ws/client";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mocks.request }),
}));

beforeEach(() => vi.clearAllMocks());

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

it("sends the current stamp with an explicit managed clone relocation", async () => {
  mocks.request.mockResolvedValueOnce({ success: true });
  await requestSessionRecover("task-1", "session-1", "relocate_and_resume", "failed", "stamp-1");
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

it("waits for a relocation response that arrives after the previous 30 second deadline", async () => {
  vi.useFakeTimers();
  try {
    mocks.request.mockImplementationOnce(
      () => new Promise((resolve) => setTimeout(() => resolve({ success: true }), 35_000)),
    );
    const request = requestSessionRecover(
      "task-1",
      "session-1",
      "relocate_and_resume",
      "failed",
      "stamp-1",
    );
    await vi.advanceTimersByTimeAsync(35_000);
    await expect(request).resolves.toBeUndefined();
  } finally {
    vi.useRealTimers();
  }
});

it("requires a stamp before requesting managed clone relocation", async () => {
  await expect(
    requestSessionRecover("task-1", "session-1", "relocate_and_resume", "failed"),
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
