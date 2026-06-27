import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { qk } from "@/lib/query/keys";
import type { Workflow, WorkflowSnapshot } from "@/lib/types/http";
import { useAllWorkflowSnapshots } from "./use-all-workflow-snapshots";

const mockFetchWorkflowSnapshot = vi.fn();

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchWorkflowSnapshot: (...args: unknown[]) => mockFetchWorkflowSnapshot(...args),
}));

const WORKSPACE_A = "workspace-A";
const WORKSPACE_B = "workspace-B";

function workflow(id: string, workspaceId: string, name = id): Workflow {
  return {
    id,
    workspace_id: workspaceId,
    name,
    sort_order: 0,
    hidden: false,
  } as Workflow;
}

function snapshot(workflowId: string, workspaceId: string, taskId = "task-1"): WorkflowSnapshot {
  return {
    workflow: workflow(workflowId, workspaceId),
    steps: [
      {
        id: `${workflowId}-step`,
        workflow_id: workflowId,
        name: "Todo",
        position: 0,
        color: "bg-blue-500",
        allow_manual_move: true,
      },
    ],
    tasks: [
      {
        id: taskId,
        workspace_id: workspaceId,
        workflow_id: workflowId,
        workflow_step_id: `${workflowId}-step`,
        position: 0,
        title: taskId,
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
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
}

function wrapperFor(queryClient: QueryClient) {
  return function QueryWrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children);
  };
}

describe("useAllWorkflowSnapshots", () => {
  beforeEach(() => {
    mockFetchWorkflowSnapshot.mockReset();
  });

  afterEach(() => {
    cleanup();
  });

  it("returns converted snapshots for workflows scoped to the active workspace", () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.workflows.all(WORKSPACE_A, { includeHidden: true }), [
      workflow("wf-A", WORKSPACE_A, "Alpha"),
    ]);
    queryClient.setQueryData(qk.workflows.all(WORKSPACE_B, { includeHidden: true }), [
      workflow("wf-B", WORKSPACE_B, "Beta"),
    ]);
    queryClient.setQueryData(qk.workflows.snapshot("wf-A"), snapshot("wf-A", WORKSPACE_A, "t-a"));
    queryClient.setQueryData(qk.workflows.snapshot("wf-B"), snapshot("wf-B", WORKSPACE_B, "t-b"));

    const { result } = renderHook(() => useAllWorkflowSnapshots(WORKSPACE_A), {
      wrapper: wrapperFor(queryClient),
    });

    expect(Object.keys(result.current.snapshots)).toEqual(["wf-A"]);
    expect(result.current.snapshots["wf-A"]?.tasks[0]?.id).toBe("t-a");
    expect(result.current.isLoading).toBe(false);
  });

  it("switches workspace scope without returning stale snapshots", () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.workflows.all(WORKSPACE_A, { includeHidden: true }), [
      workflow("wf-A", WORKSPACE_A, "Alpha"),
    ]);
    queryClient.setQueryData(qk.workflows.all(WORKSPACE_B, { includeHidden: true }), [
      workflow("wf-B", WORKSPACE_B, "Beta"),
    ]);
    queryClient.setQueryData(qk.workflows.snapshot("wf-A"), snapshot("wf-A", WORKSPACE_A, "t-a"));
    queryClient.setQueryData(qk.workflows.snapshot("wf-B"), snapshot("wf-B", WORKSPACE_B, "t-b"));

    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string }) => useAllWorkflowSnapshots(workspaceId),
      {
        initialProps: { workspaceId: WORKSPACE_A },
        wrapper: wrapperFor(queryClient),
      },
    );

    expect(Object.keys(result.current.snapshots)).toEqual(["wf-A"]);
    rerender({ workspaceId: WORKSPACE_B });
    expect(Object.keys(result.current.snapshots)).toEqual(["wf-B"]);
  });

  it("reports loading only before the first snapshot is available", async () => {
    let resolveSnapshot: (value: WorkflowSnapshot) => void = () => {};
    mockFetchWorkflowSnapshot.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveSnapshot = resolve;
        }),
    );
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.workflows.all(WORKSPACE_A, { includeHidden: true }), [
      workflow("wf-A", WORKSPACE_A, "Alpha"),
    ]);

    const { result } = renderHook(() => useAllWorkflowSnapshots(WORKSPACE_A), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current).toEqual({ snapshots: {}, isLoading: true });
    resolveSnapshot(snapshot("wf-A", WORKSPACE_A, "t-a"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.snapshots["wf-A"]?.tasks[0]?.id).toBe("t-a");
  });
});
