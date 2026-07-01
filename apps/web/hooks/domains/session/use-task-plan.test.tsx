import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { qk } from "@/lib/query/keys";
import type { TaskPlan, TaskPlanRevision } from "@/lib/types/http";
import { useTaskPlan } from "./use-task-plan";

const TEST_TASK_ID = "task-1";
const TEST_TIMESTAMP = "2026-06-24T00:00:00Z";

const apiMocks = vi.hoisted(() => ({
  createTaskPlan: vi.fn(),
  deleteTaskPlan: vi.fn(),
  getPlanRevision: vi.fn(),
  getTaskPlan: vi.fn(),
  listPlanRevisions: vi.fn(),
  revertPlanRevision: vi.fn(),
  updateTaskPlan: vi.fn(),
}));

const storeState = vi.hoisted(() => ({
  connection: { status: "connected" },
  taskPlans: {
    previewRevisionIdByTaskId: {},
    comparePairByTaskId: {},
  },
  hydrateTaskPlanLastSeen: vi.fn(),
  markTaskPlanSeen: vi.fn(),
  setPreviewRevision: vi.fn(),
  toggleComparePair: vi.fn(),
  clearComparePair: vi.fn(),
}));

vi.mock("@/lib/api/domains/plan-api", () => apiMocks);
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity },
      mutations: { retry: false },
    },
  });
}

function wrapperFor(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

function makePlan(overrides: Partial<TaskPlan> = {}): TaskPlan {
  return {
    id: "plan-1",
    task_id: TEST_TASK_ID,
    title: "Plan",
    content: "# Plan",
    created_by: "user",
    created_at: TEST_TIMESTAMP,
    updated_at: TEST_TIMESTAMP,
    ...overrides,
  };
}

function makeRevision(overrides: Partial<TaskPlanRevision> = {}): TaskPlanRevision {
  return {
    id: "revision-1",
    task_id: TEST_TASK_ID,
    revision_number: 1,
    title: "Plan",
    author_kind: "agent",
    author_name: "Agent",
    revert_of_revision_id: null,
    created_at: TEST_TIMESTAMP,
    updated_at: TEST_TIMESTAMP,
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  apiMocks.listPlanRevisions.mockResolvedValue([]);
});

describe("useTaskPlan", () => {
  it("forces explicit plan refetches past the stale window", async () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.taskPlan.detail(TEST_TASK_ID), makePlan({ content: "# Cached" }));
    queryClient.setQueryData(qk.taskPlan.revisions(TEST_TASK_ID), []);
    apiMocks.getTaskPlan.mockResolvedValue(makePlan({ content: "# Fresh" }));

    const { result } = renderHook(() => useTaskPlan(TEST_TASK_ID), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.plan?.content).toBe("# Cached"));
    await act(async () => {
      await result.current.refetch();
    });

    expect(apiMocks.getTaskPlan).toHaveBeenCalledTimes(1);
    expect(queryClient.getQueryData(qk.taskPlan.detail(TEST_TASK_ID))).toMatchObject({
      content: "# Fresh",
    });
    expect(storeState.markTaskPlanSeen).toHaveBeenCalledWith(TEST_TASK_ID, TEST_TIMESTAMP);
  });

  it("forces explicit revision loads past the stale window", async () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.taskPlan.detail(TEST_TASK_ID), makePlan());
    queryClient.setQueryData(qk.taskPlan.revisions(TEST_TASK_ID), [
      makeRevision({ id: "revision-cached", revision_number: 1, title: "Cached" }),
    ]);
    apiMocks.listPlanRevisions.mockResolvedValue([
      makeRevision({ id: "revision-fresh", revision_number: 2, title: "Fresh" }),
    ]);

    const { result } = renderHook(() => useTaskPlan(TEST_TASK_ID), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.revisions[0]?.title).toBe("Cached"));
    await act(async () => {
      await result.current.loadRevisions();
    });

    expect(apiMocks.listPlanRevisions).toHaveBeenCalledTimes(1);
    expect(queryClient.getQueryData(qk.taskPlan.revisions(TEST_TASK_ID))).toMatchObject([
      { id: "revision-fresh", revision_number: 2, title: "Fresh" },
    ]);
  });
});
