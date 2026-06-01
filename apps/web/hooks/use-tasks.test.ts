import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

const mockHydrate = vi.fn();
const mockFetchWorkflowSnapshot = vi.fn();
const mockListWorkflows = vi.fn();

type Task = { id: string; title: string };
type MockState = {
  connection: { status: string };
  workspaces: { activeId: string | null };
  kanban: { workflowId: string | null; tasks: Task[]; steps: unknown[]; isLoading?: boolean };
  hydrate: typeof mockHydrate;
};

let mockState: MockState = {
  connection: { status: "connected" },
  workspaces: { activeId: "ws-1" },
  kanban: { workflowId: null, tasks: [], steps: [] },
  hydrate: mockHydrate,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
  useAppStoreApi: () => ({
    getState: () => mockState,
    setState: (updater: (s: MockState) => MockState) => {
      mockState = updater(mockState);
    },
  }),
}));

vi.mock("@/lib/api", () => ({
  fetchWorkflowSnapshot: (...args: unknown[]) => mockFetchWorkflowSnapshot(...args),
  listWorkflows: (...args: unknown[]) => mockListWorkflows(...args),
}));

// useEnsureWorkflowSnapshot fetches via the kanban-api module directly.
vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchWorkflowSnapshot: (...args: unknown[]) => mockFetchWorkflowSnapshot(...args),
}));

import { useTasks } from "./use-tasks";
import { qk } from "@/lib/query/keys";
import type { KanbanMultiData } from "@/lib/query/query-options/kanban";

function createWrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  };
}

function seedMulti(qc: QueryClient, snapshots: KanbanMultiData["snapshots"]) {
  qc.setQueryData<KanbanMultiData>(qk.kanban.multi(), { snapshots });
}

describe("useTasks — loading and matching", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFetchWorkflowSnapshot.mockReturnValue(new Promise(() => {}));
    mockListWorkflows.mockReturnValue(new Promise(() => {}));
    mockState = {
      connection: { status: "connected" },
      workspaces: { activeId: "ws-1" },
      kanban: { workflowId: null, tasks: [], steps: [], isLoading: false },
      hydrate: mockHydrate,
    };
  });

  it("returns empty list and not-loading when workflowId is null", () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useTasks(null), { wrapper: createWrapper(qc) });
    expect(result.current.tasks).toEqual([]);
    expect(result.current.isLoading).toBe(false);
  });

  it("returns tasks for the requested workflow once the TQ cache is populated", () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    seedMulti(qc, {
      "wf-1": {
        workflowId: "wf-1",
        workflowName: "WF 1",
        steps: [],
        tasks: [
          { id: "t1", workflowStepId: "s1", title: "One", position: 0 },
          { id: "t2", workflowStepId: "s1", title: "Two", position: 1 },
        ],
      },
    });
    const { result } = renderHook(() => useTasks("wf-1"), { wrapper: createWrapper(qc) });
    expect(result.current.tasks).toHaveLength(2);
    expect(result.current.isLoading).toBe(false);
  });

  it("reports isLoading while the fetch is in flight and no snapshot is cached yet", () => {
    // The kanban→useQuery migration moved the loading signal onto the query's
    // isFetching flag. With a real workflowId, an empty cache, and a queryFn
    // that never resolves, the hook must surface isLoading=true (and no tasks).
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useTasks("wf-1"), { wrapper: createWrapper(qc) });
    expect(result.current.isLoading).toBe(true);
    expect(result.current.tasks).toEqual([]);
  });

  it("returns empty list for an unknown workflow even when the cache has data", () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    seedMulti(qc, {
      "wf-other": {
        workflowId: "wf-other",
        workflowName: "Other",
        steps: [],
        tasks: [{ id: "t1", workflowStepId: "s1", title: "One", position: 0 }],
      },
    });
    const { result } = renderHook(() => useTasks("wf-1"), { wrapper: createWrapper(qc) });
    expect(result.current.tasks).toEqual([]);
    expect(result.current.isLoading).toBe(false);
  });
});
