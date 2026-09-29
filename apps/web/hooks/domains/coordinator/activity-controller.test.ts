import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { ActivityItem, ActivityPage } from "@/lib/api/domains/coordinator-activity-api";

const mocks = vi.hoisted(() => ({ list: vi.fn(), undo: vi.fn() }));

vi.mock("@/lib/api/domains/coordinator-activity-api", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/domains/coordinator-activity-api")>()),
  listActivity: mocks.list,
  undoActivity: mocks.undo,
}));

import { ActivityController } from "./activity-controller";

type Deferred<T> = { promise: Promise<T>; resolve: (v: T) => void; reject: (e: unknown) => void };

function deferred<T>(): Deferred<T> {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function row(id: string, overrides: Partial<ActivityItem> = {}): ActivityItem {
  return {
    id,
    coordinator_id: "co-1",
    workspace_id: "ws-1",
    action_class: "create_task",
    outcome: "approved",
    authorization: "requires_approval",
    target_task_id: "t-1",
    proposal_id: "p-1",
    actor_user_id: "u-1",
    reason_code: null,
    detail: `Row ${id}`,
    edited: false,
    refusal_count: 0,
    undone_at: null,
    undone_by: null,
    undo_of_id: null,
    created_at: "2026-09-29T10:00:00Z",
    updated_at: "2026-09-29T10:00:00Z",
    undoable: true,
    target_task_identifier: "KAN-1",
    from_step_id: null,
    ...overrides,
  };
}

function page(ids: string[], next: string | null = null): ActivityPage {
  return { rows: ids.map((id) => row(id)), next_cursor: next };
}

const flush = () => new Promise((r) => setTimeout(r, 0));

function make(onArrival?: (rows: ActivityItem[]) => void) {
  return new ActivityController({
    workspaceId: "ws-1",
    coordinatorId: "co-1",
    activityClass: undefined,
    onArrival,
  });
}

async function loaded(ids = ["a", "b"], next: string | null = null) {
  mocks.list.mockResolvedValueOnce(page(ids, next));
  const ctl = make();
  ctl.start();
  await flush();
  return ctl;
}

beforeEach(() => {
  mocks.list.mockReset();
  mocks.undo.mockReset();
});

describe("first load", () => {
  it("is loading, then loaded with the rows and cursor", async () => {
    const d = deferred<ActivityPage>();
    mocks.list.mockReturnValueOnce(d.promise);
    const ctl = make();
    ctl.start();
    expect(ctl.getSnapshot().status).toBe("loading");
    d.resolve(page(["a"], "c1"));
    await flush();
    expect(ctl.getSnapshot()).toMatchObject({ status: "loaded", nextCursor: "c1" });
    expect(ctl.getSnapshot().rows.map((r) => r.id)).toEqual(["a"]);
  });

  it("fails without rows and Retry loads again", async () => {
    mocks.list.mockRejectedValueOnce(new Error("boom"));
    const ctl = make();
    ctl.start();
    await flush();
    expect(ctl.getSnapshot().status).toBe("failed");
    mocks.list.mockResolvedValueOnce(page(["a"]));
    ctl.retry();
    await flush();
    expect(ctl.getSnapshot().status).toBe("loaded");
  });

  it("ignores an event while loading and after a failure", async () => {
    const d = deferred<ActivityPage>();
    mocks.list.mockReturnValueOnce(d.promise);
    const ctl = make();
    ctl.start();
    ctl.refresh();
    expect(mocks.list).toHaveBeenCalledTimes(1);
    d.reject(new Error("x"));
    await flush();
    ctl.refresh();
    expect(mocks.list).toHaveBeenCalledTimes(1);
  });

  it("drops a response after dispose", async () => {
    const d = deferred<ActivityPage>();
    mocks.list.mockReturnValueOnce(d.promise);
    const onArrival = vi.fn();
    const ctl = make(onArrival);
    ctl.start();
    ctl.dispose();
    d.resolve(page(["a"]));
    await flush();
    expect(ctl.getSnapshot().status).toBe("loading");
    expect(onArrival).not.toHaveBeenCalled();
  });
});

describe("load more", () => {
  it("appends the next page after the cursor", async () => {
    const ctl = await loaded(["a"], "c1");
    mocks.list.mockResolvedValueOnce(page(["b"], null));
    ctl.loadMore();
    await flush();
    expect(mocks.list).toHaveBeenLastCalledWith("ws-1", "co-1", {
      class: undefined,
      before: "c1",
    });
    expect(ctl.getSnapshot().rows.map((r) => r.id)).toEqual(["a", "b"]);
    expect(ctl.getSnapshot().nextCursor).toBeNull();
  });

  it("is ignored while one is in flight", async () => {
    const ctl = await loaded(["a"], "c1");
    const d = deferred<ActivityPage>();
    mocks.list.mockReturnValueOnce(d.promise);
    ctl.loadMore();
    ctl.loadMore();
    expect(mocks.list).toHaveBeenCalledTimes(2);
  });

  it("keeps the rows and cursor when it fails", async () => {
    const ctl = await loaded(["a"], "c1");
    mocks.list.mockRejectedValueOnce(new Error("x"));
    ctl.loadMore();
    await flush();
    expect(ctl.getSnapshot()).toMatchObject({ loadMore: "failed", nextCursor: "c1" });
    expect(ctl.getSnapshot().rows).toHaveLength(1);
  });
});

describe("re-read", () => {
  it("re-reads every loaded page and replaces the rows without merging", async () => {
    const ctl = await loaded(["a"], "c1");
    mocks.list.mockResolvedValueOnce(page(["b"], null));
    ctl.loadMore();
    await flush();
    mocks.list
      .mockResolvedValueOnce(page(["z", "a"], "c9"))
      .mockResolvedValueOnce(page(["b"], null));
    ctl.refresh();
    await flush();
    expect(mocks.list).toHaveBeenNthCalledWith(3, "ws-1", "co-1", { class: undefined });
    expect(mocks.list).toHaveBeenNthCalledWith(4, "ws-1", "co-1", {
      class: undefined,
      before: "c9",
    });
    expect(ctl.getSnapshot().rows.map((r) => r.id)).toEqual(["z", "a", "b"]);
    expect(ctl.getSnapshot().nextCursor).toBeNull();
  });

  it("keeps the rows when it fails and sets no failure state", async () => {
    const ctl = await loaded(["a"]);
    mocks.list.mockRejectedValueOnce(new Error("x"));
    ctl.refresh();
    await flush();
    expect(ctl.getSnapshot()).toMatchObject({ status: "loaded", rereading: false });
    expect(ctl.getSnapshot().rows).toHaveLength(1);
  });

  it("supersedes an in-flight Load more and disables Load more meanwhile", async () => {
    const ctl = await loaded(["a"], "c1");
    const more = deferred<ActivityPage>();
    mocks.list.mockReturnValueOnce(more.promise);
    ctl.loadMore();
    const reread = deferred<ActivityPage>();
    mocks.list.mockReturnValueOnce(reread.promise);
    ctl.refresh();
    expect(ctl.getSnapshot()).toMatchObject({ rereading: true, loadMore: "idle" });
    ctl.loadMore();
    expect(mocks.list).toHaveBeenCalledTimes(3);
    more.resolve(page(["late"]));
    await flush();
    expect(ctl.getSnapshot().rows.map((r) => r.id)).toEqual(["a"]);
    reread.resolve(page(["a"], "c1"));
    await flush();
    expect(ctl.getSnapshot().rows.map((r) => r.id)).toEqual(["a"]);
    expect(ctl.getSnapshot().rereading).toBe(false);
  });

  it("coalesces every trigger during a re-read into one trailing re-read", async () => {
    const ctl = await loaded(["a"]);
    const first = deferred<ActivityPage>();
    mocks.list.mockReturnValueOnce(first.promise);
    ctl.refresh();
    ctl.refresh();
    ctl.refresh();
    mocks.list.mockResolvedValueOnce(page(["n"]));
    first.resolve(page(["a"]));
    await flush();
    expect(mocks.list).toHaveBeenCalledTimes(3);
    expect(ctl.getSnapshot().rows.map((r) => r.id)).toEqual(["n"]);
  });

  it("reports each arrival to onArrival", async () => {
    const onArrival = vi.fn();
    mocks.list.mockResolvedValue(page(["a"]));
    const ctl = make(onArrival);
    ctl.start();
    await flush();
    ctl.refresh();
    await flush();
    expect(onArrival).toHaveBeenCalledTimes(2);
  });
});

// eslint-disable-next-line max-lines-per-function -- one fixture serves every undo case
describe("undo", () => {
  it("re-reads after a 200 and keeps the row busy until that re-read settles", async () => {
    const ctl = await loaded(["a"]);
    mocks.undo.mockResolvedValueOnce(row("a"));
    const reread = deferred<ActivityPage>();
    mocks.list.mockReturnValueOnce(reread.promise);
    const pending = ctl.undo(ctl.getSnapshot().rows[0]);
    await flush();
    expect(ctl.getSnapshot().busy.has("a")).toBe(true);
    reread.resolve({ rows: [row("a", { undone_at: "2026-09-29T11:00:00Z" })], next_cursor: null });
    await pending;
    expect(ctl.getSnapshot().busy.has("a")).toBe(false);
    expect(ctl.getSnapshot().rows[0].undone_at).not.toBeNull();
    expect(ctl.getSnapshot().messages.size).toBe(0);
  });

  it("leaves the row clickable, unchanged and silent when the re-read after a 200 fails", async () => {
    const ctl = await loaded(["a"]);
    mocks.undo.mockResolvedValueOnce(row("a"));
    mocks.list.mockRejectedValueOnce(new Error("x"));
    await ctl.undo(ctl.getSnapshot().rows[0]);
    expect(ctl.getSnapshot().busy.has("a")).toBe(false);
    expect(ctl.getSnapshot().messages.size).toBe(0);
    expect(ctl.getSnapshot().rows[0].undone_at).toBeNull();
  });

  it("sends one request per row at a time", async () => {
    const ctl = await loaded(["a"]);
    const d = deferred<ActivityItem>();
    mocks.undo.mockReturnValueOnce(d.promise);
    const first = ctl.undo(ctl.getSnapshot().rows[0]);
    void ctl.undo(ctl.getSnapshot().rows[0]);
    expect(mocks.undo).toHaveBeenCalledTimes(1);
    d.reject(new Error("x"));
    await first;
  });

  it("re-reads and shows no message for already_undone", async () => {
    const ctl = await loaded(["a"]);
    mocks.undo.mockRejectedValueOnce(new ApiError("x", 409, { code: "already_undone" }));
    mocks.list.mockResolvedValueOnce(page(["a"]));
    await ctl.undo(ctl.getSnapshot().rows[0]);
    await flush();
    expect(mocks.list).toHaveBeenCalledTimes(2);
    expect(ctl.getSnapshot().messages.size).toBe(0);
  });

  it.each([
    ["moved", "activityConflictMoved"],
    ["archived", "activityConflictMoved"],
    ["agent_running", "activityConflictAgentRunning"],
    ["step_deleted", "activityConflictStepDeleted"],
    ["step_done", "activityConflictStepDone"],
    ["step_full", "activityConflictStepFull"],
    ["something_new", "activityConflictMoved"],
    [undefined, "activityConflictMoved"],
  ])("undo_conflict reason %s shows %s with no re-read", async (reason, key) => {
    const ctl = await loaded(["a"]);
    mocks.undo.mockRejectedValueOnce(new ApiError("x", 409, { code: "undo_conflict", reason }));
    await ctl.undo(ctl.getSnapshot().rows[0]);
    expect(ctl.getSnapshot().messages.get("a")?.key).toBe(key);
    expect(mocks.list).toHaveBeenCalledTimes(1);
    expect(ctl.getSnapshot().busy.has("a")).toBe(false);
  });

  it("shows the generic failure for a 500, a network error, a 403 and an unknown 409 code", async () => {
    for (const err of [
      new ApiError("x", 500, {}),
      new Error("network"),
      new ApiError("x", 403, {}),
      new ApiError("x", 409, { code: "brand_new" }),
    ]) {
      const ctl = await loaded(["a"]);
      mocks.undo.mockRejectedValueOnce(err);
      await ctl.undo(ctl.getSnapshot().rows[0]);
      expect(ctl.getSnapshot().messages.get("a")?.key).toBe("activityUndoFailed");
    }
  });

  it("a second failure replaces the message and a new confirmed Undo clears it", async () => {
    const ctl = await loaded(["a"]);
    mocks.undo.mockRejectedValueOnce(new ApiError("x", 500, {}));
    await ctl.undo(ctl.getSnapshot().rows[0]);
    mocks.undo.mockRejectedValueOnce(new ApiError("x", 409, { code: "undo_conflict" }));
    await ctl.undo(ctl.getSnapshot().rows[0]);
    expect(ctl.getSnapshot().messages.get("a")?.key).toBe("activityConflictMoved");
    const d = deferred<ActivityItem>();
    mocks.undo.mockReturnValueOnce(d.promise);
    const pending = ctl.undo(ctl.getSnapshot().rows[0]);
    expect(ctl.getSnapshot().messages.has("a")).toBe(false);
    d.reject(new Error("x"));
    await pending;
  });

  it("sets no message for a row no longer listed or under a disposed generation", async () => {
    const ctl = await loaded(["a"]);
    const d = deferred<ActivityItem>();
    mocks.undo.mockReturnValueOnce(d.promise);
    const pending = ctl.undo(ctl.getSnapshot().rows[0]);
    mocks.list.mockResolvedValueOnce(page(["other"]));
    ctl.refresh();
    await flush();
    d.reject(new ApiError("x", 500, {}));
    await pending;
    expect(ctl.getSnapshot().messages.size).toBe(0);

    const ctl2 = await loaded(["a"]);
    const d2 = deferred<ActivityItem>();
    mocks.undo.mockReturnValueOnce(d2.promise);
    const pending2 = ctl2.undo(ctl2.getSnapshot().rows[0]);
    ctl2.dispose();
    d2.reject(new ApiError("x", 500, {}));
    await pending2;
    expect(ctl2.getSnapshot().messages.size).toBe(0);
  });
});

describe("message lifecycle", () => {
  it("a message clears on the next successful re-read that started after it was set", async () => {
    const ctl = await loaded(["a"]);
    mocks.undo.mockRejectedValueOnce(new ApiError("x", 500, {}));
    await ctl.undo(ctl.getSnapshot().rows[0]);
    mocks.list.mockRejectedValueOnce(new Error("x"));
    ctl.refresh();
    await flush();
    expect(ctl.getSnapshot().messages.has("a")).toBe(true);
    mocks.list.mockResolvedValueOnce(page(["a"]));
    ctl.refresh();
    await flush();
    expect(ctl.getSnapshot().messages.has("a")).toBe(false);
  });

  it("a re-read in flight when the message was set neither clears it", async () => {
    const ctl = await loaded(["a"]);
    const inFlight = deferred<ActivityPage>();
    mocks.list.mockReturnValueOnce(inFlight.promise);
    ctl.refresh();
    mocks.undo.mockRejectedValueOnce(new ApiError("x", 500, {}));
    await ctl.undo(ctl.getSnapshot().rows[0]);
    inFlight.resolve(page(["a"]));
    await flush();
    expect(ctl.getSnapshot().messages.has("a")).toBe(true);
  });

  it("Load more clears nothing", async () => {
    const ctl = await loaded(["a"], "c1");
    mocks.undo.mockRejectedValueOnce(new ApiError("x", 500, {}));
    await ctl.undo(ctl.getSnapshot().rows[0]);
    mocks.list.mockResolvedValueOnce(page(["b"]));
    ctl.loadMore();
    await flush();
    expect(ctl.getSnapshot().messages.has("a")).toBe(true);
  });

  it("not_undoable survives its own re-read and clears on the next successful one", async () => {
    const ctl = await loaded(["a"]);
    mocks.undo.mockRejectedValueOnce(new ApiError("x", 409, { code: "not_undoable" }));
    mocks.list.mockResolvedValueOnce(page(["a"]));
    await ctl.undo(ctl.getSnapshot().rows[0]);
    await flush();
    expect(ctl.getSnapshot().messages.get("a")).toMatchObject({
      key: "activityNotUndoable",
      survives: false,
    });
    mocks.list.mockResolvedValueOnce(page(["a"]));
    ctl.refresh();
    await flush();
    expect(ctl.getSnapshot().messages.has("a")).toBe(false);
  });

  it("not_undoable is consumed by the trailing re-read when one was already in flight", async () => {
    const ctl = await loaded(["a"]);
    const inFlight = deferred<ActivityPage>();
    mocks.list.mockReturnValueOnce(inFlight.promise);
    ctl.refresh();
    mocks.undo.mockRejectedValueOnce(new ApiError("x", 409, { code: "not_undoable" }));
    await ctl.undo(ctl.getSnapshot().rows[0]);
    mocks.list.mockResolvedValueOnce(page(["a"]));
    inFlight.resolve(page(["a"]));
    await flush();
    expect(ctl.getSnapshot().messages.get("a")).toMatchObject({ survives: false });
    mocks.list.mockResolvedValueOnce(page(["a"]));
    ctl.refresh();
    await flush();
    expect(ctl.getSnapshot().messages.has("a")).toBe(false);
  });

  it("404 shows the section notice, re-reads at once and keeps it through that re-read", async () => {
    const ctl = await loaded(["a"]);
    mocks.undo.mockRejectedValueOnce(new ApiError("x", 404, {}));
    mocks.list.mockResolvedValueOnce(page([]));
    await ctl.undo(ctl.getSnapshot().rows[0]);
    await flush();
    expect(ctl.getSnapshot().notice?.key).toBe("activityGone");
    mocks.list.mockResolvedValueOnce(page([]));
    ctl.refresh();
    await flush();
    expect(ctl.getSnapshot().notice).toBeNull();
  });
});
