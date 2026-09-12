import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { COMMENTS_STORAGE_PREFIX } from "@/lib/state/slices/comments/persistence";
import type { PlanComment } from "@/lib/state/slices/comments";
import type { TaskPlan, TaskPlanCommentSnapshot } from "@/lib/types/http";
import { WebSocketRequestError } from "@/lib/ws/request-error";
import { planCommentMigrationFor } from "./plan-comment-migration";

const api = vi.hoisted(() => ({
  createTaskPlanComment: vi.fn(),
  updateTaskPlanComment: vi.fn(),
}));
const plans = vi.hoisted(() => ({ getTaskPlan: vi.fn() }));
vi.mock("@/lib/api/domains/plan-comment-api", () => api);
vi.mock("@/lib/api/domains/plan-api", () => plans);

const TASK = "task-1";
const SESSION = "session-1";
const PLAN = "plan-1";
const DATE = "2026-09-02T00:00:00Z";
const EDITED_BODY = "Edited feedback";
const plan: TaskPlan = {
  id: PLAN,
  task_id: TASK,
  title: "Plan",
  content: "Step",
  created_by: "agent",
  created_at: DATE,
  updated_at: DATE,
};
const comment: PlanComment = {
  id: "legacy-1",
  sessionId: SESSION,
  source: "plan",
  text: "Feedback",
  selectedText: "Step",
  from: 1,
  to: 5,
  createdAt: DATE,
  status: "pending",
};
const snapshot: TaskPlanCommentSnapshot = {
  task_id: TASK,
  plan_id: PLAN,
  revision: 1,
  comments: [
    {
      id: comment.id,
      task_id: TASK,
      plan_id: PLAN,
      body: comment.text,
      selected_text: comment.selectedText,
      anchor_from: 1,
      anchor_to: 5,
      version: 1,
      created_at: DATE,
      updated_at: DATE,
    },
  ],
};
let releases: Array<() => void>;

