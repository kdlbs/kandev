import { afterEach, describe, expect, it, vi } from "vitest";
import { applyStatusDrop, type StatusDropDeps } from "./use-board-drag";
import { StatusChangeRefusedError } from "@/lib/api/domains/office-status-gate";
import type { OfficeTask, OfficeTaskStatus } from "@/lib/state/slices/office/types";
import { __resetOfficeTaskContentSyncForTests } from "@/lib/state/office-task-content-sync";

afterEach(() => {
  __resetOfficeTaskContentSyncForTests();
});

function task(id: string, status: OfficeTaskStatus): OfficeTask {
  return {
    id,
    workspaceId: "workspace-1",
    identifier: id.toUpperCase(),
    title: id,
    status,
    priority: "none",
    createdAt: "2026-01-01T00:00:00.000Z",
    updatedAt: "2026-01-01T00:00:00.000Z",
  };
}

function deps(stored: OfficeTask, updateStatus: StatusDropDeps["updateStatus"]) {
  return {
    getTask: vi.fn(() => stored),
    patchTask: vi.fn(),
    updateStatus: vi.fn(updateStatus),
    onError: vi.fn(),
    onInfo: vi.fn(),
  } satisfies StatusDropDeps;
}

describe("applyStatusDrop on Done before the final workflow step", () => {
  it("keeps the card on Done when the move completed the task", async () => {
    const d = deps(task("t1", "in_progress"), async () => ({
      status: "done",
      workflowStepName: "Done",
    }));

    await applyStatusDrop("t1", "done", d);

    expect(d.patchTask).toHaveBeenCalledTimes(1);
    expect(d.patchTask).toHaveBeenCalledWith("t1", { status: "done" });
    expect(d.onInfo).not.toHaveBeenCalled();
    expect(d.onError).not.toHaveBeenCalled();
  });

  // @covers AC-OFFICE-HUMAN-DONE-001.2
  it("settles on a non-done move result and reports the step", async () => {
    const d = deps(task("t1", "in_progress"), async () => ({
      status: "in_progress",
      workflowStepName: "Ship",
    }));

    await applyStatusDrop("t1", "done", d);

    expect(d.patchTask).toHaveBeenLastCalledWith("t1", {
      status: "in_progress",
      rawStatus: "in_progress",
    });
    expect(d.onInfo).toHaveBeenCalledWith('Moved to "Ship". This step doesn\'t complete the task.');
    expect(d.onError).not.toHaveBeenCalled();
  });

  // @covers AC-OFFICE-HUMAN-DONE-001.3
  it("rolls a refused human done back to the original column", async () => {
    const before = task("t1", "in_progress");
    before.rawStatus = "IN_PROGRESS";
    const d = deps(before, async () => {
      throw new StatusChangeRefusedError("Cannot mark done: awaiting approval from CEO");
    });

    await applyStatusDrop("t1", "done", d);

    expect(d.patchTask).toHaveBeenLastCalledWith("t1", {
      status: "in_progress",
      rawStatus: "IN_PROGRESS",
    });
    expect(d.onError).toHaveBeenCalledWith("Cannot mark done: awaiting approval from CEO");
  });

  it("does not apply an older move result while a newer drop is still in flight", async () => {
    let current = task("t1", "in_progress");
    let resolveFirst!: (value: { status: OfficeTaskStatus; workflowStepName: string }) => void;
    const first = new Promise<{ status: OfficeTaskStatus; workflowStepName: string }>((res) => {
      resolveFirst = res;
    });
    let resolveSecond!: (value: { status: OfficeTaskStatus }) => void;
    const second = new Promise<{ status: OfficeTaskStatus }>((res) => {
      resolveSecond = res;
    });
    const d = {
      getTask: vi.fn(() => current),
      patchTask: vi.fn((_id: string, patch: Partial<OfficeTask>) => {
        current = { ...current, ...patch };
      }),
      updateStatus: vi.fn().mockReturnValueOnce(first).mockReturnValueOnce(second),
      onError: vi.fn(),
      onInfo: vi.fn(),
    } satisfies StatusDropDeps;

    const doneDrop = applyStatusDrop("t1", "done", d);
    const blockedDrop = applyStatusDrop("t1", "blocked", d);
    resolveFirst({ status: "in_progress", workflowStepName: "Ship" });
    await doneDrop;

    expect(current.status).toBe("blocked");

    resolveSecond({ status: "blocked" });
    await blockedDrop;
    expect(current.status).toBe("blocked");
  });

  it("explains a move queued behind the final step's WIP limit", async () => {
    const d = deps(task("t1", "in_progress"), async () => ({
      status: "in_progress",
      workflowStepName: "Done",
      queued: true,
    }));

    await applyStatusDrop("t1", "done", d);

    expect(d.onInfo).toHaveBeenCalledWith(
      'Queued for "Done". The task finishes when the step has room.',
    );
  });
});
