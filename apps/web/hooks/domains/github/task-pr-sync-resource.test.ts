import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";
import {
  createTaskPRSyncResource,
  type TaskPRSyncRequester,
  type TaskPRSyncScope,
} from "./task-pr-sync-resource";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function createStore(workspaceId: string | null = "workspace-1", generation = 0) {
  return createAppStore({
    workspaces: { items: [], activeId: workspaceId },
    workspaceContextGeneration: generation,
  });
}

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "association-1",
    workspace_id: "workspace-1",
    task_id: "task-1",
    owner: "acme",
    repo: "app",
    pr_number: 1,
    pr_url: "https://github.com/acme/app/pull/1",
    pr_title: "Task PR",
    head_branch: "feature/task-1",
    base_branch: "main",
    author_login: "octocat",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "clean",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 0,
    deletions: 0,
    created_at: "2026-08-27T00:00:00Z",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "2026-08-27T00:00:00Z",
    ...overrides,
  };
}

const scope: TaskPRSyncScope = {
  workspaceId: "workspace-1",
  workspaceContextGeneration: 0,
  taskId: "task-1",
};

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("task PR sync resource request ownership", () => {
  // @covers AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.1
  // @covers AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.2
  it("joins active requests and publishes one loaded snapshot to every lease", async () => {
    const store = createStore();
    const requester = vi.fn<TaskPRSyncRequester>();
    const pending = deferred<{ prs: TaskPR[] }>();
    requester.mockReturnValue(pending.promise);
    const resource = createTaskPRSyncResource(store, requester);
    const firstListener = vi.fn();
    const secondListener = vi.fn();

    const releaseFirst = resource.subscribe(scope, firstListener);
    const releaseSecond = resource.subscribe(scope, secondListener);
    const firstRefresh = resource.refresh(scope);
    const secondRefresh = resource.refresh(scope);

    expect(requester).toHaveBeenCalledTimes(1);
    expect(firstRefresh).toBe(secondRefresh);

    pending.resolve({ prs: [] });
    await firstRefresh;

    expect(resource.getSnapshot(scope)).toBe(true);
    expect(firstListener).toHaveBeenCalled();
    expect(secondListener).toHaveBeenCalled();
    releaseFirst();
    releaseSecond();
  });

  // @covers AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.4
  it("keeps retries after one release and stops them after the final release", async () => {
    const store = createStore();
    const requester = vi.fn<TaskPRSyncRequester>().mockResolvedValue({ prs: [] });
    const resource = createTaskPRSyncResource(store, requester, {
      retryDelayMs: 10,
      maxRetries: 3,
    });

    const releaseFirst = resource.subscribe(scope, vi.fn());
    const releaseSecond = resource.subscribe(scope, vi.fn());
    await Promise.resolve();

    await vi.advanceTimersByTimeAsync(10);
    expect(requester).toHaveBeenCalledTimes(2);

    releaseFirst();
    await vi.advanceTimersByTimeAsync(10);
    expect(requester).toHaveBeenCalledTimes(3);

    releaseSecond();
    await vi.advanceTimersByTimeAsync(100);
    expect(requester).toHaveBeenCalledTimes(3);
  });

  // @covers AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.3
  it("keeps task, workspace, generation, and store scopes independent", async () => {
    const store = createStore();
    const requester = vi.fn<TaskPRSyncRequester>().mockResolvedValue({ prs: [] });
    const resource = createTaskPRSyncResource(store, requester, { retryDelayMs: 100 });
    const scopes = [
      scope,
      { ...scope, taskId: "task-2" },
      { ...scope, workspaceId: "workspace-2" },
      { ...scope, workspaceContextGeneration: 1 },
    ];

    const releases = scopes.map((item) => resource.subscribe(item, vi.fn()));
    await Promise.resolve();

    expect(requester).toHaveBeenCalledTimes(4);
    releases.forEach((release) => release());

    const secondStore = createStore();
    const secondResource = createTaskPRSyncResource(secondStore, requester, {
      retryDelayMs: 100,
    });
    const release = secondResource.subscribe(scope, vi.fn());
    await Promise.resolve();

    expect(requester).toHaveBeenCalledTimes(5);
    release();
  });

  it("allows a Strict Mode-style release and reacquisition to rejoin the request", async () => {
    const store = createStore();
    const requester = vi.fn<TaskPRSyncRequester>();
    const pending = deferred<{ prs: TaskPR[] }>();
    requester.mockReturnValue(pending.promise);
    const resource = createTaskPRSyncResource(store, requester);

    const releaseFirst = resource.subscribe(scope, vi.fn());
    releaseFirst();
    const releaseSecond = resource.subscribe(scope, vi.fn());

    expect(requester).toHaveBeenCalledTimes(1);

    pending.resolve({ prs: [] });
    await resource.refresh(scope);
    releaseSecond();
  });
});

