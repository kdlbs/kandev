import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, waitFor } from "@testing-library/react";
import { type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { qk } from "@/lib/query/keys";
import { taskId, workflowId, workspaceId, type Task, type WorkflowStep } from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";

type NewTaskDropdownProps = {
  workspaceId: string | null;
  workflowId: string | null;
  steps: Array<{ id: string; title: string; color?: string; events?: Record<string, unknown> }>;
  activeTaskId: string | null;
  activeTaskTitle: string;
};

type MockState = {
  workspaces: { activeId: string | null };
  tasks: { activeTaskId: string | null; activeSessionId: string | null };
  taskSessions: { items: Record<string, unknown> };
  kanban: {
    workflowId: string | null;
    steps: KanbanState["steps"];
    tasks: KanbanState["tasks"];
  };
  environmentIdBySessionId: Record<string, string | undefined>;
  setActiveTask: ReturnType<typeof vi.fn>;
  setActiveSession: ReturnType<typeof vi.fn>;
};

const capturedDropdownProps: NewTaskDropdownProps[] = [];
let mockState: MockState;
let mockDockviewState: {
  sidebarGroupId: string;
  centerGroupId: string;
  rightTopGroupId: string;
  rightBottomGroupId: string;
};

function defaultMockState(): MockState {
  return {
    workspaces: { activeId: "ws-1" },
    tasks: { activeTaskId: "task-1", activeSessionId: null },
    taskSessions: { items: {} },
    kanban: { workflowId: null, steps: [], tasks: [] },
    environmentIdBySessionId: {},
    setActiveTask: vi.fn(),
    setActiveSession: vi.fn(),
  };
}

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
  useAppStoreApi: () => ({ getState: () => mockState }),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: typeof mockDockviewState) => unknown) =>
    selector(mockDockviewState),
  performLayoutSwitch: vi.fn(),
}));

vi.mock("@/components/task/new-task-dropdown", () => ({
  NewTaskDropdown: (props: NewTaskDropdownProps) => {
    capturedDropdownProps.push(props);
    return null;
  },
}));

vi.mock("@/components/task/new-session-dialog", () => ({
  NewSessionDialog: () => null,
}));

vi.mock("@/components/task/dockview-add-panel-items", () => ({
  AddPanelMenuItems: () => null,
  MENU_ITEM_CLASS: "",
}));

vi.mock("@/components/task/dockview-group-actions", () => ({
  GroupSplitCloseActionsView: () => null,
  useDockviewGroupWidth: () => 320,
}));

vi.mock("@/components/task/repository-scripts-menu", () => ({
  useActiveSessionDevScript: () => null,
}));

vi.mock("@/hooks/domains/session/use-user-shells", () => ({
  useUserShells: vi.fn(),
}));

vi.mock("@/hooks/domains/session/use-ensure-default-terminal-ordinary", () => ({
  useEnsureDefaultTerminalOrdinary: vi.fn(),
}));

vi.mock("@/hooks/use-environment-session-id", () => ({
  useEnvironmentId: () => null,
}));

vi.mock("@/hooks/domains/github/use-task-pr", () => ({
  useTaskPR: () => ({ prs: [] }),
}));

vi.mock("@/hooks/domains/workspace/use-repository-cache", () => ({
  useAllCachedRepositories: () => [],
}));

vi.mock("@/hooks/domains/workspace/use-repository-scripts", () => ({
  useRepositoryScripts: () => ({ scripts: [] }),
}));

vi.mock("@/lib/api", () => ({
  startProcess: vi.fn(),
}));

vi.mock("@/lib/api/domains/user-shell-api", () => ({
  createUserShell: vi.fn(),
}));

vi.mock("@/lib/links", () => ({
  replaceTaskUrl: vi.fn(),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: vi.fn(() => new Promise(() => {})),
}));

vi.mock("@/lib/api/domains/workflow-api", () => ({
  listWorkflowSteps: vi.fn(),
}));

import { RightHeaderActions } from "./dockview-header-actions";

const TIMESTAMP = "2026-01-01T00:00:00Z";

function makeTask(overrides: Partial<Task> = {}): Task {
  return {
    id: taskId("task-1"),
    workspace_id: workspaceId("ws-1"),
    workflow_id: workflowId("wf-query"),
    workflow_step_id: "step-query",
    position: 1,
    title: "Query task title",
    description: "",
    state: "CREATED",
    priority: 0,
    repositories: [],
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    ...overrides,
  };
}

function makeStep(overrides: Partial<WorkflowStep> = {}): WorkflowStep {
  return {
    id: "step-query",
    workflow_id: workflowId("wf-query"),
    name: "Query step",
    position: 1,
    color: "bg-green-500",
    events: { on_enter: [{ type: "auto_start_agent" }] },
    allow_manual_move: true,
    prompt: "",
    is_start_step: true,
    show_in_command_panel: true,
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    ...overrides,
  };
}

function wrapper(client: QueryClient) {
  return function TestWrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("RightHeaderActions", () => {
  beforeEach(() => {
    capturedDropdownProps.length = 0;
    mockState = defaultMockState();
    mockDockviewState = {
      sidebarGroupId: "sidebar-group",
      centerGroupId: "center-group",
      rightTopGroupId: "right-top-group",
      rightBottomGroupId: "right-bottom-group",
    };
  });

  it("passes task-create context from Query caches when legacy kanban mirrors are empty", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(qk.tasks.detail("task-1"), makeTask());
    client.setQueryData(qk.workflows.steps("wf-query"), [makeStep()]);

    const props = {
      group: { id: "sidebar-group", panels: [] },
      containerApi: {},
    } as unknown as Parameters<typeof RightHeaderActions>[0];

    render(<RightHeaderActions {...props} />, { wrapper: wrapper(client) });

    await waitFor(() => expect(capturedDropdownProps).toHaveLength(1));
    expect(capturedDropdownProps[0]).toMatchObject({
      workspaceId: "ws-1",
      workflowId: "wf-query",
      activeTaskId: "task-1",
      activeTaskTitle: "Query task title",
      steps: [
        {
          id: "step-query",
          title: "Query step",
          color: "bg-green-500",
          events: { on_enter: [{ type: "auto_start_agent" }] },
        },
      ],
    });
  });
});
