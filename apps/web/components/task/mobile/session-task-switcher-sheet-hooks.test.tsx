import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import type { AppState } from "@/lib/state/store";
import { repositoryId, sessionId, taskId, workflowId, workspaceId } from "@/lib/types/ids";
import type { Repository, Task, Workflow, WorkflowSnapshot, Workspace } from "@/lib/types/http";
import { useSheetActions, useSheetData } from "./session-task-switcher-sheet-hooks";

const mocks = vi.hoisted(() => ({
  fetchWorkflowSnapshot: vi.fn(),
  launchSession: vi.fn(async (_request: unknown) => ({})),
  listRepositories: vi.fn(),
  listTaskSessions: vi.fn(),
  listWorkflows: vi.fn(),
  replaceTaskUrl: vi.fn(),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchWorkflowSnapshot: (...args: unknown[]) => mocks.fetchWorkflowSnapshot(...args),
  listWorkflows: (...args: unknown[]) => mocks.listWorkflows(...args),
}));

vi.mock("@/lib/api/domains/workspace-api", () => ({
  listRepositories: (...args: unknown[]) => mocks.listRepositories(...args),
}));

vi.mock("@/lib/api/domains/session-api", () => ({
  fetchTaskSession: vi.fn(),
  listSessionTurns: vi.fn(),
  listTaskSessionMessages: vi.fn(),
  listTaskSessions: (...args: unknown[]) => mocks.listTaskSessions(...args),
  searchSessionMessages: vi.fn(),
}));

vi.mock("@/lib/services/session-launch-service", () => ({
  launchSession: (request: unknown) => mocks.launchSession(request),
}));

vi.mock("@/lib/links", () => ({
  replaceTaskUrl: (...args: unknown[]) => mocks.replaceTaskUrl(...args),
}));

const WORKSPACE_ID = workspaceId("workspace-1");
const OTHER_WORKSPACE_ID = workspaceId("workspace-2");
const WORKFLOW_ID = workflowId("workflow-1");
const OTHER_WORKFLOW_ID = workflowId("workflow-2");
const STEP_ID = "step-1";
const OTHER_STEP_ID = "step-2";
const TASK_ID = taskId("task-1");
const OTHER_TASK_ID = taskId("task-2");
const SESSION_ID = sessionId("session-1");
const CREATED_AT = "2026-06-24T00:00:00Z";

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity },
      mutations: { retry: false },
    },
  });
}

function seedQueryData(queryClient: QueryClient) {
  queryClient.setQueryData(qk.workflows.all(WORKSPACE_ID, { includeHidden: true }), [
    workflow(WORKFLOW_ID, WORKSPACE_ID, "Build"),
  ]);
  queryClient.setQueryData(
    qk.workflows.snapshot(WORKFLOW_ID),
    snapshot(WORKFLOW_ID, WORKSPACE_ID, [
      task({ id: TASK_ID, title: "Query task", primarySessionId: SESSION_ID }),
    ]),
  );
  queryClient.setQueryData(qk.workspaces.repositories(WORKSPACE_ID), [repository()]);
}

function wrapperFor(queryClient: QueryClient) {
  const initialState = {
    workspaces: {
      activeId: WORKSPACE_ID,
      items: [workspace(WORKSPACE_ID, "Workspace"), workspace(OTHER_WORKSPACE_ID, "Other")],
    },
    workflows: { activeId: WORKFLOW_ID },
  } as Partial<AppState>;

  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <StateProvider initialState={initialState}>{children}</StateProvider>
      </QueryClientProvider>
    );
  };
}

function renderActions(queryClient: QueryClient, onOpenChange = vi.fn()) {
  return renderHook(
    () => ({
      actions: useSheetActions(WORKSPACE_ID, onOpenChange),
      store: useAppStoreApi(),
    }),
    { wrapper: wrapperFor(queryClient) },
  );
}

function workflow(id: string, workspace: string, name: string): Workflow {
  return {
    id: workflowId(id),
    workspace_id: workspaceId(workspace),
    name,
    sort_order: 0,
    hidden: false,
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
  };
}

function workspace(id: string, name: string): Workspace {
  return {
    id: workspaceId(id),
    name,
    owner_id: "owner-1",
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
  };
}

function repository(): Repository {
  return {
    id: repositoryId("repo-1"),
    workspace_id: WORKSPACE_ID,
    name: "kandev",
    source_type: "local",
    local_path: "/work/kandev",
    provider: "github",
    provider_repo_id: "repo-1",
    provider_owner: "kdlbs",
    provider_name: "kandev",
    default_branch: "main",
    worktree_branch_prefix: "",
    pull_before_worktree: false,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
  };
}

type TaskFixture = {
  id: string;
  title: string;
  workflow?: string;
  workspace?: string;
  step?: string;
  primarySessionId?: string | null;
};

