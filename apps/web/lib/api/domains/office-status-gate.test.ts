import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../client";

const updateTaskMock = vi.hoisted(() => vi.fn());

vi.mock("./office-extended-api", async () => {
  const actual =
    await vi.importActual<typeof import("./office-extended-api")>("./office-extended-api");
  return {
    ...actual,
    updateTask: updateTaskMock,
  };
});

import { updateTask } from "./office-extended-api";
import {
  ApprovalGateError,
  StatusChangeRefusedError,
  finalStepMoveNotice,
  updateTaskStatusOrTranslateGate,
} from "./office-status-gate";

afterEach(() => {
  vi.clearAllMocks();
});

const APPROVALS_PENDING_MESSAGE = "approvals pending";

describe("updateTaskStatusOrTranslateGate", () => {
  it("rethrows a non-gate error unchanged", async () => {
    const notFound = new ApiError("not found", 404, { error: "not found" });
    updateTaskMock.mockRejectedValueOnce(notFound);

    await expect(updateTaskStatusOrTranslateGate("t-1", "done")).rejects.toBe(notFound);
    expect(updateTask).toHaveBeenCalledWith("t-1", { status: "done" });
  });

  it("rethrows a 409 with no pending_approvers unchanged", async () => {
    const conflict = new ApiError("conflict", 409, { error: "conflict" });
    updateTaskMock.mockRejectedValueOnce(conflict);

    await expect(updateTaskStatusOrTranslateGate("t-1", "done")).rejects.toBe(conflict);
  });

  it("defaults the redirected status to in_review when the gate body carries none", async () => {
    updateTaskMock.mockRejectedValueOnce(
      new ApiError(APPROVALS_PENDING_MESSAGE, 409, {
        error: APPROVALS_PENDING_MESSAGE,
        pending_approvers: [{ agent_profile_id: "a1", name: "CEO" }],
      }),
    );

    const err = await updateTaskStatusOrTranslateGate("t-1", "done").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApprovalGateError);
    expect((err as ApprovalGateError).redirectedStatus).toBe("in_review");
    expect((err as ApprovalGateError).message).toBe("Cannot mark done: awaiting approval from CEO");
  });

  it("carries the gate body's redirected status through unchanged", async () => {
    updateTaskMock.mockRejectedValueOnce(
      new ApiError(APPROVALS_PENDING_MESSAGE, 409, {
        error: APPROVALS_PENDING_MESSAGE,
        pending_approvers: [{ agent_profile_id: "a1", name: "CEO" }],
        status: "blocked",
      }),
    );

    const err = await updateTaskStatusOrTranslateGate("t-1", "done").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApprovalGateError);
    expect((err as ApprovalGateError).redirectedStatus).toBe("blocked");
  });

  it("uses the workflow-step message for a position gate", async () => {
    updateTaskMock.mockRejectedValueOnce(
      new ApiError("workflow step pending", 409, {
        error: "workflow step pending",
        reason: "workflow_step",
        pending_approvers: [],
        status: "in_review",
      }),
    );

    const err = await updateTaskStatusOrTranslateGate("t-1", "done").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApprovalGateError);
    expect((err as ApprovalGateError).message).toBe(
      "Cannot mark done: move the task to the final workflow step first",
    );
    expect((err as ApprovalGateError).redirectedStatus).toBe("in_review");
  });
});

describe("updateTaskStatusOrTranslateGate final-step outcomes", () => {
  // @covers AC-OFFICE-HUMAN-DONE-001.2
  it("returns the status and step the backend moved the task to", async () => {
    updateTaskMock.mockResolvedValueOnce({
      ok: true,
      status: "in_progress",
      workflow_step: { id: "step-done", name: "Done" },
    });

    await expect(updateTaskStatusOrTranslateGate("t-1", "done")).resolves.toEqual({
      status: "in_progress",
      workflowStepName: "Done",
    });
  });

  it("defaults the outcome to the requested status for a plain ok", async () => {
    updateTaskMock.mockResolvedValueOnce({ ok: true });

    await expect(updateTaskStatusOrTranslateGate("t-1", "todo")).resolves.toEqual({
      status: "todo",
    });
  });
});

describe("updateTaskStatusOrTranslateGate refusals", () => {
  // @covers AC-OFFICE-HUMAN-DONE-001.3
  it("throws StatusChangeRefusedError naming approvers when a 409 has a reason but no status", async () => {
    updateTaskMock.mockRejectedValueOnce(
      new ApiError(APPROVALS_PENDING_MESSAGE, 409, {
        error: APPROVALS_PENDING_MESSAGE,
        reason: "approvals",
        pending_approvers: [{ agent_profile_id: "a1", name: "CEO" }],
      }),
    );

    const err = await updateTaskStatusOrTranslateGate("t-1", "done").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(StatusChangeRefusedError);
    expect(err).not.toBeInstanceOf(ApprovalGateError);
    expect((err as Error).message).toBe("Cannot mark done: awaiting approval from CEO");
  });

  it("translates a completion-gate refusal", async () => {
    updateTaskMock.mockRejectedValueOnce(
      new ApiError("blocked", 409, {
        error: "blocked",
        reason: "completion_gate",
        code: "task_completion_gate_blocked",
      }),
    );

    const err = await updateTaskStatusOrTranslateGate("t-1", "done").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(StatusChangeRefusedError);
    expect((err as Error).message).toBe(
      "Cannot mark done: the task's completion criteria are not verified",
    );
  });

  it("translates a move refusal through its move code", async () => {
    updateTaskMock.mockRejectedValueOnce(
      new ApiError("active session", 409, {
        error: "active session",
        reason: "workflow_move",
        code: "task_move_active_session",
      }),
    );

    const err = await updateTaskStatusOrTranslateGate("t-1", "done").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(StatusChangeRefusedError);
    expect((err as Error).message).toBe("Stop the active session before moving the task.");
  });

  it("falls back to the generic refusal for a move refusal without a code", async () => {
    updateTaskMock.mockRejectedValueOnce(
      new ApiError("refused", 409, { error: "refused", reason: "workflow_move" }),
    );

    const err = await updateTaskStatusOrTranslateGate("t-1", "done").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(StatusChangeRefusedError);
    expect((err as Error).message).toBe(
      "Cannot mark done: the task could not be moved to the final workflow step",
    );
  });
});

describe("finalStepMoveNotice", () => {
  it("reports a queued Done request without promising completion", async () => {
    updateTaskMock.mockResolvedValueOnce({
      ok: true,
      status: "todo",
      queued: true,
      workflow_step: { id: "ship", name: "Ship" },
    });
    const outcome = await updateTaskStatusOrTranslateGate("t-1", "done");
    expect(outcome).toEqual({ status: "todo", queued: true, workflowStepName: "Ship" });
    expect(finalStepMoveNotice("done", outcome)).toBe('Queued for "Ship". Waiting for room.');
  });

  it("is null when the task reached the requested status", () => {
    expect(finalStepMoveNotice("done", { status: "done", workflowStepName: "Done" })).toBeNull();
  });

  // @covers AC-OFFICE-HUMAN-DONE-001.2
  it("names the step when the move did not complete the task", () => {
    expect(finalStepMoveNotice("done", { status: "in_progress", workflowStepName: "Ship" })).toBe(
      'Moved to "Ship". This step doesn\'t complete the task.',
    );
  });
});
