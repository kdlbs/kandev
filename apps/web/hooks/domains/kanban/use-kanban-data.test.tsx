import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import type { AppState } from "@/lib/state/store";
import {
  taskId as toTaskId,
  workflowId as toWorkflowId,
  workspaceId as toWorkspaceId,
  type Task,
  type Workflow,
  type WorkflowSnapshot,
} from "@/lib/types/http";
import { useKanbanData } from "./use-kanban-data";

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchWorkflowSnapshot: vi.fn(),
  listWorkflows: vi.fn(),
}));

vi.mock("@/lib/api/domains/workspace-api", () => ({
  listRepositories: vi.fn(),
}));

const WORKSPACE_ID = toWorkspaceId("workspace-1");
const WORKFLOW_ID = toWorkflowId("workflow-1");
const STEP_ID = "step-1";
const CREATED_AT = "2026-06-24T00:00:00Z";

function queryClientWithBoardData() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  queryClient.setQueryData(qk.workflows.all(WORKSPACE_ID, { includeHidden: true }), [workflow()]);
  queryClient.setQueryData(
    qk.workflows.snapshot(WORKFLOW_ID),
    snapshot([task("task-1", "Query task"), task("task-2", "Other task")]),
  );
  queryClient.setQueryData(qk.workspaces.repositories(WORKSPACE_ID), []);
  return queryClient;
}

function wrapperFor(queryClient: QueryClient) {
  const initialState = {
    workspaces: {
      activeId: WORKSPACE_ID,
      items: [
        {
          id: WORKSPACE_ID,
          name: "Workspace",
          owner_id: "owner-1",
          created_at: CREATED_AT,
          updated_at: CREATED_AT,
        },
      ],
    },
    workflows: { activeId: WORKFLOW_ID },
    kanban: {
      workflowId: WORKFLOW_ID,
      steps: [{ id: "legacy-step", title: "Legacy", color: "bg-red-500", position: 0 }],
      tasks: [{ id: "legacy-task", workflowStepId: "legacy-step", title: "Legacy", position: 0 }],
      isLoading: false,
    },
    userSettings: {
      ...defaultSettingsState.userSettings,
      loaded: true,
      workspaceId: WORKSPACE_ID,
      workflowId: WORKFLOW_ID,
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
    workflow: workflow(),
    steps: [
      {
        id: STEP_ID,
        workflow_id: WORKFLOW_ID,
        name: "Query Todo",
        position: 0,
        color: "bg-blue-500",
        allow_manual_move: true,
      },
    ],
    tasks,
  };
}

describe("useKanbanData", () => {
  afterEach(() => {
    cleanup();
  });

  it("uses workflow snapshot query data instead of the active kanban store mirror", () => {
    const { result } = renderHook(
      () =>
        useKanbanData({
          onWorkspaceChange: vi.fn(),
          onWorkflowChange: vi.fn(),
          searchQuery: "query",
        }),
      { wrapper: wrapperFor(queryClientWithBoardData()) },
    );

    expect(result.current.boardState.workflowId).toBe(WORKFLOW_ID);
    expect(result.current.activeSteps).toEqual([
      expect.objectContaining({ id: STEP_ID, title: "Query Todo" }),
    ]);
    expect(result.current.filteredTasks).toEqual([
      expect.objectContaining({ id: "task-1", title: "Query task" }),
    ]);
  });
});
