import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  branchRecoveryDetails,
  getWorkspaceRecoveryStatus,
  managedCloneRelocationRecoveryDetails,
  contextContinuationDetails,
  requestSessionRecover,
  resumeSession,
  recoveryInspectionBusyDetails,
  recoveryInspectionBusyMessage,
  resolveRequestErrorMessage,
  sessionRecoveryGuardDetails,
  sessionDeliveryRecoveryResponse,
  sessionDeliveryRecoveryMessage,
} from "./session-recovery-service";
import { WebSocketRequestError } from "@/lib/ws/client";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mocks.request }),
}));

const CONTINUE_FROM_HISTORY = "continue_from_history";

beforeEach(() => vi.clearAllMocks());

const recoveryAction = "session.recover";

describe("recoveryInspectionBusyDetails", () => {
  it("recognizes only the structured inspection-contention conflict", () => {
    const error = new WebSocketRequestError("busy", "CONFLICT", {
      kind: "recovery_inspection_busy",
    });

    expect(recoveryInspectionBusyDetails(error)).toEqual({ kind: "recovery_inspection_busy" });
    expect(recoveryInspectionBusyDetails(new Error("workspace recovery inspection is busy"))).toBe(
      null,
    );
    expect(
      recoveryInspectionBusyDetails(
        new WebSocketRequestError("other conflict", "CONFLICT", { kind: "unrelated" }),
      ),
    ).toBeNull();
  });

  it("uses localized copy for typed contention and leaves unrelated transport errors unchanged", () => {
    const t = (key: string) =>
      key === "task:workspaceRecoveryInspectionBusy" ? "localized busy" : key;
    const busy = new WebSocketRequestError("raw conflict", "CONFLICT", {
      kind: "recovery_inspection_busy",
    });

    expect(recoveryInspectionBusyMessage(t)).toBe("localized busy");
    expect(resolveRequestErrorMessage(busy, t)).toBe("localized busy");
    expect(resolveRequestErrorMessage(new Error("raw transport failure"), t)).toBe(
      "raw transport failure",
    );
  });
});

describe("native session resume", () => {
  it("returns the native resume response and allows a full launch timeout", async () => {
    const response = {
      success: true,
      task_id: "task-1",
      session_id: "session-1",
      state: "WAITING_FOR_INPUT",
      worktree_path: "/workspace",
      worktree_branch: "main",
    };
    mocks.request.mockResolvedValueOnce(response);
    await expect(resumeSession("task-1", "session-1", "failed")).resolves.toBe(response);
    expect(mocks.request).toHaveBeenCalledWith(
      recoveryAction,
      { task_id: "task-1", session_id: "session-1", action: "resume" },
      60_000,
    );
  });

  it("surfaces a refused resume instead of hydrating a successful session", async () => {
    mocks.request.mockResolvedValueOnce({ success: false, error: "runtime unavailable" });
    await expect(resumeSession("task-1", "session-1", "failed")).rejects.toThrow(
      "runtime unavailable",
    );
  });
});

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
        recovery_action: CONTINUE_FROM_HISTORY,
        reason: "native_state_missing",
        generation: 3,
      },
    );
    expect(contextContinuationDetails(error)).toMatchObject({
      kind: "session_restore_required",
      recovery_action: CONTINUE_FROM_HISTORY,
      reason: "native_state_missing",
    });
    expect(
      contextContinuationDetails(new WebSocketRequestError("unknown", "INTERNAL_ERROR")),
    ).toBeNull();
  });

  it("accepts the explicit continuation action as a distinct protocol action", async () => {
    mocks.request.mockResolvedValue({ success: true });
    await expect(
      requestSessionRecover({
        taskId: "task-1",
        sessionId: "session-1",
        action: CONTINUE_FROM_HISTORY,
        failureMessage: "failed",
      }),
    ).resolves.toBeUndefined();
    expect(mocks.request).toHaveBeenCalledWith(
      recoveryAction,
      { task_id: "task-1", session_id: "session-1", action: CONTINUE_FROM_HISTORY },
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
    recoveryAction,
    {
      task_id: "task-1",
      session_id: "session-1",
      action: "relocate_and_resume",
      error_stamp: "stamp-1",
    },
    30 * 60 * 1000,
  );
});

