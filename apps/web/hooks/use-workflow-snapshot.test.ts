import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { WorkflowSnapshot } from "@/lib/types/http";
import { useWorkflowSnapshot } from "./use-workflow-snapshot";

const mockFetchWorkflowSnapshot = vi.fn();

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchWorkflowSnapshot: (...args: unknown[]) => mockFetchWorkflowSnapshot(...args),
}));

const WORKFLOW_ID = "workflow-1";
const WORKSPACE_ID = "workspace-1";

function makeSnapshot(workflowId: string): WorkflowSnapshot {
  return {
    workflow: {
      id: workflowId,
      workspace_id: WORKSPACE_ID,
      name: workflowId,
      sort_order: 0,
      hidden: false,
    },
    steps: [
      {
        id: "step-1",
        workflow_id: workflowId,
        name: "Todo",
        position: 0,
        color: "bg-blue-500",
        allow_manual_move: true,
      },
    ],
    tasks: [
      {
        id: "task-1",
        workspace_id: WORKSPACE_ID,
        workflow_id: workflowId,
        workflow_step_id: "step-1",
        position: 0,
        title: "Task",
        description: "",
        state: "TODO",
        priority: 0,
        repositories: [],
        created_at: "2026-06-24T00:00:00Z",
        updated_at: "2026-06-24T00:00:00Z",
      },
    ],
  } as unknown as WorkflowSnapshot;
}

function createQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
}

function wrapperFor(queryClient: QueryClient, initialState?: Partial<AppState>) {
  return function Wrapper({ children }: { children: ReactNode }) {
    const tree = createElement(QueryClientProvider, { client: queryClient }, children);
    if (!initialState) return tree;
    return createElement(StateProvider, { initialState, children: tree });
  };
}

describe("useWorkflowSnapshot", () => {
  beforeEach(() => {
    mockFetchWorkflowSnapshot.mockReset();
  });

  afterEach(() => {
    cleanup();
  });

  it("cold-loads and converts a workflow snapshot through the query option", async () => {
    mockFetchWorkflowSnapshot.mockResolvedValue(makeSnapshot(WORKFLOW_ID));
    const queryClient = createQueryClient();

    const { result } = renderHook(() => useWorkflowSnapshot(WORKFLOW_ID), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.snapshotState?.workflowId).toBe(WORKFLOW_ID));
    expect(result.current.snapshotState?.steps[0]?.title).toBe("Todo");
    expect(result.current.snapshotState?.tasks[0]?.title).toBe("Task");
    expect(mockFetchWorkflowSnapshot).toHaveBeenCalledWith(WORKFLOW_ID, expect.anything());
  });

  it("does not return boot-hydrated store data while Query loads", () => {
    mockFetchWorkflowSnapshot.mockReturnValue(new Promise(() => {}));
    const queryClient = createQueryClient();
    const initialState = {
      kanban: {
        workflowId: WORKFLOW_ID,
        isLoading: false,
        steps: [{ id: "step-1", title: "Todo", color: "bg-blue-500", position: 0 }],
        tasks: [{ id: "task-1", workflowStepId: "step-1", title: "Task", position: 0 }],
      },
    } as Partial<AppState>;

    const { result } = renderHook(() => useWorkflowSnapshot(WORKFLOW_ID), {
      wrapper: wrapperFor(queryClient, initialState),
    });

    expect(result.current.snapshotState).toBeNull();
  });

  it("does nothing when workflowId is null", () => {
    const queryClient = createQueryClient();

    const { result } = renderHook(() => useWorkflowSnapshot(null), {
      wrapper: wrapperFor(queryClient),
    });

    expect(mockFetchWorkflowSnapshot).not.toHaveBeenCalled();
    expect(result.current.snapshotState).toBeNull();
  });
});
