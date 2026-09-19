import { afterEach, describe, expect, it, vi } from "vitest";
import { applyStatusDrop, type StatusDropDeps } from "./use-board-drag";
import { ApprovalGateError } from "@/lib/api/domains/office-status-gate";
import type { OfficeTask, OfficeTaskStatus } from "@/lib/state/slices/office/types";
import { __resetOfficeTaskContentSyncForTests } from "@/lib/state/office-task-content-sync";

const GATE_MESSAGE = "Cannot mark done: awaiting approval from Ada, Grace";

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

/**
 * Unlike `deps`, `getTask` here reflects every prior `patchTask` call, the
 * way the real board (backed by the Zustand store) does. The interleaving
 * tests below need a second drop to see the first drop's still-optimistic
 * patch, not a snapshot frozen at test setup.
 */
function liveDeps(initial: OfficeTask) {
  let current = initial;
  const d = {
    getTask: vi.fn(() => current),
    patchTask: vi.fn((_id: string, patch: Partial<OfficeTask>) => {
      current = { ...current, ...patch };
    }),
    updateStatus: vi.fn(),
    onError: vi.fn(),
  } satisfies StatusDropDeps;
  return { d, getCurrent: () => current };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
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

  it("rolls the card back to its prior status when the mutation fails", async () => {
    const before = task("t1", "in_review");
    // rawStatus is set by the store on ingestion and must survive a rollback.
    before.rawStatus = "REVIEW";
    const d = deps(before, async () => {
      throw new Error("boom");
    });

    await applyStatusDrop("t1", "done", d);

    expect(d.patchTask).toHaveBeenNthCalledWith(1, "t1", { status: "done" });
    expect(d.patchTask).toHaveBeenNthCalledWith(2, "t1", {
      status: before.status,
      rawStatus: before.rawStatus,
    });
    expect(d.onError).toHaveBeenCalledWith("boom");
  });

  it("surfaces the approver-gate sentence rather than a bare failure", async () => {
    // A plain Error (anything that isn't the typed ApprovalGateError below)
    // still rolls back to the snapshot and surfaces its message verbatim.
    const gate = GATE_MESSAGE;
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
    const gate = GATE_MESSAGE;
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

    expect(d.patchTask).toHaveBeenNthCalledWith(2, "t1", {
      status: before.status,
      rawStatus: before.rawStatus,
    });
    expect(d.onError).toHaveBeenCalledTimes(1);
  });
});

describe("applyStatusDrop generation guard — overlapping status mutations", () => {
  it("older-fails-after-newer-succeeds: the older drop's rollback does not clobber the newer, server-confirmed status", async () => {
    const { d, getCurrent } = liveDeps(task("t1", "todo"));
    const older = deferred<void>();
    const newer = deferred<void>();
    d.updateStatus.mockImplementationOnce(() => older.promise);
    d.updateStatus.mockImplementationOnce(() => newer.promise);

    // Two fast consecutive drags on the same card: the first (todo ->
    // in_progress) is still in flight when the second (in_progress ->
    // blocked) is dropped.
    const p1 = applyStatusDrop("t1", "in_progress", d);
    const p2 = applyStatusDrop("t1", "blocked", d);

    newer.resolve();
    await p2;
    expect(getCurrent().status).toBe("blocked");

    older.reject(new Error("boom"));
    await p1;

    // The stale failure must not roll the board back past the newer,
    // already-succeeded status, but it still surfaces its own toast.
    expect(getCurrent().status).toBe("blocked");
    expect(d.onError).toHaveBeenCalledTimes(1);
  });

  it("gate-redirect superseded by a newer success does not clobber the newer status", async () => {
    const { d, getCurrent } = liveDeps(task("t1", "todo"));
    const older = deferred<void>();
    const newer = deferred<void>();
    d.updateStatus.mockImplementationOnce(() => older.promise);
    d.updateStatus.mockImplementationOnce(() => newer.promise);

    const p1 = applyStatusDrop("t1", "done", d);
    const p2 = applyStatusDrop("t1", "blocked", d);

    newer.resolve();
    await p2;
    expect(getCurrent().status).toBe("blocked");

    // The older drop's gate redirect resolves after the newer drop already
    // succeeded: redirecting to "in_review" here would clobber the newer,
    // server-confirmed "blocked" status the same way an unconditional
    // rollback would.
    const gate = GATE_MESSAGE;
    older.reject(new ApprovalGateError(gate, "in_review"));
    await p1;

    expect(getCurrent().status).toBe("blocked");
    expect(d.onError).toHaveBeenCalledWith(gate);
  });

  it("a newer gate redirect is not clobbered by an older, later-resolving plain failure", async () => {
    const { d, getCurrent } = liveDeps(task("t1", "todo"));
    const older = deferred<void>();
    const newer = deferred<void>();
    d.updateStatus.mockImplementationOnce(() => older.promise);
    d.updateStatus.mockImplementationOnce(() => newer.promise);

    // The older drop (todo -> in_progress) is still in flight when the newer
    // one (-> done) is dropped and hits the approver gate first, redirecting
    // to "in_review" server-side.
    const p1 = applyStatusDrop("t1", "in_progress", d);
    const p2 = applyStatusDrop("t1", "done", d);

    const gate = GATE_MESSAGE;
    newer.reject(new ApprovalGateError(gate, "in_review"));
    await p2;
    expect(getCurrent().status).toBe("in_review");

    // The older drop then fails with a plain error. It must not be able to
    // roll the board back past the newer, server-confirmed gate redirect.
    older.reject(new Error("boom"));
    await p1;

    expect(getCurrent().status).toBe("in_review");
    expect(getCurrent().rawStatus).toBe("in_review");
    expect(d.onError).toHaveBeenCalledTimes(2);
  });

  it("restores the baseline when an older drop fails before a newer drop", async () => {
    const { d, getCurrent } = liveDeps(task("t1", "todo"));
    const older = deferred<void>();
    const newer = deferred<void>();
    d.updateStatus.mockImplementationOnce(() => older.promise);
    d.updateStatus.mockImplementationOnce(() => newer.promise);

    const p1 = applyStatusDrop("t1", "in_progress", d);
    const p2 = applyStatusDrop("t1", "blocked", d);

    older.reject(new Error("older failed"));
    await p1;
    expect(getCurrent().status).toBe("blocked");

    newer.reject(new Error("newer failed"));
    await p2;

    expect(getCurrent().status).toBe("todo");
    expect(d.onError).toHaveBeenCalledTimes(2);
  });

  it("keeps an older approval-gate redirect when a newer drop fails", async () => {
    const { d, getCurrent } = liveDeps(task("t1", "todo"));
    const older = deferred<void>();
    const newer = deferred<void>();
    d.updateStatus.mockImplementationOnce(() => older.promise);
    d.updateStatus.mockImplementationOnce(() => newer.promise);

    const p1 = applyStatusDrop("t1", "done", d);
    const p2 = applyStatusDrop("t1", "blocked", d);

    older.reject(new ApprovalGateError(GATE_MESSAGE, "in_review"));
    await p1;
    expect(getCurrent().status).toBe("blocked");

    newer.reject(new Error("newer failed"));
    await p2;

    expect(getCurrent().status).toBe("in_review");
    expect(getCurrent().rawStatus).toBe("in_review");
    expect(d.onError).toHaveBeenCalledTimes(2);
  });
});
