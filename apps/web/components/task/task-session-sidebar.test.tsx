import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import type { AppState } from "@/lib/state/store";
import type { Message, Task, WorkflowSnapshot } from "@/lib/types/http";
import { sessionId, taskId, workflowId, workspaceId } from "@/lib/types/ids";
import { TaskSessionSidebar } from "./task-session-sidebar";

const WORKSPACE_ID = workspaceId("workspace-1");
const WORKFLOW_ID = workflowId("workflow-1");
const STEP_TODO = "step-todo";
const STEP_DONE = "step-done";
const PRIMARY_SESSION_ID = "session-1";
const CREATED_AT = "2026-06-24T00:00:00Z";

const actionMocks = vi.hoisted(() => ({
  archiveAndSwitch: vi.fn(),
  deleteTaskById: vi.fn(),
  moveTaskById: vi.fn(),
  renameTaskById: vi.fn(),
}));

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => "/t/task-1",
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));

vi.mock("@/hooks/use-task-actions", () => ({
  useArchiveAndSwitchTask: () => actionMocks.archiveAndSwitch,
  useTaskActions: () => ({
    archiveTaskById: actionMocks.archiveAndSwitch,
    deleteTaskById: actionMocks.deleteTaskById,
    moveTaskById: actionMocks.moveTaskById,
    renameTaskById: actionMocks.renameTaskById,
  }),
}));

vi.mock("@/hooks/domains/kanban/use-workspace-sidebar-tasks", () => ({
  useWorkspaceSidebarTasks: () => ({
    allTasks: [
      {
        id: "task-1",
        title: "Query task",
        state: "TODO",
        workflowId: WORKFLOW_ID,
        workflowStepId: STEP_TODO,
        primaryExecutorType: "remote-docker",
        primarySessionId: PRIMARY_SESSION_ID,
        createdAt: CREATED_AT,
        updatedAt: CREATED_AT,
        _workflowId: WORKFLOW_ID,
      },
    ],
    allSteps: [
      { id: STEP_TODO, title: "Todo", color: "bg-blue-500", position: 0 },
      { id: STEP_DONE, title: "Done", color: "bg-green-500", position: 1 },
    ],
    stepsByWorkflowId: {
      [WORKFLOW_ID]: [
        { id: STEP_TODO, title: "Todo", color: "bg-blue-500", position: 0 },
        { id: STEP_DONE, title: "Done", color: "bg-green-500", position: 1 },
      ],
    },
    workflows: [{ id: WORKFLOW_ID, name: "Build", hidden: false }],
    isLoading: false,
  }),
}));

vi.mock("@/hooks/domains/workspace/use-repository-cache", () => ({
  useCachedRepositories: () => [],
}));

vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: () => ({ data: [] }),
}));

vi.mock("@/hooks/domains/github/use-task-pr", () => ({
  useWorkspacePRs: () => ({}),
}));

vi.mock("@/hooks/domains/sidebar/use-effective-sidebar-view", () => ({
  useEffectiveSidebarView: () => ({
    id: "all",
    name: "All tasks",
    filters: [],
    sort: { key: "updatedAt", direction: "desc" },
    groupBy: "none",
    collapsedGroups: [],
  }),
}));

vi.mock("@/hooks/domains/sidebar/use-sidebar-task-prefs", () => ({
  useSidebarTaskPrefs: () => ({
    pinnedTaskIds: [],
    orderedTaskIds: [],
    subtaskOrderByParentId: {},
    togglePinnedTask: vi.fn(),
    handleReorderGroup: vi.fn(),
    handleReorderSubtasks: vi.fn(),
  }),
}));

vi.mock("@/lib/sidebar/apply-view", () => ({
  applyView: (tasks: unknown[]) => [{ key: "all", label: "All tasks", tasks }],
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => null,
}));

vi.mock("./task-archived-context", () => ({
  useArchivedTaskState: () => ({ isArchived: false }),
}));

