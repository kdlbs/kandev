import { Suspense, startTransition, useRef, type ReactNode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { registerTaskPlansHandlers } from "@/lib/ws/handlers/task-plans";
import type { TaskPlan } from "@/lib/types/http";
import { useTaskPlan } from "./use-task-plan";
import { usePlanDraft } from "./use-plan-draft";

const transport = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => transport }));
afterEach(() => {
  cleanup();
  transport.request.mockReset();
  localStorage.clear();
  vi.restoreAllMocks();
});

const TASK = "refresh-task";
const old: TaskPlan = {
  id: "old-plan",
  task_id: TASK,
  title: "Old title",
  content: "Old content",
  created_by: "agent",
  created_at: "2026-10-20T00:00:00Z",
  updated_at: "2026-10-20T00:00:00Z",
};
const newer: TaskPlan = {
  ...old,
  id: "new-plan",
  title: "New title",
  content: "New agent content",
  updated_at: "2026-10-21T00:00:00Z",
  implementation_started_at: "2026-10-21T00:00:00Z",
  implementation_started_session_id: "session",
  implementation_started_by: "agent",
  comments_revision: 7,
};
function deferred() {
  let resolve!: (value: TaskPlan | null) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<TaskPlan | null>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}
function wrapper({ children }: { children: ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}
function useConsumer(taskId: string | null = TASK, visible = true) {
  const store = useAppStoreApi();
  const plan = useTaskPlan(taskId, { visible });
  const editorWrapperRef = useRef<HTMLDivElement>(null);
  const draft = usePlanDraft({
    plan: plan.plan,
    isSaving: plan.isSaving,
    savePlan: plan.savePlan,
    editorWrapperRef,
    taskId,
  });
  return { store, ...plan, draft };
}
function mount() {
  const hook = renderHook(() => useConsumer(), { wrapper });
  act(() => {
    hook.result.current.store.getState().setTaskPlan(TASK, old);
    hook.result.current.store.getState().markTaskPlanSeen(TASK);
  });
  return hook;
}
function deliver(
  store: ReturnType<typeof useAppStoreApi>,
  event: "created" | "updated" | "deleted",
) {
  const handlers = registerTaskPlansHandlers(store);
  if (event === "deleted")
    handlers["task.plan.deleted"]!({
      type: "notification",
      action: "task.plan.deleted",
      payload: old,
    });
  else if (event === "updated")
    handlers["task.plan.updated"]!({
      type: "notification",
      action: "task.plan.updated",
      payload: newer,
    });
  else
    handlers["task.plan.created"]!({
      type: "notification",
      action: "task.plan.created",
      payload: newer,
    });
}

// @covers AC-TASKS-DOCUMENTS-003.4, AC-TASKS-DOCUMENTS-003.8
it.each(["updated", "deleted", "created"] as const)(
  "preserves observed %s against an older snapshot and keeps the real draft synchronized",
  async (event) => {
    const reply = deferred();
    transport.request.mockReturnValue(reply.promise);
    const h = mount();
    if (event === "created") act(() => h.result.current.store.getState().setTaskPlan(TASK, null));
    let request!: Promise<void>;
    act(() => {
      request = h.result.current.refetch();
    });
    act(() => deliver(h.result.current.store, event));
    const expected = event === "deleted" ? null : newer;
    expect(h.result.current.plan).toEqual(expected);
    expect(h.result.current.draft.draftContent).toBe(expected?.content ?? "");
    const observed = h.result.current.store.getState().taskPlans;
    await act(async () => {
      // Creation races with an earlier absence response; update/delete cover
      // non-null stale snapshots through the same value-independent token guard.
      reply.resolve(event === "created" ? null : old);
      await request;
    });
    expect.soft(h.result.current.plan).toEqual(expected);
    expect.soft(h.result.current.draft.draftContent).toBe(expected?.content ?? "");
    expect
      .soft(h.result.current.store.getState().taskPlans.lastSeenUpdatedAtByTaskId[TASK])
      .toBe(observed.lastSeenUpdatedAtByTaskId[TASK]);
    expect(h.result.current.store.getState().taskPlans.loadedByTaskId[TASK]).toBe(true);
  },
);

it("keeps a newer consumer's loading and result when an older read finishes", async () => {
  const first = deferred();
  const second = deferred();
  transport.request.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
  const h = renderHook(() => ({ a: useConsumer(), b: useConsumer() }), { wrapper });
  act(() => h.result.current.a.store.getState().setTaskPlan(TASK, old));
  let a!: Promise<void>;
  let b!: Promise<void>;
  act(() => {
    a = h.result.current.a.refetch();
    b = h.result.current.b.refetch();
  });
  await act(async () => {
    first.resolve(old);
    await a;
  });
  expect.soft(h.result.current.a.isLoading).toBe(true);
  await act(async () => {
    second.resolve(newer);
    await b;
  });
  expect(h.result.current.a.plan).toEqual(newer);
  expect(h.result.current.b.draft.draftContent).toBe(newer.content);
  expect(h.result.current.a.isLoading).toBe(false);
});

it.each([old, newer, null])("accepts current snapshot %j and marks it seen", async (value) => {
  transport.request.mockResolvedValue(value);
  const h = mount();
  await act(async () => {
    await h.result.current.refetch();
  });
  expect(h.result.current.plan).toEqual(value);
  expect(h.result.current.draft.draftContent).toBe(value?.content ?? "");
  expect(h.result.current.store.getState().taskPlans.lastSeenUpdatedAtByTaskId[TASK]).toBe(
    value?.updated_at ?? "",
  );
  expect(h.result.current.isLoading).toBe(false);
  expect(h.result.current.error).toBeNull();
});

it("reports a current failure without changing the plan, then accepts a refresh after a live event", async () => {
  vi.spyOn(console, "error").mockImplementation(() => undefined);
  transport.request.mockRejectedValueOnce(new Error("Current failure")).mockResolvedValueOnce(old);
  const h = mount();
  await act(async () => {
    await h.result.current.refetch();
  });
  expect(h.result.current.error).toBe("Current failure");
  expect(h.result.current.plan).toEqual(old);
  expect(h.result.current.isLoading).toBe(false);
  act(() => deliver(h.result.current.store, "updated"));
  await act(async () => {
    await h.result.current.refetch();
  });
  // A later request may legitimately return different content: no timestamp ordering.
  expect(h.result.current.plan).toEqual(old);
  expect(h.result.current.error).toBeNull();
});

it.each(["success", "failure"])("ignores retired %s after a later read fails", async (outcome) => {
  vi.spyOn(console, "error").mockImplementation(() => undefined);
  const first = deferred();
  transport.request
    .mockReturnValueOnce(first.promise)
    .mockRejectedValueOnce(new Error("New failure"));
  const h = mount();
  let a!: Promise<void>;
  act(() => {
    a = h.result.current.refetch();
  });
  await act(async () => {
    await h.result.current.refetch();
  });
  await act(async () => {
    if (outcome === "failure") first.reject(new Error("Old failure"));
    else first.resolve(newer);
    await a;
  });
  expect(h.result.current.error).toBe("New failure");
  expect(h.result.current.plan).toEqual(old);
  expect(h.result.current.isLoading).toBe(false);
});

it("keeps observed metadata and comment identity when an old failure settles", async () => {
  const reply = deferred();
  transport.request.mockReturnValue(reply.promise);
  const h = mount();
  let request!: Promise<void>;
  act(() => {
    request = h.result.current.refetch();
  });
  act(() => {
    deliver(h.result.current.store, "updated");
    h.result.current.store.getState().setTaskPlanComments(TASK, {
      task_id: TASK,
      plan_id: newer.id,
      revision: 7,
      comments: [],
    });
  });
  await act(async () => {
    reply.reject(new Error("Old failure"));
    await request;
  });
  expect(h.result.current.plan).toEqual(newer);
  expect(h.result.current.store.getState().taskPlans.commentsByTaskId[TASK]?.plan_id).toBe(
    newer.id,
  );
  expect(h.result.current.store.getState().taskPlans.lastSeenUpdatedAtByTaskId[TASK]).toBe(
    old.updated_at,
  );
  expect(h.result.current.error).toBeNull();
});

it.each(["updated", "deleted"] as const)(
  "does not clear a fresh read's loading after an observed %s retires an old read",
  async (event) => {
    const first = deferred();
    const second = deferred();
    transport.request.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const h = mount();
    let a!: Promise<void>;
    let b!: Promise<void>;
    act(() => {
      a = h.result.current.refetch();
    });
    act(() => deliver(h.result.current.store, event));
    act(() => {
      b = h.result.current.refetch();
    });
    await act(async () => {
      first.resolve(old);
      await a;
    });
    expect(h.result.current.isLoading).toBe(true);
    expect(h.result.current.plan).toEqual(event === "deleted" ? null : newer);
    await act(async () => {
      second.resolve(newer);
      await b;
    });
    expect(h.result.current.isLoading).toBe(false);
    expect(h.result.current.plan).toEqual(newer);
  },
);

it("admits only the latest consumer when it finishes before an older consumer", async () => {
  const first = deferred();
  const second = deferred();
  transport.request.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
  const h = renderHook(() => ({ a: useConsumer(), b: useConsumer() }), { wrapper });
  let a!: Promise<void>;
  let b!: Promise<void>;
  act(() => {
    a = h.result.current.a.refetch();
    b = h.result.current.b.refetch();
  });
  await act(async () => {
    second.resolve(newer);
    await b;
    first.resolve(old);
    await a;
  });
  expect(h.result.current.a.plan).toEqual(newer);
  expect(h.result.current.b.isLoading).toBe(false);
});

it("isolates independent providers and retires a request on cache clear", async () => {
  const first = deferred();
  const second = deferred();
  transport.request.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
  const a = mount();
  const b = mount();
  let ar!: Promise<void>;
  let br!: Promise<void>;
  act(() => {
    ar = a.result.current.refetch();
    br = b.result.current.refetch();
  });
  act(() => {
    a.result.current.store.getState().clearTaskPlan(TASK);
    deliver(a.result.current.store, "deleted");
  });
  await act(async () => {
    first.resolve(old);
    second.resolve(newer);
    await ar;
    await br;
  });
  expect(a.result.current.plan).toBeNull();
  expect(b.result.current.plan).toEqual(newer);
});

it.each(["success", "failure"])(
  "keeps task navigation and round trips isolated from outgoing %s",
  async (outcome) => {
    const reply = deferred();
    transport.request.mockReturnValue(reply.promise);
    const h = renderHook(({ id }) => useConsumer(id), { wrapper, initialProps: { id: TASK } });
    const store = h.result.current.store;
    act(() => {
      store.getState().setTaskPlan(TASK, old);
      store.getState().setTaskPlan("other", { ...old, task_id: "other", content: "Other" });
    });
    let request!: Promise<void>;
    act(() => {
      request = h.result.current.refetch();
    });
    h.rerender({ id: "other" });
    h.rerender({ id: TASK });
    await act(async () => {
      if (outcome === "failure") reply.reject(new Error("Outgoing failure"));
      else reply.resolve(newer);
      await request;
    });
    expect(h.result.current.error).toBeNull();
    expect(h.result.current.plan).toEqual(outcome === "success" ? newer : old);
    expect(store.getState().taskPlans.byTaskId.other?.content).toBe("Other");
    expect(store.getState().taskPlans.loadingByTaskId[TASK]).toBe(false);
  },
);

it("settles an admitted store read after its initiating consumer unmounts", async () => {
  const reply = deferred();
  transport.request.mockReturnValue(reply.promise);
  const h = mount();
  const store = h.result.current.store;
  let request!: Promise<void>;
  act(() => {
    request = h.result.current.refetch();
  });
  h.unmount();
  await act(async () => {
    reply.resolve(newer);
    await request;
  });
  expect(store.getState().taskPlans.byTaskId[TASK]).toEqual(newer);
  expect(store.getState().taskPlans.loadingByTaskId[TASK]).toBe(false);
});

it("protects initial and hidden-to-visible refreshes using the same admission boundary", async () => {
  const initial = deferred();
  const visible = deferred();
  transport.request.mockImplementation((action: string) =>
    action === "task.plan.revisions.list" ? Promise.resolve({ revisions: [] }) : initial.promise,
  );
  const h = renderHook(({ shown }) => useConsumer(TASK, shown), {
    wrapper,
    initialProps: { shown: false },
  });
  act(() => h.result.current.store.getState().setConnectionStatus("connected"));
  expect(transport.request).toHaveBeenCalledWith("task.plan.get", { task_id: TASK });
  act(() => deliver(h.result.current.store, "updated"));
  await act(async () => {
    initial.resolve(old);
    await initial.promise;
  });
  expect(h.result.current.plan).toEqual(newer);
  transport.request.mockReturnValueOnce(visible.promise);
  h.rerender({ shown: true });
  expect(h.result.current.isLoading).toBe(true);
  act(() => deliver(h.result.current.store, "deleted"));
  await act(async () => {
    visible.resolve(old);
    await visible.promise;
  });
  expect(h.result.current.plan).toBeNull();
  expect(h.result.current.draft.draftContent).toBe("");
  expect(h.result.current.isLoading).toBe(false);
});

it("isolates concurrent tasks in one store", async () => {
  const first = deferred();
  const second = deferred();
  transport.request.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
  const h = renderHook(() => ({ a: useConsumer(), b: useConsumer("other") }), { wrapper });
  let a!: Promise<void>;
  let b!: Promise<void>;
  act(() => {
    a = h.result.current.a.refetch();
    b = h.result.current.b.refetch();
  });
  act(() => deliver(h.result.current.a.store, "updated"));
  const other = { ...old, task_id: "other", content: "Other task" };
  await act(async () => {
    first.resolve(old);
    second.resolve(other);
    await a;
    await b;
  });
  expect(h.result.current.a.plan).toEqual(newer);
  expect(h.result.current.b.plan).toEqual(other);
  expect(h.result.current.b.isLoading).toBe(false);
});

it("retires an old plan even when deletion repeats already observed absence", async () => {
  const reply = deferred();
  transport.request.mockReturnValue(reply.promise);
  const h = mount();
  let request!: Promise<void>;
  act(() => h.result.current.store.getState().setTaskPlan(TASK, null));
  act(() => {
    request = h.result.current.refetch();
  });
  act(() => deliver(h.result.current.store, "deleted"));
  await act(async () => {
    reply.resolve(old);
    await request;
  });
  expect(h.result.current.plan).toBeNull();
  expect(h.result.current.isLoading).toBe(false);
});

it("preserves newer comments and unread state when an old success settles", async () => {
  const reply = deferred();
  transport.request.mockReturnValue(reply.promise);
  const h = mount();
  let request!: Promise<void>;
  act(() => {
    request = h.result.current.refetch();
  });
  const comments = { task_id: TASK, plan_id: newer.id, revision: 7, comments: [] };
  act(() => {
    deliver(h.result.current.store, "updated");
    h.result.current.store.getState().setTaskPlanComments(TASK, comments);
  });
  await act(async () => {
    reply.resolve(old);
    await request;
  });
  expect(h.result.current.store.getState().taskPlans.commentsByTaskId[TASK]).toEqual(comments);
  expect(h.result.current.store.getState().taskPlans.lastSeenUpdatedAtByTaskId[TASK]).toBe(
    old.updated_at,
  );
  expect(h.result.current.plan).toEqual(newer);
});

it("retires a snapshot when the cache is cleared without a replacement event", async () => {
  const reply = deferred();
  transport.request.mockReturnValue(reply.promise);
  const h = mount();
  const store = h.result.current.store;
  let request!: Promise<void>;
  act(() => {
    request = h.result.current.refetch();
  });
  act(() => store.getState().clearTaskPlan(TASK));
  await act(async () => {
    reply.resolve(old);
    await request;
  });
  expect(store.getState().taskPlans.byTaskId[TASK]).toBeUndefined();
  expect(store.getState().taskPlans.loadingByTaskId[TASK]).toBeUndefined();
  expect(store.getState().taskPlans.loadedByTaskId[TASK]).toBeUndefined();
  expect(store.getState().taskPlans.lastSeenUpdatedAtByTaskId[TASK]).toBeUndefined();
});

it("preserves an accepted own save against an older snapshot", async () => {
  const reply = deferred();
  transport.request
    .mockReturnValueOnce(reply.promise)
    .mockResolvedValueOnce({ ...newer, created_by: "user" });
  const h = mount();
  let request!: Promise<void>;
  act(() => {
    request = h.result.current.refetch();
  });
  act(() => h.result.current.draft.setDraftContent(newer.content));
  await act(async () => {
    await h.result.current.draft.attemptSave(newer.content, newer.title);
  });
  await act(async () => {
    reply.resolve(old);
    await request;
  });
  expect(h.result.current.plan?.content).toBe(newer.content);
  expect(h.result.current.draft.draftContent).toBe(newer.content);
});

it("does not let a retained outgoing refetch callback clear or replace the current view's error", async () => {
  const currentFailure = "Current task failure";
  vi.spyOn(console, "error").mockImplementation(() => undefined);
  const outgoing = deferred();
  transport.request
    .mockRejectedValueOnce(new Error(currentFailure))
    .mockReturnValueOnce(outgoing.promise);
  const h = renderHook(({ id }) => useConsumer(id), { wrapper, initialProps: { id: TASK } });
  act(() => {
    h.result.current.store.getState().setTaskPlan(TASK, old);
    h.result.current.store.getState().setTaskPlan("other", { ...old, task_id: "other" });
  });
  const outgoingRefetch = h.result.current.refetch;
  h.rerender({ id: "other" });
  await act(async () => {
    await h.result.current.refetch();
  });
  expect(h.result.current.error).toBe(currentFailure);
  let request!: Promise<void>;
  act(() => {
    request = outgoingRefetch();
  });
  expect.soft(h.result.current.error).toBe(currentFailure);
  await act(async () => {
    outgoing.reject(new Error("Outgoing task failure"));
    await request;
  });
  expect(h.result.current.error).toBe(currentFailure);
  expect(h.result.current.store.getState().taskPlans.loadingByTaskId[TASK]).toBe(false);
});

it("keeps the committed task's read error when another task render suspends", async () => {
  vi.spyOn(console, "error").mockImplementation(() => undefined);
  const reply = deferred();
  transport.request.mockReturnValue(reply.promise);
  const blocked = new Promise<void>(() => undefined);
  const rendered: string[] = [];
  const h = renderHook(
    ({ id, suspend }) => {
      const value = useConsumer(id);
      rendered.push(id);
      if (suspend) throw blocked;
      return value;
    },
    {
      initialProps: { id: TASK, suspend: false },
      wrapper: ({ children }: { children: ReactNode }) => (
        <StateProvider>
          <Suspense fallback={null}>{children}</Suspense>
        </StateProvider>
      ),
    },
  );
  act(() => h.result.current.store.getState().setTaskPlan(TASK, old));
  let request!: Promise<void>;
  act(() => {
    request = h.result.current.refetch();
  });
  act(() => {
    startTransition(() => h.rerender({ id: "uncommitted-task", suspend: true }));
  });
  expect(rendered).toContain("uncommitted-task");
  expect(h.result.current.plan).toEqual(old);
  await act(async () => {
    reply.reject(new Error("Committed task failure"));
    await request;
  });
  expect.soft(h.result.current.error).toBe("Committed task failure");
  expect(h.result.current.isLoading).toBe(false);
  h.rerender({ id: TASK, suspend: false });
});
