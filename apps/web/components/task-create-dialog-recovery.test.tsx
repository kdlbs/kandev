/* eslint-disable max-lines-per-function -- recovery lifecycle regressions exercise close, reopen, and scope changes. */
import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import {
  loadTaskCreationReceipt,
  saveTaskCreationReceipt,
  createTaskCreationReceipt,
} from "@/lib/task-creation-receipt";

const lookupMock = vi.fn();
const fetchTaskMock = vi.fn();
vi.mock("@/lib/api", () => ({
  createTask: vi.fn(),
  updateTask: vi.fn(),
  fetchTask: (...args: unknown[]) => fetchTaskMock(...args),
  fetchTaskByExternalId: (...args: unknown[]) => lookupMock(...args),
}));

import { isRecoverySubmitBlocked, useTaskCreationRecovery } from "./task-create-dialog-recovery";

const ARGS = {
  enabled: true,
  open: true,
  userId: "user-1",
  workspaceId: "ws-1",
  draftId: "task",
};

function payload() {
  return {
    workspace_id: "ws-1",
    workflow_id: "wf-1",
    title: "Incident task",
    description: "Task brief",
    state: "IN_PROGRESS" as const,
    start_agent: true,
  } as Parameters<typeof createTaskCreationReceipt>[0]["payload"];
}

afterEach(() => {
  window.sessionStorage.clear();
  lookupMock.mockReset();
  fetchTaskMock.mockReset();
});

describe("useTaskCreationRecovery restoration", () => {
  it("restores a submitted receipt as uncertain without creating anything", async () => {
    const stored = createTaskCreationReceipt({
      scope: { userId: "user-1", workspaceId: "ws-1", draftId: "task" },
      payload: payload(),
      runtimeId: "runtime-1",
    });
    saveTaskCreationReceipt(stored);

    const { result } = renderHook(() => useTaskCreationRecovery(ARGS));
    await waitFor(() => expect(result.current.receipt?.submissionId).toBe(stored.submissionId));
    expect(result.current.receipt?.state).toBe("unknown");
    expect(result.current.receipt?.everUncertain).toBe(true);
    expect(lookupMock).not.toHaveBeenCalled();
    expect(fetchTaskMock).not.toHaveBeenCalled();
  });

  it("does not restore a receipt from another workspace", async () => {
    const stored = createTaskCreationReceipt({
      scope: { userId: "user-1", workspaceId: "ws-2", draftId: "task" },
      payload: payload(),
      runtimeId: "runtime-1",
    });
    saveTaskCreationReceipt(stored);

    const { result } = renderHook(() => useTaskCreationRecovery(ARGS));
    await waitFor(() => expect(result.current.receipt).toBeNull());
  });

  it("keeps the in-memory receipt through close and reopen when storage refuses the write", () => {
    const spy = vi.spyOn(window.sessionStorage, "setItem").mockImplementation(() => {
      throw new Error("quota exceeded");
    });
    try {
      const { result, rerender } = renderHook(
        (args: typeof ARGS) => useTaskCreationRecovery(args),
        { initialProps: ARGS },
      );
      let started: ReturnType<typeof result.current.beginSubmission>;
      act(() => {
        started = result.current.beginSubmission(payload());
      });
      const externalId = started!.receipt.externalId;
      expect(result.current.storageUnavailable).toBe(true);

      rerender({ ...ARGS, open: false });
      rerender({ ...ARGS, open: true });

      expect(result.current.receipt?.externalId).toBe(externalId);
      expect(result.current.storageUnavailable).toBe(true);
      let again: ReturnType<typeof result.current.beginSubmission>;
      act(() => {
        again = result.current.beginSubmission({
          ...payload(),
          description: "A new ordinary submission",
        } as never);
      });
      expect(again!.retrying).toBe(true);
      expect(again!.receipt.externalId).toBe(externalId);
    } finally {
      spy.mockRestore();
    }
  });
});