function write(rows = [comment], sessionId = SESSION) {
  window.sessionStorage.setItem(`${COMMENTS_STORAGE_PREFIX}${sessionId}`, JSON.stringify(rows));
}
function saved() {
  return JSON.parse(window.sessionStorage.getItem(`${COMMENTS_STORAGE_PREFIX}${SESSION}`) ?? "[]");
}
function setup(connected = true) {
  const store = createAppStore();
  store.getState().setTaskPlan(TASK, plan);
  store.getState().setConnectionStatus(connected ? "connected" : "disconnected");
  const recovery = planCommentMigrationFor(store, TASK);
  const detach = recovery.attach(vi.fn().mockResolvedValue(undefined));
  releases.push(detach);
  recovery.update({ sessionIds: [SESSION], complete: true, loading: false });
  const state = () => store.getState().taskPlans.commentsMigrationByTaskId[TASK];
  return { store, recovery, detach, state };
}
function settle() {
  return vi.advanceTimersByTimeAsync(0);
}
function deferred() {
  let resolve!: (value: TaskPlanCommentSnapshot) => void;
  const promise = new Promise<TaskPlanCommentSnapshot>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function editedSnapshot(body = EDITED_BODY, version = 2): TaskPlanCommentSnapshot {
  return { ...snapshot, revision: version, comments: [{ ...snapshot.comments[0], body, version }] };
}

async function acknowledgeDuringLocalEdit(edited = { ...comment, text: EDITED_BODY }) {
  write();
  const pending = deferred();
  api.createTaskPlanComment
    .mockReturnValueOnce(pending.promise)
    .mockRejectedValue(new WebSocketRequestError("UUID already used", "plan_comments_changed"));
  const fixture = setup();
  await settle();
  write([edited]);
  fixture.recovery.update({ sessionIds: [SESSION], complete: true, loading: false });
  pending.resolve(snapshot);
  await settle();
  return fixture;
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.resetAllMocks();
  window.sessionStorage.clear();
  releases = [];
  api.createTaskPlanComment.mockResolvedValue(snapshot);
  plans.getTaskPlan.mockResolvedValue(plan);
});
afterEach(() => {
  for (const release of releases) release();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("task-scoped plan comment recovery", () => {
  it("resolves an unknown plan without relying on an ordinary comment loader", async () => {
    write();
    const store = createAppStore();
    store.getState().setConnectionStatus("connected");
    const recovery = planCommentMigrationFor(store, TASK);
    releases.push(recovery.attach(vi.fn()));
    recovery.update({ sessionIds: [SESSION], complete: true, loading: false });
    await settle();
    expect(store.getState().taskPlans.commentsMigrationByTaskId[TASK]).toMatchObject({
      status: "complete",
      pendingCount: 0,
    });
    expect(plans.getTaskPlan).toHaveBeenCalledOnce();
    expect(saved()).toEqual([]);
  });

  // @covers AC-TASKS-PLAN-COMMENTS-004.2, AC-TASKS-PLAN-COMMENTS-004.6
  it("uses three connected attempts then bounded background backoff with the original UUID", async () => {
    write();
    api.createTaskPlanComment.mockRejectedValue(new Error("offline"));
    const { state } = setup();
    await settle();
    expect(state()).toMatchObject({ status: "retrying", pendingCount: 1 });
    for (const [index, delay] of [1000, 2000, 30000, 60000, 120000, 120000].entries()) {
      await vi.advanceTimersByTimeAsync(delay - 1);
      expect(api.createTaskPlanComment).toHaveBeenCalledTimes(index + 1);
      await vi.advanceTimersByTimeAsync(1);
      expect(api.createTaskPlanComment).toHaveBeenCalledTimes(index + 2);
    }
    expect(state()).toMatchObject({ status: "failed", pendingCount: 1, failure: "transient" });
    expect(
      api.createTaskPlanComment.mock.calls.every(
        ([input]) => input.id === comment.id && input.body === comment.text,
      ),
    ).toBe(true);
    expect(saved()).toEqual([comment]);
  });

  it("coalesces multiple surfaces and resume signals around one in-flight upload", async () => {
    write();
    const pending = deferred();
    api.createTaskPlanComment.mockReturnValue(pending.promise);
    const { store, recovery, state, detach } = setup();
    const other = planCommentMigrationFor(store, TASK);
    expect(other).toBe(recovery);
    releases.push(other.attach(vi.fn()));
    other.update({ sessionIds: [SESSION], complete: true, loading: false });
    recovery.wake();
    other.wake();
    await settle();
    detach();
    pending.resolve(snapshot);
    await settle();
    expect(api.createTaskPlanComment).toHaveBeenCalledOnce();
    expect(state()).toMatchObject({ status: "complete", pendingCount: 0 });
  });

  it("spends no attempts while hidden or offline and resumes without a manual retry", async () => {
    write();
    const { store, recovery, state } = setup(false);
    await vi.advanceTimersByTimeAsync(120000);
    expect(api.createTaskPlanComment).not.toHaveBeenCalled();
    expect(state()?.pendingCount).toBe(1);
    const visibility = vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
    store.getState().setConnectionStatus("connected");
    await settle();
    expect(api.createTaskPlanComment).not.toHaveBeenCalled();
    visibility.mockReturnValue("visible");
    await recovery.wake();
    expect(state()?.status).toBe("complete");
  });

  it("stops timers when the last surface unmounts and resumes on remount", async () => {
    write();
    api.createTaskPlanComment.mockRejectedValueOnce(new Error("offline"));
    const { recovery, detach, state } = setup();
    await settle();
    detach();
    await vi.advanceTimersByTimeAsync(120000);
    expect(api.createTaskPlanComment).toHaveBeenCalledOnce();
    releases.push(recovery.attach(vi.fn()));
    recovery.update({ sessionIds: [SESSION], complete: true, loading: false });
    await settle();
    expect(state()?.status).toBe("complete");
  });

  it("does not acknowledge a late result after task departure, including immediate remount", async () => {
    write();
    const old = deferred();
    const next = deferred();
    api.createTaskPlanComment.mockReturnValueOnce(old.promise).mockReturnValueOnce(next.promise);
    const { recovery, detach, store } = setup();
    await settle();
    detach();
    releases.push(recovery.attach(vi.fn()));
    recovery.update({ sessionIds: [SESSION], complete: true, loading: false });
    old.resolve(snapshot);
    await settle();
    expect(saved()).toEqual([comment]);
    expect(store.getState().taskPlans.commentsByTaskId[TASK]).toBeUndefined();
    next.resolve(snapshot);
    await settle();
    expect(saved()).toEqual([]);
  });
});

describe("legacy recovery identity", () => {
  it("surfaces exhausted plan lookup failures for known drafts and later recovers", async () => {
    write();
    plans.getTaskPlan.mockRejectedValue(new Error("offline"));
    const store = createAppStore();
    store.getState().setConnectionStatus("connected");
    const recovery = planCommentMigrationFor(store, TASK);
    releases.push(recovery.attach(vi.fn()));
    recovery.update({ sessionIds: [SESSION], complete: true, loading: false });
    await vi.advanceTimersByTimeAsync(3000);
    expect(plans.getTaskPlan).toHaveBeenCalledTimes(3);
    expect(store.getState().taskPlans.commentsMigrationByTaskId[TASK]).toMatchObject({
      status: "failed",
      pendingCount: 1,
      failure: "transient",
    });
    expect(saved()).toEqual([comment]);
    plans.getTaskPlan.mockResolvedValue(plan);
    await vi.advanceTimersByTimeAsync(30000);
    expect(store.getState().taskPlans.commentsMigrationByTaskId[TASK]).toMatchObject({
      status: "complete",
      pendingCount: 0,
    });
    expect(saved()).toEqual([]);
  });

  it("does not let a hidden wake suppress the immediate visible retry", async () => {
    write();
    api.createTaskPlanComment.mockRejectedValueOnce(new Error("offline"));
    const { recovery, state } = setup();
    await settle();
    const visibility = vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
    document.dispatchEvent(new Event("visibilitychange"));
    await recovery.wake();
    visibility.mockReturnValue("visible");
    await recovery.wake();
    expect(api.createTaskPlanComment).toHaveBeenCalledTimes(2);
    expect(state()).toMatchObject({ status: "complete", pendingCount: 0 });
  });

  it("does not acknowledge a stale generation after a plan is deleted and restored", async () => {
    write();
    const old = deferred();
    const next = deferred();
    api.createTaskPlanComment.mockReturnValueOnce(old.promise).mockReturnValueOnce(next.promise);
    const { store } = setup();
    await settle();
    store.getState().setTaskPlan(TASK, null);
    store.getState().setTaskPlan(TASK, plan);
    old.resolve(snapshot);
    await settle();
    expect(saved()).toEqual([comment]);
    expect(store.getState().taskPlans.commentsByTaskId[TASK]).toBeUndefined();
    next.resolve(snapshot);
    await settle();
    expect(saved()).toEqual([]);
  });
});

describe("legacy edits during acknowledgement", () => {
  it("updates an acknowledged body at its original UUID and exact version", async () => {
    api.updateTaskPlanComment.mockResolvedValue(editedSnapshot());
    const { store, state } = await acknowledgeDuringLocalEdit();
    expect(saved()).toEqual([{ ...comment, text: EDITED_BODY }]);
    await vi.advanceTimersByTimeAsync(1000);
    expect(state()).toMatchObject({ status: "complete", pendingCount: 0 });
    expect(store.getState().taskPlans.commentsByTaskId[TASK]).toEqual(editedSnapshot());
    expect(api.createTaskPlanComment).toHaveBeenCalledOnce();
    expect(api.updateTaskPlanComment).toHaveBeenCalledWith({
      taskId: TASK,
      planId: PLAN,
      id: comment.id,
      expectedVersion: 1,
      body: EDITED_BODY,
    });
    expect(saved()).toEqual([]);
  });

  it("retains local feedback without overwriting a conflicting server edit", async () => {
    const remote = editedSnapshot("Another client's feedback", 3);
    api.updateTaskPlanComment.mockRejectedValue(
      new WebSocketRequestError("Changed version", "plan_comments_changed", { snapshot: remote }),
    );
    const { store, state } = await acknowledgeDuringLocalEdit();
    await vi.advanceTimersByTimeAsync(121000);
    expect(api.updateTaskPlanComment).toHaveBeenCalledOnce();
    expect(state()).toMatchObject({ status: "failed", pendingCount: 1, failure: "conflict" });
    expect(saved()).toEqual([{ ...comment, text: EDITED_BODY }]);
    expect(store.getState().taskPlans.commentsByTaskId[TASK]).toEqual(remote);
  });

  it("reconciles an accepted update whose response was lost", async () => {
    api.updateTaskPlanComment.mockRejectedValueOnce(new Error("response lost")).mockRejectedValue(
      new WebSocketRequestError("Changed version", "plan_comments_changed", {
        snapshot: editedSnapshot(),
      }),
    );
    const { state } = await acknowledgeDuringLocalEdit();
    await vi.advanceTimersByTimeAsync(3000);
    expect(state()).toMatchObject({ status: "complete", pendingCount: 0 });
    expect(api.createTaskPlanComment).toHaveBeenCalledOnce();
    expect(api.updateTaskPlanComment).toHaveBeenCalledTimes(2);
    expect(saved()).toEqual([]);
  });

  it("does not reinterpret a changed legacy selection as a body-only update", async () => {
    const edited = { ...comment, text: EDITED_BODY, selectedText: "Other step" };
    const { state } = await acknowledgeDuringLocalEdit(edited);
    await vi.advanceTimersByTimeAsync(1000);
    expect(state()).toMatchObject({ status: "failed", pendingCount: 1, failure: "conflict" });
    expect(api.createTaskPlanComment).toHaveBeenCalledOnce();
    expect(api.updateTaskPlanComment).not.toHaveBeenCalled();
    expect(saved()).toEqual([edited]);
  });
});

describe("legacy feedback retention", () => {
  it("keeps a paused partial recovery quiet when a plan still exists", async () => {
    write([comment, { ...comment, id: "legacy-2" }]);
    const pending = deferred();
    api.createTaskPlanComment.mockReturnValueOnce(pending.promise);
    const { store, state } = setup();
    await settle();
    store.getState().setConnectionStatus("disconnected");
    pending.resolve(snapshot);
    await settle();
    expect(state()).toMatchObject({ status: "idle", pendingCount: 1 });
    expect(api.createTaskPlanComment).toHaveBeenCalledOnce();
  });

  it("retains identified drafts when later storage scans are empty or unavailable", async () => {
    write();
    const { recovery, state } = setup(false);
    window.sessionStorage.clear();
    recovery.update({ sessionIds: [SESSION], complete: true, loading: false });
    expect(state()?.pendingCount).toBe(1);
    const read = vi.spyOn(window.sessionStorage, "getItem").mockImplementation(() => {
      throw new DOMException("denied", "SecurityError");
    });
    recovery.update({ sessionIds: [SESSION], complete: true, loading: false });
    expect(state()?.pendingCount).toBe(1);
    expect(read).toHaveBeenCalled();
  });

  it("retries refused cleanup without uploading the acknowledged feedback again", async () => {
    write();
    const removal = vi.spyOn(window.sessionStorage, "removeItem").mockImplementation(() => {
      throw new DOMException("denied", "SecurityError");
    });
    const { state } = setup();
    await settle();
    expect(state()?.pendingCount).toBe(1);
    expect(saved()).toEqual([comment]);
    removal.mockRestore();
    await vi.advanceTimersByTimeAsync(1000);
    expect(api.createTaskPlanComment).toHaveBeenCalledOnce();
    expect(saved()).toEqual([]);
    expect(state()?.status).toBe("complete");
  });

  it.each(["validation_error", "unauthorized", "plan_comments_changed"])(
    "requires explicit retry for %s instead of looping mutations",
    async (code) => {
      write();
      api.createTaskPlanComment.mockRejectedValue(new WebSocketRequestError("rejected", code));
      const { recovery, state } = setup();
      await settle();
      recovery.wake();
      await vi.advanceTimersByTimeAsync(120000);
      expect(api.createTaskPlanComment).toHaveBeenCalledOnce();
      expect(state()).toMatchObject({ status: "failed", pendingCount: 1 });
      api.createTaskPlanComment.mockResolvedValue(snapshot);
      await recovery.retry();
      expect(saved()).toEqual([]);
    },
  );

  it("rescans newly discovered task sessions without touching another task's records", async () => {
    write([comment], "foreign-session");
    const { recovery, state } = setup();
    await settle();
    expect(state()?.status).toBe("complete");
    expect(api.createTaskPlanComment).not.toHaveBeenCalled();
    write();
    recovery.update({ sessionIds: [SESSION], complete: true, loading: false });
    await settle();
    expect(state()?.pendingCount).toBe(0);
    expect(
      JSON.parse(
        window.sessionStorage.getItem(`${COMMENTS_STORAGE_PREFIX}foreign-session`) ?? "[]",
      ),
    ).toEqual([comment]);
  });
});