vi.mock("./task-switcher", () => ({
  TaskSwitcher: ({
    grouped,
    onArchiveTask,
    onMoveToStep,
  }: {
    grouped: Array<{
      tasks: Array<{ id: string; title: string; hasPendingPermission?: boolean }>;
    }>;
    onArchiveTask: (taskId: string) => void;
    onMoveToStep: (taskId: string, workflowId: string, targetStepId: string) => void;
  }) => {
    const tasks = grouped.flatMap((group) => group.tasks);
    return (
      <div data-testid="task-switcher">
        {tasks.map((task) => (
          <div
            key={task.id}
            data-testid={`sidebar-task-${task.id}`}
            data-pending-permission={String(task.hasPendingPermission ?? false)}
          >
            {task.title}
          </div>
        ))}
        <button type="button" data-testid="archive-task" onClick={() => onArchiveTask("task-1")}>
          Archive
        </button>
        <button
          type="button"
          data-testid="move-task"
          onClick={() => onMoveToStep("task-1", WORKFLOW_ID, STEP_DONE)}
        >
          Move
        </button>
      </div>
    );
  },
}));

vi.mock("./task-session-sidebar-dialogs", () => ({
  SidebarDialogs: ({ actions }: { actions: { archivingTask: null | { title: string } } }) => (
    <div data-testid="sidebar-dialogs" data-archive-title={actions.archivingTask?.title ?? ""} />
  ),
}));

function rawTask(id: string, stepId = STEP_TODO, position = 0): Task {
  return {
    id: taskId(id),
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

function pendingPermissionMessage(): Message {
  return {
    id: "message-1",
    session_id: sessionId(PRIMARY_SESSION_ID),
    task_id: taskId("task-1"),
    turn_id: "turn-1",
    author_type: "agent",
    author_id: "agent-1",
    content: "permission",
    type: "permission_request",
    metadata: { status: "pending", pending_id: "pending-1" },
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
  } as Message;
}

function renderSidebar(
  setupQueryClient?: (queryClient: ReturnType<typeof makeQueryClient>) => void,
) {
  const queryClient = makeQueryClient();
  queryClient.setQueryData(
    qk.workflows.snapshot(WORKFLOW_ID),
    snapshot([rawTask("task-1", STEP_TODO, 0), rawTask("task-2", STEP_DONE, 0)]),
  );
  setupQueryClient?.(queryClient);
  const initialState = {
    workspaces: { activeId: WORKSPACE_ID },
    workflows: { activeId: WORKFLOW_ID },
    tasks: { activeTaskId: "task-1", activeSessionId: null },
    messages: { bySession: {}, metaBySession: {} },
    kanban: { workflowId: null, steps: [], tasks: [] },
    kanbanMulti: { snapshots: {} },
  } as unknown as Partial<AppState>;

  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <StateProvider initialState={initialState}>{children}</StateProvider>
    </QueryClientProvider>
  );

  const view = render(
    <TaskSessionSidebar workspaceId={WORKSPACE_ID} workflowId={WORKFLOW_ID} hideFilterBar />,
    { wrapper },
  );
  return { ...view, queryClient };
}

beforeEach(() => {
  vi.clearAllMocks();
});

afterEach(() => cleanup());

describe("TaskSessionSidebar Query metadata", () => {
  it("opens archive metadata from Query-owned sidebar tasks when legacy kanban is empty", async () => {
    renderSidebar();

    await act(async () => {
      screen.getByTestId("archive-task").click();
    });

    expect(screen.getByTestId("sidebar-dialogs").dataset.archiveTitle).toBe("Query task");
  });

  it("moves sidebar tasks through workflow snapshot Query caches when legacy kanbanMulti is empty", async () => {
    const { queryClient } = renderSidebar();

    await act(async () => {
      screen.getByTestId("move-task").click();
    });

    expect(actionMocks.moveTaskById).toHaveBeenCalledWith("task-1", {
      workflow_id: WORKFLOW_ID,
      workflow_step_id: STEP_DONE,
      position: 1,
    });
    expect(
      queryClient
        .getQueryData<WorkflowSnapshot>(qk.workflows.snapshot(WORKFLOW_ID))
        ?.tasks.find((task) => task.id === "task-1")?.workflow_step_id,
    ).toBe(STEP_DONE);
  });

  it("derives pending permission state from Query-owned session messages", () => {
    renderSidebar((queryClient) => {
      queryClient.setQueryData(qk.session.messages(PRIMARY_SESSION_ID), {
        messages: [pendingPermissionMessage()],
        hasMore: false,
        oldestCursor: "message-1",
      });
    });

    expect(screen.getByTestId("sidebar-task-task-1").getAttribute("data-pending-permission")).toBe(
      "true",
    );
  });
});