describe("useTaskCreationRecovery identity ownership", () => {
  it("reuses one identity until the submission resolves", () => {
    const { result } = renderHook(() => useTaskCreationRecovery(ARGS));
    let first: ReturnType<typeof result.current.beginSubmission>;
    act(() => {
      first = result.current.beginSubmission(payload());
    });
    expect(first!.retrying).toBe(false);

    let second: ReturnType<typeof result.current.beginSubmission>;
    act(() => {
      second = result.current.beginSubmission({
        ...payload(),
        description: "Changed later",
      } as never);
    });
    expect(second!.retrying).toBe(true);
    expect(second!.receipt.externalId).toBe(first!.receipt.externalId);
    expect(second!.receipt.payload.description).toBe("Task brief");
  });

  it("fences owner generations across close, reopen, and scope changes", () => {
    const { result, rerender } = renderHook((args: typeof ARGS) => useTaskCreationRecovery(args), {
      initialProps: ARGS,
    });
    const initial = result.current.getGeneration();
    expect(result.current.isOwnerCurrent(initial)).toBe(true);

    rerender({ ...ARGS, open: false });
    expect(result.current.isOwnerCurrent(initial)).toBe(false);
    const closed = result.current.getGeneration();
    expect(closed).not.toBe(initial);

    rerender({ ...ARGS, open: true });
    expect(result.current.isOwnerCurrent(closed)).toBe(false);
    const reopened = result.current.getGeneration();
    expect(result.current.isOwnerCurrent(reopened)).toBe(true);

    rerender({ ...ARGS, open: true, workspaceId: "ws-2" });
    expect(result.current.isOwnerCurrent(reopened)).toBe(false);
  });

  it("keeps a verified task ID when a later malformed response has none", () => {
    const { result } = renderHook(() => useTaskCreationRecovery(ARGS));
    let started: ReturnType<typeof result.current.beginSubmission>;
    act(() => {
      started = result.current.beginSubmission(payload());
    });
    let verified: ReturnType<typeof result.current.applyCreateResult>;
    act(() => {
      verified = result.current.applyCreateResult(started!.receipt, {
        kind: "unknown",
        taskId: "task-1",
      });
    });
    let malformed: ReturnType<typeof result.current.applyCreateResult> = null;
    act(() => {
      malformed = result.current.applyCreateResult(verified!, {
        kind: "unknown",
        taskId: null,
      });
    });
    expect(malformed).toMatchObject({ taskId: "task-1", state: "unknown" });
  });

  it("keeps a known task ID and blocks replay after a malformed success", () => {
    const { result } = renderHook(() => useTaskCreationRecovery(ARGS));
    let started: ReturnType<typeof result.current.beginSubmission>;
    act(() => {
      started = result.current.beginSubmission(payload());
    });
    act(() => {
      result.current.applyCreateResult(started!.receipt, {
        kind: "unknown",
        taskId: "task-1",
      });
    });
    expect(result.current.receipt).toMatchObject({
      state: "unknown",
      taskId: "task-1",
      replayable: false,
      everUncertain: true,
    });
  });

  it("ignores a late result after the owner scope changed", () => {
    const { result, rerender } = renderHook((args: typeof ARGS) => useTaskCreationRecovery(args), {
      initialProps: ARGS,
    });
    let started: ReturnType<typeof result.current.beginSubmission>;
    act(() => {
      started = result.current.beginSubmission(payload());
    });

    rerender({ ...ARGS, workspaceId: "ws-2" });
    expect(result.current.receipt).toBeNull();

    let applied: ReturnType<typeof result.current.applyCreateResult>;
    act(() => {
      applied = result.current.applyCreateResult(started!.receipt, {
        kind: "accepted",
        taskId: "task-1",
        taskSessionId: "session-1",
        queuedForStepId: null,
      });
    });
    // The late result updates only its own retained receipt; it never becomes
    // the current dialog owner.
    expect(applied!).toMatchObject({ state: "accepted", taskId: "task-1" });
    expect(result.current.receipt).toBeNull();
    expect(
      loadTaskCreationReceipt({ userId: "user-1", workspaceId: "ws-1", draftId: "task" }),
    ).toMatchObject({ submissionId: started!.receipt.submissionId, state: "accepted" });
  });
});

