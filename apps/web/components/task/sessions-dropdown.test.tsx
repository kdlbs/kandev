import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { qk } from "@/lib/query/keys";
import {
  sessionId,
  taskId,
  workflowId,
  workspaceId,
  type Task,
  type TaskSession,
} from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";

type MockState = {
  tasks: { activeTaskId: string | null; activeSessionId: string | null };
  kanban: { tasks: KanbanState["tasks"] };
  taskSessions: { items: Record<string, unknown> };
  environmentIdBySessionId: Record<string, string | undefined>;
  setActiveSession: ReturnType<typeof vi.fn>;
};

const mockUseTaskSessions = vi.fn();
let mockState: MockState;

vi.mock("@tabler/icons-react", () => ({
  IconStack2: () => <span />,
  IconPlus: () => <span />,
  IconStar: () => <span data-testid="primary-star" />,
  IconPlayerPlayFilled: () => <span />,
  IconTrash: () => <span />,
}));

vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DropdownMenuContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DropdownMenuSeparator: () => <div />,
  DropdownMenuTrigger: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  TooltipContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
  useAppStoreApi: () => ({ getState: () => mockState }),
}));

vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: (...args: unknown[]) => mockUseTaskSessions(...args),
}));

vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: () => ({ agentProfiles: [] }),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  performLayoutSwitch: vi.fn(),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => null,
}));

vi.mock("@/lib/ui/state-icons", () => ({
  getSessionStateIcon: () => <span />,
}));

vi.mock("@/components/task-create-dialog", () => ({
  TaskCreateDialog: () => null,
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: vi.fn(() => new Promise(() => {})),
}));

import { SessionsDropdown } from "./sessions-dropdown";

const TIMESTAMP = "2026-01-01T00:00:00Z";

function makeTask(): Task {
  return {
    id: taskId("task-1"),
    workspace_id: workspaceId("ws-1"),
    workflow_id: workflowId("wf-1"),
    workflow_step_id: "step-1",
    position: 1,
    title: "Query task",
    description: "",
    state: "CREATED",
    priority: 0,
    primary_session_id: sessionId("session-primary"),
    repositories: [],
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };
}

function makeSession(id: string): TaskSession {
  return {
    id: sessionId(id),
    task_id: taskId("task-1"),
    state: "COMPLETED",
    started_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    is_primary: false,
  };
}

function wrapper(client: QueryClient) {
  return function TestWrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("SessionsDropdown", () => {
  beforeEach(() => {
    mockState = {
      tasks: { activeTaskId: "task-1", activeSessionId: null },
      kanban: { tasks: [] },
      taskSessions: { items: {} },
      environmentIdBySessionId: {},
      setActiveSession: vi.fn(),
    };
    mockUseTaskSessions.mockReturnValue({
      sessions: [makeSession("session-other"), makeSession("session-primary")],
      loadSessions: vi.fn(),
    });
  });

  it("marks the primary session from task detail Query when kanban tasks are empty", () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(qk.tasks.detail("task-1"), makeTask());

    render(<SessionsDropdown taskId="task-1" />, { wrapper: wrapper(client) });

    expect(screen.getAllByTestId("primary-star")).toHaveLength(1);
  });
});
