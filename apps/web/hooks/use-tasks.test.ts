import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";

const mockHydrate = vi.fn();
const mockSetState = vi.fn();
const mockFetchWorkflowSnapshot = vi.fn();

type Task = { id: string; title: string };
type MockState = {
  connection: { status: string };
  kanban: { workflowId: string | null; tasks: Task[]; steps: unknown[]; isLoading?: boolean };
  hydrate: typeof mockHydrate;
};

let mockState: MockState = {
  connection: { status: "connected" },
  kanban: { workflowId: null, tasks: [], steps: [] },
  hydrate: mockHydrate,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
  useAppStoreApi: () => ({
    getState: () => mockState,
    setState: (updater: (s: MockState) => MockState) => {
      // Apply the updater so useWorkflowSnapshot's isLoading transitions are
      // visible to subsequent reads in the same render cycle. Without this
      // the mock would silently swallow side effects.
      mockState = updater(mockState);
      mockSetState(mockState);
    },
  }),
}));

vi.mock("@/lib/api", () => ({
  fetchWorkflowSnapshot: (...args: unknown[]) => mockFetchWorkflowSnapshot(...args),
}));

vi.mock("@/lib/ssr/mapper", () => ({
  snapshotToState: (snapshot: unknown) => ({ snapshot }),
}));

import { useTasks } from "./use-tasks";

function setMockState(patch: Partial<MockState["kanban"]> & { workflowId?: string | null }) {
  mockState = {
    ...mockState,
    kanban: { ...mockState.kanban, ...patch },
  };
}

describe("useTasks — loading and matching", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Never resolves so we observe pre-resolution loading state.
    mockFetchWorkflowSnapshot.mockReturnValue(new Promise(() => {}));
    mockState = {
      connection: { status: "connected" },
      kanban: { workflowId: null, tasks: [], steps: [], isLoading: false },
      hydrate: mockHydrate,
    };
  });

  it("returns empty list and not-loading when workflowId is null", () => {
    const { result } = renderHook(() => useTasks(null));
    expect(result.current.tasks).toEqual([]);
    expect(result.current.isLoading).toBe(false);
  });

  it("reports loading once useWorkflowSnapshot has flipped kanban.isLoading", () => {
    setMockState({ workflowId: null, isLoading: true });
    const { result } = renderHook(() => useTasks("wf-1"));
    expect(result.current.tasks).toEqual([]);
    expect(result.current.isLoading).toBe(true);
  });

  it("does not report loading when fetch has settled but workflowId mismatches", () => {
    // Simulates a failed snapshot fetch — `kanban.isLoading` is back to false
    // and `kanban.workflowId` never advanced to the requested id. Caller
    // shows the empty-state UI rather than an infinite skeleton.
    setMockState({ workflowId: null, isLoading: false });
    const { result } = renderHook(() => useTasks("wf-1"));
    expect(result.current.tasks).toEqual([]);
    expect(result.current.isLoading).toBe(false);
  });

  it("returns tasks and not-loading once kanban.workflowId matches", () => {
    setMockState({
      workflowId: "wf-1",
      tasks: [
        { id: "t1", title: "One" },
        { id: "t2", title: "Two" },
      ],
      isLoading: false,
    });
    const { result } = renderHook(() => useTasks("wf-1"));
    expect(result.current.tasks).toHaveLength(2);
    expect(result.current.isLoading).toBe(false);
  });

  it("filters out tasks that belong to a different workflow snapshot", () => {
    setMockState({
      workflowId: "wf-other",
      tasks: [{ id: "t1", title: "One" }],
      isLoading: false,
    });
    const { result } = renderHook(() => useTasks("wf-1"));
    expect(result.current.tasks).toEqual([]);
    expect(result.current.isLoading).toBe(false);
  });

  it("respects kanban.isLoading even when workflowId already matches", () => {
    setMockState({
      workflowId: "wf-1",
      tasks: [{ id: "t1", title: "One" }],
      isLoading: true,
    });
    const { result } = renderHook(() => useTasks("wf-1"));
    expect(result.current.isLoading).toBe(true);
  });
});
