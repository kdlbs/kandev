import { describe, expect, it, vi } from "vitest";
import { applyStatusDrop, type StatusDropDeps } from "./use-board-drag";
import { ApprovalGateError } from "@/lib/api/domains/office-status-gate";
import type { OfficeTask, OfficeTaskStatus } from "@/lib/state/slices/office/types";

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

function deps(
  stored: OfficeTask | undefined,
  updateStatus: StatusDropDeps["updateStatus"] = vi.fn().mockResolvedValue(undefined),
) {
  return {
    getTask: vi.fn(() => stored),
    patchTask: vi.fn(),
    updateStatus: vi.fn(updateStatus),
    onError: vi.fn(),
  } satisfies StatusDropDeps;
}

describe("applyStatusDrop", () => {
  it("patches the store then sends the status when a card crosses columns", async () => {
    const d = deps(task("t1", "todo"));

    await applyStatusDrop("t1", "in_progress", d);

    expect(d.patchTask).toHaveBeenCalledWith("t1", { status: "in_progress" });
    expect(d.updateStatus).toHaveBeenCalledWith("t1", "in_progress");
    expect(d.onError).not.toHaveBeenCalled();
  });

  it("applies the optimistic patch before awaiting the mutation", async () => {
    const order: string[] = [];
    const d = deps(task("t1", "todo"), async () => {
      order.push("update");
    });
    d.patchTask.mockImplementation(() => {
      order.push("patch");
    });

    await applyStatusDrop("t1", "done", d);

    expect(order).toEqual(["patch", "update"]);
  });

  it("is a no-op when the card is dropped back on its own column", async () => {
    const d = deps(task("t1", "todo"));

    await applyStatusDrop("t1", "todo", d);

    expect(d.patchTask).not.toHaveBeenCalled();
    expect(d.updateStatus).not.toHaveBeenCalled();
  });

  it("is a no-op when the dragged id is not a known task", async () => {
    const d = deps(undefined);

    await applyStatusDrop("ghost", "done", d);

    expect(d.patchTask).not.toHaveBeenCalled();
    expect(d.updateStatus).not.toHaveBeenCalled();
    expect(d.onError).not.toHaveBeenCalled();
  });

  it("rolls the card back to its whole prior snapshot when the mutation fails", async () => {
    const before = task("t1", "in_review");
    // rawStatus is set by the store on ingestion and must survive a rollback,
    // which is why the snapshot goes back whole rather than status-only.
    before.rawStatus = "REVIEW";
    const d = deps(before, async () => {
      throw new Error("boom");
    });

    await applyStatusDrop("t1", "done", d);

    expect(d.patchTask).toHaveBeenNthCalledWith(1, "t1", { status: "done" });
    expect(d.patchTask).toHaveBeenNthCalledWith(2, "t1", before);
    expect(d.onError).toHaveBeenCalledWith("boom");
  });

  it("surfaces the approver-gate sentence rather than a bare failure", async () => {
    // A plain Error (anything that isn't the typed ApprovalGateError below)
    // still rolls back to the snapshot and surfaces its message verbatim.
    const gate = "Cannot mark done: awaiting approval from Ada, Grace";
    const d = deps(task("t1", "in_review"), async () => {
      throw new Error(gate);
    });

    await applyStatusDrop("t1", "done", d);

    expect(d.onError).toHaveBeenCalledWith(gate);
  });

  it("settles on the redirected status instead of rolling back, when the approver gate redirects", async () => {
    // The approver gate doesn't reject the move: the backend redirects it to
    // in_review server-side, persists and broadcasts that, and only then
    // returns 409 (updateTaskStatusOrTranslateGate throws ApprovalGateError
    // for exactly this case). Rolling back to the pre-drop snapshot here
    // would show a status the server no longer holds.
    const before = task("t1", "todo");
    const gate = "Cannot mark done: awaiting approval from Ada, Grace";
    const d = deps(before, async () => {
      throw new ApprovalGateError(gate, "in_review");
    });

    await applyStatusDrop("t1", "done", d);

    expect(d.patchTask).toHaveBeenNthCalledWith(1, "t1", { status: "done" });
    // Patched to the redirected status only, not the whole snapshot: a
    // spread would reinstate the snapshot's stale rawStatus and the card
    // would re-normalize back to the old column.
    expect(d.patchTask).toHaveBeenNthCalledWith(2, "t1", { status: "in_review" });
    expect(d.patchTask).toHaveBeenCalledTimes(2);
    expect(d.onError).toHaveBeenCalledWith(gate);
  });

  it("still rolls back when the failure is not an Error", async () => {
    const before = task("t1", "todo");
    const d = deps(before, async () => {
      throw "string rejection";
    });

    await applyStatusDrop("t1", "blocked", d);

    expect(d.patchTask).toHaveBeenNthCalledWith(2, "t1", before);
    expect(d.onError).toHaveBeenCalledTimes(1);
  });
});
