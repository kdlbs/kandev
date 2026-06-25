import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { qk } from "@/lib/query/keys";
import { taskId, workflowId, workspaceId, type Task, type WorkflowStep } from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";

const mockFetchTask = vi.fn();
const mockUseTasks = vi.fn();
const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const WORKSPACE_ID = "ws-1";
const WORKFLOW_ID = "wf-1";
const QUERY_STEP_ID = "query-step";
const TIMESTAMP = "2026-01-01T00:00:00Z";

type MockState = {
  kanban: {
    tasks: KanbanState["tasks"];
    steps: KanbanState["steps"];
  };
  tasks: {
    activeTaskId: string | null;
  };
  taskSessions: {
    items: Record<string, { state?: string | null; is_passthrough?: boolean }>;
  };
  previewPanel: {
    openBySessionId: Record<string, boolean>;
    stageBySessionId: Record<string, string>;
    urlBySessionId: Record<string, string>;
  };
  processes: {
    devProcessBySessionId: Record<string, string | undefined>;
    processesById: Record<string, { status?: string | null } | undefined>;
  };
};

function defaultMockState(): MockState {
  return {
    kanban: { tasks: [], steps: [] },
    tasks: { activeTaskId: null },
    taskSessions: { items: {} },
    previewPanel: { openBySessionId: {}, stageBySessionId: {}, urlBySessionId: {} },
    processes: { devProcessBySessionId: {}, processesById: {} },
  };
}

let mockState: MockState = defaultMockState();

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/hooks/use-tasks", () => ({
  useTasks: (...args: unknown[]) => mockUseTasks(...args),
}));

vi.mock("@/components/task/task-page-inner", () => ({
  TaskPageInner: () => null,
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: (...args: unknown[]) => mockFetchTask(...args),
  fetchWorkflowSnapshot: vi.fn(),
  getSubtaskCount: vi.fn(),
  listTasksByWorkspace: vi.fn(),
  listWorkflows: vi.fn(),
}));

vi.mock("@/lib/api/domains/workflow-api", () => ({
  listWorkflowSteps: vi.fn(),
}));

import { useSessionPanelState, useTaskDetails, useWorkflowStepsMapped } from "./task-page-content";

function makeTask(
  id: string,
  title: string,
  workflow = WORKFLOW_ID,
  overrides: Partial<Task> = {},
): Task {
  return {
    id: taskId(id),
    workspace_id: workspaceId(WORKSPACE_ID),
    workflow_id: workflowId(workflow),
    workflow_step_id: "step-1",
    position: 1,
    title,
    description: "",
    state: "CREATED",
    priority: 0,
    repositories: [],
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    ...overrides,
  };
}

function makeStep(id: string, name: string, overrides: Partial<WorkflowStep> = {}): WorkflowStep {
  return {
    id,
    workflow_id: workflowId(WORKFLOW_ID),
    name,
    position: 1,
    color: "bg-blue-500",
    events: { on_enter: [{ type: "enable_plan_mode" }] },
    allow_manual_move: true,
    prompt: "Do the thing",
    is_start_step: false,
    show_in_command_panel: true,
    agent_profile_id: "agent-1",
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    ...overrides,
  };
}

function makeKanbanTask(
  id: string,
  overrides: Partial<KanbanState["tasks"][number]> = {},
): KanbanState["tasks"][number] {
  return {
    id,
    workflowStepId: "legacy-step",
    title: "Legacy task",
    description: "legacy",
    position: 1,
    state: "CREATED",
    updatedAt: TIMESTAMP,
    ...overrides,
  };
}

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
}

function wrapper(client = makeQueryClient()) {
  return function TestWrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client }, children);
  };
}

