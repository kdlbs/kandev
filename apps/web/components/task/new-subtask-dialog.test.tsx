import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { qk } from "@/lib/query/keys";
import { taskId, workflowId, workspaceId, type Task } from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";

type MockState = {
  kanban: { workflowId: string | null; tasks: KanbanState["tasks"] };
  workspaces: { activeId: string | null };
  tasks: { activeSessionId: string | null };
  taskSessions: { items: Record<string, { agent_profile_id?: string; repository_id?: string }> };
  sessionWorktreesBySessionId: { itemsBySessionId: Record<string, string[]> };
  worktrees: { items: Record<string, { branch?: string | null } | undefined> };
  messages: { bySession: Record<string, Array<{ author_type?: string; content?: string }>> };
};

const capturedSubmitWorkflowIds: Array<string | null> = [];
let mockState: MockState;

vi.mock("@kandev/ui/dialog", () => ({
  Dialog: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: ReactNode }) => <h2>{children}</h2>,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/components/task-create-dialog-options", () => ({
  useAgentProfileOptions: () => [],
  useExecutorProfileOptions: () => [],
}));

vi.mock("@/components/task-create-dialog-handlers", () => ({
  useDialogHandlers: () => ({}),
}));

vi.mock("@/components/task-create-dialog-effects", () => ({
  useDiscoverReposEffect: vi.fn(),
  useGitHubUrlErrorEffect: vi.fn(),
}));

vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: () => ({ agentProfiles: [], executors: [] }),
}));

vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: () => ({ repositories: [] }),
}));

vi.mock("@/hooks/use-is-utility-configured", () => ({
  useIsUtilityConfigured: () => false,
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

vi.mock("@/lib/local-storage", () => ({
  getLocalStorage: () => null,
}));

vi.mock("@/lib/settings/constants", () => ({
  STORAGE_KEYS: { LAST_EXECUTOR_PROFILE_ID: "last-executor-profile-id" },
}));

vi.mock("@/components/task/new-subtask-form-state", () => ({
  defaultSubtaskWorkspaceMode: (branch: string | null) =>
    branch ? "inherit_parent" : "new_workspace",
  useSubtaskFormState: () => ({
    repositories: [],
    setRepositories: vi.fn(),
    agentProfileId: "",
    setAgentProfileId: vi.fn(),
    executorProfileId: "",
    setExecutorProfileId: vi.fn(),
    useRemote: false,
    remoteRepos: [],
    prInfoByUrl: {},
    discoveredRepositories: [],
  }),
}));

vi.mock("@/components/task/new-subtask-form-parts", () => ({
  PromptZone: () => null,
  SubtaskFormBody: (props: { title: string }) => (
    <div data-testid="subtask-form-title">{props.title}</div>
  ),
}));

vi.mock("@/components/task/session-context-summary", () => ({
  applySummarizeSessionResult: vi.fn(),
}));

vi.mock("@/components/task/use-subtask-submit", () => ({
  useSubtaskPromptZone: () => ({
    promptRef: { current: null },
    attachments: { attachments: [] },
    contextItems: [],
    handleEnhancePrompt: vi.fn(),
    isEnhancingPrompt: false,
    resolvePrompt: () => "prompt",
  }),
  useSubtaskSubmit: (opts: { workflowId: string | null }) => {
    capturedSubmitWorkflowIds.push(opts.workflowId);
    return { handleSubmit: vi.fn() };
  },
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: vi.fn(() => new Promise(() => {})),
  getSubtaskCount: vi.fn(() => Promise.resolve({ count: 3 })),
}));

import { NewSubtaskDialog } from "./new-subtask-dialog";

function makeTask(): Task {
  return {
    id: taskId("task-parent"),
    workspace_id: workspaceId("ws-query"),
    workflow_id: workflowId("wf-query"),
    workflow_step_id: "step-1",
    position: 1,
    title: "Parent task",
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

describe("NewSubtaskDialog", () => {
  beforeEach(() => {
    capturedSubmitWorkflowIds.length = 0;
    mockState = {
      kanban: { workflowId: null, tasks: [] },
      workspaces: { activeId: "ws-active" },
      tasks: { activeSessionId: null },
      taskSessions: { items: {} },
      sessionWorktreesBySessionId: { itemsBySessionId: {} },
      worktrees: { items: {} },
      messages: { bySession: {} },
    };
  });

  it("uses Query task detail and subtask count when legacy kanban mirrors are empty", async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(qk.tasks.detail("task-parent"), makeTask());

    render(
      <NewSubtaskDialog
        open
        onOpenChange={vi.fn()}
        parentTaskId="task-parent"
        parentTaskTitle="Parent task"
      />,
      { wrapper: wrapper(client) },
    );

    await waitFor(() =>
      expect(screen.getByTestId("subtask-form-title").textContent).toBe("Parent task / Subtask 4"),
    );
    expect(capturedSubmitWorkflowIds.at(-1)).toBe("wf-query");
  });
});
