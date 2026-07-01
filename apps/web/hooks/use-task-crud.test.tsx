import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import type { AppState } from "@/lib/state/store";
import {
  taskId as toTaskId,
  workflowId as toWorkflowId,
  workspaceId as toWorkspaceId,
} from "@/lib/types/ids";
import type { Task, WorkflowSnapshot } from "@/lib/types/http";
import { useTaskCRUD } from "./use-task-crud";

const actionMocks = vi.hoisted(() => ({
  archiveTaskById: vi.fn(),
  deleteTaskById: vi.fn(),
}));

vi.mock("@/hooks/use-task-actions", () => ({
  useTaskActions: () => ({
    archiveTaskById: actionMocks.archiveTaskById,
    deleteTaskById: actionMocks.deleteTaskById,
  }),
}));

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

function task(id: string): Task {
  return {
    id: toTaskId(id),
    workspace_id: WORKSPACE_ID,
    workflow_id: WORKFLOW_ID,
    workflow_step_id: STEP_ID,
    position: 0,
    title: id,
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
      },
    ],
    tasks,
  } as WorkflowSnapshot;
}

function cardTask(id: string) {
  return {
    id,
    title: id,
    workflowStepId: STEP_ID,
    position: 0,
  };
}

describe("useTaskCRUD", () => {
  beforeEach(() => {
    actionMocks.archiveTaskById.mockReset();
    actionMocks.deleteTaskById.mockReset();
    actionMocks.archiveTaskById.mockResolvedValue({});
    actionMocks.deleteTaskById.mockResolvedValue({});
  });

  afterEach(() => {
    cleanup();
  });

  it("removes deleted tasks from workflow snapshot query caches", async () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData(
      qk.workflows.snapshot(WORKFLOW_ID),
      snapshot([task("task-1"), task("task-2")]),
    );
    const { result } = renderHook(() => useTaskCRUD(), { wrapper: wrapperFor(queryClient) });

    await act(async () => {
      await result.current.handleDelete(cardTask("task-1"));
    });

    expect(actionMocks.deleteTaskById).toHaveBeenCalledWith("task-1", undefined);
    expect(
      queryClient
        .getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WORKFLOW_ID))
        ?.tasks.map((item) => item.id),
    ).toEqual(["task-2"]);
  });

  it("removes archived tasks from workflow snapshot query caches", async () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData(
      qk.workflows.snapshot(WORKFLOW_ID),
      snapshot([task("task-1"), task("task-2")]),
    );
    const { result } = renderHook(() => useTaskCRUD(), { wrapper: wrapperFor(queryClient) });

    await act(async () => {
      await result.current.handleArchive(cardTask("task-1"));
    });

    expect(actionMocks.archiveTaskById).toHaveBeenCalledWith("task-1", undefined);
    expect(
      queryClient
        .getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WORKFLOW_ID))
        ?.tasks.map((item) => item.id),
    ).toEqual(["task-2"]);
  });
});