describe("useWorkflowStepsMapped", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFetchTask.mockReturnValue(new Promise(() => {}));
    mockState = defaultMockState();
  });

  it("maps workflow steps from the workflow-step Query cache when kanban steps are empty", () => {
    const client = makeQueryClient();
    mockState = {
      ...defaultMockState(),
      tasks: { activeTaskId: TASK_ID },
    };
    client.setQueryData(qk.tasks.detail(TASK_ID), makeTask(TASK_ID, "Query task"));
    client.setQueryData(qk.workflows.steps(WORKFLOW_ID), [makeStep("step-query", "Query step")]);

    const { result } = renderHook(() => useWorkflowStepsMapped(), {
      wrapper: wrapper(client),
    });

    expect(result.current).toEqual([
      {
        id: "step-query",
        name: "Query step",
        color: "bg-blue-500",
        position: 1,
        events: { on_enter: [{ type: "enable_plan_mode" }] },
        allow_manual_move: true,
        prompt: "Do the thing",
        is_start_step: false,
        agent_profile_id: "agent-1",
      },
    ]);
  });
});

describe("useSessionPanelState", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFetchTask.mockReturnValue(new Promise(() => {}));
    mockState = defaultMockState();
  });

  it("resolves the task workflow step from task detail Query data", () => {
    const client = makeQueryClient();
    mockState = {
      ...defaultMockState(),
      tasks: { activeTaskId: TASK_ID },
      taskSessions: {
        items: { [SESSION_ID]: { state: "RUNNING", is_passthrough: false } },
      },
    };
    client.setQueryData(
      qk.tasks.detail(TASK_ID),
      makeTask(TASK_ID, "Query task", WORKFLOW_ID, { workflow_step_id: QUERY_STEP_ID }),
    );

    const { result } = renderHook(() => useSessionPanelState(SESSION_ID), {
      wrapper: wrapper(client),
    });

    expect(result.current.sessionWorkflowStepId).toBe(QUERY_STEP_ID);
  });
});

describe("useTaskDetails", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFetchTask.mockReturnValue(new Promise(() => {}));
    mockState = defaultMockState();
  });

  it("uses the initial task for first paint without fetching the same task", () => {
    const initialTask = makeTask(TASK_ID, "Initial task");

    const { result } = renderHook(() => useTaskDetails(TASK_ID, initialTask), {
      wrapper: wrapper(),
    });

    expect(result.current.task?.title).toBe("Initial task");
    expect(mockFetchTask).not.toHaveBeenCalled();
    expect(mockUseTasks).toHaveBeenCalledWith(WORKFLOW_ID);
  });

  it("loads a changed active task through the task query option", async () => {
    const initialTask = makeTask(TASK_ID, "Initial task");
    const loadedTask = makeTask("task-2", "Loaded task", "wf-2");
    mockFetchTask.mockResolvedValue(loadedTask);

    const { result } = renderHook(() => useTaskDetails("task-2", initialTask), {
      wrapper: wrapper(),
    });

    await waitFor(() => expect(result.current.task?.id).toBe("task-2"));

    expect(result.current.task?.title).toBe("Loaded task");
    expect(mockFetchTask).toHaveBeenCalledWith("task-2", {
      init: { signal: expect.any(AbortSignal) },
    });
    expect(mockUseTasks).toHaveBeenLastCalledWith("wf-2");
  });

  it("does not let legacy kanban task data override cached task detail", () => {
    const client = makeQueryClient();
    mockState = {
      ...defaultMockState(),
      kanban: {
        ...defaultMockState().kanban,
        tasks: [
          makeKanbanTask(TASK_ID, {
            title: "Legacy title",
            workflowStepId: "legacy-step",
          }),
        ],
      },
    };
    client.setQueryData(
      qk.tasks.detail(TASK_ID),
      makeTask(TASK_ID, "Query title", WORKFLOW_ID, { workflow_step_id: QUERY_STEP_ID }),
    );

    const { result } = renderHook(() => useTaskDetails(TASK_ID, makeTask(TASK_ID, "Initial")), {
      wrapper: wrapper(client),
    });

    expect(result.current.task?.title).toBe("Query title");
    expect(result.current.task?.workflow_step_id).toBe(QUERY_STEP_ID);
  });

  it("does not synthesize task details from the legacy kanban mirror", () => {
    mockState = {
      ...defaultMockState(),
      kanban: {
        ...defaultMockState().kanban,
        tasks: [makeKanbanTask("task-legacy", { title: "Legacy only" })],
      },
    };

    const { result } = renderHook(() => useTaskDetails("task-legacy", null), {
      wrapper: wrapper(),
    });

    expect(result.current.task).toBeNull();
  });
});
