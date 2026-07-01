import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import {
  taskId as toTaskId,
  workflowId as toWorkflowId,
  workspaceId as toWorkspaceId,
  type Task,
  type WorkflowSnapshot,
} from "@/lib/types/http";
import { useTasks } from "./use-tasks";

const mockFetchWorkflowSnapshot = vi.fn();

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchWorkflowSnapshot: (...args: unknown[]) => mockFetchWorkflowSnapshot(...args),
}));

const WORKFLOW_ID = toWorkflowId("wf-1");
const WORKSPACE_ID = toWorkspaceId("workspace-1");
const STEP_ID = "step-1";
const CREATED_AT = "2026-06-24T00:00:00Z";

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: Infinity,
      },
    },
  });
}

function wrapperFor(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    const tree = createElement(QueryClientProvider, { client: queryClient }, children);
    return createElement(StateProvider, { children: tree });
  };
}

function task(id: string, title: string): Task {
  return {
    id: toTaskId(id),
    workspace_id: WORKSPACE_ID,
    workflow_id: WORKFLOW_ID,
    workflow_step_id: STEP_ID,
    position: 0,
    title,
    description: "",
    state: "TODO",
    priority: 0,
    repositories: [],
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
  };
}

function snapshot(tasks: Task[]): WorkflowSnapshot {
  return {
    workflow: {
      id: WORKFLOW_ID,
      workspace_id: WORKSPACE_ID,
      name: "Build",
      sort_order: 0,
      hidden: false,
      created_at: CREATED_AT,
      updated_at: CREATED_AT,
    },
    steps: [
      {
        id: STEP_ID,
        workflow_id: WORKFLOW_ID,
        name: "Todo",
        position: 0,
        color: "bg-blue-500",
        allow_manual_move: true,
      },
    ],
    tasks,
  };
}

describe("useTasks", () => {
  beforeEach(() => {
    mockFetchWorkflowSnapshot.mockReset();
  });

  afterEach(() => {
    cleanup();
  });

  it("returns empty list and not-loading when workflowId is null", () => {
    const { result } = renderHook(() => useTasks(null), {
      wrapper: wrapperFor(createQueryClient()),
    });

    expect(result.current.tasks).toEqual([]);
    expect(result.current.isLoading).toBe(false);
    expect(mockFetchWorkflowSnapshot).not.toHaveBeenCalled();
  });

  it("returns tasks from the workflow snapshot query cache", () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.workflows.snapshot(WORKFLOW_ID), snapshot([task("task-1", "One")]));

    const { result } = renderHook(() => useTasks(WORKFLOW_ID), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current.tasks).toEqual([
      expect.objectContaining({ id: "task-1", title: "One", workflowStepId: STEP_ID }),
    ]);
    expect(result.current.isLoading).toBe(false);
  });

  it("does not surface tasks while Query loads", () => {
    mockFetchWorkflowSnapshot.mockReturnValue(new Promise(() => {}));

    const { result } = renderHook(() => useTasks(WORKFLOW_ID), {
      wrapper: wrapperFor(createQueryClient()),
    });

    expect(result.current.tasks).toEqual([]);
    expect(result.current.isLoading).toBe(true);
  });

  it("does not show loading when Query already has tasks", () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.workflows.snapshot(WORKFLOW_ID), snapshot([task("task-1", "One")]));

    const { result } = renderHook(() => useTasks(WORKFLOW_ID), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current.tasks).toHaveLength(1);
    expect(result.current.isLoading).toBe(false);
  });

  it("settles to not-loading when an empty Query snapshot resolves", async () => {
    mockFetchWorkflowSnapshot.mockResolvedValue(snapshot([]));

    const { result } = renderHook(() => useTasks(WORKFLOW_ID), {
      wrapper: wrapperFor(createQueryClient()),
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.tasks).toEqual([]);
  });
});
