import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { registerGitlabBridge } from "@/lib/query/bridge/gitlab";
import { qk } from "@/lib/query/keys";
import type { TaskMR, TaskMRsResponse } from "@/lib/types/gitlab";

const TASK_ID = "task-1";
const WS_ID = "ws-1";
const MR_UPDATED_EVENT = "gitlab.task_mr.updated";

/**
 * Minimal fake WebSocketClient — only the `on` method used by the bridge.
 */
type Handler<T> = (msg: T) => void;

function makeFakeWs() {
  const listeners = new Map<string, Set<Handler<unknown>>>();

  return {
    on: vi.fn(<T>(type: string, handler: Handler<T>) => {
      const set = listeners.get(type) ?? new Set();
      set.add(handler as Handler<unknown>);
      listeners.set(type, set);
      return () => {
        const s = listeners.get(type);
        s?.delete(handler as Handler<unknown>);
      };
    }),
    emit: (type: string, message: unknown) => {
      listeners.get(type)?.forEach((h) => h(message));
    },
  };
}

function makeMR(overrides: Partial<TaskMR> = {}): TaskMR {
  return {
    id: "mr-1",
    task_id: TASK_ID,
    host: "https://gitlab.com",
    project_path: "acme/api",
    mr_iid: 1,
    mr_url: "https://gitlab.com/acme/api/-/merge_requests/1",
    mr_title: "Test MR",
    head_branch: "feat",
    base_branch: "main",
    author_username: "alice",
    state: "open",
    approval_state: "",
    pipeline_state: "",
    merge_status: "",
    draft: false,
    approval_count: 0,
    required_approvals: 0,
    pipeline_jobs_total: 0,
    pipeline_jobs_pass: 0,
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

const INITIAL_CACHE: TaskMRsResponse = {
  task_mrs: {
    [TASK_ID]: [makeMR()],
  },
};

function makeClient(wsId = WS_ID, initial?: TaskMRsResponse) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  if (initial) {
    qc.setQueryData(qk.gitlab.mrs(wsId), initial);
  }
  return qc;
}

describe("registerGitlabBridge", () => {
  let ws: ReturnType<typeof makeFakeWs>;
  let qc: QueryClient;

  beforeEach(() => {
    ws = makeFakeWs();
    qc = makeClient(WS_ID, INITIAL_CACHE);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- fake ws satisfies the interface
    registerGitlabBridge(ws as any, qc);
  });

  it("registers a handler for gitlab.task_mr.updated", () => {
    expect(ws.on).toHaveBeenCalledWith(MR_UPDATED_EVENT, expect.any(Function));
  });

  it("upserts an MR into an existing task slot", () => {
    const updated = makeMR({ mr_title: "renamed" });
    ws.emit(MR_UPDATED_EVENT, { payload: updated });

    const data = qc.getQueryData<TaskMRsResponse>(qk.gitlab.mrs(WS_ID));
    expect(data?.task_mrs[TASK_ID]).toHaveLength(1);
    expect(data?.task_mrs[TASK_ID]?.[0]?.mr_title).toBe("renamed");
  });

  it("appends a new MR when the (repo, project, iid) key differs", () => {
    const second = makeMR({ id: "mr-2", mr_iid: 2, repository_id: "repo-b" });
    ws.emit(MR_UPDATED_EVENT, { payload: second });

    const data = qc.getQueryData<TaskMRsResponse>(qk.gitlab.mrs(WS_ID));
    expect(data?.task_mrs[TASK_ID]).toHaveLength(2);
  });

  it("creates a task slot when the task has no existing MRs", () => {
    const mr = makeMR({ task_id: "task-new" });
    ws.emit(MR_UPDATED_EVENT, { payload: mr });

    const data = qc.getQueryData<TaskMRsResponse>(qk.gitlab.mrs(WS_ID));
    expect(data?.task_mrs["task-new"]).toHaveLength(1);
    expect(data?.task_mrs["task-new"]?.[0]?.task_id).toBe("task-new");
  });

  it("is a no-op when task_id is missing from the payload", () => {
    const mr = makeMR({ task_id: "" });
    ws.emit(MR_UPDATED_EVENT, { payload: mr });

    const data = qc.getQueryData<TaskMRsResponse>(qk.gitlab.mrs(WS_ID));
    expect(data?.task_mrs[TASK_ID]).toHaveLength(1);
    expect(Object.keys(data?.task_mrs ?? {})).toHaveLength(1);
  });

  it("updates across multiple workspace caches", () => {
    const qc2 = makeClient("ws-2", { task_mrs: { [TASK_ID]: [] } });
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    registerGitlabBridge(ws as any, qc2);

    const updated = makeMR({ mr_title: "multi-ws" });
    ws.emit(MR_UPDATED_EVENT, { payload: updated });

    const d1 = qc.getQueryData<TaskMRsResponse>(qk.gitlab.mrs(WS_ID));
    const d2 = qc2.getQueryData<TaskMRsResponse>(qk.gitlab.mrs("ws-2"));
    expect(d1?.task_mrs[TASK_ID]?.[0]?.mr_title).toBe("multi-ws");
    expect(d2?.task_mrs[TASK_ID]?.[0]?.mr_title).toBe("multi-ws");
  });

  it("cleanup function removes all handlers", () => {
    qc = makeClient(WS_ID, INITIAL_CACHE);
    const ws2 = makeFakeWs();
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const cleanup = registerGitlabBridge(ws2 as any, qc);
    cleanup();

    ws2.emit(MR_UPDATED_EVENT, { payload: makeMR({ mr_title: "should not apply" }) });
    const data = qc.getQueryData<TaskMRsResponse>(qk.gitlab.mrs(WS_ID));
    expect(data?.task_mrs[TASK_ID]?.[0]?.mr_title).toBe("Test MR");
  });
});