describe("task PR sync resource lifecycle", () => {
  // @covers AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.5
  it("stops retrying after a permanent response", async () => {
    const store = createStore();
    const requester = vi.fn<TaskPRSyncRequester>().mockResolvedValue({
      prs: [],
      permanent: true,
    });
    const resource = createTaskPRSyncResource(store, requester, { retryDelayMs: 10 });
    const release = resource.subscribe(scope, vi.fn());

    await Promise.resolve();
    await vi.advanceTimersByTimeAsync(100);

    expect(requester).toHaveBeenCalledTimes(1);
    expect(resource.getSnapshot(scope)).toBe(true);
    release();
  });

  it("publishes discovered pull requests to the scoped store", async () => {
    const store = createStore();
    const requester = vi.fn<TaskPRSyncRequester>().mockResolvedValue({ prs: [makePR()] });
    const resource = createTaskPRSyncResource(store, requester, { retryDelayMs: 10 });
    const release = resource.subscribe(scope, vi.fn());

    await Promise.resolve();

    expect(store.getState().taskPRs.byTaskId[scope.taskId]).toEqual([makePR()]);
    expect(resource.getSnapshot(scope)).toBe(true);
    await vi.advanceTimersByTimeAsync(100);
    expect(requester).toHaveBeenCalledTimes(1);
    release();
  });

  it("marks a task loaded only after the final transient failure", async () => {
    const store = createStore();
    const requester = vi.fn<TaskPRSyncRequester>().mockRejectedValue(new Error("sync unavailable"));
    const resource = createTaskPRSyncResource(store, requester, {
      retryDelayMs: 10,
      maxRetries: 2,
    });
    const release = resource.subscribe(scope, vi.fn());

    await Promise.resolve();
    expect(resource.getSnapshot(scope)).toBe(false);

    await vi.advanceTimersByTimeAsync(10);
    expect(resource.getSnapshot(scope)).toBe(false);
    await vi.advanceTimersByTimeAsync(10);
    expect(resource.getSnapshot(scope)).toBe(true);
    expect(requester).toHaveBeenCalledTimes(3);

    await vi.advanceTimersByTimeAsync(100);
    expect(requester).toHaveBeenCalledTimes(3);
    release();
  });

  it("anchors the retry delay to the start of the previous request", async () => {
    const store = createStore();
    const requester = vi.fn<TaskPRSyncRequester>();
    const first = deferred<{ prs: TaskPR[] }>();
    const second = deferred<{ prs: TaskPR[] }>();
    requester.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const resource = createTaskPRSyncResource(store, requester, { retryDelayMs: 10 });
    const release = resource.subscribe(scope, vi.fn());

    expect(requester).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(8);
    first.resolve({ prs: [] });
    await vi.advanceTimersByTimeAsync(0);

    await vi.advanceTimersByTimeAsync(1);
    expect(requester).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(requester).toHaveBeenCalledTimes(2);

    second.resolve({ prs: [] });
    await vi.advanceTimersByTimeAsync(0);
    release();
  });

  it("stops the retry schedule when another path discovers a pull request", async () => {
    const store = createStore();
    const requester = vi.fn<TaskPRSyncRequester>().mockResolvedValue({ prs: [] });
    const resource = createTaskPRSyncResource(store, requester, { retryDelayMs: 10 });
    const release = resource.subscribe(scope, vi.fn());

    await Promise.resolve();
    store.getState().setTaskPR("task-1", makePR(), {
      workspaceId: scope.workspaceId,
      workspaceContextGeneration: scope.workspaceContextGeneration,
    });
    await vi.advanceTimersByTimeAsync(100);

    expect(requester).toHaveBeenCalledTimes(1);
    release();
  });
});

