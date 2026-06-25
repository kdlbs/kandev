import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { qk } from "@/lib/query/keys";
import { taskId, workflowId, workspaceId, type Task } from "@/lib/types/http";
import type { CommandItem } from "@/lib/commands/types";

const mocks = vi.hoisted(() => ({
  commands: [] as CommandItem[],
  addBrowser: vi.fn(),
  addChanges: vi.fn(),
  addPlan: vi.fn(),
  addTerminal: vi.fn(),
}));

const appState = {
  tasks: { activeTaskId: "task-1" as string | null },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof appState) => unknown) => selector(appState),
}));

vi.mock("@/hooks/use-register-commands", () => ({
  useRegisterCommands: (commands: CommandItem[]) => {
    mocks.commands = commands;
  },
}));

vi.mock("@/hooks/use-git-operations", () => ({
  useGitOperations: () => ({
    push: vi.fn(),
    pull: vi.fn(),
    rebase: vi.fn(),
    merge: vi.fn(),
  }),
}));

vi.mock("@/hooks/use-git-with-feedback", () => ({
  useGitWithFeedback: () => vi.fn(),
}));

vi.mock("@/hooks/use-panel-actions", () => ({
  usePanelActions: () => ({
    addBrowser: mocks.addBrowser,
    addChanges: mocks.addChanges,
    addPlan: mocks.addPlan,
    addTerminal: mocks.addTerminal,
  }),
}));

vi.mock("@/components/vcs/vcs-dialogs", () => ({
  useVcsDialogs: () => ({
    openCommitDialog: vi.fn(),
    openPRDialog: vi.fn(),
  }),
}));

vi.mock("@/components/task/new-session-dialog", () => ({
  NewSessionDialog: () => null,
}));

vi.mock("@/components/task/new-subtask-dialog", () => ({
  NewSubtaskDialog: ({ open, parentTaskTitle }: { open: boolean; parentTaskTitle: string }) =>
    open ? <div data-testid="new-subtask-dialog" data-parent-title={parentTaskTitle} /> : null,
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => null,
}));

vi.mock("@/lib/ws/workspace-files", () => ({
  createFile: vi.fn(),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: {
    getState: () => ({ addFileEditorPanel: vi.fn() }),
  },
}));

import { SessionCommands } from "./session-commands";

function makeTask(): Task {
  return {
    id: taskId("task-1"),
    workspace_id: workspaceId("workspace-1"),
    workflow_id: workflowId("workflow-1"),
    workflow_step_id: "step-1",
    position: 0,
    title: "Query parent task",
    description: "",
    state: "CREATED",
    priority: 0,
    repositories: [],
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: Infinity,
      },
    },
  });
}

describe("SessionCommands", () => {
  beforeEach(() => {
    mocks.commands = [];
    appState.tasks.activeTaskId = "task-1";
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("passes the Query task title to the subtask dialog", () => {
    const client = createQueryClient();
    client.setQueryData(qk.tasks.detail("task-1"), makeTask());

    render(
      <QueryClientProvider client={client}>
        <SessionCommands sessionId="session-1" hasWorktree={false} />
      </QueryClientProvider>,
    );

    const command = mocks.commands.find((item) => item.id === "subtask-create");
    expect(command).toBeTruthy();

    act(() => command?.action?.());

    expect(screen.getByTestId("new-subtask-dialog").dataset.parentTitle).toBe("Query parent task");
  });
});
