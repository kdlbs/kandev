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
import { useSwimlaneMove } from "./use-swimlane-move";

const actionMocks = vi.hoisted(() => ({
  moveTaskById: vi.fn(),
}));

vi.mock("@/hooks/use-task-actions", () => ({
  useTaskActions: () => ({
    moveTaskById: actionMocks.moveTaskById,
  }),
}));

const WORKSPACE_ID = toWorkspaceId("workspace-1");
const WORKFLOW_ID = toWorkflowId("workflow-1");
const STEP_TODO = "step-todo";
const STEP_DONE = "step-done";
const CREATED_AT = "2026-06-24T00:00:00Z";

function createQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
}

function wrapperFor(queryClient: QueryClient) {
  const initialState = {
    kanbanMulti: { snapshots: {} },
  } as Partial<AppState>;

  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <StateProvider initialState={initialState}>{children}</StateProvider>
      </QueryClientProvider>
    );
  };
}

function rawTask(id: string, stepId = STEP_TODO, position = 0): Task {
  return {
    id: toTaskId(id),
    workspace_id: WORKSPACE_ID,
    workflow_id: WORKFLOW_ID,
    workflow_step_id: stepId,
    position,
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
        id: STEP_TODO,
        workflow_id: WORKFLOW_ID,
        name: "Todo",
        position: 0,
        color: "bg-blue-500",
      },
      {
        id: STEP_DONE,
        workflow_id: WORKFLOW_ID,
        name: "Done",
        position: 1,
        color: "bg-green-500",
      },
    ],
    tasks,
  } as WorkflowSnapshot;
}

function cardTask(id: string, stepId = STEP_TODO, position = 0) {
  return {
    id,
    title: id,
    workflowStepId: stepId,
    position,
    primarySessionId: null,
  };
}

describe("useSwimlaneMove", () => {
  beforeEach(() => {
    actionMocks.moveTaskById.mockReset();
    actionMocks.moveTaskById.mockResolvedValue({});
  });

  afterEach(() => {
    cleanup();
  });

  it("moves tasks in the workflow snapshot query cache without kanbanMulti", async () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.workflows.snapshot(WORKFLOW_ID), snapshot([rawTask("task-1")]));
    const { result } = renderHook(() => useSwimlaneMove(WORKFLOW_ID, { onMoveError: vi.fn() }), {
      wrapper: wrapperFor(queryClient),
    });

    await act(async () => {
      await result.current.moveTask(cardTask("task-1"), STEP_DONE);
    });

    expect(actionMocks.moveTaskById).toHaveBeenCalledWith("task-1", {
      workflow_id: WORKFLOW_ID,
      workflow_step_id: STEP_DONE,
      position: 0,
    });
    expect(
      queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WORKFLOW_ID))?.tasks[0]
        ?.workflow_step_id,
    ).toBe(STEP_DONE);
  });

  it("rolls back the workflow snapshot query cache when a swimlane move fails", async () => {
    actionMocks.moveTaskById.mockRejectedValueOnce(new Error("move failed"));
    const onMoveError = vi.fn();
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.workflows.snapshot(WORKFLOW_ID), snapshot([rawTask("task-1")]));
    const { result } = renderHook(() => useSwimlaneMove(WORKFLOW_ID, { onMoveError }), {
      wrapper: wrapperFor(queryClient),
    });

    await act(async () => {
      await result.current.moveTask(cardTask("task-1"), STEP_DONE);
    });

    expect(
      queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WORKFLOW_ID))?.tasks[0]
        ?.workflow_step_id,
    ).toBe(STEP_TODO);
    expect(onMoveError).toHaveBeenCalledWith({
      message: "move failed",
      taskId: "task-1",
      sessionId: null,
    });
  });
});
