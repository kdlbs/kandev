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
  kanban: { tasks: KanbanState["tasks"] };
};

const mockUseTaskSessions = vi.fn();
let mockState: MockState;
const TIMESTAMP = "2026-01-01T00:00:00Z";

vi.mock("@tabler/icons-react", () => ({
  IconMessagePlus: () => <span />,
  IconStar: () => <span data-testid="primary-star" />,
}));

vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenuItem: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DropdownMenuLabel: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DropdownMenuSeparator: () => <div />,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: (...args: unknown[]) => mockUseTaskSessions(...args),
}));

vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: () => ({ agentProfiles: [] }),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: { api: null; centerGroupId: string }) => unknown) =>
    selector({ api: null, centerGroupId: "center" }),
}));

vi.mock("@/components/agent-logo", () => ({
  AgentLogo: () => null,
}));

vi.mock("@/lib/ui/state-icons", () => ({
  getSessionStateIcon: () => <span />,
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: vi.fn(() => new Promise(() => {})),
}));

import { SessionReopenMenuItems } from "./session-reopen-menu";

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
    started_at: id === "session-primary" ? "2026-01-01T00:01:00Z" : TIMESTAMP,
    updated_at: TIMESTAMP,
    is_primary: false,
  };
}

function wrapper(client: QueryClient) {
  return function TestWrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("SessionReopenMenuItems", () => {
  beforeEach(() => {
    mockState = { kanban: { tasks: [] } };
    mockUseTaskSessions.mockReturnValue({
      sessions: [makeSession("session-other"), makeSession("session-primary")],
    });
  });

  it("marks the primary session from task detail Query when kanban tasks are empty", () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(qk.tasks.detail("task-1"), makeTask());

    render(<SessionReopenMenuItems taskId="task-1" />, { wrapper: wrapper(client) });

    expect(screen.getAllByTestId("primary-star")).toHaveLength(1);
  });
});