function task({
  id,
  title,
  workflow = WORKFLOW_ID,
  workspace = WORKSPACE_ID,
  step = STEP_ID,
  primarySessionId = null,
}: TaskFixture): Task {
  return {
    id: taskId(id),
    workspace_id: workspaceId(workspace),
    workflow_id: workflowId(workflow),
    workflow_step_id: step,
    position: 0,
    title,
    description: "",
    state: "TODO",
    priority: 0,
    repositories: [
      {
        id: "task-repo-1",
        repository_id: repositoryId("repo-1"),
        base_branch: "main",
        position: 0,
      },
    ],
    primary_session_id: primarySessionId ? sessionId(primarySessionId) : null,
    primary_executor_type: "local_docker",
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
  } as Task;
}

function snapshot(workflowValue: string, _workspace: string, tasks: Task[]): WorkflowSnapshot {
  const stepId = workflowValue === OTHER_WORKFLOW_ID ? OTHER_STEP_ID : STEP_ID;
  return {
    workflow:
      workflowValue === OTHER_WORKFLOW_ID
        ? workflow(OTHER_WORKFLOW_ID, OTHER_WORKSPACE_ID, "Other Build")
        : workflow(WORKFLOW_ID, WORKSPACE_ID, "Build"),
    steps: [
      {
        id: stepId,
        workflow_id: workflowId(workflowValue),
        name: workflowValue === OTHER_WORKFLOW_ID ? "Other Todo" : "Query Todo",
        position: 0,
        color: "bg-blue-500",
        allow_manual_move: true,
      },
    ],
    tasks,
  };
}

describe("session task switcher sheet hooks", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.listWorkflows.mockImplementation(async (workspace: string) => ({
      workflows:
        workspace === OTHER_WORKSPACE_ID
          ? [workflow(OTHER_WORKFLOW_ID, OTHER_WORKSPACE_ID, "Other Build")]
          : [workflow(WORKFLOW_ID, WORKSPACE_ID, "Build")],
    }));
    mocks.fetchWorkflowSnapshot.mockImplementation(async (workflow: string) =>
      workflow === OTHER_WORKFLOW_ID
        ? snapshot(OTHER_WORKFLOW_ID, OTHER_WORKSPACE_ID, [
            task({
              id: OTHER_TASK_ID,
              title: "Other task",
              workflow: OTHER_WORKFLOW_ID,
              workspace: OTHER_WORKSPACE_ID,
              step: OTHER_STEP_ID,
            }),
          ])
        : snapshot(WORKFLOW_ID, WORKSPACE_ID, [
            task({ id: TASK_ID, title: "Query task", primarySessionId: SESSION_ID }),
          ]),
    );
    mocks.listRepositories.mockResolvedValue({ repositories: [] });
    mocks.listTaskSessions.mockResolvedValue({ sessions: [] });
    window.history.replaceState({}, "", "/");
  });

  afterEach(() => {
    cleanup();
  });

  it("builds sheet data from Query snapshots when legacy kanban mirrors are empty", () => {
    const queryClient = createQueryClient();
    seedQueryData(queryClient);

    const { result } = renderHook(() => useSheetData(WORKSPACE_ID), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current.dialogSteps).toEqual([
      expect.objectContaining({ id: STEP_ID, title: "Query Todo" }),
    ]);
    expect(result.current.tasksWithRepositories).toEqual([
      expect.objectContaining({
        id: TASK_ID,
        title: "Query task",
        workflowStepTitle: "Query Todo",
        primarySessionId: SESSION_ID,
      }),
    ]);
  });

  it("selects a task primary session from Query snapshots when legacy task mirrors are empty", () => {
    const queryClient = createQueryClient();
    seedQueryData(queryClient);
    const onOpenChange = vi.fn();
    const { result } = renderActions(queryClient, onOpenChange);

    act(() => {
      result.current.actions.handleSelectTask(TASK_ID);
    });

    expect(result.current.store.getState().tasks.activeTaskId).toBe(TASK_ID);
    expect(result.current.store.getState().tasks.activeSessionId).toBe(SESSION_ID);
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(mocks.replaceTaskUrl).toHaveBeenCalledWith(TASK_ID);
  });

  it("writes created tasks to the workflow snapshot cache instead of the legacy kanban mirror", () => {
    const queryClient = createQueryClient();
    seedQueryData(queryClient);
    const { result } = renderActions(queryClient);
    const created = task({ id: "task-new", title: "Created from mobile" });

    act(() => {
      result.current.actions.handleTaskCreated(created, "create", {
        taskSessionId: "session-new",
      });
    });

    const cached = queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WORKFLOW_ID));
    expect(cached?.tasks.map((item) => item.title)).toContain("Created from mobile");
    expect(cached?.tasks.find((item) => item.id === "task-new")?.primary_session_id).toBe(
      "session-new",
    );
    expect("kanban" in result.current.store.getState()).toBe(false);
  });

  it("switches workspace without a legacy kanban mirror", async () => {
    const queryClient = createQueryClient();
    seedQueryData(queryClient);
    const onOpenChange = vi.fn();
    const { result } = renderActions(queryClient, onOpenChange);

    await act(async () => {
      await result.current.actions.handleWorkspaceChange(OTHER_WORKSPACE_ID);
    });

    expect(result.current.store.getState().workflows.activeId).toBe(OTHER_WORKFLOW_ID);
    expect(result.current.store.getState().tasks.activeTaskId).toBe(OTHER_TASK_ID);
    expect("kanban" in result.current.store.getState()).toBe(false);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
