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
  tasks: { activeTaskId: string | null };
  kanban: { tasks: KanbanState["tasks"] };
  taskSessions: { items: Record<string, TaskSession & { is_primary?: boolean }> };
  taskSessionsByTask: { itemsByTaskId: Record<string, TaskSession[]> };
  sessionModels: { bySessionId: Record<string, unknown> };
  activeModel: { bySessionId: Record<string, string | null> };
};

let mockState: MockState;

vi.mock("@tabler/icons-react", () => ({
  IconStar: () => <span data-testid="primary-star" />,
}));

vi.mock("dockview-react", () => ({
  DockviewDefaultTab: () => <span data-testid="dockview-tab-title" />,
}));

vi.mock("@kandev/ui/context-menu", () => ({
  ContextMenu: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  ContextMenuContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  ContextMenuItem: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  ContextMenuSeparator: () => <div />,
  ContextMenuTrigger: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@kandev/ui/alert-dialog", () => ({
  AlertDialog: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  AlertDialogAction: ({ children }: { children: ReactNode }) => <button>{children}</button>,
  AlertDialogCancel: ({ children }: { children: ReactNode }) => <button>{children}</button>,
  AlertDialogContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  AlertDialogDescription: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  AlertDialogFooter: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  AlertDialogHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  AlertDialogTitle: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/hooks/domains/session/use-session-actions", () => ({
  useSessionActions: () => ({
    setPrimary: vi.fn(),
    stop: vi.fn(),
    resume: vi.fn(),
    remove: vi.fn(),
  }),
  isSessionStoppable: () => false,
  isSessionDeletable: () => false,
  isSessionResumable: () => false,
}));

vi.mock("@/components/task/share/share-button", () => ({
  shareableSessionStateClient: () => false,
}));

vi.mock("@/components/task/share/share-dialog", () => ({
  ShareDialog: () => null,
}));

vi.mock("@/components/task/handoff-profile-menu-items", () => ({
  HandoffContextMenuSub: () => null,
}));

vi.mock("@/components/task/new-session-dialog", () => ({
  NewSessionDialog: () => null,
}));

vi.mock("@/components/model-config-selector", () => ({
  usableConfigOptions: () => [],
}));

vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: () => ({ agentProfiles: [] }),
}));

vi.mock("@/components/agent-logo", () => ({
  AgentLogo: () => null,
}));

vi.mock("@/components/grid-spinner", () => ({
  GridSpinner: () => null,
}));

vi.mock("@/components/task/use-tab-maximize", () => ({
  useTabMaximizeOnDoubleClick: () => vi.fn(),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: vi.fn(() => new Promise(() => {})),
}));

import { SessionTab } from "./session-tab";

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
    started_at: id === "session-primary" ? TIMESTAMP : "2026-01-01T00:01:00Z",
    updated_at: TIMESTAMP,
    is_primary: false,
  };
}

function wrapper(client: QueryClient) {
  return function TestWrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("SessionTab", () => {
  beforeEach(() => {
    const primary = makeSession("session-primary");
    const other = makeSession("session-other");
    mockState = {
      tasks: { activeTaskId: "task-1" },
      kanban: { tasks: [] },
      taskSessions: { items: { "session-primary": primary, "session-other": other } },
      taskSessionsByTask: { itemsByTaskId: { "task-1": [primary, other] } },
      sessionModels: { bySessionId: {} },
      activeModel: { bySessionId: {} },
    };
  });

  it("marks the primary session from task detail Query when kanban tasks are empty", () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(qk.tasks.detail("task-1"), makeTask());
    const props = {
      api: {
        id: "session:session-primary",
        title: "",
        isActive: true,
        group: { id: "group-1" },
        onDidActiveChange: () => ({ dispose: vi.fn() }),
        setTitle: vi.fn(),
      },
      containerApi: { getPanel: vi.fn(), removePanel: vi.fn() },
    } as unknown as Parameters<typeof SessionTab>[0];

    render(<SessionTab {...props} />, { wrapper: wrapper(client) });

    expect(screen.getAllByTestId("primary-star")).toHaveLength(1);
  });
});