describe("useTaskCreationRecovery read-only operations", () => {
  it("applies a found lookup and never posts", async () => {
    const { result } = renderHook(() => useTaskCreationRecovery(ARGS));
    let started: ReturnType<typeof result.current.beginSubmission>;
    act(() => {
      started = result.current.beginSubmission(payload());
    });
    lookupMock.mockResolvedValue({
      id: "task-1",
      workspace_id: "ws-1",
      external_id: started!.receipt.externalId,
      creation_complete: false,
    });

    let observation: Awaited<ReturnType<typeof result.current.lookup>>;
    await act(async () => {
      observation = await result.current.lookup(started!.receipt);
      result.current.applyLookupResult(started!.receipt, observation!);
    });

    expect(result.current.receipt).toMatchObject({
      state: "incomplete",
      taskId: "task-1",
      creationComplete: false,
    });
  });

  it("treats a lookup miss as uncertainty, never as permission", async () => {
    const { result } = renderHook(() => useTaskCreationRecovery(ARGS));
    let started: ReturnType<typeof result.current.beginSubmission>;
    act(() => {
      started = result.current.beginSubmission(payload());
    });
    lookupMock.mockRejectedValue(new ApiError("not found", 404, { error: "not found" }));

    await act(async () => {
      const observation = await result.current.lookup(started!.receipt);
      const next = result.current.applyLookupResult(started!.receipt, observation);
      expect(next?.state).toBe("unknown");
    });
    expect(result.current.receipt?.everUncertain).toBe(true);
    expect(result.current.receipt?.externalId).toBe(started!.receipt.externalId);
  });

  it("marks a deleted known task unavailable without allocating a replacement", async () => {
    const { result } = renderHook(() => useTaskCreationRecovery(ARGS));
    let started: ReturnType<typeof result.current.beginSubmission>;
    act(() => {
      started = result.current.beginSubmission(payload());
    });
    act(() => {
      result.current.applyCreateResult(started!.receipt, {
        kind: "accepted",
        taskId: "task-1",
        taskSessionId: "session-1",
        queuedForStepId: null,
      });
    });
    fetchTaskMock.mockRejectedValue(new ApiError("not found", 404, { error: "not found" }));

    await act(async () => {
      await result.current.hydrateTask(result.current.receipt!);
    });
    expect(result.current.receipt?.state).toBe("unavailable");
  });
});

describe("isRecoverySubmitBlocked", () => {
  it("blocks ordinary submission only after a receipt leaves submitting", () => {
    const base = createTaskCreationReceipt({
      scope: { userId: "user-1", workspaceId: "ws-1", draftId: "task" },
      payload: payload(),
      runtimeId: "runtime-1",
    });
    expect(isRecoverySubmitBlocked(null)).toBe(false);
    expect(isRecoverySubmitBlocked({ ...base, state: "submitting" })).toBe(false);
    expect(isRecoverySubmitBlocked({ ...base, state: "unknown" })).toBe(true);
    expect(isRecoverySubmitBlocked({ ...base, state: "accepted" })).toBe(true);
  });
});

describe("lookup misses preserve a known task state", () => {
  it("keeps accepted and unavailable states while marking uncertainty", () => {
    const { result } = renderHook(() => useTaskCreationRecovery(ARGS));
    let started: ReturnType<typeof result.current.beginSubmission>;
    act(() => {
      started = result.current.beginSubmission(payload());
    });
    let accepted: ReturnType<typeof result.current.applyCreateResult>;
    act(() => {
      accepted = result.current.applyCreateResult(started!.receipt, {
        kind: "accepted",
        taskId: "task-1",
        taskSessionId: "session-1",
        queuedForStepId: null,
      });
    });
    let missed: ReturnType<typeof result.current.applyLookupMiss> = null;
    act(() => {
      missed = result.current.applyLookupMiss(accepted!);
    });
    expect(missed).toMatchObject({ state: "accepted", taskId: "task-1", everUncertain: true });

    let unavailable: ReturnType<typeof result.current.applyCreateResult>;
    act(() => {
      unavailable = result.current.applyCreateResult(missed!, {
        kind: "unknown",
        taskId: "task-1",
      });
    });
    let missedAgain: ReturnType<typeof result.current.applyLookupMiss> = null;
    act(() => {
      missedAgain = result.current.applyLookupMiss(unavailable!);
    });
    expect(missedAgain).toMatchObject({ state: "unknown", taskId: "task-1" });
  });
});