describe("task PR sync resource recovery", () => {
  // @covers AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.5
  it("starts one fresh retry window on the connected transition", async () => {
    const store = createStore();
    const requester = vi.fn<TaskPRSyncRequester>().mockResolvedValue({ prs: [] });
    const resource = createTaskPRSyncResource(store, requester, {
      retryDelayMs: 10,
      maxRetries: 2,
    });
    const release = resource.subscribe(scope, vi.fn());

    await Promise.resolve();
    await vi.advanceTimersByTimeAsync(30);
    const callsAfterExhaustion = requester.mock.calls.length;

    store.getState().setConnectionStatus("connected", null);
    await Promise.resolve();
    expect(requester.mock.calls.length).toBe(callsAfterExhaustion + 1);

    store.getState().setConnectionStatus("connected", null);
    await Promise.resolve();
    expect(requester.mock.calls.length).toBe(callsAfterExhaustion + 1);

    await vi.advanceTimersByTimeAsync(10);
    expect(requester.mock.calls.length).toBe(callsAfterExhaustion + 2);
    release();
  });

  it("abandons a pending pre-reconnect request and refreshes immediately", async () => {
    const store = createStore();
    const requester = vi.fn<TaskPRSyncRequester>();
    const first = deferred<{ prs: TaskPR[] }>();
    const second = deferred<{ prs: TaskPR[] }>();
    requester.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const resource = createTaskPRSyncResource(store, requester);
    const release = resource.subscribe(scope, vi.fn());

    expect(requester).toHaveBeenCalledTimes(1);
    store.getState().setConnectionStatus("connected", null);
    expect(requester).toHaveBeenCalledTimes(2);

    second.resolve({ prs: [] });
    first.resolve({ prs: [makePR()] });
    await vi.advanceTimersByTimeAsync(0);

    expect(store.getState().taskPRs.byTaskId[scope.taskId]).toBeUndefined();
    release();
  });

  // @covers AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.6
  it("rejects a response from an inactive workspace context", async () => {
    const store = createStore();
    const requester = vi.fn<TaskPRSyncRequester>();
    const pending = deferred<{ prs: TaskPR[] }>();
    requester.mockReturnValue(pending.promise);
    const resource = createTaskPRSyncResource(store, requester);
    const release = resource.subscribe(scope, vi.fn());

    store.setState((state) => ({
      ...state,
      workspaces: { ...state.workspaces, activeId: "workspace-2" },
      workspaceContextGeneration: 1,
    }));
    pending.resolve({ prs: [makePR()] });
    await resource.refresh(scope);

    expect(store.getState().taskPRs.byTaskId[scope.taskId]).toBeUndefined();
    expect(resource.getSnapshot(scope)).toBe(false);
    release();
  });

  it("does not publish an unresolved response after the final release", async () => {
    const store = createStore();
    const requester = vi.fn<TaskPRSyncRequester>();
    const pending = deferred<{ prs: TaskPR[] }>();
    requester.mockReturnValueOnce(pending.promise).mockResolvedValue({ prs: [] });
    const resource = createTaskPRSyncResource(store, requester);
    const release = resource.subscribe(scope, vi.fn());

    release();
    pending.resolve({ prs: [makePR()] });
    await resource.refresh(scope);

    expect(store.getState().taskPRs.byTaskId[scope.taskId]).toBeUndefined();
    expect(resource.getSnapshot(scope)).toBe(false);

    const reacquired = resource.subscribe(scope, vi.fn());
    await Promise.resolve();
    expect(requester).toHaveBeenCalledTimes(2);
    reacquired();
  });
});
