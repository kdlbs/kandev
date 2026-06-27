import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { qk } from "@/lib/query/keys";

const archiveAndSwitchMock = vi.fn();
const toastMock = vi.fn();
const handleSendMessageMock = vi.fn();
const taskPRsByTaskId = vi.hoisted(() => ({
  value: {
    "task-1": [{ pr_number: 42, state: "merged" }],
  } as Record<string, Array<{ pr_number: number; state: string }>>,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockState) => unknown) => selector(mockState),
  useAppStoreApi: () => ({ getState: () => mockState }),
}));

vi.mock("@/hooks/use-task-actions", () => ({
  useArchiveAndSwitchTask: () => archiveAndSwitchMock,
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: toastMock }),
}));

vi.mock("@/components/github/pr-status-chip", () => ({
  PRStatusChip: () => null,
}));

vi.mock("@/components/task/share/share-button", () => ({
  ShareButton: () => null,
  shareableSessionStateClient: () => false,
}));

vi.mock("@/components/task/chat/chat-input-container", () => ({
  ChatInputContainer: () => null,
}));

vi.mock("@/components/task/chat/todo-indicator", () => ({
  TodoIndicator: () => null,
}));

vi.mock("@/hooks/use-keyboard-shortcut", () => ({
  useKeyboardShortcut: () => undefined,
}));

vi.mock("@/hooks/use-message-handler", () => ({
  buildTaskMentionsContext: vi.fn(),
  useMessageHandler: () => ({ handleSendMessage: handleSendMessageMock }),
}));

vi.mock("@/hooks/domains/kanban/use-plan-actions", () => ({
  usePlanActions: () => ({
    implementPlanHandler: vi.fn(),
    proceedStepName: null,
    proceed: vi.fn(),
    isMoving: false,
  }),
}));

vi.mock("@/hooks/domains/kanban/use-all-workflow-snapshots", () => ({
  useAllWorkflowSnapshots: () => ({ snapshots: {} }),
}));

vi.mock("@/hooks/domains/session/use-executor-environment-availability", () => ({
  useExecutorEnvironmentAvailability: () => ({
    unavailable: false,
    status: null,
  }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({
    request: vi.fn().mockResolvedValue({ prs: taskPRsByTaskId.value["task-1"] }),
    send: vi.fn(),
  }),
}));

vi.mock("@/lib/local-storage", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/local-storage")>()),
  markPRClosedBannerDismissed: vi.fn(),
  markPRMergedBannerDismissed: vi.fn(),
  wasPRClosedBannerDismissed: () => false,
  wasPRMergedBannerDismissed: () => false,
}));

const mockState = {
  clearPendingPrUrlForTaskPR: vi.fn(),
  workspaces: { activeId: "workspace-1" },
  taskPRs: {
    get byTaskId() {
      return taskPRsByTaskId.value;
    },
  },
};

import { PRMergedBanner, useSubmitHandler } from "./chat-input-area";

function createTestQueryClient() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  queryClient.setQueryData(
    qk.integrations.github.taskPr("task-1"),
    taskPRsByTaskId.value["task-1"],
  );
  return queryClient;
}

function renderWithQuery(ui: ReactNode) {
  return render(<QueryClientProvider client={createTestQueryClient()}>{ui}</QueryClientProvider>);
}

function queryWrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={createTestQueryClient()}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  archiveAndSwitchMock.mockResolvedValue(undefined);
  handleSendMessageMock.mockResolvedValue(undefined);
  taskPRsByTaskId.value = {
    "task-1": [{ pr_number: 42, state: "merged" }],
  };
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe("PRMergedBanner", () => {
  it("archives without showing a success toast", async () => {
    renderWithQuery(<PRMergedBanner taskId="task-1" />);

    fireEvent.click(screen.getByTestId("pr-merged-archive-button"));

    await waitFor(() => expect(archiveAndSwitchMock).toHaveBeenCalledWith("task-1"));
    expect(toastMock).not.toHaveBeenCalled();
  });

  it("keeps the failure toast when archive fails", async () => {
    archiveAndSwitchMock.mockRejectedValueOnce(new Error("archive failed"));
    renderWithQuery(<PRMergedBanner taskId="task-1" />);

    fireEvent.click(screen.getByTestId("pr-merged-archive-button"));

    await waitFor(() =>
      expect(toastMock).toHaveBeenCalledWith({
        description: "Failed to archive task",
        variant: "error",
      }),
    );
  });
});

describe("useSubmitHandler", () => {
  function panelState(overrides = {}) {
    return {
      resolvedSessionId: "session-1",
      taskId: "task-1",
      sessionModel: null,
      activeModel: null,
      isAgentBusy: false,
      activeDocument: null,
      planComments: [],
      pendingPRFeedback: [],
      contextFiles: [],
      prompts: [],
      markCommentsSent: vi.fn(),
      clearSessionPlanComments: vi.fn(),
      handleClearPRFeedback: vi.fn(),
      clearEphemeral: vi.fn(),
      addContextFile: vi.fn(),
      planModeEnabled: false,
      ...overrides,
    } as never;
  }

  it("shows a toast when sending fails", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    handleSendMessageMock.mockRejectedValueOnce(new Error("WebSocket request timed out"));
    const { result } = renderHook(() => useSubmitHandler(panelState()), { wrapper: queryWrapper });

    await act(async () => {
      await result.current.handleSubmit("hello");
    });

    expect(toastMock).toHaveBeenCalledWith({
      title: "Message send status unknown",
      description:
        "The connection dropped or timed out. Refresh the task to confirm whether it went through.",
      variant: "error",
    });
  });
});
