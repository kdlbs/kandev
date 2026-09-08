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
import { ApprovalGateError, updateTaskStatusOrTranslateGate } from "./office-status-gate";

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
});
