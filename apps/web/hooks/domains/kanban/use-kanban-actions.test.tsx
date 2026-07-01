import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import type { AppState } from "@/lib/state/store";
import {
  taskId as toTaskId,
  workflowId as toWorkflowId,
  workspaceId as toWorkspaceId,
} from "@/lib/types/ids";
import type { Task, Workflow, WorkflowSnapshot } from "@/lib/types/http";
import { useKanbanActions } from "./use-kanban-actions";

const WORKSPACE_ID = toWorkspaceId("workspace-1");
const WORKFLOW_ID = toWorkflowId("workflow-1");
const STEP_ID = "step-1";
const CREATED_AT = "2026-06-24T00:00:00Z";

function createQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
}

function wrapperFor(queryClient: QueryClient) {
  const initialState = {
    workspaces: { activeId: WORKSPACE_ID },
    workflows: { activeId: WORKFLOW_ID },
    kanban: {
      workflowId: null,
      steps: [],
      tasks: [],
      isLoading: false,
    },
  } as Partial<AppState>;

  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <StateProvider initialState={initialState}>{children}</StateProvider>
      </QueryClientProvider>
    );
  };
}

function workflow(): Workflow {
  return {
    id: WORKFLOW_ID,
    workspace_id: WORKSPACE_ID,
    name: "Build",
    sort_order: 0,
    hidden: false,
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
  };
}

function task(id: string, title = id): Task {
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
  } as Task;
}

function snapshot(tasks: Task[]): WorkflowSnapshot {
  return {
    workflow: workflow(),
    steps: [
      {
        id: STEP_ID,
        workflow_id: WORKFLOW_ID,
        name: "Todo",
        position: 0,
        color: "bg-blue-500",
      },
    ],
    tasks,
  } as WorkflowSnapshot;
}

function seedWorkflows(queryClient: QueryClient) {
  queryClient.setQueryData(qk.workflows.all(WORKSPACE_ID, { includeHidden: true }), [workflow()]);
}

describe("useKanbanActions", () => {
  afterEach(() => {
    cleanup();
  });

  it("adds created tasks to the workflow snapshot query cache", () => {
    const queryClient = createQueryClient();
    seedWorkflows(queryClient);
    queryClient.setQueryData(qk.workflows.snapshot(WORKFLOW_ID), snapshot([]));
    const { result } = renderHook(
      () =>
        useKanbanActions({
          workspaceState: { activeId: WORKSPACE_ID },
          workflowsState: { activeId: WORKFLOW_ID },
        }),
      { wrapper: wrapperFor(queryClient) },
    );

    act(() => {
      result.current.handleDialogSuccess(task("task-1", "Created from dialog"), "create");
    });

    expect(
      queryClient
        .getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WORKFLOW_ID))
        ?.tasks.map((item) => item.title),
    ).toEqual(["Created from dialog"]);
  });

  it("updates edited task metadata in the workflow snapshot query cache", () => {
    const queryClient = createQueryClient();
    seedWorkflows(queryClient);
    queryClient.setQueryData(qk.workflows.snapshot(WORKFLOW_ID), snapshot([task("task-1", "Old")]));
    const { result } = renderHook(
      () =>
        useKanbanActions({
          workspaceState: { activeId: WORKSPACE_ID },
          workflowsState: { activeId: WORKFLOW_ID },
        }),
      { wrapper: wrapperFor(queryClient) },
    );

    act(() => {
      result.current.handleDialogSuccess(task("task-1", "Edited"), "edit");
    });

    expect(
      queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WORKFLOW_ID))?.tasks[0]
        ?.title,
    ).toBe("Edited");
  });
});
