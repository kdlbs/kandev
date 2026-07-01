import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { qk } from "@/lib/query/keys";
import { taskId, workflowId, workspaceId, type Task } from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";

type MockState = {
  kanban: { tasks: KanbanState["tasks"] };
  tasks: { activeSessionId: string | null };
  taskSessions: { items: Record<string, unknown> };
  messages: { bySession: Record<string, Array<{ author_type?: string; content?: string }>> };
  setActiveSession: ReturnType<typeof vi.fn>;
};

let mockState: MockState;

vi.mock("@kandev/ui/dialog", () => ({
  Dialog: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogFooter: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: ReactNode }) => <h2>{children}</h2>,
}));

vi.mock("@kandev/ui/button", () => ({
  Button: ({ children, ...props }: { children: ReactNode }) => (
    <button {...props}>{children}</button>
  ),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/components/task-create-dialog-selectors", () => ({
  AgentSelector: () => null,
}));

vi.mock("@/components/task-create-dialog-options", () => ({
  useAgentProfileOptions: () => [{ value: "agent-1", label: "Agent" }],
}));

vi.mock("@/hooks/use-summarize-session", () => ({
  useSummarizeSession: () => ({ summarize: vi.fn(), isSummarizing: false }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: () => ({ sessions: [], loadSessions: vi.fn() }),
}));

vi.mock("@/hooks/domains/settings/use-remote-auth-specs", () => ({
  useRemoteAuthSpecs: () => ({ specs: [], loaded: true }),
}));

vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: () => ({
    agentProfiles: [{ id: "agent-1", label: "Agent", agent_name: "codex" }],
    executors: [{ id: "executor-1", name: "Executor" }],
  }),
}));

vi.mock("@/hooks/domains/session/use-task-executor-profile", () => ({
  useTaskExecutorProfile: () => null,
}));

vi.mock("@/lib/agent-executor-compat", () => ({
  isAgentConfiguredOnExecutor: () => true,
}));

vi.mock("@/hooks/use-is-utility-configured", () => ({
  useIsUtilityConfigured: () => false,
}));

vi.mock("@/hooks/use-utility-agent-generator", () => ({
  useUtilityAgentGenerator: () => ({ enhancePrompt: vi.fn(), isEnhancingPrompt: false }),
}));

vi.mock("@/components/task/session-dialog-shared", () => ({
  EnvironmentBadges: () => null,
  ContextSelect: () => null,
  toContextItems: () => [],
  useDialogAttachments: () => ({
    attachments: [],
    isDragging: false,
    fileInputRef: { current: null },
    handleRemoveAttachment: vi.fn(),
    handlePaste: vi.fn(),
    handleDragOver: vi.fn(),
    handleDragLeave: vi.fn(),
    handleDrop: vi.fn(),
    handleAttachClick: vi.fn(),
    handleFileInputChange: vi.fn(),
  }),
}));

vi.mock("@/components/task/new-session-form-prompt", () => ({
  SessionPromptField: () => null,
}));

vi.mock("@/components/task/new-session-form-actions", () => ({
  useSessionContextChange: () => vi.fn(),
  useSessionLaunchSubmit: () =>
    vi.fn((event?: { preventDefault?: () => void }) => event?.preventDefault?.()),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: {
    getState: () => ({ api: null, centerGroupId: "center" }),
  },
}));

vi.mock("@/lib/state/dockview-panel-actions", () => ({
  addSessionPanel: vi.fn(),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: vi.fn(() => new Promise(() => {})),
}));

import { NewSessionDialog } from "./new-session-dialog";

function makeTask(): Task {
  return {
    id: taskId("task-1"),
    workspace_id: workspaceId("ws-1"),
    workflow_id: workflowId("wf-1"),
    workflow_step_id: "step-1",
    position: 1,
    title: "Query task title",
    description: "",
    state: "CREATED",
    priority: 0,
    repositories: [],
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

function wrapper(client: QueryClient) {
  return function TestWrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("NewSessionDialog", () => {
  beforeEach(() => {
    mockState = {
      kanban: { tasks: [] },
      tasks: { activeSessionId: null },
      taskSessions: { items: {} },
      messages: { bySession: {} },
      setActiveSession: vi.fn(),
    };
  });

  it("uses the task title from task detail Query when kanban tasks are empty", () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(qk.tasks.detail("task-1"), makeTask());

    render(<NewSessionDialog open onOpenChange={vi.fn()} taskId="task-1" />, {
      wrapper: wrapper(client),
    });

    expect(screen.getByText("Query task title")).toBeTruthy();
  });
});