it("reads durable workspace recovery status without launching the session", async () => {
  const projection = {
    task_id: "task-1",
    environment_id: "environment-1",
    session_id: "session-1",
    operation_id: "operation-1",
    attempt_id: "attempt-1",
    ownership_generation: "generation-1",
    revision: "3",
    kind: "managed_clone_relocation",
    state: "running",
    phase: "restoring",
    repository_position: 2,
    repository_total: 2,
    completed_slots: 1,
    workspace_complete: false,
    agent_ready: false,
    runner_live: true,
    started_at: "2026-10-05T12:00:00Z",
    updated_at: "2026-10-05T12:01:00Z",
  };
  mocks.request.mockResolvedValueOnce({ workspace_recovery: projection });

  await expect(getWorkspaceRecoveryStatus("task-1", "session-1", "unavailable")).resolves.toEqual(
    projection,
  );
  expect(mocks.request).toHaveBeenCalledWith(
    "session.workspace_recovery.get",
    { task_id: "task-1", session_id: "session-1" },
    10_000,
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
    recoveryAction,
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

it("treats historical continued as restored instead of showing an uncertainty warning", () => {
  const response = sessionDeliveryRecoveryResponse({
    task_id: "task",
    session_id: "session",
    outcome: "continued",
    recovery_revision: 1,
  });
  expect(response?.outcome).toBe("continued");
  expect(response && sessionDeliveryRecoveryMessage(response, (key) => key)).toBe(
    "task:deliveryRecoveryAttached",
  );
});

it("rejects the retired restored-blocked continuation outcome", () => {
  expect(
    sessionDeliveryRecoveryResponse({
      task_id: "task",
      session_id: "session",
      outcome: "restored_blocked",
      recovery_revision: 1,
    }),
  ).toBeNull();
});

it("returns the typed retry outcome and rejects malformed retry results", async () => {
  mocks.request.mockResolvedValueOnce({
    task_id: "task-1",
    session_id: "session-1",
    outcome: "blocked",
    reason: "missing_canonical_submission",
    recovery_revision: 4,
  });
  await expect(
    requestSessionRecover({
      taskId: "task-1",
      sessionId: "session-1",
      action: "retry_connection",
      failureMessage: "failed",
    }),
  ).resolves.toMatchObject({
    outcome: "blocked",
    reason: "missing_canonical_submission",
    recovery_revision: 4,
  });

  mocks.request.mockResolvedValueOnce({ success: true });
  await expect(
    requestSessionRecover({
      taskId: "task-1",
      sessionId: "session-1",
      action: "retry_connection",
      failureMessage: "failed",
    }),
  ).rejects.toThrow("failed");
});

it("translates typed delivery outcomes and bounded block reasons", () => {
  const t = (key: string) => key;
  const base = {
    task_id: "task-1",
    session_id: "session-1",
    recovery_revision: 2,
  };
  expect(sessionDeliveryRecoveryMessage({ ...base, outcome: "attached" }, t)).toBe(
    "task:deliveryRecoveryAttached",
  );
  expect(sessionDeliveryRecoveryMessage({ ...base, outcome: "settled" }, t)).toBe(
    "task:deliveryRecoverySettled",
  );
  expect(sessionDeliveryRecoveryMessage({ ...base, outcome: "uncertain" }, t)).toBe(
    "task:deliveryRecoveryUncertain",
  );
  expect(sessionDeliveryRecoveryMessage({ ...base, outcome: "unavailable" }, t)).toBe(
    "task:deliveryRecoveryUnavailable",
  );
  expect(
    sessionDeliveryRecoveryMessage(
      { ...base, outcome: "blocked", reason: "missing_canonical_submission" },
      t,
    ),
  ).toBe("task:deliveryRecoveryMissingSubmission");
  expect(
    sessionDeliveryRecoveryMessage(
      { ...base, outcome: "blocked", reason: "delivery_identity_mismatch" },
      t,
    ),
  ).toBe("task:deliveryRecoveryOwnershipBlocked");
});

it.each([
  ["delivery_output_paused", "task:deliveryRecoveryOutputPaused"],
  ["delivery_cancellation_pending", "task:deliveryRecoveryCancellationPending"],
  ["delivery_storage_pressure", "task:deliveryRecoveryStoragePressure"],
])("reports verified delivery state %s independently of process death", (reason, key) => {
  expect(
    sessionDeliveryRecoveryMessage(
      { task_id: "task", session_id: "session", outcome: "blocked", recovery_revision: 1, reason },
      (value) => value,
    ),
  ).toBe(key);
});
